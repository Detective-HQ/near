package nearoneclick

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

const (
	providerName    = "near_1click"
	catalogueSource = "NEAR Intents 1Click token catalogue"
)

type rawToken struct {
	AssetID         string   `json:"assetId"`
	Blockchain      string   `json:"blockchain"`
	Symbol          string   `json:"symbol"`
	Name            string   `json:"name"`
	Decimals        *int     `json:"decimals"`
	ContractAddress string   `json:"contractAddress"`
	Price           *float64 `json:"price"`
	PriceUpdatedAt  string   `json:"priceUpdatedAt"`
}

type rawQuoteResponse struct {
	CorrelationID string                     `json:"correlationId"`
	Timestamp     string                     `json:"timestamp"`
	Signature     string                     `json:"signature"`
	QuoteRequest  map[string]json.RawMessage `json:"quoteRequest"`
	Quote         map[string]json.RawMessage `json:"quote"`
}

type rawStatusResponse struct {
	CorrelationID string          `json:"correlationId"`
	Status        string          `json:"status"`
	UpdatedAt     string          `json:"updatedAt"`
	SwapDetails   json.RawMessage `json:"swapDetails"`
}

type quoteExpectation struct {
	OriginAsset      domain.Asset
	DestinationAsset domain.Asset
	AmountAtomic     *big.Int
	Recipient        string
	RefundTo         string
	SlippageBPS      int
	Dry              bool
	Deadline         string
	DepositMode      string
}

func normalizeTokens(body []byte, observedAt time.Time) ([]domain.Asset, error) {
	var raw []rawToken
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, domain.Wrap(domain.CodeProviderInvalidCatalogue,
			"The swap provider returned an invalid asset catalogue.", err)
	}

	seen := make(map[string]struct{}, len(raw))
	assets := make([]domain.Asset, 0, len(raw))
	for _, token := range raw {
		if err := validateToken(token); err != nil {
			return nil, err
		}
		if _, duplicate := seen[token.AssetID]; duplicate {
			return nil, domain.New(domain.CodeProviderInvalidCatalogue,
				"The swap provider returned duplicate asset identifiers.")
		}
		seen[token.AssetID] = struct{}{}

		name := token.Name
		if strings.TrimSpace(name) == "" {
			name = token.Symbol
		}
		tokenObservedAt := observedAt
		if parsed, err := time.Parse(time.RFC3339, token.PriceUpdatedAt); err == nil {
			tokenObservedAt = parsed.UTC()
		}

		assets = append(assets, domain.Asset{
			ID:              token.AssetID,
			ProviderAssetID: token.AssetID,
			Chain:           token.Blockchain,
			Symbol:          truncateRunes(token.Symbol, 40),
			Name:            truncateRunes(name, 100),
			Decimals:        *token.Decimals,
			ContractAddress: token.ContractAddress,
			Source:          catalogueSource,
			Enabled:         true,
			ObservedAt:      tokenObservedAt,
		})
	}
	return assets, nil
}

func validateToken(token rawToken) error {
	if strings.TrimSpace(token.AssetID) == "" || len(token.AssetID) > 512 {
		return domain.New(domain.CodeProviderInvalidCatalogue,
			"The swap provider returned an invalid asset entry.")
	}
	if strings.TrimSpace(token.Blockchain) == "" {
		return domain.New(domain.CodeProviderInvalidCatalogue,
			"The swap provider returned an invalid asset entry.")
	}
	if strings.TrimSpace(token.Symbol) == "" {
		return domain.New(domain.CodeProviderInvalidCatalogue,
			"The swap provider returned an invalid asset entry.")
	}
	if token.Decimals == nil || *token.Decimals < 0 || *token.Decimals > domain.MaxDecimals {
		return domain.New(domain.CodeProviderInvalidCatalogue,
			"The swap provider returned an invalid asset entry.")
	}
	return nil
}

