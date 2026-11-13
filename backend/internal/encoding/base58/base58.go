// Package base58 implements the Bitcoin base58 alphabet used by Solana and
// 1Click signature payloads. Leading zero bytes are preserved as '1' characters.
package base58

import (
	"fmt"
	"math/big"
	"strings"
)

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var radix = big.NewInt(58)

// Encode returns the base58 representation of input. An empty input encodes to
// "1", matching the 1Click SDK's base58 helper.
func Encode(input []byte) string {
	value := new(big.Int).SetBytes(input)
	mod := new(big.Int)
	var encoded []byte
	for value.Sign() > 0 {
		value.QuoRem(value, radix, mod)
		encoded = append(encoded, alphabet[mod.Int64()])
	}
	for _, b := range input {
		if b != 0 {
			break
		}
		encoded = append(encoded, alphabet[0])
	}
	for left, right := 0, len(encoded)-1; left < right; left, right = left+1, right-1 {
		encoded[left], encoded[right] = encoded[right], encoded[left]
	}
	if len(encoded) == 0 {
		return string(alphabet[0])
	}
	return string(encoded)
}

// Decode parses a base58 string. It rejects an empty string and reports the
// first invalid character. Leading '1' characters decode to leading zero bytes.
func Decode(input string) ([]byte, error) {
	if input == "" {
		return nil, fmt.Errorf("base58 value is empty")
	}
	value := new(big.Int)
	for _, char := range input {
		index := strings.IndexRune(alphabet, char)
		if index < 0 {
			return nil, fmt.Errorf("invalid base58 character %q", char)
		}
		value.Mul(value, radix)
		value.Add(value, big.NewInt(int64(index)))
	}
	decoded := value.Bytes()
	leadingZeros := 0
	for leadingZeros < len(input) && input[leadingZeros] == '1' {
		leadingZeros++
	}
	return append(make([]byte, leadingZeros), decoded...), nil
}
