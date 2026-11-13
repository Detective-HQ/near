package nearoneclick

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

var testNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func TestNewValidatesBaseURL(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{BaseURL: "http://provider.example.test"}); err == nil {
		t.Fatal("expected non-local HTTP base URL to be rejected")
	}
	if _, err := New(Config{BaseURL: "https://provider.example.test/path?x=1"}); err == nil {
		t.Fatal("expected base URL query to be rejected")
	}
	if _, err := New(Config{BaseURL: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("localhost HTTP should be allowed for tests: %v", err)
	}
}

func TestGetTokensNormalizesCatalogue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/tokens" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("authorization") != "" {
			t.Error("catalogue request should not send the partner JWT")
		}
		writeFile(t, w, "tokens.json")
	}, nil)

	assets, err := client.GetTokens(context.Background())
	if err != nil {
		t.Fatalf("GetTokens() error: %v", err)
	}
	if len(assets) != 3 {
		t.Fatalf("len(assets) = %d", len(assets))
	}
	first := assets[0]
	if first.ID != "nep141:wrap.near" || first.ProviderAssetID != "nep141:wrap.near" {
		t.Fatalf("unexpected ids: %+v", first)
	}
	if first.Chain != "near" || first.Decimals != 24 || first.ContractAddress != "wrap.near" {
		t.Fatalf("unexpected asset: %+v", first)
	}
	if first.Name != "Wrapped NEAR" || !first.Enabled {
		t.Fatalf("unexpected name/enabled: %+v", first)
	}
	if !first.ObservedAt.Equal(time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("ObservedAt = %s", first.ObservedAt)
	}
	if assets[1].Name != "USDC" {
		t.Fatalf("name should fall back to symbol, got %q", assets[1].Name)
	}
}

func TestGetTokensRejectsInvalidEntries(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"assetId":"a","blockchain":"near","symbol":"A","decimals":40}]`)
	}, nil)
	if _, err := client.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderInvalidCatalogue) {
		t.Fatalf("err = %v, want PROVIDER_INVALID_CATALOGUE", err)
	}

	duplicate := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
			{"assetId":"a","blockchain":"near","symbol":"A","decimals":6},
			{"assetId":"a","blockchain":"near","symbol":"A","decimals":6}
		]`)
	}, nil)
	if _, err := duplicate.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderInvalidCatalogue) {
		t.Fatalf("err = %v, want PROVIDER_INVALID_CATALOGUE", err)
	}

	notArray := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}, nil)
	if _, err := notArray.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderInvalidCatalogue) {
		t.Fatalf("err = %v, want PROVIDER_INVALID_CATALOGUE", err)
	}
}

