package nearoneclick

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// The signature scheme mirrors the official 1Click TypeScript SDK:
//
//	message   = base58(sha256(stableJSON({...signedRequest, ...signedQuote, timestamp})))
//	signature = base58(ed25519 signature over the ASCII message)
//	publicKey = base58 ed25519 key, optionally prefixed with "ed25519:"
//
// Go's encoding/json sorts map keys, which matches json-stable-stringify for the
// primitive values used here.

const ed25519Prefix = "ed25519:"

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var signedRequestFields = []string{
	"swapType",
	"originAsset",
	"depositType",
	"destinationAsset",
	"amount",
	"refundTo",
	"refundType",
	"recipient",
	"recipientType",
	"deadline",
}

var signedRequestOptionalStrings = []string{
	"referral",
	"virtualChainRecipient",
	"virtualChainRefundRecipient",
	"customRecipientMsg",
}

var signedQuoteRequiredStrings = []string{
	"amountIn",
	"amountInFormatted",
	"amountInUsd",
	"minAmountIn",
	"amountOut",
	"amountOutFormatted",
	"amountOutUsd",
	"minAmountOut",
}

var signedQuoteOptionalStrings = []string{
	"depositAddress",
	"depositMemo",
	"deadline",
	"timeWhenInactive",
	"refundFee",
	"withdrawFee",
}

// quoteHash returns the base58-encoded SHA-256 of the canonical signed payload.
func quoteHash(request, quote map[string]json.RawMessage, timestamp string) (string, error) {
	requestFields, err := canonicalSignedRequest(request)
	if err != nil {
		return "", err
	}
	dry, ok, err := rawBool(request, "dry")
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errMissingSignedField("dry")
	}
	quoteFields, err := canonicalSignedQuote(dry, quote)
	if err != nil {
		return "", err
	}

	merged := make(map[string]any, len(requestFields)+len(quoteFields)+1)
	for key, value := range requestFields {
		merged[key] = value
	}
	for key, value := range quoteFields {
		merged[key] = value
	}
	merged["timestamp"] = timestamp

	data, err := marshalCanonical(merged)
	if err != nil {
		return "", fmt.Errorf("marshal signed quote payload: %w", err)
	}
	sum := sha256.Sum256(data)
	return base58Encode(sum[:]), nil
}

// marshalCanonical matches JSON.stringify for the primitive values in a signed
// payload: map keys sorted, no extra whitespace, and no HTML escaping.
func marshalCanonical(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func canonicalSignedRequest(raw map[string]json.RawMessage) (map[string]any, error) {
	out := make(map[string]any, 16)

	dry, ok, err := rawBool(raw, "dry")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errMissingSignedField("dry")
	}
	out["dry"] = dry

	slippage, ok, err := rawInt(raw, "slippageTolerance")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errMissingSignedField("slippageTolerance")
	}
	out["slippageTolerance"] = slippage

	for _, key := range signedRequestFields {
		value, present, err := canonicalString(raw, key)
		if err != nil {
			return nil, err
		}
		if present {
			out[key] = value
		}
	}

	if waiting, ok, err := rawInt(raw, "quoteWaitingTimeMs"); err != nil {
		return nil, err
	} else if ok && waiting != 0 {
		out["quoteWaitingTimeMs"] = waiting
	}

	for _, key := range signedRequestOptionalStrings {
		value, present, err := rawString(raw, key)
		if err != nil {
			return nil, err
		}
		if present && value != "" {
			out[key] = value
		}
	}
	return out, nil
}

func canonicalSignedQuote(dry bool, raw map[string]json.RawMessage) (map[string]any, error) {
	out := make(map[string]any, len(signedQuoteRequiredStrings)+8)

	for _, key := range signedQuoteRequiredStrings {
		// Required fields are copied by the SDK even when their value is null;
		// only a missing field (undefined) is dropped from the signed payload.
		value, present, err := canonicalString(raw, key)
		if err != nil {
			return nil, err
		}
		if present {
			out[key] = value
		}
	}

	if dry {
		return out, nil
	}

	for _, key := range signedQuoteOptionalStrings {
		value, present, err := rawString(raw, key)
		if err != nil {
			return nil, err
		}
		if present && value != "" {
			out[key] = value
		}
	}

	if estimate, ok, err := rawInt(raw, "timeEstimate"); err != nil {
		return nil, err
	} else if ok && estimate != 0 {
		out["timeEstimate"] = estimate
	}
	return out, nil
}

