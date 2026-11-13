package aptos

import (
	"errors"
	"strings"
	"testing"
)

func canonicalShort(short string) string {
	return "0x" + strings.Repeat("0", 64-len(short)) + short
}

func TestParseAddress(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"0x1":                           canonicalShort("1"),
		"1":                             canonicalShort("1"),
		"0x01":                          canonicalShort("1"),
		"0xABCDEF":                      canonicalShort("abcdef"),
		"0x" + strings.Repeat("AB", 32): canonicalShort(strings.Repeat("ab", 32)),
		canonicalShort("1"):             canonicalShort("1"),
		"0x" + strings.Repeat("f", 63):  canonicalShort(strings.Repeat("f", 63)),
	}
	for value, want := range cases {
		got, err := ParseAddress(value)
		if err != nil {
			t.Fatalf("ParseAddress(%q): %v", value, err)
		}
		if got != want {
			t.Fatalf("ParseAddress(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestParseAddressRejectsInvalid(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"",
		"0x",
		"0X1",
		"0x" + strings.Repeat("a", 65),
		"0xg",
		" 1",
		"1 ",
		"-1",
		"0x1::coin",
	} {
		if _, err := ParseAddress(value); !errors.Is(err, ErrInvalidAddress) {
			t.Fatalf("ParseAddress(%q) error = %v, want ErrInvalidAddress", value, err)
		}
	}
}

func TestDecodeAddress(t *testing.T) {
	t.Parallel()
	raw, err := DecodeAddress("0x1")
	if err != nil {
		t.Fatalf("DecodeAddress(): %v", err)
	}
	if len(raw) != 32 || raw[31] != 1 {
		t.Fatalf("DecodeAddress() = %x", raw)
	}
}

func TestNormalizeType(t *testing.T) {
	t.Parallel()
	aptosCoin := canonicalShort("1") + "::aptos_coin::AptosCoin"
	coin := canonicalShort("1") + "::coin::Coin"
	cases := map[string]string{
		"u8":                         "u8",
		"bool":                       "bool",
		"address":                    "address",
		"vector<u8>":                 "vector<u8>",
		"vector<vector<u64>>":        "vector<vector<u64>>",
		"0x1::aptos_coin::AptosCoin": aptosCoin,
		"1::aptos_coin::AptosCoin":   aptosCoin,
		"0x1::coin::Coin<0x1::aptos_coin::AptosCoin>":         coin + "<" + aptosCoin + ">",
		" 0x1 :: coin :: Coin < u8 , vector < u8 > > ":        coin + "<u8, vector<u8>>",
		"vector<0x1::coin::Coin<0x1::aptos_coin::AptosCoin>>": "vector<" + coin + "<" + aptosCoin + ">>",
	}
	for input, want := range cases {
		got, err := NormalizeType(input)
		if err != nil {
			t.Fatalf("NormalizeType(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseTypeShape(t *testing.T) {
	t.Parallel()
	parsed, err := ParseType("vector<0x1::coin::Coin<u64>>")
	if err != nil {
		t.Fatalf("ParseType(): %v", err)
	}
	if parsed.Vector == nil || parsed.Vector.Module != "coin" || len(parsed.Vector.TypeArgs) != 1 {
		t.Fatalf("unexpected shape: %+v", parsed)
	}
	if !parsed.Vector.TypeArgs[0].IsPrimitive() {
		t.Fatalf("u64 should be primitive: %+v", parsed.Vector.TypeArgs[0])
	}
}

func TestParseTypeRejectsInvalid(t *testing.T) {
	t.Parallel()
	deep := strings.Repeat("vector<", maxTypeDepth+2) + "u8" + strings.Repeat(">", maxTypeDepth+2)
	for _, value := range []string{
		"",
		"u7",
		"vector",
		"vector<>",
		"vector<u8, u64>",
		"0x1::coin",
		"0x1::coin::",
		"0x1::::Coin",
		"0x1::coin::Coin<>",
		"0x1::coin::Coin<u8",
		"0x1::coin::Coin<u8,>",
		"0x1::coin::Coin<u8>>",
		"0x1::coin::Coin<u8> extra",
		"0xg::coin::Coin",
		deep,
	} {
		if _, err := ParseType(value); !errors.Is(err, ErrInvalidMoveType) {
			t.Fatalf("ParseType(%q) error = %v, want ErrInvalidMoveType", value, err)
		}
	}
}

func FuzzParseType(f *testing.F) {
	f.Add("0x1::coin::Coin<0x1::aptos_coin::AptosCoin>")
	f.Add("vector<u8>")
	f.Add("0x1::coin::Coin<")
	f.Fuzz(func(t *testing.T, value string) {
		parsed, err := ParseType(value)
		if err != nil {
			return
		}
		canonical := parsed.String()
		again, err := ParseType(canonical)
		if err != nil || again.String() != canonical {
			t.Fatalf("reparse(%q) = %q, %v", canonical, again.String(), err)
		}
	})
}

func FuzzParseAddress(f *testing.F) {
	f.Add("0x1")
	f.Add("0x" + strings.Repeat("f", 64))
	f.Fuzz(func(t *testing.T, value string) {
		normalized, err := ParseAddress(value)
		if err != nil {
			return
		}
		again, err := ParseAddress(normalized)
		if err != nil || again != normalized {
			t.Fatalf("reparse(%q) = %q, %v", normalized, again, err)
		}
	})
}
