package near

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAccountID(t *testing.T) {
	t.Parallel()
	valid := []string{
		"alice.near",
		"bob.testnet",
		"a-b_c.near",
		"sub.domain.near",
		"12.near",
		strings.Repeat("a", 64),
		strings.Repeat("ab", 32),
		"00" + strings.Repeat("0", 62),
		"a1_b2-c3.near",
	}
	for _, value := range valid {
		got, err := ParseAccountID(value)
		if err != nil {
			t.Fatalf("ParseAccountID(%q): %v", value, err)
		}
		if got != value {
			t.Fatalf("ParseAccountID(%q) = %q", value, got)
		}
	}
	invalid := []string{
		"",
		"a",
		strings.Repeat("a", 65),
		"Alice.near",
		"ALICE.NEAR",
		".alice.near",
		"alice.near.",
		"alice..near",
		"-alice.near",
		"alice-.near",
		"alice_.near",
		"alice.-near",
		"alice.near-",
		"alice near",
		"alice@near",
		"alice/near",
		"alice--near",
		"alice__near",
		"alice-_near",
		"абв",
		" Alice.near",
	}
	for _, value := range invalid {
		if _, err := ParseAccountID(value); !errors.Is(err, ErrInvalidAccountID) {
			t.Fatalf("ParseAccountID(%q) error = %v, want ErrInvalidAccountID", value, err)
		}
	}
}

func TestIsImplicit(t *testing.T) {
	t.Parallel()
	implicit := strings.Repeat("ab", 32)
	if !IsImplicit(implicit) {
		t.Fatalf("IsImplicit(%q) = false", implicit)
	}
	for _, value := range []string{"alice.near", strings.ToUpper(implicit), implicit[:63], implicit + "a"} {
		if IsImplicit(value) {
			t.Fatalf("IsImplicit(%q) = true", value)
		}
	}
}

func FuzzParseAccountID(f *testing.F) {
	f.Add("alice.near")
	f.Add(strings.Repeat("0", 64))
	f.Add("Alice.near")
	f.Fuzz(func(t *testing.T, value string) {
		got, err := ParseAccountID(value)
		if err != nil {
			return
		}
		if got != value || !IsValid(got) {
			t.Fatalf("accepted %q returned %q", value, got)
		}
	})
}
