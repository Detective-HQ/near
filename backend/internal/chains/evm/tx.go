package evm

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseChainID accepts a decimal chain ID or an RPC-style 0x-prefixed quantity
// and returns its decimal value. Zero is rejected.
func ParseChainID(value string) (uint64, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return 0, fmt.Errorf("%w: empty or padded value", ErrInvalidChainID)
	}
	var id uint64
	var err error
	if strings.HasPrefix(value, "0x") {
		digits := value[2:]
		if digits == "" || len(digits) > 16 {
			return 0, fmt.Errorf("%w: %q", ErrInvalidChainID, value)
		}
		id, err = strconv.ParseUint(digits, 16, 64)
	} else {
		if len(value) > 1 && value[0] == '0' {
			return 0, fmt.Errorf("%w: leading zero in %q", ErrInvalidChainID, value)
		}
		if len(value) > 20 {
			return 0, fmt.Errorf("%w: %q", ErrInvalidChainID, value)
		}
		id, err = strconv.ParseUint(value, 10, 64)
	}
	if err != nil || id == 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidChainID, value)
	}
	return id, nil
}

// ParseTxHash validates a 0x-prefixed 32-byte transaction hash and returns its
// lowercase form.
func ParseTxHash(value string) (string, error) {
	if !strings.HasPrefix(value, "0x") || len(value) != 2+64 {
		return "", fmt.Errorf("%w: expected 0x followed by 64 hex characters", ErrInvalidTxHash)
	}
	body := value[2:]
	for _, char := range body {
		if !isHex(char) {
			return "", fmt.Errorf("%w: invalid character %q", ErrInvalidTxHash, char)
		}
	}
	return "0x" + strings.ToLower(body), nil
}

// ValidateTxHash reports whether value is a valid EVM transaction hash.
func ValidateTxHash(value string) bool {
	_, err := ParseTxHash(value)
	return err == nil
}

func isHex(char rune) bool {
	return (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')
}
