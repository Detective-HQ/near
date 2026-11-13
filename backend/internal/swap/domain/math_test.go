package domain

import (
	"errors"
	"math/big"
	"strings"
	"testing"
)

func requireCode(t *testing.T, err error, want Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	if got := CodeOf(err); got != want {
		t.Fatalf("expected code %s, got %s (%v)", want, got, err)
	}
}

func TestParseUnitsValid(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		decimals int
		want     string
	}{
		{name: "zero decimals", value: "1", decimals: 0, want: "1"},
		{name: "whole amount", value: "123", decimals: 18, want: "123000000000000000000"},
		{name: "fraction", value: "1.23", decimals: 2, want: "123"},
		{name: "pads fraction", value: "1.2", decimals: 2, want: "120"},
		{name: "leading zeros", value: "0001.20", decimals: 2, want: "120"},
		{name: "one atomic unit", value: "0.000000000000000001", decimals: 18, want: "1"},
		{name: "max precision", value: "1", decimals: 36, want: "1" + strings.Repeat("0", 36)},
		{name: "max input digits", value: strings.Repeat("9", 78), decimals: 0, want: strings.Repeat("9", 78)},
		{name: "decimal point only in fraction", value: "0.5", decimals: 1, want: "5"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseUnits(test.value, test.decimals)
			if err != nil {
				t.Fatalf("ParseUnits(%q, %d) error: %v", test.value, test.decimals, err)
			}
			if got.String() != test.want {
				t.Fatalf("ParseUnits(%q, %d) = %s, want %s", test.value, test.decimals, got, test.want)
			}
		})
	}
}

func TestParseUnitsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		decimals int
		wantCode Code
	}{
		{name: "empty", value: "", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "space", value: " ", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "negative", value: "-1", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "explicit positive", value: "+1", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "exponent", value: "1e18", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "comma", value: "1,000", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "leading dot", value: ".5", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "trailing dot", value: "1.", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "double dot", value: "1..2", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "zero", value: "0", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "zero fraction", value: "0.0", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "leading zero amount", value: "00", decimals: 18, wantCode: CodeInvalidAmount},
		{name: "too long", value: strings.Repeat("1", MaxAmountDigits+1), decimals: 0, wantCode: CodeInvalidAmount},
		{name: "too many decimals", value: "1.234", decimals: 2, wantCode: CodeTooManyDecimals},
		{name: "negative precision", value: "1", decimals: -1, wantCode: CodeInvalidDecimals},
		{name: "precision too high", value: "1", decimals: MaxDecimals + 1, wantCode: CodeInvalidDecimals},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseUnits(test.value, test.decimals)
			requireCode(t, err, test.wantCode)
		})
	}
}

func TestFormatUnits(t *testing.T) {
	tests := []struct {
		name     string
		atomic   *big.Int
		decimals int
		want     string
	}{
		{name: "zero", atomic: big.NewInt(0), decimals: 18, want: "0"},
		{name: "one atomic unit", atomic: big.NewInt(1), decimals: 18, want: "0.000000000000000001"},
		{name: "fraction", atomic: big.NewInt(123), decimals: 2, want: "1.23"},
		{name: "trailing zeros trimmed", atomic: big.NewInt(120), decimals: 2, want: "1.2"},
		{name: "whole from fractional digits", atomic: big.NewInt(1000), decimals: 3, want: "1"},
		{name: "negative", atomic: big.NewInt(-123), decimals: 2, want: "-1.23"},
		{name: "zero decimals", atomic: big.NewInt(42), decimals: 0, want: "42"},
		{name: "nil", atomic: nil, decimals: 18, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FormatUnits(test.atomic, test.decimals); got != test.want {
				t.Fatalf("FormatUnits(%v, %d) = %q, want %q", test.atomic, test.decimals, got, test.want)
			}
		})
	}
}

