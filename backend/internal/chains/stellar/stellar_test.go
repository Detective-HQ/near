package stellar

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestStrKeyVectors(t *testing.T) {
	t.Parallel()
	vectors := []struct {
		address string
		rawHex  string
	}{
		{
			address: "GBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTECA",
			rawHex:  "4cb5abf6ad79fbf5abbccafcc269d85cd2651ed4b885b5869f241aedf0a5ba29",
		},
		{
			address: "GB2CFOMIOWMANDRSYRCIVFE23MUQ2D2OGW46AGYO4XY2DZQA7YTHIJGX",
			rawHex:  "7422b9887598068e32c4448a949adb290d0f4e35b9e01b0ee5f1a1e600fe2674",
		},
		{
			address: "GBLJTKOO7BYOF7YMAIVWO2E4Y5X6AXUQSFOF6AKD7E2WZJZPJL7ZTETK",
			rawHex:  "5699a9cef870e2ff0c022b67689cc76fe05e90915c5f0143f9356ca72f4aff99",
		},
	}
	for _, vector := range vectors {
		raw, err := hex.DecodeString(vector.rawHex)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeAccountID(vector.address)
		if err != nil {
			t.Fatalf("DecodeAccountID(%q): %v", vector.address, err)
		}
		if !bytes.Equal(decoded, raw) {
			t.Fatalf("DecodeAccountID(%q) = %x, want %s", vector.address, decoded, vector.rawHex)
		}
		encoded, err := EncodeAccountID(raw)
		if err != nil {
			t.Fatalf("EncodeAccountID(%x): %v", raw, err)
		}
		if encoded != vector.address {
			t.Fatalf("EncodeAccountID(%x) = %q, want %q", raw, encoded, vector.address)
		}
		if parsed, err := ParseAccountID(vector.address); err != nil || parsed != vector.address {
			t.Fatalf("ParseAccountID(%q) = %q, %v", vector.address, parsed, err)
		}
	}
}

func TestStrKeyRejectsInvalid(t *testing.T) {
	t.Parallel()
	valid := "GBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTECA"
	tampered := valid[:len(valid)-1] + "B"
	for _, value := range []string{
		"",
		strings.ToLower(valid),
		valid[:len(valid)-1],
		valid + "A",
		tampered,
		"SBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTEC",
		"GBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTEC0",
	} {
		if _, err := ParseAccountID(value); !errors.Is(err, ErrInvalidAddress) {
			t.Fatalf("ParseAccountID(%q) error = %v, want ErrInvalidAddress", value, err)
		}
	}
	if _, err := EncodeAccountID(make([]byte, 31)); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("EncodeAccountID(short) error = %v", err)
	}
}

func TestMemoValidation(t *testing.T) {
	t.Parallel()
	valid := []Memo{
		{},
		{Type: MemoNone},
		{Type: MemoText, Text: "hello"},
		{Type: MemoText, Text: strings.Repeat("a", 28)},
		{Type: MemoText, Text: strings.Repeat("😀", 7)},
		{Type: MemoID, ID: 0},
		{Type: MemoID, ID: ^uint64(0)},
		{Type: MemoHash, Hash: make([]byte, 32)},
		{Type: MemoReturn, Hash: make([]byte, 32)},
	}
	for _, memo := range valid {
		if err := memo.Validate(); err != nil {
			t.Fatalf("Validate(%+v): %v", memo, err)
		}
	}
	invalid := []Memo{
		{Type: MemoText, Text: ""},
		{Type: MemoText, Text: strings.Repeat("a", 29)},
		{Type: MemoText, Text: "ok", ID: 1},
		{Type: MemoText, Text: string([]byte{0xff})},
		{Type: MemoID, Text: "x"},
		{Type: MemoID, Hash: make([]byte, 32)},
		{Type: MemoHash, Hash: make([]byte, 31)},
		{Type: MemoHash, Hash: make([]byte, 33)},
		{Type: MemoReturn, ID: 1, Hash: make([]byte, 32)},
		{Type: MemoNone, Text: "x"},
		{Type: MemoNone, ID: 1},
		{Type: MemoType("memo"), Text: "x"},
	}
	for _, memo := range invalid {
		if err := memo.Validate(); !errors.Is(err, ErrInvalidMemo) {
			t.Fatalf("Validate(%+v) error = %v, want ErrInvalidMemo", memo, err)
		}
	}
}

func TestMemoHelpers(t *testing.T) {
	t.Parallel()
	if memo, err := TextMemo("hello"); err != nil || memo.Type != MemoText {
		t.Fatalf("TextMemo() = %+v, %v", memo, err)
	}
	if _, err := TextMemo(strings.Repeat("a", 29)); !errors.Is(err, ErrInvalidMemo) {
		t.Fatalf("TextMemo(too long) error = %v", err)
	}
	if memo := IDMemo(42); memo.ID != 42 || memo.Type != MemoID {
		t.Fatalf("IDMemo() = %+v", memo)
	}
	if memo, err := HashMemo(make([]byte, 32)); err != nil || memo.Type != MemoHash {
		t.Fatalf("HashMemo() = %+v, %v", memo, err)
	}
	if _, err := HashMemo(make([]byte, 31)); !errors.Is(err, ErrInvalidMemo) {
		t.Fatalf("HashMemo(short) error = %v", err)
	}
}

func FuzzParseAccountID(f *testing.F) {
	f.Add("GBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTECA")
	f.Add("not-an-account")
	f.Fuzz(func(t *testing.T, value string) {
		address, err := ParseAccountID(value)
		if err != nil {
			return
		}
		if address != value || !ValidateAccountID(address) {
			t.Fatalf("accepted %q returned %q", value, address)
		}
	})
}

func FuzzMemoValidate(f *testing.F) {
	f.Add("", "", uint64(0), []byte(nil))
	f.Add(string(MemoText), "memo", uint64(1), []byte(nil))
	f.Fuzz(func(t *testing.T, kind, text string, id uint64, hash []byte) {
		memo := Memo{Type: MemoType(kind), Text: text, ID: id, Hash: hash}
		_ = memo.Validate()
	})
}
