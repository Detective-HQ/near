// Package evm validates Ethereum-style addresses, chain IDs, transaction
// hashes, and ERC-20 transfer calldata.
package evm

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/sha3"
)

// AddressLength is the byte length of an EVM address.
const AddressLength = 20

var (
	// ErrInvalidAddress is returned for malformed or unchecksummed addresses.
	ErrInvalidAddress = errors.New("invalid EVM address")
	// ErrChecksumMismatch is returned when a mixed-case address fails EIP-55.
	ErrChecksumMismatch = errors.New("EIP-55 checksum mismatch")
	// ErrInvalidChainID is returned for malformed chain IDs.
	ErrInvalidChainID = errors.New("invalid EVM chain ID")
	// ErrInvalidTxHash is returned for malformed transaction hashes.
	ErrInvalidTxHash = errors.New("invalid EVM transaction hash")
)

// ParseAddress validates an address and returns its EIP-55 checksummed form.
//
// Policy: the 0x prefix is required. All-lowercase and all-uppercase inputs are
// accepted and normalized. Any mixed-case input must already be a correct
// EIP-55 checksum; otherwise ErrChecksumMismatch is returned. This rejects
// silently mistyped checksummed addresses while remaining compatible with
// wallets that emit lowercase addresses.
func ParseAddress(value string) (string, error) {
	if !strings.HasPrefix(value, "0x") || len(value) != 2+2*AddressLength {
		return "", fmt.Errorf("%w: expected 0x followed by 40 hex characters", ErrInvalidAddress)
	}
	body := value[2:]
	if _, err := hex.DecodeString(body); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	lower := strings.ToLower(body)
	checksummed := checksumHex(lower)
	if body != lower && body != strings.ToUpper(body) {
		if body != checksummed {
			return "", fmt.Errorf("%w: expected 0x%s", ErrChecksumMismatch, checksummed)
		}
		return "0x" + checksummed, nil
	}
	return "0x" + checksummed, nil
}

// IsAddress reports whether value is a valid EVM address under ParseAddress.
func IsAddress(value string) bool {
	_, err := ParseAddress(value)
	return err == nil
}

// DecodeAddress validates an address and returns its 20 raw bytes.
func DecodeAddress(value string) ([]byte, error) {
	if _, err := ParseAddress(value); err != nil {
		return nil, err
	}
	return hex.DecodeString(value[2:])
}

// Checksum returns the EIP-55 checksummed form of a 20-byte address.
func Checksum(address []byte) (string, error) {
	if len(address) != AddressLength {
		return "", fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidAddress, AddressLength, len(address))
	}
	return "0x" + checksumHex(hex.EncodeToString(address)), nil
}

// checksumHex applies EIP-55 to a lowercase, unprefixed hex string.
func checksumHex(lower string) string {
	hash := sha3.NewLegacyKeccak256()
	_, _ = hash.Write([]byte(lower))
	digest := hash.Sum(nil)
	out := []byte(lower)
	for i, char := range out {
		if char < 'a' || char > 'f' {
			continue
		}
		// Each byte of the digest encodes two hex nibbles.
		nibble := digest[i/2]
		if i%2 == 0 {
			nibble >>= 4
		}
		nibble &= 0x0f
		if nibble >= 8 {
			out[i] = char - ('a' - 'A')
		}
	}
	return string(out)
}