func TestFormatUnitsMax(t *testing.T) {
	tests := []struct {
		name        string
		atomic      *big.Int
		decimals    int
		maxFraction int
		want        string
	}{
		{name: "truncates", atomic: big.NewInt(123456789), decimals: 8, maxFraction: 3, want: "1.234"},
		{name: "zero fraction digits", atomic: big.NewInt(123), decimals: 2, maxFraction: 0, want: "1"},
		{name: "keeps trailing zeros before trim", atomic: big.NewInt(120), decimals: 2, maxFraction: 2, want: "1.2"},
		{name: "full precision", atomic: big.NewInt(1), decimals: 4, maxFraction: 4, want: "0.0001"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := FormatUnitsMax(test.atomic, test.decimals, test.maxFraction)
			if err != nil {
				t.Fatalf("FormatUnitsMax error: %v", err)
			}
			if got != test.want {
				t.Fatalf("FormatUnitsMax = %q, want %q", got, test.want)
			}
		})
	}

	if _, err := FormatUnitsMax(big.NewInt(1), 18, 19); err == nil {
		t.Fatal("expected maxFraction greater than decimals to fail")
	}
	if _, err := FormatUnitsMax(big.NewInt(1), 18, -1); err == nil {
		t.Fatal("expected negative maxFraction to fail")
	}
	if _, err := FormatUnitsMax(big.NewInt(1), MaxDecimals+1, 1); err == nil {
		t.Fatal("expected invalid decimals to fail")
	}
	if _, err := FormatUnitsMax(nil, 18, 18); err == nil {
		t.Fatal("expected nil amount to fail")
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	tests := []struct {
		value    string
		decimals int
	}{
		{value: "1", decimals: 0},
		{value: "1.23", decimals: 2},
		{value: "1.20", decimals: 2},
		{value: "0.000000000000000001", decimals: 18},
		{value: "123456789012345678901234567890", decimals: 6},
		{value: "1.000000000000000000000000000000000001", decimals: 36},
	}

	for _, test := range tests {
		atomic, err := ParseUnits(test.value, test.decimals)
		if err != nil {
			t.Fatalf("ParseUnits(%q, %d): %v", test.value, test.decimals, err)
		}
		formatted := FormatUnits(atomic, test.decimals)
		again, err := ParseUnits(formatted, test.decimals)
		if err != nil {
			t.Fatalf("ParseUnits(%q, %d) after formatting %q: %v", test.value, test.decimals, formatted, err)
		}
		if again.Cmp(atomic) != 0 {
			t.Fatalf("round trip mismatch for %q (%d decimals): %s -> %q -> %s", test.value, test.decimals, atomic, formatted, again)
		}
	}
}

func TestValidateUint256(t *testing.T) {
	if err := ValidateUint256(big.NewInt(0)); err != nil {
		t.Fatalf("zero should be valid: %v", err)
	}
	if err := ValidateUint256(MaxUint256()); err != nil {
		t.Fatalf("max uint256 should be valid: %v", err)
	}
	over := new(big.Int).Add(MaxUint256(), big.NewInt(1))
	if err := ValidateUint256(over); err == nil {
		t.Fatal("expected value above uint256 to fail")
	}
	if err := ValidateUint256(big.NewInt(-1)); err == nil {
		t.Fatal("expected negative value to fail")
	}
	if err := ValidateUint256(nil); err == nil {
		t.Fatal("expected nil value to fail")
	}
}

func FuzzParseUnitsRoundTrip(f *testing.F) {
	seeds := []struct {
		value    string
		decimals int
	}{
		{"1", 0},
		{"1.23", 2},
		{"0.000000000000000001", 18},
		{"12345678901234567890", 6},
		{"0", 18},
		{"-1", 18},
	}
	for _, seed := range seeds {
		f.Add(seed.value, seed.decimals)
	}

	f.Fuzz(func(t *testing.T, value string, decimals int) {
		if len(value) > 256 || decimals < 0 || decimals > MaxDecimals {
			return
		}
		atomic, err := ParseUnits(value, decimals)
		if err != nil {
			return
		}
		formatted := FormatUnits(atomic, decimals)
		again, err := ParseUnits(formatted, decimals)
		if err != nil {
			t.Fatalf("formatted amount %q is not parseable: %v", formatted, err)
		}
		if again.Cmp(atomic) != 0 {
			t.Fatalf("round trip mismatch for %q: %s != %s", value, atomic, again)
		}
	})
}

func TestParseUnitsErrorUnwrap(t *testing.T) {
	_, err := ParseUnits("-1", 18)
	if !errors.Is(err, New(CodeInvalidAmount, "different message")) {
		t.Fatal("expected errors.Is to match by domain code")
	}
}

func TestMustParseUnits(t *testing.T) {
	got := MustParseUnits("1.5", 1)
	if got.String() != "15" {
		t.Fatalf("MustParseUnits = %s, want 15", got)
	}
}

func TestMustParseUnitsPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected MustParseUnits to panic on invalid input")
		}
	}()
	MustParseUnits("not-a-number", 18)
}
