package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFrom(nil)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.AppEnv != EnvLocal {
		t.Fatalf("AppEnv = %s, want local", cfg.AppEnv)
	}
	if cfg.HTTP.Addr != ":8081" {
		t.Fatalf("HTTP.Addr = %q, want :8081", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s", cfg.HTTP.ReadHeaderTimeout)
	}
	if cfg.HTTP.ReadTimeout != 15*time.Second || cfg.HTTP.WriteTimeout != 30*time.Second || cfg.HTTP.IdleTimeout != 60*time.Second {
		t.Fatalf("unexpected HTTP timeouts: %+v", cfg.HTTP)
	}
	if cfg.HTTP.MaxHeaderBytes != 1048576 {
		t.Fatalf("MaxHeaderBytes = %d", cfg.HTTP.MaxHeaderBytes)
	}
	if cfg.Database.URL != "" {
		t.Fatalf("Database.URL = %q, want empty for local", cfg.Database.URL)
	}
	if cfg.Database.MaxConns != 10 || cfg.Database.MinConns != 0 {
		t.Fatalf("unexpected DB pool settings: %+v", cfg.Database)
	}
	if cfg.Redis.URL != "redis://localhost:6379" {
		t.Fatalf("Redis.URL = %q", cfg.Redis.URL)
	}
	if cfg.OneClick.BaseURL != "https://1click.chaindefuser.com" {
		t.Fatalf("OneClick.BaseURL = %q", cfg.OneClick.BaseURL)
	}
	if cfg.OneClick.Timeout != 10*time.Second || cfg.OneClick.QuoteDeadline != 180*time.Second {
		t.Fatalf("unexpected OneClick timings: %+v", cfg.OneClick)
	}
	if cfg.OneClick.MaxResponseBytes != 2<<20 {
		t.Fatalf("MaxResponseBytes = %d", cfg.OneClick.MaxResponseBytes)
	}
	if cfg.Execution.EnableLiveExecution {
		t.Fatal("live execution must default to false")
	}
	if cfg.Execution.QuoteMode != QuoteModeLive {
		t.Fatalf("QuoteMode = %s, want live", cfg.Execution.QuoteMode)
	}
	if cfg.MarketData.Mode != MarketDataLive {
		t.Fatalf("MarketData.Mode = %s, want live", cfg.MarketData.Mode)
	}
	if cfg.MarketData.TTL != 300*time.Second || cfg.MarketData.Timeout != 10*time.Second {
		t.Fatalf("unexpected market data settings: %+v", cfg.MarketData)
	}
	if cfg.Catalogue.TTL != 300*time.Second {
		t.Fatalf("Catalogue.TTL = %s", cfg.Catalogue.TTL)
	}
	if cfg.Worker.PollInterval != 5*time.Second || cfg.Worker.StatusMaxAge != 60*time.Minute {
		t.Fatalf("unexpected worker settings: %+v", cfg.Worker)
	}
	if cfg.Worker.PollConcurrency != 10 {
		t.Fatalf("PollConcurrency = %d", cfg.Worker.PollConcurrency)
	}
	if cfg.Log.Level != slog.LevelInfo || cfg.Log.Format != "json" {
		t.Fatalf("unexpected log settings: %+v", cfg.Log)
	}
}

