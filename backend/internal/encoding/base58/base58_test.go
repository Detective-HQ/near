package base58

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	vectors := [][]byte{
		{0},
		{0, 0, 1},
		{1, 2, 3, 4, 5},
		[]byte("the quick brown fox"),
		bytes.Repeat([]byte{0xff}, 32),
	}
	for _, input := range vectors {
		encoded := Encode(input)
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(%q): %v", encoded, err)
		}
		if !bytes.Equal(decoded, input) {
			t.Fatalf("round trip %x -> %q -> %x", input, encoded, decoded)
		}
	}
}

func TestKnownVectors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input []byte
		want  string
	}{
		{[]byte{}, "1"},
		{[]byte{0}, "1"},
		{[]byte{0, 0}, "11"},
		{[]byte("hello world"), "StV1DL6CwTryKyV"},
	}
	for _, tc := range cases {
		if got := Encode(tc.input); got != tc.want {
			t.Fatalf("Encode(%x) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDecodeRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", "0", "O", "I", "l", "hello!"} {
		if _, err := Decode(input); err == nil {
			t.Fatalf("Decode(%q) succeeded, want error", input)
		}
	}
}
