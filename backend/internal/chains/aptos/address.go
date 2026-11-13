// Package aptos validates Aptos account addresses and Move type tags.
package aptos

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// AddressLength is the byte length of an Aptos account address.
const AddressLength = 32

// ErrInvalidAddress is returned for malformed Aptos addresses.
var ErrInvalidAddress = errors.New("invalid Aptos address")

// ParseAddress validates an address with an optional 0x prefix and returns the
// canonical 0x-prefixed, 64-hex-character form. Short addresses are left-padded
// with zeroes. Both upper- and lowercase hex are accepted and normalized.
func ParseAddress(value string) (string, error) {
	body := strings.TrimPrefix(value, "0x")
	if body == "" || len(body) > 2*AddressLength {
		return "", fmt.Errorf("%w: expected 1-%d hex characters", ErrInvalidAddress, 2*AddressLength)
	}
	if strings.HasPrefix(value, "0X") {
		return "", fmt.Errorf("%w: prefix must be lowercase 0x", ErrInvalidAddress)
	}
	for _, char := range body {
		if !isHex(char) {
			return "", fmt.Errorf("%w: invalid character %q", ErrInvalidAddress, char)
		}
	}
	if len(body)%2 != 0 {
		body = "0" + body
	}
	raw, err := hex.DecodeString(body)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	padded := make([]byte, AddressLength)
	copy(padded[AddressLength-len(raw):], raw)
	return "0x" + hex.EncodeToString(padded), nil
}

// DecodeAddress validates an address and returns its 32 raw bytes.
func DecodeAddress(value string) ([]byte, error) {
	normalized, err := ParseAddress(value)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(normalized[2:])
}

// IsValid reports whether value is a valid Aptos address.
func IsValid(value string) bool {
	_, err := ParseAddress(value)
	return err == nil
}

func isHex(char rune) bool {
	return (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')
}