func TestGetDryQuoteSendsExactInputAndReturnsNoDepositData(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/quote" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("authorization"); got != "Bearer server-only-secret" {
			t.Errorf("authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["dry"] != true {
			t.Errorf("dry = %v", body["dry"])
		}
		if body["swapType"] != "EXACT_INPUT" || body["depositType"] != "ORIGIN_CHAIN" {
			t.Errorf("unexpected body: %v", body)
		}
		if body["amount"] != "1000000000000000000000000" {
			t.Errorf("amount = %v", body["amount"])
		}
		if _, present := body["deadline"]; present {
			t.Error("dry quote must not request a deadline")
		}
		writeFile(t, w, "quote_dry.json")
	}, nil)

	result, err := client.GetDryQuote(context.Background(), providers.DryQuoteRequest{
		OriginAsset:      nearAsset("nep141:wrap.near", 24),
		DestinationAsset: nearAsset("nep141:usdc.near", 6),
		AmountAtomic:     new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil),
		Recipient:        "recipient.near",
		RefundTo:         "refund.near",
		SlippageBPS:      100,
	})
	if err != nil {
		t.Fatalf("GetDryQuote() error: %v", err)
	}
	if result.Quote.Mode != domain.QuoteModeProviderDry {
		t.Errorf("mode = %q", result.Quote.Mode)
	}
	if result.Quote.ProviderQuoteID != "dry-correlation-1" {
		t.Errorf("provider quote id = %q", result.Quote.ProviderQuoteID)
	}
	if result.Quote.AmountOutEstimateAtomic.String() != "9900000" {
		t.Errorf("amount out = %s", result.Quote.AmountOutEstimateAtomic)
	}
	if result.Quote.MinAmountOutAtomic.String() != "9800000" {
		t.Errorf("min amount out = %s", result.Quote.MinAmountOutAtomic)
	}
	if result.Quote.AmountInDecimal != "1" {
		t.Errorf("amount in decimal = %q", result.Quote.AmountInDecimal)
	}
	if result.Quote.DepositAddress != "" || result.Quote.DepositMemo != "" || !result.Quote.ExpiresAt.IsZero() {
		t.Errorf("dry quote leaked deposit data: %+v", result.Quote)
	}
	if result.SignatureVerified {
		t.Error("signature must not be marked verified without a public key")
	}
	if len(result.Raw) == 0 || len(result.Quote.RawResponse) == 0 {
		t.Error("raw response should be preserved")
	}
	if result.AmountOutFormatted != "9.9" || result.MinAmountOutFormatted != "9.8" {
		t.Errorf("formatted amounts = %q / %q", result.AmountOutFormatted, result.MinAmountOutFormatted)
	}
}

func TestGetExecutableQuoteNormalizesDepositInstructions(t *testing.T) {
	t.Parallel()
	deadline := testNow.Add(3 * time.Minute)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["dry"] != false {
			t.Errorf("dry = %v", body["dry"])
		}
		if body["deadline"] != "2026-11-10T12:03:00Z" || body["depositMode"] != "SIMPLE" {
			t.Errorf("unexpected executable fields: %v", body)
		}
		writeFile(t, w, "quote_executable.json")
	}, nil)

	result, err := client.GetExecutableQuote(context.Background(), providers.ExecutableQuoteRequest{
		DryQuoteRequest: providers.DryQuoteRequest{
			OriginAsset:      evmAsset("eip155:1/erc20:0x1111111111111111111111111111111111111111", 6),
			DestinationAsset: evmAsset("eip155:8453/erc20:0x2222222222222222222222222222222222222222", 6),
			AmountAtomic:     big.NewInt(1_000_000),
			Recipient:        "0x3333333333333333333333333333333333333333",
			RefundTo:         "0x4444444444444444444444444444444444444444",
			SlippageBPS:      100,
		},
		Deadline:    deadline,
		DepositMode: "SIMPLE",
	})
	if err != nil {
		t.Fatalf("GetExecutableQuote() error: %v", err)
	}
	if result.Quote.Mode != domain.QuoteModeProviderExecutable {
		t.Errorf("mode = %q", result.Quote.Mode)
	}
	if result.Quote.DepositAddress != "0x1234567890abcdef1234567890abcdef12345678" {
		t.Errorf("deposit address = %q", result.Quote.DepositAddress)
	}
	if !result.Quote.ExpiresAt.Equal(deadline) {
		t.Errorf("expires at = %s, want %s", result.Quote.ExpiresAt, deadline)
	}
	if !result.TimeWhenInactive.Equal(testNow.Add(time.Minute)) {
		t.Errorf("timeWhenInactive = %s", result.TimeWhenInactive)
	}
	if result.TimeEstimateSeconds != 30 {
		t.Errorf("time estimate = %d", result.TimeEstimateSeconds)
	}
	if verified, _ := result.Quote.FeeData["signatureVerified"].(bool); verified {
		t.Error("signature must not be marked verified without a public key")
	}
}

