// Package stellar validates Stellar StrKey account IDs and memos.
package stellar

import (
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	accountIDVersion byte = 6 << 3
	accountIDPayload      = 32
	accountIDLength       = 56
)

var (
	// ErrInvalidAddress is returned for malformed or unsupported StrKeys.
	ErrInvalidAddress = errors.New("invalid Stellar account ID")
	strKeyEncoding    = base32.StdEncoding.WithPadding(base32.NoPadding)
)

// ParseAccountID validates a G... StrKey account ID and returns it unchanged.
// Lowercase input is rejected because StrKeys are conventionally uppercase and
// mixed-case variants would be ambiguous.
func ParseAccountID(value string) (string, error) {
	if _, err := DecodeAccountID(value); err != nil {
		return "", err
	}
	return value, nil
}

// DecodeAccountID validates a G... StrKey and returns its 32-byte ed25519 key.
func DecodeAccountID(value string) ([]byte, error) {
	if len(value) != accountIDLength || value[0] != 'G' || value != strings.ToUpper(value) {
		return nil, fmt.Errorf("%w: expected 56-character uppercase G... StrKey", ErrInvalidAddress)
	}
	decoded, err := strKeyEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	if len(decoded) != 1+accountIDPayload+2 {
		return nil, fmt.Errorf("%w: unexpected decoded length %d", ErrInvalidAddress, len(decoded))
	}
	if decoded[0] != accountIDVersion {
		return nil, fmt.Errorf("%w: unsupported version byte %d", ErrInvalidAddress, decoded[0])
	}
	want := binary.LittleEndian.Uint16(decoded[len(decoded)-2:])
	if got := crc16XModem(decoded[:len(decoded)-2]); got != want {
		return nil, fmt.Errorf("%w: checksum mismatch", ErrInvalidAddress)
	}
	return append([]byte(nil), decoded[1:1+accountIDPayload]...), nil
}

// EncodeAccountID builds a G... StrKey from a 32-byte ed25519 public key.
func EncodeAccountID(payload []byte) (string, error) {
	if len(payload) != accountIDPayload {
		return "", fmt.Errorf("%w: expected %d-byte payload, got %d", ErrInvalidAddress, accountIDPayload, len(payload))
	}
	raw := make([]byte, 0, 1+accountIDPayload+2)
	raw = append(raw, accountIDVersion)
	raw = append(raw, payload...)
	var checksum [2]byte
	binary.LittleEndian.PutUint16(checksum[:], crc16XModem(raw))
	raw = append(raw, checksum[:]...)
	return strKeyEncoding.EncodeToString(raw), nil
}

// ValidateAccountID reports whether value is a valid G... StrKey.
func ValidateAccountID(value string) bool {
	_, err := ParseAccountID(value)
	return err == nil
}

// crc16XModem implements CRC-16/XMODEM (poly 0x1021, init 0x0000).
func crc16XModem(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