// VerifySignedQuote verifies a provider signature using the documented public
// key. An empty public key reports false without an error; a malformed key or
// signature is an error so callers can distinguish configuration mistakes from
// a signature that simply does not match.
func VerifySignedQuote(
	request, quote map[string]json.RawMessage,
	timestamp, signature, publicKey string,
) (bool, error) {
	if strings.TrimSpace(publicKey) == "" {
		return false, nil
	}
	hash, err := quoteHash(request, quote, timestamp)
	if err != nil {
		return false, err
	}
	signatureBytes, err := decodeBase58(strings.TrimPrefix(strings.TrimSpace(signature), ed25519Prefix))
	if err != nil {
		return false, fmt.Errorf("decode quote signature: %w", err)
	}
	publicKeyBytes, err := decodeBase58(strings.TrimPrefix(strings.TrimSpace(publicKey), ed25519Prefix))
	if err != nil {
		return false, fmt.Errorf("decode quote public key: %w", err)
	}
	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("quote public key is %d bytes, want %d", len(publicKeyBytes), ed25519.PublicKeySize)
	}
	if len(signatureBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("quote signature is %d bytes, want %d", len(signatureBytes), ed25519.SignatureSize)
	}
	return ed25519.Verify(ed25519.PublicKey(publicKeyBytes), []byte(hash), signatureBytes), nil
}

func errMissingSignedField(name string) error {
	return fmt.Errorf("signed quote is missing %q", name)
}

// canonicalString distinguishes a missing key (undefined in the SDK, omitted
// from the payload) from an explicit null, which JSON.stringify preserves.
func canonicalString(raw map[string]json.RawMessage, key string) (any, bool, error) {
	value, ok := raw[key]
	if !ok || len(value) == 0 {
		return nil, false, nil
	}
	if string(value) == "null" {
		return nil, true, nil
	}
	var out string
	if err := json.Unmarshal(value, &out); err != nil {
		return nil, false, fmt.Errorf("decode %q: %w", key, err)
	}
	return out, true, nil
}

func rawString(raw map[string]json.RawMessage, key string) (string, bool, error) {
	value, ok := raw[key]
	if !ok || len(value) == 0 || string(value) == "null" {
		return "", false, nil
	}
	var out string
	if err := json.Unmarshal(value, &out); err != nil {
		return "", false, fmt.Errorf("decode %q: %w", key, err)
	}
	return out, true, nil
}

func rawInt(raw map[string]json.RawMessage, key string) (int64, bool, error) {
	value, ok := raw[key]
	if !ok || len(value) == 0 || string(value) == "null" {
		return 0, false, nil
	}
	var out int64
	if err := json.Unmarshal(value, &out); err != nil {
		return 0, false, fmt.Errorf("decode %q: %w", key, err)
	}
	return out, true, nil
}

func rawBool(raw map[string]json.RawMessage, key string) (bool, bool, error) {
	value, ok := raw[key]
	if !ok || len(value) == 0 || string(value) == "null" {
		return false, false, nil
	}
	var out bool
	if err := json.Unmarshal(value, &out); err != nil {
		return false, false, fmt.Errorf("decode %q: %w", key, err)
	}
	return out, true, nil
}

func base58Encode(input []byte) string {
	value := new(big.Int).SetBytes(input)
	radix := big.NewInt(58)
	mod := new(big.Int)
	var encoded []byte
	for value.Sign() > 0 {
		value.QuoRem(value, radix, mod)
		encoded = append(encoded, base58Alphabet[mod.Int64()])
	}
	for _, b := range input {
		if b != 0 {
			break
		}
		encoded = append(encoded, base58Alphabet[0])
	}
	for left, right := 0, len(encoded)-1; left < right; left, right = left+1, right-1 {
		encoded[left], encoded[right] = encoded[right], encoded[left]
	}
	if len(encoded) == 0 {
		return string(base58Alphabet[0])
	}
	return string(encoded)
}

func decodeBase58(input string) ([]byte, error) {
	if input == "" {
		return nil, fmt.Errorf("base58 value is empty")
	}
	value := new(big.Int)
	radix := big.NewInt(58)
	for _, char := range input {
		index := strings.IndexRune(base58Alphabet, char)
		if index < 0 {
			return nil, fmt.Errorf("invalid base58 character %q", char)
		}
		value.Mul(value, radix)
		value.Add(value, big.NewInt(int64(index)))
	}
	decoded := value.Bytes()
	leadingZeros := 0
	for leadingZeros < len(input) && input[leadingZeros] == '1' {
		leadingZeros++
	}
	return append(make([]byte, leadingZeros), decoded...), nil
}