func TestLoadFromOverrides(t *testing.T) {
	cfg, err := LoadFrom([]string{
		"APP_ENV=staging",
		"DATABASE_URL=postgres://udi:secret@db:5432/udi",
		"HTTP_ADDR=0.0.0.0:9090",
		"ONECLICK_BASE_URL=https://provider.example.test/",
		"ONECLICK_JWT=jwt-value",
		"ENABLE_LIVE_EXECUTION=true",
		"SWAP_QUOTE_MODE=live",
		"MARKET_DATA_MODE=fixture",
		"MARKET_DATA_TTL_SECONDS=60",
		"MARKET_DATA_TIMEOUT_MS=2500",
		"ASSET_CATALOGUE_TTL_SECONDS=120",
		"SWAP_STATUS_POLL_SECONDS=2",
		"SWAP_STATUS_MAX_AGE_MINUTES=15",
		"SWAP_STATUS_POLL_CONCURRENCY=4",
		"ONECLICK_TIMEOUT_MS=5000",
		"ONECLICK_QUOTE_DEADLINE_SECONDS=90",
		"ONECLICK_MAX_RESPONSE_BYTES=4096",
		"ONECLICK_QUOTE_PUBLIC_KEY=public-key",
		"LOG_LEVEL=debug",
		"LOG_FORMAT=text",
		"REDIS_URL=rediss://cache:6380",
	})
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.AppEnv != EnvStaging {
		t.Fatalf("AppEnv = %s", cfg.AppEnv)
	}
	if cfg.HTTP.Addr != "0.0.0.0:9090" {
		t.Fatalf("HTTP.Addr = %q", cfg.HTTP.Addr)
	}
	if cfg.OneClick.BaseURL != "https://provider.example.test" {
		t.Fatalf("base URL was not trimmed: %q", cfg.OneClick.BaseURL)
	}
	if cfg.OneClick.JWT != "jwt-value" || !cfg.Execution.EnableLiveExecution {
		t.Fatal("JWT or execution gate not loaded")
	}
	if cfg.MarketData.Mode != MarketDataFixture || cfg.MarketData.TTL != 60*time.Second {
		t.Fatalf("unexpected market data: %+v", cfg.MarketData)
	}
	if cfg.Catalogue.TTL != 2*time.Minute {
		t.Fatalf("Catalogue.TTL = %s", cfg.Catalogue.TTL)
	}
	if cfg.Worker.PollInterval != 2*time.Second || cfg.Worker.StatusMaxAge != 15*time.Minute || cfg.Worker.PollConcurrency != 4 {
		t.Fatalf("unexpected worker: %+v", cfg.Worker)
	}
	if cfg.OneClick.Timeout != 5*time.Second || cfg.OneClick.QuoteDeadline != 90*time.Second {
		t.Fatalf("unexpected provider timings: %+v", cfg.OneClick)
	}
	if cfg.OneClick.MaxResponseBytes != 4096 || cfg.OneClick.QuotePublicKey != "public-key" {
		t.Fatalf("unexpected provider limits: %+v", cfg.OneClick)
	}
	if cfg.Log.Level != slog.LevelDebug || cfg.Log.Format != "text" {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
	}
	if cfg.Redis.URL != "rediss://cache:6380" {
		t.Fatalf("Redis.URL = %q", cfg.Redis.URL)
	}
}

func TestLegacyAPIHostPortFallback(t *testing.T) {
	cfg, err := LoadFrom([]string{
		"API_HOST=127.0.0.1",
		"API_PORT=8082",
	})
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:8082" {
		t.Fatalf("HTTP.Addr = %q, want 127.0.0.1:8082", cfg.HTTP.Addr)
	}
}

func TestLiveExecutionValidation(t *testing.T) {
	t.Run("requires JWT", func(t *testing.T) {
		_, err := LoadFrom([]string{"ENABLE_LIVE_EXECUTION=true", "SWAP_QUOTE_MODE=live"})
		requireConfigError(t, err, "ONECLICK_JWT")
	})

	t.Run("requires live quote mode", func(t *testing.T) {
		_, err := LoadFrom([]string{"ENABLE_LIVE_EXECUTION=true", "SWAP_QUOTE_MODE=demo", "ONECLICK_JWT=x"})
		requireConfigError(t, err, "SWAP_QUOTE_MODE=live")
	})

	t.Run("accepts live execution with JWT", func(t *testing.T) {
		_, err := LoadFrom([]string{"ENABLE_LIVE_EXECUTION=true", "SWAP_QUOTE_MODE=live", "ONECLICK_JWT=x"})
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
	})
}

func TestInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		environ   []string
		wantField string
	}{
		{"app env", []string{"APP_ENV=prod"}, "APP_ENV"},
		{"quote mode", []string{"SWAP_QUOTE_MODE=paper"}, "SWAP_QUOTE_MODE"},
		{"market mode", []string{"MARKET_DATA_MODE=sandbox"}, "MARKET_DATA_MODE"},
		{"log level", []string{"LOG_LEVEL=verbose"}, "LOG_LEVEL"},
		{"log format", []string{"LOG_FORMAT=yaml"}, "LOG_FORMAT"},
		{"http address missing port", []string{"HTTP_ADDR=8081"}, "HTTP_ADDR"},
		{"http address zero port", []string{"HTTP_ADDR=:0"}, "HTTP_ADDR"},
		{"redis scheme", []string{"REDIS_URL=http://localhost:6379"}, "REDIS_URL"},
		{"provider scheme", []string{"ONECLICK_BASE_URL=ftp://provider"}, "ONECLICK_BASE_URL"},
		{"provider credentials", []string{"ONECLICK_BASE_URL=https://user:pass@provider"}, "ONECLICK_BASE_URL"},
		{"database scheme", []string{"APP_ENV=staging", "DATABASE_URL=mysql://db/udi"}, "DATABASE_URL"},
		{"database required", []string{"APP_ENV=production"}, "DATABASE_URL"},
		{"poll seconds low", []string{"SWAP_STATUS_POLL_SECONDS=0"}, "SWAP_STATUS_POLL_SECONDS"},
		{"poll seconds high", []string{"SWAP_STATUS_POLL_SECONDS=61"}, "SWAP_STATUS_POLL_SECONDS"},
		{"max age", []string{"SWAP_STATUS_MAX_AGE_MINUTES=0"}, "SWAP_STATUS_MAX_AGE_MINUTES"},
		{"catalogue ttl", []string{"ASSET_CATALOGUE_TTL_SECONDS=0"}, "ASSET_CATALOGUE_TTL_SECONDS"},
		{"quote deadline low", []string{"ONECLICK_QUOTE_DEADLINE_SECONDS=10"}, "ONECLICK_QUOTE_DEADLINE_SECONDS"},
		{"pool min above max", []string{"DB_MIN_CONNS=11"}, "DB_MIN_CONNS"},
		{"max header", []string{"HTTP_MAX_HEADER_BYTES=0"}, "HTTP_MAX_HEADER_BYTES"},
		{"concurrency high", []string{"SWAP_STATUS_POLL_CONCURRENCY=101"}, "SWAP_STATUS_POLL_CONCURRENCY"},
		{"market timeout", []string{"MARKET_DATA_TIMEOUT_MS=0"}, "MARKET_DATA_TIMEOUT_MS"},
		{"provider response limit", []string{"ONECLICK_MAX_RESPONSE_BYTES=0"}, "ONECLICK_MAX_RESPONSE_BYTES"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadFrom(test.environ)
			requireConfigError(t, err, test.wantField)
		})
	}
}

func TestSecretsAreRedacted(t *testing.T) {
	cfg, err := LoadFrom([]string{
		"APP_ENV=production",
		"DATABASE_URL=postgres://udi:db-password@db:5432/udi",
		"REDIS_URL=redis://:redis-password@cache:6379",
		"ONECLICK_JWT=super-secret-jwt",
	})
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	rendered := []string{
		fmt.Sprint(cfg),
		cfg.String(),
		cfg.LogValue().String(),
	}
	for _, text := range rendered {
		for _, secret := range []string{"db-password", "redis-password", "super-secret-jwt"} {
			if strings.Contains(text, secret) {
				t.Fatalf("rendered config leaked %q: %s", secret, text)
			}
		}
	}
	if !strings.Contains(cfg.String(), "[REDACTED]") {
		t.Fatalf("String() should mark the JWT as redacted: %s", cfg.String())
	}
}

func TestDSNSecretsAreRedacted(t *testing.T) {
	cfg, err := LoadFrom([]string{
		"APP_ENV=production",
		"DATABASE_URL=host=db user=udi password=dsn-password dbname=udi",
	})
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	for _, text := range []string{cfg.String(), cfg.LogValue().String()} {
		if strings.Contains(text, "dsn-password") {
			t.Fatalf("rendered config leaked the DSN password: %s", text)
		}
		if !strings.Contains(text, "[REDACTED]") {
			t.Fatalf("rendered config did not redact the DSN: %s", text)
		}
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := strings.Join([]string{
		"# comment",
		"",
		"UDI_TEST_DOTENV_PLAIN=plain-value",
		`UDI_TEST_DOTENV_DOUBLE="hello world"`,
		"UDI_TEST_DOTENV_SINGLE='x=y'",
		"export UDI_TEST_DOTENV_EXPORTED=exported",
		"UDI_TEST_DOTENV_EXISTING=from-file",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	keys := []string{
		"UDI_TEST_DOTENV_PLAIN",
		"UDI_TEST_DOTENV_DOUBLE",
		"UDI_TEST_DOTENV_SINGLE",
		"UDI_TEST_DOTENV_EXPORTED",
		"UDI_TEST_DOTENV_EXISTING",
	}
	for _, key := range keys {
		t.Cleanup(func() { _ = os.Unsetenv(key) })
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
	t.Setenv("UDI_TEST_DOTENV_EXISTING", "from-process")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	want := map[string]string{
		"UDI_TEST_DOTENV_PLAIN":    "plain-value",
		"UDI_TEST_DOTENV_DOUBLE":   "hello world",
		"UDI_TEST_DOTENV_SINGLE":   "x=y",
		"UDI_TEST_DOTENV_EXPORTED": "exported",
		"UDI_TEST_DOTENV_EXISTING": "from-process",
	}
	for key, expected := range want {
		if got := os.Getenv(key); got != expected {
			t.Fatalf("%s = %q, want %q", key, got, expected)
		}
	}
}

func TestLoadDotEnvMissingFileIsIgnored(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("missing file should be ignored: %v", err)
	}
}

func TestLoadDotEnvRejectsMalformedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NOT_A_PAIR\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := LoadDotEnv(path); err == nil {
		t.Fatal("expected malformed line to fail")
	}
}

func TestLogLevelAliases(t *testing.T) {
	tests := []struct {
		value string
		want  slog.Level
	}{
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"DEBUG", slog.LevelDebug},
		{" info ", slog.LevelInfo},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			cfg, err := LoadFrom([]string{"LOG_LEVEL=" + test.value})
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			if cfg.Log.Level != test.want {
				t.Fatalf("Log.Level = %s, want %s", cfg.Log.Level, test.want)
			}
		})
	}
}

