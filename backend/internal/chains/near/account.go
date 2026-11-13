// Package near validates NEAR account IDs. NEAR addresses are either implicit
// accounts (64 lowercase hex characters) or named accounts (2-64 lowercase
// characters with dot-separated segments).
package near

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxAccountLength is the protocol limit for account IDs in bytes.
	MaxAccountLength = 64
	// MinAccountLength is the protocol minimum for named accounts.
	MinAccountLength = 2
	implicitLength   = 64
)

var (
	// ErrInvalidAccountID is returned for malformed NEAR account IDs.
	ErrInvalidAccountID = errors.New("invalid NEAR account ID")
	namedPattern        = regexp.MustCompile(`^(([a-z\d]+[-_])*[a-z\d]+)(\.([a-z\d]+[-_])*[a-z\d]+)*$`)
	implicitPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ParseAccountID validates and returns a NEAR account ID. It does not trim
// whitespace: callers should reject padded input rather than silently fix it.
func ParseAccountID(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidAccountID)
	}
	if len(value) < MinAccountLength || len(value) > MaxAccountLength {
		return "", fmt.Errorf("%w: must be %d-%d bytes", ErrInvalidAccountID, MinAccountLength, MaxAccountLength)
	}
	if value != strings.ToLower(value) {
		return "", fmt.Errorf("%w: must be lowercase", ErrInvalidAccountID)
	}
	if implicitPattern.MatchString(value) {
		return value, nil
	}
	if !namedPattern.MatchString(value) {
		return "", fmt.Errorf("%w: %q", ErrInvalidAccountID, value)
	}
	return value, nil
}

// IsValid reports whether value is a valid NEAR account ID.
func IsValid(value string) bool {
	_, err := ParseAccountID(value)
	return err == nil
}

// IsImplicit reports whether value is a 64-hex implicit account.
func IsImplicit(value string) bool {
	return len(value) == implicitLength && implicitPattern.MatchString(value)
}
