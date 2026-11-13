package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
)

const (
	minIdempotencyKeyLength = 8
	maxIdempotencyKeyLength = 128
)

// ValidateIdempotencyKey checks that the provided idempotency key meets the
// required length and character constraints.
func ValidateIdempotencyKey(key string) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return domain.New(domain.CodeIdempotencyKeyRequired, "Idempotency-Key header is required.")
	}
	if len(trimmed) < minIdempotencyKeyLength || len(trimmed) > maxIdempotencyKeyLength {
		return domain.Newf(domain.CodeValidationFailed,
			"Idempotency-Key must be between %d and %d characters.",
			minIdempotencyKeyLength, maxIdempotencyKeyLength)
	}
	for _, char := range trimmed {
		if char < 32 || char > 126 {
			return domain.New(domain.CodeValidationFailed, "Idempotency-Key contains invalid characters.")
		}
	}
	return nil
}

// CanonicalPayloadHash computes a deterministic SHA-256 hex hash of the input
// payload by round-tripping through generic maps/slices to ensure sorted keys.
func CanonicalPayloadHash(payload any) (string, error) {
	if payload == nil {
		return "", domain.New(domain.CodeValidationFailed, "Payload cannot be nil.")
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", domain.Wrap(domain.CodeInvalidJSON, "Could not encode payload for idempotency.", err)
	}

	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "", domain.Wrap(domain.CodeInvalidJSON, "Could not parse payload for idempotency.", err)
	}

	canonical, err := json.Marshal(generic)
	if err != nil {
		return "", domain.Wrap(domain.CodeInvalidJSON, "Could not canonicalize payload for idempotency.", err)
	}

	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
