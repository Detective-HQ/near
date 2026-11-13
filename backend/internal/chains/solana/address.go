// Package solana validates Solana public keys, derives associated token
// accounts, and resolves SPL token accounts over JSON-RPC.
package solana

import (
	"errors"
	"fmt"

	"universal-defi-intelligence/internal/encoding/base58"
)

// PublicKeyLength is the byte length of a Solana public key.
const PublicKeyLength = 32

var (
	// ErrInvalidAddress is returned for malformed Solana public keys.
	ErrInvalidAddress = errors.New("invalid Solana address")
	// ErrTokenAccountNotFound is returned when an owner has no token account.
	ErrTokenAccountNotFound = errors.New("SPL token account not found")
)

// ParseAddress validates a base58 public key and returns its canonical form.
func ParseAddress(value string) (string, error) {
	raw, err := DecodeAddress(value)
	if err != nil {
		return "", err
	}
	return base58.Encode(raw), nil
}

// DecodeAddress validates a base58 public key and returns its 32 raw bytes.
func DecodeAddress(value string) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidAddress)
	}
	for _, char := range value {
		switch char {
		case '0', 'O', 'I', 'l':
			return nil, fmt.Errorf("%w: invalid base58 character %q", ErrInvalidAddress, char)
		}
	}
	raw, err := base58.Decode(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	if len(raw) != PublicKeyLength {
		return nil, fmt.Errorf("%w: decoded to %d bytes, want %d", ErrInvalidAddress, len(raw), PublicKeyLength)
	}
	return raw, nil
}

// EncodeAddress returns the base58 form of a 32-byte public key.
func EncodeAddress(raw []byte) (string, error) {
	if len(raw) != PublicKeyLength {
		return "", fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidAddress, PublicKeyLength, len(raw))
	}
	return base58.Encode(raw), nil
}

// IsValid reports whether value is a valid Solana public key.
func IsValid(value string) bool {
	_, err := ParseAddress(value)
	return err == nil
}