func normalizeQuote(body []byte, exp quoteExpectation, publicKey string, now time.Time) (providers.QuoteResult, error) {
	var payload rawQuoteResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return providers.QuoteResult{}, domain.Wrap(domain.CodeProviderInvalidResponse,
			"The swap provider returned invalid JSON.", err)
	}

	if strings.TrimSpace(payload.CorrelationID) == "" || payload.QuoteRequest == nil || payload.Quote == nil {
		return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
			"The swap provider returned an incomplete quote.")
	}

	timestamp, err := time.Parse(time.RFC3339, payload.Timestamp)
	if err != nil {
		return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
			"The swap provider returned an invalid quote timestamp.")
	}

	if strings.TrimSpace(payload.Signature) == "" {
		return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
			"The swap provider returned an unsigned quote.")
	}

	amountOut, err := requiredDigits(payload.Quote, "amountOut")
	if err != nil {
		return providers.QuoteResult{}, err
	}
	minAmountOut, err := requiredDigits(payload.Quote, "minAmountOut")
	if err != nil {
		return providers.QuoteResult{}, err
	}
	if minAmountOut.Cmp(amountOut) > 0 {
		return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
			"The provider minimum output exceeds its quoted output.")
	}

	if err := validateEchoedRequest(exp, payload.QuoteRequest); err != nil {
		return providers.QuoteResult{}, err
	}

	signatureVerified := false
	if strings.TrimSpace(publicKey) != "" {
		verified, verifyErr := VerifySignedQuote(payload.QuoteRequest, payload.Quote, payload.Timestamp, payload.Signature, publicKey)
		if verifyErr != nil {
			return providers.QuoteResult{}, domain.Wrap(domain.CodeProviderSignatureInvalid,
				"The provider quote signature could not be verified.", verifyErr)
		}
		if !verified {
			return providers.QuoteResult{}, domain.New(domain.CodeProviderSignatureInvalid,
				"The provider quote signature could not be verified.")
		}
		signatureVerified = true
	}

	if exp.Dry {
		for _, key := range []string{"depositAddress", "depositMemo", "deadline", "timeWhenInactive"} {
			if hasNonNull(payload.Quote, key) {
				return providers.QuoteResult{}, domain.New(domain.CodeProviderUnexpectedData,
					"The provider returned unexpected deposit instructions for a dry quote.")
			}
		}
	}

	var depositAddress, depositMemo, deadline string
	var expiresAt time.Time
	var timeWhenInactive time.Time

	if !exp.Dry {
		depositAddress, err = requiredString(payload.Quote, "depositAddress")
		if err != nil || len(depositAddress) > 512 {
			return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
				"The provider did not return a valid deposit address.")
		}
		if memo, present, memoErr := rawString(payload.Quote, "depositMemo"); memoErr != nil {
			return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
				"The provider returned an invalid deposit memo.")
		} else if present {
			depositMemo = memo
		}
		deadline, err = requiredString(payload.Quote, "deadline")
		if err != nil {
			return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
				"The provider did not return a valid future deposit deadline.")
		}
		expiresAt, err = time.Parse(time.RFC3339, deadline)
		if err != nil || !expiresAt.After(now) {
			return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
				"The provider did not return a valid future deposit deadline.")
		}
		if raw, present := payload.Quote["timeWhenInactive"]; present && hasNonNull(payload.Quote, "timeWhenInactive") {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
					"The provider returned an invalid inactive timestamp.")
			}
			parsed, parseErr := time.Parse(time.RFC3339, text)
			if parseErr != nil {
				return providers.QuoteResult{}, domain.New(domain.CodeProviderInvalidQuote,
					"The provider returned an invalid inactive timestamp.")
			}
			timeWhenInactive = parsed
		}
	}

	amountOutFormatted, _, _ := rawString(payload.Quote, "amountOutFormatted")
	minAmountOutFormatted, _, _ := rawString(payload.Quote, "minAmountOutFormatted")
	amountInFormatted, _, _ := rawString(payload.Quote, "amountInFormatted")
	timeEstimate, _, _ := rawInt(payload.Quote, "timeEstimate")

	mode := domain.QuoteModeProviderDry
	warning := "Live provider estimate only. This dry quote contains no deposit instructions and cannot be used to send funds."
	if !exp.Dry {
		mode = domain.QuoteModeProviderExecutable
		warning = "Executable provider quote. Check the amount, destination, refund address, deposit address, expiry and wallet transaction before signing."
	}

	feeData := map[string]any{
		"providerTimestamp":     payload.Timestamp,
		"signatureVerified":     signatureVerified,
		"amountOutFormatted":    amountOutFormatted,
		"minAmountOutFormatted": minAmountOutFormatted,
		"warning":               warning,
	}
	if amountInFormatted != "" {
		feeData["amountInFormatted"] = amountInFormatted
	}
	if timeEstimate != 0 {
		feeData["timeEstimateSeconds"] = timeEstimate
	}
	if !timeWhenInactive.IsZero() {
		feeData["timeWhenInactive"] = timeWhenInactive
	}

	quote := domain.Quote{
		ID:                      "qte_" + uuid.NewString(),
		ProviderQuoteID:         payload.CorrelationID,
		ProviderName:            providerName,
		Mode:                    mode,
		OriginAssetID:           exp.OriginAsset.ID,
		DestinationAssetID:      exp.DestinationAsset.ID,
		AmountInAtomic:          new(big.Int).Set(exp.AmountAtomic),
		AmountInDecimal:         domain.FormatUnits(exp.AmountAtomic, exp.OriginAsset.Decimals),
		AmountOutEstimateAtomic: amountOut,
		MinAmountOutAtomic:      minAmountOut,
		SlippageBPS:             exp.SlippageBPS,
		FeeData:                 feeData,
		DepositAddress:          depositAddress,
		DepositMemo:             depositMemo,
		ExpiresAt:               expiresAt,
		RawResponse:             append([]byte(nil), body...),
	}

	return providers.QuoteResult{
		Quote:                 quote,
		ProviderQuoteID:       payload.CorrelationID,
		SignatureVerified:     signatureVerified,
		ProviderTimestamp:     timestamp.UTC(),
		TimeWhenInactive:      timeWhenInactive,
		TimeEstimateSeconds:   int(timeEstimate),
		AmountOutFormatted:    amountOutFormatted,
		MinAmountOutFormatted: minAmountOutFormatted,
		Warning:               warning,
		Raw:                   append([]byte(nil), body...),
	}, nil
}

