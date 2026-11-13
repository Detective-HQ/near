package domain

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const (
	// MaxAmountDigits mirrors the prototype's input cap. It is deliberately
	// generous for human-readable amounts while still bounding parsing work.
	MaxAmountDigits = 80
	// MaxDecimals is the largest supported asset precision.
	MaxDecimals = 36
)

var decimalAmountPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// ParseUnits converts a positive human-readable decimal string into its exact
// atomic integer representation.
//
// Rules:
//   - only digits and at most one decimal point are allowed;
//   - no sign, exponent, comma, whitespace, or empty fraction;
//   - the fraction may not exceed the asset's decimals;
//   - the resulting amount must be greater than zero.
//
// No floating-point arithmetic is used at any point.
func ParseUnits(value string, decimals int) (*big.Int, error) {
	if value == "" || len(value) > MaxAmountDigits || !decimalAmountPattern.MatchString(value) {
		return nil, New(CodeInvalidAmount, "Enter a positive decimal amount.")
	}
	if decimals < 0 || decimals > MaxDecimals {
		return nil, New(CodeInvalidDecimals, "Asset precision is invalid.")
	}

	whole, fraction, _ := strings.Cut(value, ".")
	if len(fraction) > decimals {
		return nil, Newf(CodeTooManyDecimals, "This asset supports at most %d decimal places.", decimals)
	}

	// Left-pad the fraction to the asset precision and parse the concatenation
	// as one integer. This avoids multiple rounding-prone conversions.
	scaled := whole + fraction + strings.Repeat("0", decimals-len(fraction))
	atomic, ok := new(big.Int).SetString(scaled, 10)
	if !ok {
		return nil, New(CodeInvalidAmount, "Enter a positive decimal amount.")
	}
	if atomic.Sign() <= 0 {
		return nil, New(CodeInvalidAmount, "Amount must be greater than zero.")
	}
	return atomic, nil
}

// FormatUnits formats an atomic integer as an exact decimal string, trimmed of
// trailing fractional zeros. A nil amount returns an empty string.
//
// Invalid precision is a programming error; callers with untrusted precision
// should use FormatUnitsMax, which returns an error.
func FormatUnits(atomic *big.Int, decimals int) string {
	out, err := FormatUnitsMax(atomic, decimals, decimals)
	if err != nil {
		return ""
	}
	return out
}

// FormatUnitsMax formats an atomic integer with at most maxFraction fractional
// digits. Truncation is used; no rounding is performed.
func FormatUnitsMax(atomic *big.Int, decimals, maxFraction int) (string, error) {
	if atomic == nil {
		return "", New(CodeInvalidAmount, "Atomic amount is required.")
	}
	if decimals < 0 || decimals > MaxDecimals {
		return "", New(CodeInvalidDecimals, "Asset precision is invalid.")
	}
	if maxFraction < 0 || maxFraction > decimals {
		return "", New(CodeInvalidDecimals, "Maximum fraction digits are invalid.")
	}

	negative := atomic.Sign() < 0
	absolute := new(big.Int).Abs(atomic)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	fraction := new(big.Int)
	whole := new(big.Int)
	whole.QuoRem(absolute, scale, fraction)

	fractionText := fraction.String()
	if decimals > 0 {
		fractionText = strings.Repeat("0", decimals-len(fractionText)) + fractionText
	}
	if len(fractionText) > maxFraction {
		fractionText = fractionText[:maxFraction]
	}
	fractionText = strings.TrimRight(fractionText, "0")

	out := whole.String()
	if fractionText != "" {
		out += "." + fractionText
	}
	if negative {
		out = "-" + out
	}
	return out, nil
}

// ValidateUint256 rejects values outside the EVM uint256 range.
func ValidateUint256(value *big.Int) error {
	if value == nil {
		return New(CodeInvalidAmount, "Atomic amount is required.")
	}
	if value.Sign() < 0 {
		return New(CodeInvalidAmount, "Amount must not be negative.")
	}
	if value.BitLen() > 256 {
		return Newf(CodeInvalidAmount, "Amount exceeds the maximum uint256 value (%s).", MaxUint256())
	}
	return nil
}

// MaxUint256 returns 2^256 - 1.
func MaxUint256() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
}

// MustParseUnits is a test/seed helper. It panics on invalid input and must not
// be used with untrusted request data.
func MustParseUnits(value string, decimals int) *big.Int {
	atomic, err := ParseUnits(value, decimals)
	if err != nil {
		panic(fmt.Sprintf("parse units %q: %v", value, err))
	}
	return atomic
}