func TestGetExecutableQuoteRejectsInvalidDeadline(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("provider should not be called for an invalid deadline")
	}, nil)
	_, err := client.GetExecutableQuote(context.Background(), providers.ExecutableQuoteRequest{
		DryQuoteRequest: providers.DryQuoteRequest{
			OriginAsset:      nearAsset("a", 6),
			DestinationAsset: nearAsset("b", 6),
			AmountAtomic:     big.NewInt(1),
			Recipient:        "recipient",
			RefundTo:         "refund",
		},
		Deadline: testNow,
	})
	if !domain.IsCode(err, domain.CodeInvalidDeadline) {
		t.Fatalf("err = %v, want INVALID_DEADLINE", err)
	}
}

func TestQuoteRejectsMismatchedSignedRequest(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		payload := readFile(t, "quote_dry.json")
		var body map[string]any
		_ = json.Unmarshal(payload, &body)
		request := body["quoteRequest"].(map[string]any)
		request["amount"] = "2"
		encoded, _ := json.Marshal(body)
		_, _ = w.Write(encoded)
	}, nil)

	_, err := client.GetDryQuote(context.Background(), dryRequest())
	if !domain.IsCode(err, domain.CodeProviderQuoteMismatch) {
		t.Fatalf("err = %v, want PROVIDER_QUOTE_MISMATCH", err)
	}
}

func TestDryQuoteRejectsDepositInstructions(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		payload := readFile(t, "quote_dry.json")
		var body map[string]any
		_ = json.Unmarshal(payload, &body)
		quote := body["quote"].(map[string]any)
		quote["depositAddress"] = "must-not-escape"
		encoded, _ := json.Marshal(body)
		_, _ = w.Write(encoded)
	}, nil)

	_, err := client.GetDryQuote(context.Background(), dryRequest())
	if !domain.IsCode(err, domain.CodeProviderUnexpectedData) {
		t.Fatalf("err = %v, want PROVIDER_UNEXPECTED_DEPOSIT_DATA", err)
	}
}

func TestQuoteSignatureVerification(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	signedBody := signedQuoteFixture(t, privateKey, true)

	verifiedClient := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(signedBody)
	}, func(cfg *Config) {
		cfg.QuotePublicKey = "ed25519:" + base58Encode(publicKey)
	})
	result, err := verifiedClient.GetDryQuote(context.Background(), dryRequest())
	if err != nil {
		t.Fatalf("GetDryQuote() error: %v", err)
	}
	if !result.SignatureVerified {
		t.Error("expected signature to be verified")
	}

	invalidClient := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		payload := readFile(t, "quote_dry.json")
		var body map[string]any
		_ = json.Unmarshal(payload, &body)
		body["signature"] = base58Encode(make([]byte, ed25519.SignatureSize))
		encoded, _ := json.Marshal(body)
		_, _ = w.Write(encoded)
	}, func(cfg *Config) {
		cfg.QuotePublicKey = base58Encode(publicKey)
	})
	if _, err := invalidClient.GetDryQuote(context.Background(), dryRequest()); !domain.IsCode(err, domain.CodeProviderSignatureInvalid) {
		t.Fatalf("err = %v, want PROVIDER_SIGNATURE_INVALID", err)
	}
}

func TestHTTPErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status     int
		header     http.Header
		wantCode   domain.Code
		retryAfter time.Duration
	}{
		{status: http.StatusUnauthorized, wantCode: domain.CodeProviderAuthFailed},
		{status: http.StatusForbidden, wantCode: domain.CodeProviderAuthFailed},
		{status: http.StatusTooManyRequests, header: http.Header{"Retry-After": []string{"7"}}, wantCode: domain.CodeProviderRateLimited, retryAfter: 7 * time.Second},
		{status: http.StatusInternalServerError, wantCode: domain.CodeProviderDelayed},
		{status: http.StatusBadRequest, wantCode: domain.CodeProviderRequestRejected},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				for key, values := range tc.header {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"message":"sensitive upstream detail"}`)
			}, nil)
			_, err := client.GetDryQuote(context.Background(), dryRequest())
			typed, ok := domain.AsError(err)
			if !ok || typed.Code != tc.wantCode {
				t.Fatalf("err = %v, want %s", err, tc.wantCode)
			}
			if typed.Message == "" || strings.Contains(typed.Message, "sensitive") {
				t.Fatalf("error message leaked upstream detail: %q", typed.Message)
			}
			if typed.RetryAfter != tc.retryAfter {
				t.Fatalf("retry after = %s, want %s", typed.RetryAfter, tc.retryAfter)
			}
		})
	}
}

func TestProviderTimeoutAndOversizedResponse(t *testing.T) {
	t.Parallel()
	slow := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, `{}`)
	}, func(cfg *Config) {
		cfg.Timeout = 100 * time.Millisecond
	})
	if _, err := slow.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeRPCTimeout) {
		t.Fatalf("err = %v, want RPC_TIMEOUT", err)
	}

	// The default ceiling is 2 MB; one byte over must be rejected.
	oversized := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("a", defaultMaxResponseBytes+1))
	}, nil)
	if _, err := oversized.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderResponseTooLarge) {
		t.Fatalf("err = %v, want PROVIDER_RESPONSE_TOO_LARGE", err)
	}
}

func TestMalformedResponseAndMissingConfiguration(t *testing.T) {
	t.Parallel()
	malformed := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not-json`)
	}, nil)
	if _, err := malformed.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderInvalidResponse) {
		t.Fatalf("err = %v, want PROVIDER_INVALID_RESPONSE", err)
	}

	noJWT := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("authenticated request should not be sent without a JWT")
	}, func(cfg *Config) {
		cfg.JWT = ""
	})
	if _, err := noJWT.GetDryQuote(context.Background(), dryRequest()); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("err = %v, want PROVIDER_NOT_CONFIGURED", err)
	}
	if noJWT.Health().Configured {
		t.Error("Health().Configured should be false without a JWT")
	}
}

func TestStatusAndSubmitDeposit(t *testing.T) {
	t.Parallel()
	var calls []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("authorization"); got != "Bearer server-only-secret" {
			t.Errorf("authorization = %q", got)
		}
		switch {
		case r.URL.Path == "/v0/status":
			calls = append(calls, "status")
			if r.URL.Query().Get("depositAddress") != "0x1234567890abcdef1234567890abcdef12345678" {
				t.Errorf("deposit address = %q", r.URL.Query().Get("depositAddress"))
			}
			if r.URL.Query().Get("depositMemo") != "memo / 1" {
				t.Errorf("deposit memo = %q", r.URL.Query().Get("depositMemo"))
			}
			writeFile(t, w, "status.json")
		case r.URL.Path == "/v0/deposit/submit":
			calls = append(calls, "submit")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["txHash"] != "0x"+strings.Repeat("a", 64) || body["depositAddress"] != "0x1234567890abcdef1234567890abcdef12345678" {
				t.Errorf("unexpected submit body: %v", body)
			}
			if _, present := body["memo"]; present {
				t.Errorf("memo should be omitted when empty: %v", body)
			}
			writeFile(t, w, "submit.json")
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}, nil)

	status, err := client.GetStatus(context.Background(), providers.StatusRequest{
		DepositAddress: "0x1234567890abcdef1234567890abcdef12345678",
		DepositMemo:    "memo / 1",
	})
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if status.ProviderStatus != domain.ProviderStatusProcessing {
		t.Errorf("status = %q", status.ProviderStatus)
	}
	if status.SourceTxHash != "0x"+strings.Repeat("a", 64) || status.DestinationTxHash != "0x"+strings.Repeat("b", 64) {
		t.Errorf("unexpected tx hashes: %+v", status)
	}

	ack, err := client.SubmitDepositTx(context.Background(), providers.DepositRequest{
		TxHash:         "0x" + strings.Repeat("a", 64),
		DepositAddress: "0x1234567890abcdef1234567890abcdef12345678",
	})
	if err != nil {
		t.Fatalf("SubmitDepositTx() error: %v", err)
	}
	if ack.ProviderStatus != domain.ProviderStatusKnownDepositTx {
		t.Errorf("ack status = %q", ack.ProviderStatus)
	}
	if len(calls) != 2 {
		t.Errorf("calls = %v", calls)
	}
}

