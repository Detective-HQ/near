package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpapi"
	"universal-defi-intelligence/internal/risk"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "api: %v\n", err)
		os.Exit(1)
	}
}

func runHealthcheck() error {
	client := &http.Client{Timeout: 3 * time.Second}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	if !strings.HasPrefix(addr, ":") && !strings.HasPrefix(addr, "http://") {
		if idx := strings.LastIndex(addr, ":"); idx != -1 {
			addr = addr[idx:]
		}
	}
	if strings.HasPrefix(addr, ":") {
		addr = "http://127.0.0.1" + addr
	}
	resp, err := client.Get(addr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		// In local development, allow running even if DATABASE_URL is not set by providing safe defaults
		fmt.Printf("Notice: config.Load warning (%v), falling back to local defaults for demo mode\n", err)
		cfg = &config.Config{
			AppEnv: config.EnvLocal,
			HTTP: config.HTTPConfig{
				Addr:              ":8081",
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       15 * time.Second,
				WriteTimeout:      15 * time.Second,
				IdleTimeout:       60 * time.Second,
				MaxHeaderBytes:    1 << 20,
			},
			Log: config.LogConfig{
				Level:  slog.LevelInfo,
				Format: "text",
			},
		}
	}

	logger := newLogger(cfg.Log)
	logger.Info("starting go backend http api",
		"addr", cfg.HTTP.Addr,
		"env", cfg.AppEnv,
	)

	store := httpapi.NewStateStore()
	riskEngine := risk.NewEngine(risk.Config{
		MaxSlippageBps:     500,
		WarningSlippageBps: 200,
		CatalogueMaxAge:    2 * time.Hour,
	})
	authManager := auth.NewManager(auth.Config{
		AuthRequired: false, // Default off per roadmap; flag-activated
		NonceTTL:     10 * time.Minute,
		SessionTTL:   24 * time.Hour,
	})

	handler := httpapi.NewHandlerWithOptions(httpapi.HandlerOptions{
		Store:       store,
		RiskEngine:  riskEngine,
		AuthManager: authManager,
		Config:      cfg,
	})

	router := httpapi.NewRouter(handler, logger)
	server := httpapi.NewServer(cfg, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api server listening", "addr", cfg.HTTP.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down api server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}
		logger.Info("api server stopped cleanly")
	}

	return nil
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	options := &slog.HandlerOptions{Level: cfg.Level}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, options)
	} else {
		handler = slog.NewTextHandler(os.Stdout, options)
	}
	return slog.New(handler)
}
