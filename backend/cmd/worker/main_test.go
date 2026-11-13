package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"universal-defi-intelligence/internal/config"
)

func TestHeartbeatStopsWhenContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	if err := heartbeat(ctx, logger); !errors.Is(err, context.Canceled) {
		t.Fatalf("heartbeat error = %v, want context.Canceled", err)
	}
}

func TestNewLoggerHonoursConfig(t *testing.T) {
	cases := []struct {
		name        string
		cfg         config.LogConfig
		debugEnable bool
	}{
		{"json info", config.LogConfig{Level: slog.LevelInfo, Format: "json"}, false},
		{"text debug", config.LogConfig{Level: slog.LevelDebug, Format: "text"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := newLogger(tc.cfg)
			if logger == nil {
				t.Fatal("newLogger returned nil")
			}
			if got := logger.Enabled(context.Background(), slog.LevelDebug); got != tc.debugEnable {
				t.Fatalf("debug enabled = %v, want %v", got, tc.debugEnable)
			}
		})
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