func TestStatusRejectsInvalidShape(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"PROCESSING","updatedAt":"2026-11-10T12:02:00Z","swapDetails":[]}`)
	}, nil)
	_, err := client.GetStatus(context.Background(), providers.StatusRequest{DepositAddress: "address"})
	if !domain.IsCode(err, domain.CodeProviderInvalidStatus) {
		t.Fatalf("err = %v, want PROVIDER_INVALID_STATUS", err)
	}
}

func TestSubmitDepositValidatesInput(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input should not reach the provider")
	}, nil)
	_, err := client.SubmitDepositTx(context.Background(), providers.DepositRequest{DepositAddress: "address"})
	if !domain.IsCode(err, domain.CodeInvalidTxHash) {
		t.Fatalf("err = %v, want INVALID_TRANSACTION_HASH", err)
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc, mutate func(*Config)) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := Config{
		JWT:              "server-only-secret",
		Timeout:          2 * time.Second,
		MaxResponseBytes: 2_000_000,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:              func() time.Time { return testNow },
	}
	if mutate != nil {
		mutate(&cfg)
	}
	cfg.BaseURL = server.URL
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return client
}

func dryRequest() providers.DryQuoteRequest {
	return providers.DryQuoteRequest{
		OriginAsset:      nearAsset("nep141:wrap.near", 24),
		DestinationAsset: nearAsset("nep141:usdc.near", 6),
		AmountAtomic:     new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil),
		Recipient:        "recipient.near",
		RefundTo:         "refund.near",
		SlippageBPS:      100,
	}
}

func nearAsset(id string, decimals int) domain.Asset {
	return domain.Asset{ID: id, ProviderAssetID: id, Chain: "near", Symbol: "TKN", Decimals: decimals}
}

func evmAsset(id string, decimals int) domain.Asset {
	return domain.Asset{ID: id, ProviderAssetID: id, Chain: "eth", Symbol: "USDC", Decimals: decimals}
}

func signedQuoteFixture(t *testing.T, privateKey ed25519.PrivateKey, dry bool) []byte {
	t.Helper()
	fixture := "quote_dry.json"
	if !dry {
		fixture = "quote_executable.json"
	}
	var payload struct {
		CorrelationID string                     `json:"correlationId"`
		Timestamp     string                     `json:"timestamp"`
		QuoteRequest  map[string]json.RawMessage `json:"quoteRequest"`
		Quote         map[string]json.RawMessage `json:"quote"`
	}
	if err := json.Unmarshal(readFile(t, fixture), &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	hash, err := quoteHash(payload.QuoteRequest, payload.Quote, payload.Timestamp)
	if err != nil {
		t.Fatalf("quoteHash() error: %v", err)
	}
	signature := ed25519.Sign(privateKey, []byte(hash))

	body := map[string]any{
		"correlationId": payload.CorrelationID,
		"timestamp":     payload.Timestamp,
		"signature":     base58Encode(signature),
		"quoteRequest":  payload.QuoteRequest,
		"quote":         payload.Quote,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal signed payload: %v", err)
	}
	return encoded
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func writeFile(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	w.Header().Set("content-type", "application/json")
	_, _ = w.Write(readFile(t, name))
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	if got := parseRetryAfter("5", testNow); got != 5*time.Second {
		t.Errorf("seconds = %s", got)
	}
	future := testNow.Add(30 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(future, testNow); got != 30*time.Second {
		t.Errorf("http date = %s", got)
	}
	if got := parseRetryAfter("invalid", testNow); got != 0 {
		t.Errorf("invalid = %s", got)
	}
}
