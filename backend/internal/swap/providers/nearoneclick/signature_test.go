package nearoneclick

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
)

// These hashes were produced by the official @defuse-protocol/one-click-sdk-
// typescript `quoteHash()` for the same redacted fixtures. They pin the
// canonical JSON and base58 implementation to the SDK.
func TestQuoteHashMatchesOfficialSDK(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fixture string
		want    string
	}{
		{fixture: "quote_dry.json", want: "7pozkY3rSdu7Kpmakt6j99FGmMhsonR8MmGJ4SaScqQB"},
		{fixture: "quote_executable.json", want: "3ZvFm2FZYHzbdAfc1ZhYnB4wMm5iPpsDiEEUaBfk5FVP"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()
			var payload struct {
				Timestamp    string                     `json:"timestamp"`
				QuoteRequest map[string]json.RawMessage `json:"quoteRequest"`
				Quote        map[string]json.RawMessage `json:"quote"`
			}
			if err := json.Unmarshal(readFile(t, tc.fixture), &payload); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			hash, err := quoteHash(payload.QuoteRequest, payload.Quote, payload.Timestamp)
			if err != nil {
				t.Fatalf("quoteHash() error: %v", err)
			}
			if hash != tc.want {
				t.Fatalf("hash = %s, want %s", hash, tc.want)
			}
		})
	}
}

func TestBase58RoundTrip(t *testing.T) {
	t.Parallel()
	cases := [][]byte{
		{},
		{0},
		{0, 0, 1},
		{1, 2, 3, 4, 5},
		[]byte("the quick brown fox"),
	}
	for _, input := range cases {
		encoded := base58Encode(input)
		decoded, err := decodeBase58(encoded)
		if err != nil {
			t.Fatalf("decode %q: %v", encoded, err)
		}
		if len(input) == 0 {
			continue
		}
		// decodeBase58 preserves leading zero bytes, including for inputs that
		// start with zero.
		if string(decoded) != string(input) {
			t.Fatalf("round trip %v -> %q -> %v", input, encoded, decoded)
		}
	}
}

func TestVerifySignedQuoteWithGeneratedKey(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	body := signedQuoteFixture(t, privateKey, true)
	var payload struct {
		Timestamp    string                     `json:"timestamp"`
		Signature    string                     `json:"signature"`
		QuoteRequest map[string]json.RawMessage `json:"quoteRequest"`
		Quote        map[string]json.RawMessage `json:"quote"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode signed payload: %v", err)
	}

	verified, err := VerifySignedQuote(payload.QuoteRequest, payload.Quote, payload.Timestamp, payload.Signature, base58Encode(publicKey))
	if err != nil {
		t.Fatalf("VerifySignedQuote() error: %v", err)
	}
	if !verified {
		t.Fatal("expected generated signature to verify")
	}

	if verified, err := VerifySignedQuote(payload.QuoteRequest, payload.Quote, payload.Timestamp, payload.Signature, ""); err != nil || verified {
		t.Fatalf("empty key should report false without error, got %t/%v", verified, err)
	}
}
