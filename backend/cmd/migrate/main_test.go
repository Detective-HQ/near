package main

import (
	"strings"
	"testing"
)

func TestRunRejectsMissingOrInvalidArguments(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"unknown command", []string{"migrate"}},
		{"up with extra arg", []string{"up", "extra"}},
		{"down-to missing version", []string{"down-to"}},
		{"down-to non-numeric", []string{"down-to", "abc"}},
		{"down-to negative", []string{"down-to", "-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.args)
			if err == nil {
				t.Fatalf("run(%v) returned nil, want an error", tc.args)
			}
			if !strings.Contains(err.Error(), "usage:") && !strings.Contains(err.Error(), "invalid target version") {
				t.Fatalf("run(%v) error = %q, want usage or target-version error", tc.args, err)
			}
		})
	}
}

func TestUsageErrorMentionsCommands(t *testing.T) {
	message := usageError().Error()
	for _, command := range []string{"up", "down", "down-to", "status", "version", "check"} {
		if !strings.Contains(message, command) {
			t.Fatalf("usage error %q is missing command %q", message, command)
		}
	}
}
