package stellar

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// MemoType mirrors Stellar's MemoType union.
type MemoType string

const (
	MemoNone   MemoType = "none"
	MemoText   MemoType = "text"
	MemoID     MemoType = "id"
	MemoHash   MemoType = "hash"
	MemoReturn MemoType = "return"
)

// MemoTextMaxBytes is the protocol limit for MEMO_TEXT.
const MemoTextMaxBytes = 28

// MemoHashLength is the byte length of MEMO_HASH and MEMO_RETURN.
const MemoHashLength = 32

// ErrInvalidMemo is returned for malformed or oversized memos.
var ErrInvalidMemo = errors.New("invalid Stellar memo")

// Memo is a Stellar memo. Only the field matching Type is used.
type Memo struct {
	Type MemoType
	Text string
	ID   uint64
	Hash []byte
}

// IsZero reports whether the memo is absent.
func (m Memo) IsZero() bool {
	return m.Type == "" || m.Type == MemoNone
}

// Validate enforces Stellar memo byte-length limits.
func (m Memo) Validate() error {
	switch m.Type {
	case "", MemoNone:
		if m.Text != "" || m.ID != 0 || len(m.Hash) != 0 {
			return fmt.Errorf("%w: none memo must not carry a value", ErrInvalidMemo)
		}
	case MemoText:
		if m.Text == "" {
			return fmt.Errorf("%w: text must not be empty", ErrInvalidMemo)
		}
		if !utf8.ValidString(m.Text) {
			return fmt.Errorf("%w: text must be valid UTF-8", ErrInvalidMemo)
		}
		if len([]byte(m.Text)) > MemoTextMaxBytes {
			return fmt.Errorf("%w: text is %d bytes, maximum is %d", ErrInvalidMemo, len([]byte(m.Text)), MemoTextMaxBytes)
		}
		if m.ID != 0 || len(m.Hash) != 0 {
			return fmt.Errorf("%w: text memo carries extra fields", ErrInvalidMemo)
		}
	case MemoID:
		if m.Text != "" || len(m.Hash) != 0 {
			return fmt.Errorf("%w: id memo carries extra fields", ErrInvalidMemo)
		}
	case MemoHash, MemoReturn:
		if len(m.Hash) != MemoHashLength {
			return fmt.Errorf("%w: %s must be %d bytes, got %d", ErrInvalidMemo, m.Type, MemoHashLength, len(m.Hash))
		}
		if m.Text != "" || m.ID != 0 {
			return fmt.Errorf("%w: %s memo carries extra fields", ErrInvalidMemo, m.Type)
		}
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidMemo, m.Type)
	}
	return nil
}

// TextMemo builds and validates a MEMO_TEXT value.
func TextMemo(value string) (Memo, error) {
	memo := Memo{Type: MemoText, Text: value}
	if err := memo.Validate(); err != nil {
		return Memo{}, err
	}
	return memo, nil
}

// IDMemo builds a MEMO_ID value.
func IDMemo(value uint64) Memo {
	return Memo{Type: MemoID, ID: value}
}

// HashMemo builds a MEMO_HASH value.
func HashMemo(value []byte) (Memo, error) {
	memo := Memo{Type: MemoHash, Hash: append([]byte(nil), value...)}
	if err := memo.Validate(); err != nil {
		return Memo{}, err
	}
	return memo, nil
}
