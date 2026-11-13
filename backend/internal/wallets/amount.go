package wallets

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

var ErrInvalidAmount = errors.New("invalid transfer amount")

// parseDecimal parses a non-negative base-10 integer no larger than maxBits.
func parseDecimal(value string, maxBits int) (*big.Int, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return nil, fmt.Errorf("%w: empty or padded value", ErrInvalidAmount)
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return nil, fmt.Errorf("%w: %q is not a base-10 integer", ErrInvalidAmount, value)
		}
	}
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAmount, value)
	}
	if amount.BitLen() > maxBits {
		return nil, fmt.Errorf("%w: exceeds %d bits", ErrInvalidAmount, maxBits)
	}
	return amount, nil
}

func parsePositiveDecimal(value string, maxBits int) (*big.Int, error) {
	amount, err := parseDecimal(value, maxBits)
	if err != nil {
		return nil, err
	}
	if amount.Sign() == 0 {
		return nil, fmt.Errorf("%w: must be greater than zero", ErrInvalidAmount)
	}
	return amount, nil
}