func TestLoadFromRejectsUnparseableValues(t *testing.T) {
	_, err := LoadFrom([]string{"HTTP_READ_TIMEOUT_SECONDS=soon"})
	requireConfigError(t, err, "parse environment")
}

func TestDatabaseDSNAcceptedOutsideLocal(t *testing.T) {
	cfg, err := LoadFrom([]string{
		"APP_ENV=staging",
		"DATABASE_URL=host=db port=5432 user=udi password=secret dbname=udi",
	})
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !strings.Contains(cfg.Database.URL, "host=db") {
		t.Fatalf("Database.URL = %q", cfg.Database.URL)
	}
}

func TestDatabaseURLCanBeEmptyLocally(t *testing.T) {
	cfg, err := LoadFrom([]string{"APP_ENV=local", "DATABASE_URL="})
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Database.URL != "" {
		t.Fatalf("Database.URL = %q, want empty", cfg.Database.URL)
	}
}

func TestURLValidationRejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    string
	}{
		{"redis missing host", []string{"REDIS_URL=redis://"}, "REDIS_URL"},
		{"provider malformed", []string{"ONECLICK_BASE_URL=http://[::1"}, "ONECLICK_BASE_URL"},
		{"database malformed", []string{"APP_ENV=staging", "DATABASE_URL=postgres://[::1"}, "DATABASE_URL"},
		{"database missing host", []string{"APP_ENV=staging", "DATABASE_URL=postgres:///udi"}, "DATABASE_URL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadFrom(test.environ)
			requireConfigError(t, err, test.want)
		})
	}
}

func TestEnvironmentMapIgnoresMalformedEntries(t *testing.T) {
	got := environmentMap([]string{"A=1", "BROKEN", "=missing-key", "B=2=3"})
	if got["A"] != "1" || got["B"] != "2=3" {
		t.Fatalf("environmentMap = %#v", got)
	}
	if _, ok := got["BROKEN"]; ok {
		t.Fatal("malformed entry should be ignored")
	}
}

func TestUnquote(t *testing.T) {
	tests := []struct {
		value string
		want  string
		ok    bool
	}{
		{`"hello"`, "hello", true},
		{`'hello'`, "hello", true},
		{`hello`, "", false},
		{`"unterminated`, "", false},
		{`"`, "", false},
		{``, "", false},
	}
	for _, test := range tests {
		got, ok := unquote(test.value)
		if got != test.want || ok != test.ok {
			t.Fatalf("unquote(%q) = (%q, %t), want (%q, %t)", test.value, got, ok, test.want, test.ok)
		}
	}
}

func TestLoadDotEnvSkipsCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\n\n   \nUDI_TEST_DOTENV_EMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("UDI_TEST_DOTENV_EMPTY") })
	_ = os.Unsetenv("UDI_TEST_DOTENV_EMPTY")
	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if value, ok := os.LookupEnv("UDI_TEST_DOTENV_EMPTY"); !ok || value != "" {
		t.Fatalf("empty value was not set: %q, %t", value, ok)
	}
}

func requireConfigError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected configuration error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}