func validateEchoedRequest(exp quoteExpectation, echoed map[string]json.RawMessage) error {
	dry, ok, err := rawBool(echoed, "dry")
	if err != nil || !ok || dry != exp.Dry {
		return quoteMismatch()
	}
	slippage, ok, err := rawInt(echoed, "slippageTolerance")
	if err != nil || !ok || slippage != int64(exp.SlippageBPS) {
		return quoteMismatch()
	}

	expected := map[string]string{
		"swapType":         "EXACT_INPUT",
		"depositType":      "ORIGIN_CHAIN",
		"refundType":       "ORIGIN_CHAIN",
		"recipientType":    "DESTINATION_CHAIN",
		"originAsset":      exp.OriginAsset.ID,
		"destinationAsset": exp.DestinationAsset.ID,
		"amount":           exp.AmountAtomic.String(),
		"recipient":        exp.Recipient,
		"refundTo":         exp.RefundTo,
	}
	if !exp.Dry {
		expected["deadline"] = exp.Deadline
		expected["depositMode"] = exp.DepositMode
	}
	for key, want := range expected {
		got, present, err := rawString(echoed, key)
		if err != nil || !present || got != want {
			return quoteMismatch()
		}
	}
	return nil
}

func quoteMismatch() error {
	return domain.New(domain.CodeProviderQuoteMismatch,
		"The provider quote does not match the requested swap.")
}

func normalizeStatus(body []byte, now time.Time) (providers.StatusResult, error) {
	var payload rawStatusResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return providers.StatusResult{}, domain.Wrap(domain.CodeProviderInvalidStatus,
			"The swap provider returned an invalid status response.", err)
	}
	if strings.TrimSpace(payload.Status) == "" || strings.TrimSpace(payload.UpdatedAt) == "" {
		return providers.StatusResult{}, domain.New(domain.CodeProviderInvalidStatus,
			"The swap provider returned an invalid status response.")
	}
	updatedAt, err := time.Parse(time.RFC3339, payload.UpdatedAt)
	if err != nil {
		return providers.StatusResult{}, domain.New(domain.CodeProviderInvalidStatus,
			"The swap provider returned an invalid status timestamp.")
	}
	details, err := decodeSwapDetails(payload.SwapDetails)
	if err != nil {
		return providers.StatusResult{}, err
	}

	return providers.StatusResult{
		ProviderStatus:    domain.ProviderStatus(payload.Status).Normalize(),
		UpdatedAt:         updatedAt.UTC(),
		SwapDetails:       details,
		SourceTxHash:      firstHash(details, "originChainTxHashes"),
		DestinationTxHash: firstHash(details, "destinationChainTxHashes"),
		RefundTxHash:      firstHash(details, "refundChainTxHashes"),
		Raw:               append([]byte(nil), body...),
	}, nil
}

func decodeSwapDetails(raw json.RawMessage) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, domain.New(domain.CodeProviderInvalidStatus,
			"The swap provider returned an invalid status response.")
	}
	var details map[string]any
	if err := json.Unmarshal(trimmed, &details); err != nil {
		return nil, domain.New(domain.CodeProviderInvalidStatus,
			"The swap provider returned an invalid status response.")
	}
	return details, nil
}

func firstHash(details map[string]any, key string) string {
	list, ok := details[key].([]any)
	if !ok {
		return ""
	}
	for _, entry := range list {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hash, _ := item["hash"].(string)
		if strings.TrimSpace(hash) != "" {
			return hash
		}
	}
	return ""
}

func requiredString(raw map[string]json.RawMessage, key string) (string, error) {
	value, present, err := rawString(raw, key)
	if err != nil || !present || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing %q", key)
	}
	return value, nil
}

func requiredDigits(raw map[string]json.RawMessage, key string) (*big.Int, error) {
	value, err := requiredString(raw, key)
	if err != nil || !isDigits(value) {
		return nil, domain.New(domain.CodeProviderInvalidQuote,
			"The swap provider returned an incomplete quote.")
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok || parsed.Sign() < 0 {
		return nil, domain.New(domain.CodeProviderInvalidQuote,
			"The swap provider returned an incomplete quote.")
	}
	return parsed, nil
}

func hasNonNull(raw map[string]json.RawMessage, key string) bool {
	value, ok := raw[key]
	return ok && len(value) > 0 && string(value) != "null"
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func truncateRunes(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
