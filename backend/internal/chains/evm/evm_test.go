package evm

import (
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
)

func TestParseAddressEIP55Vectors(t *testing.T) {
	t.Parallel()
	vectors := []string{
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
	}
	for _, address := range vectors {
		got, err := ParseAddress(address)
		if err != nil {
			t.Fatalf("ParseAddress(%q): %v", address, err)
		}
		if got != address {
			t.Fatalf("ParseAddress(%q) = %q", address, got)
		}
		lower, err := ParseAddress(strings.ToLower(address))
		if err != nil {
			t.Fatalf("ParseAddress(lower %q): %v", address, err)
		}
		if lower != address {
			t.Fatalf("ParseAddress(lower %q) = %q, want %q", address, lower, address)
		}
	}
}

func TestParseAddressRejectsInvalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value string
		err   error
	}{
		{"", ErrInvalidAddress},
		{"5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", ErrInvalidAddress},
		{"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAe", ErrInvalidAddress},
		{"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAedd", ErrInvalidAddress},
		{"0xzzAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", ErrInvalidAddress},
		{"0x5Aaeb6053F3E94C9b9A09f33669435E7Ef1BeAed", ErrChecksumMismatch},
		{" 0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", ErrInvalidAddress},
	}
	for _, tc := range cases {
		if _, err := ParseAddress(tc.value); !errors.Is(err, tc.err) {
			t.Fatalf("ParseAddress(%q) error = %v, want %v", tc.value, err, tc.err)
		}
	}
}

func TestChecksumAndDecode(t *testing.T) {
	t.Parallel()
	raw, err := hex.DecodeString("5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
	if err != nil {
		t.Fatal(err)
	}
	checksummed, err := Checksum(raw)
	if err != nil {
		t.Fatalf("Checksum(): %v", err)
	}
	if checksummed != "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed" {
		t.Fatalf("Checksum() = %q", checksummed)
	}
	decoded, err := DecodeAddress(checksummed)
	if err != nil {
		t.Fatalf("DecodeAddress(): %v", err)
	}
	if string(decoded) != string(raw) {
		t.Fatalf("DecodeAddress() = %x, want %x", decoded, raw)
	}
	if _, err := Checksum(raw[:19]); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("Checksum(short) error = %v", err)
	}
}

func TestParseChainID(t *testing.T) {
	t.Parallel()
	valid := map[string]uint64{
		"1":      1,
		"0x1":    1,
		"8453":   8453,
		"0x2105": 8453,
		"56":     56,
	}
	for value, want := range valid {
		got, err := ParseChainID(value)
		if err != nil {
			t.Fatalf("ParseChainID(%q): %v", value, err)
		}
		if got != want {
			t.Fatalf("ParseChainID(%q) = %d, want %d", value, got, want)
		}
	}
	for _, value := range []string{"", "0", "00", "01", "0x", "0x0", "0x00", " 1", "1 ", "-1", "abc", "+1", "0x10000000000000000"} {
		if _, err := ParseChainID(value); !errors.Is(err, ErrInvalidChainID) {
			t.Fatalf("ParseChainID(%q) error = %v, want ErrInvalidChainID", value, err)
		}
	}
}

func TestParseTxHash(t *testing.T) {
	t.Parallel()
	hash := "0x" + strings.Repeat("aB", 32)
	got, err := ParseTxHash(hash)
	if err != nil {
		t.Fatalf("ParseTxHash(): %v", err)
	}
	if got != strings.ToLower(hash) {
		t.Fatalf("ParseTxHash() = %q", got)
	}
	for _, value := range []string{
		"",
		strings.Repeat("a", 64),
		"0x" + strings.Repeat("a", 63),
		"0x" + strings.Repeat("a", 65),
		"0x" + strings.Repeat("g", 64),
	} {
		if _, err := ParseTxHash(value); !errors.Is(err, ErrInvalidTxHash) {
			t.Fatalf("ParseTxHash(%q) error = %v", value, err)
		}
	}
}

func TestERC20TransferData(t *testing.T) {
	t.Parallel()
	recipient := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	data, err := ERC20TransferData(recipient, big.NewInt(1))
	if err != nil {
		t.Fatalf("ERC20TransferData(): %v", err)
	}
	want := "a9059cbb" +
		"000000000000000000000000" + "5aaeb6053f3e94c9b9a09f33669435e7ef1beaed" +
		"0000000000000000000000000000000000000000000000000000000000000001"
	if got := hex.EncodeToString(data); got != want {
		t.Fatalf("calldata = %s, want %s", got, want)
	}
	decodedRecipient, amount, err := DecodeERC20Transfer(data)
	if err != nil {
		t.Fatalf("DecodeERC20Transfer(): %v", err)
	}
	if decodedRecipient != recipient || amount.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("decoded = %s %s", decodedRecipient, amount)
	}

	hexData, err := ERC20TransferDataHex(recipient, big.NewInt(1))
	if err != nil || hexData != "0x"+want {
		t.Fatalf("ERC20TransferDataHex() = %q, %v", hexData, err)
	}

	tooLarge := new(big.Int).Lsh(big.NewInt(1), 256)
	for _, amount := range []*big.Int{nil, big.NewInt(-1), tooLarge} {
		if _, err := ERC20TransferData(recipient, amount); !errors.Is(err, ErrInvalidCallData) {
			t.Fatalf("ERC20TransferData(%v) error = %v", amount, err)
		}
	}
}

func TestDecodeERC20TransferRejectsInvalid(t *testing.T) {
	t.Parallel()
	valid := make([]byte, 68)
	copy(valid, ERC20TransferSelector[:])
	if _, _, err := DecodeERC20Transfer(valid); err != nil {
		t.Fatalf("valid zero-address transfer rejected: %v", err)
	}
	for _, data := range [][]byte{
		nil,
		make([]byte, 67),
		append([]byte{0, 0, 0, 0}, make([]byte, 64)...),
		func() []byte {
			out := append([]byte{}, valid...)
			out[4] = 1
			return out
		}(),
	} {
		if _, _, err := DecodeERC20Transfer(data); !errors.Is(err, ErrInvalidCallData) {
			t.Fatalf("DecodeERC20Transfer(%x) error = %v", data, err)
		}
	}
}

func TestNormalizeCallData(t *testing.T) {
	t.Parallel()
	if got, err := NormalizeCallData("0xA9059CBB"); err != nil || got != "0xa9059cbb" {
		t.Fatalf("NormalizeCallData() = %q, %v", got, err)
	}
	for _, value := range []string{"", "0x1", "a905", "0xzz"} {
		if _, err := NormalizeCallData(value); !errors.Is(err, ErrInvalidCallData) {
			t.Fatalf("NormalizeCallData(%q) error = %v", value, err)
		}
	}
	if got, err := NormalizeCallData("0x"); err != nil || got != "0x" {
		t.Fatalf("NormalizeCallData(empty data) = %q, %v", got, err)
	}
}

func FuzzParseAddress(f *testing.F) {
	f.Add("0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed")
	f.Add(strings.Repeat("a", 42))
	f.Fuzz(func(t *testing.T, value string) {
		got, err := ParseAddress(value)
		if err != nil {
			return
		}
		if len(got) != 42 || !strings.HasPrefix(got, "0x") {
			t.Fatalf("accepted address %q returned %q", value, got)
		}
		again, err := ParseAddress(got)
		if err != nil || again != got {
			t.Fatalf("reparse(%q) = %q, %v", got, again, err)
		}
	})
}
