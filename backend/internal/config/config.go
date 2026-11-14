// Package config loads and validates the backend runtime configuration.
//
// The configuration is deliberately parsed into typed structs instead of being
// passed around as raw environment strings. Invalid values fail fast at startup
// so the API and worker never run with an ambiguous timeout, mode, or secret.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Environment identifies the deployment environment.
type Environment string

const (
	EnvLocal      Environment = "local"
	EnvStaging    Environment = "staging"
	EnvProduction Environment = "production"
)

// QuoteMode controls whether quotes come from local fixtures or a live provider.
type QuoteMode string

const (
	QuoteModeDemo QuoteMode = "demo"
	QuoteModeLive QuoteMode = "live"
)

// MarketDataMode controls whether analytics use fixtures or live public sources.
type MarketDataMode string

const (
	MarketDataFixture MarketDataMode = "fixture"
	MarketDataLive    MarketDataMode = "live"
)

// Config is the fully validated backend configuration.
type Config struct {
	AppEnv     Environment
	HTTP       HTTPConfig
	Worker     WorkerConfig
	Database   DatabaseConfig
	Redis      RedisConfig
	OneClick   OneClickConfig
	Execution  ExecutionConfig
	MarketData MarketDataConfig
	Catalogue  CatalogueConfig
	Security   SecurityConfig
	Log        LogConfig
}

// SecurityConfig contains authentication and HTTP edge protections.
type SecurityConfig struct {
	// AuthRequired forces wallet sign-in for every swap and activity route.
	AuthRequired bool
	SessionTTL   time.Duration
	// CORSAllowedOrigins is an explicit allowlist. Empty means no cross-origin
	// access (the Next.js proxy is same-origin and needs none).
	CORSAllowedOrigins []string
	// TrustedProxies are the networks whose X-Forwarded-For header is honored
	// for client IP attribution (rate limiting and logs).
	TrustedProxies []netip.Prefix
	MaxBodyBytes   int64
	RateLimitRPS   float64
	RateLimitBurst int
	// MetricsToken, when set, is required as a bearer token on /metrics.
	MetricsToken string
	// AdminToken, when set, is required as a bearer token on /api/v1/admin/* operations routes.
	AdminToken string
	// WorkerMetricsAddr, when set, makes the worker serve /metrics there.
	WorkerMetricsAddr string
}

// HTTPConfig contains HTTP server limits and timeouts.
type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// WorkerConfig contains background polling limits.
type WorkerConfig struct {
	PollInterval    time.Duration
	StatusMaxAge    time.Duration
	PollConcurrency int
}

// DatabaseConfig contains PostgreSQL connection settings.
type DatabaseConfig struct {
	URL            string
	MaxConns       int32
	MinConns       int32
	ConnectTimeout time.Duration
	PingTimeout    time.Duration
	// RequireTLS rejects connection strings that allow plaintext
	// (sslmode=disable/allow/prefer or none). Defaults to true in production.
	RequireTLS bool
	// PgBouncer switches pgx to a query mode without cached named prepared
	// statements, which PgBouncer in transaction mode (Neon, Supabase pooled
	// endpoints) cannot route. Migrations should use a direct, unpooled URL.
	PgBouncer bool
}

// RedisConfig contains Redis connection settings.
type RedisConfig struct {
	URL     string
	Timeout time.Duration
}

// OneClickConfig contains NEAR Intents 1Click provider settings.
type OneClickConfig struct {
	BaseURL          string
	JWT              string
	Timeout          time.Duration
	QuoteDeadline    time.Duration
	MaxResponseBytes int64
	QuotePublicKey   string
}

// ExecutionConfig contains the live-execution safety gate.
type ExecutionConfig struct {
	EnableLiveExecution bool
	QuoteMode           QuoteMode
}

// MarketDataConfig contains analytics source settings.
type MarketDataConfig struct {
	Mode    MarketDataMode
	TTL     time.Duration
	Timeout time.Duration
}

// CatalogueConfig contains the token catalogue cache settings.
type CatalogueConfig struct {
	TTL time.Duration
}

// LogConfig contains structured logging settings.
type LogConfig struct {
	Level  slog.Level
	Format string
}

// rawConfig is the wire representation of the environment. All values are kept
// as strings/integers here and converted into typed fields by build.
type rawConfig struct {
	AppEnv string `env:"APP_ENV" envDefault:"local"`

	HTTPAddr string `env:"HTTP_ADDR"`
	APIHost  string `env:"API_HOST"`
	APIPort  string `env:"API_PORT"`

	HTTPReadHeaderTimeoutSeconds int `env:"HTTP_READ_HEADER_TIMEOUT_SECONDS" envDefault:"5"`
	HTTPReadTimeoutSeconds       int `env:"HTTP_READ_TIMEOUT_SECONDS" envDefault:"15"`
	HTTPWriteTimeoutSeconds      int `env:"HTTP_WRITE_TIMEOUT_SECONDS" envDefault:"30"`
	HTTPIdleTimeoutSeconds       int `env:"HTTP_IDLE_TIMEOUT_SECONDS" envDefault:"60"`
	HTTPMaxHeaderBytes           int `env:"HTTP_MAX_HEADER_BYTES" envDefault:"1048576"`

	DatabaseURL             string `env:"DATABASE_URL"`
	DBMaxConns              int32  `env:"DB_MAX_CONNS" envDefault:"10"`
	DBMinConns              int32  `env:"DB_MIN_CONNS" envDefault:"0"`
	DBConnectTimeoutSeconds int    `env:"DB_CONNECT_TIMEOUT_SECONDS" envDefault:"5"`
	DBPingTimeoutSeconds    int    `env:"DB_PING_TIMEOUT_SECONDS" envDefault:"2"`
	DBRequireTLS            string `env:"DB_REQUIRE_TLS"`
	DBPgBouncer             bool   `env:"DB_PGBOUNCER" envDefault:"false"`

	RedisURL            string `env:"REDIS_URL" envDefault:"redis://localhost:6379"`
	RedisTimeoutSeconds int    `env:"REDIS_TIMEOUT_SECONDS" envDefault:"2"`

	OneClickBaseURL              string `env:"ONECLICK_BASE_URL" envDefault:"https://1click.chaindefuser.com"`
	OneClickJWT                  string `env:"ONECLICK_JWT"`
	OneClickTimeoutMS            int    `env:"ONECLICK_TIMEOUT_MS" envDefault:"10000"`
	OneClickQuoteDeadlineSeconds int    `env:"ONECLICK_QUOTE_DEADLINE_SECONDS" envDefault:"180"`
	OneClickMaxResponseBytes     int64  `env:"ONECLICK_MAX_RESPONSE_BYTES" envDefault:"2097152"`
	OneClickQuotePublicKey       string `env:"ONECLICK_QUOTE_PUBLIC_KEY"`

	EnableLiveExecution       bool   `env:"ENABLE_LIVE_EXECUTION" envDefault:"false"`
	SwapQuoteMode             string `env:"SWAP_QUOTE_MODE" envDefault:"live"`
	MarketDataMode            string `env:"MARKET_DATA_MODE" envDefault:"live"`
	MarketDataTTLSeconds      int    `env:"MARKET_DATA_TTL_SECONDS" envDefault:"300"`
	MarketDataTimeoutMS       int    `env:"MARKET_DATA_TIMEOUT_MS" envDefault:"10000"`
	AssetCatalogueTTLSeconds  int    `env:"ASSET_CATALOGUE_TTL_SECONDS" envDefault:"300"`
	SwapStatusPollSeconds     int    `env:"SWAP_STATUS_POLL_SECONDS" envDefault:"5"`
	SwapStatusMaxAgeMinutes   int    `env:"SWAP_STATUS_MAX_AGE_MINUTES" envDefault:"60"`
	SwapStatusPollConcurrency int    `env:"SWAP_STATUS_POLL_CONCURRENCY" envDefault:"10"`

	AuthRequired          bool   `env:"AUTH_REQUIRED" envDefault:"false"`
	AuthSessionTTLMinutes int    `env:"AUTH_SESSION_TTL_MINUTES" envDefault:"1440"`
	CORSAllowedOrigins    string `env:"CORS_ALLOWED_ORIGINS"`
	TrustedProxyCIDRs     string `env:"TRUSTED_PROXY_CIDRS" envDefault:"127.0.0.1/32,::1/128"`
	HTTPMaxBodyBytes      int64  `env:"HTTP_MAX_BODY_BYTES" envDefault:"65536"`
	RateLimitRPS          int    `env:"RATE_LIMIT_RPS" envDefault:"20"`
	RateLimitBurst        int    `env:"RATE_LIMIT_BURST" envDefault:"40"`
	MetricsToken          string `env:"METRICS_TOKEN"`
	AdminToken            string `env:"ADMIN_TOKEN"`
	WorkerMetricsAddr     string `env:"WORKER_METRICS_ADDR"`

	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`
}

// Load optionally reads a .env file from the current directory and then parses
// the process environment. Existing environment variables always win over .env.
func Load() (*Config, error) {
	if err := LoadDotEnv(".env"); err != nil {
		return nil, fmt.Errorf("config: load .env: %w", err)
	}
	return LoadFrom(os.Environ())
}

// LoadFrom parses and validates configuration from a list of KEY=VALUE pairs.
// It is exported so tests can provide an isolated environment.
func LoadFrom(environ []string) (*Config, error) {
	var raw rawConfig
	opts := env.Options{Environment: environmentMap(environ)}
	if err := env.ParseWithOptions(&raw, opts); err != nil {
		return nil, fmt.Errorf("config: parse environment: %w", err)
	}
	cfg, err := build(raw)
	if err != nil {
		return nil, fmt.Errorf("config: invalid configuration: %w", err)
	}
	return cfg, nil
}

// LoadDotEnv applies a simple KEY=VALUE .env file without interpolation.
// Missing files are ignored. Values already present in the process environment
// are never overwritten.
func LoadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("line %d: expected KEY=VALUE", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("line %d: empty key", lineNumber+1)
		}
		if len(value) >= 2 {
			if unquoted, ok := unquote(value); ok {
				value = unquoted
			}
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("line %d: set %s: %w", lineNumber+1, key, err)
		}
	}
	return nil
}

func unquote(value string) (string, bool) {
	if len(value) < 2 {
		return "", false
	}
	switch {
	case value[0] == '"' && value[len(value)-1] == '"':
		return value[1 : len(value)-1], true
	case value[0] == '\'' && value[len(value)-1] == '\'':
		return value[1 : len(value)-1], true
	default:
		return "", false
	}
}

func environmentMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func build(raw rawConfig) (*Config, error) {
	var problems []error
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	appEnv := Environment(strings.ToLower(strings.TrimSpace(raw.AppEnv)))
	switch appEnv {
	case EnvLocal, EnvStaging, EnvProduction:
	default:
		add("APP_ENV must be one of local, staging, production (got %q)", raw.AppEnv)
	}

	httpAddr := strings.TrimSpace(raw.HTTPAddr)
	if httpAddr == "" {
		host := strings.TrimSpace(raw.APIHost)
		port := strings.TrimSpace(raw.APIPort)
		if port == "" {
			port = "8081"
		}
		httpAddr = net.JoinHostPort(host, port)
	}
	if err := validateHostPort(httpAddr); err != nil {
		add("HTTP_ADDR: %v", err)
	}

	if raw.HTTPReadHeaderTimeoutSeconds <= 0 {
		add("HTTP_READ_HEADER_TIMEOUT_SECONDS must be greater than zero")
	}
	if raw.HTTPReadTimeoutSeconds <= 0 {
		add("HTTP_READ_TIMEOUT_SECONDS must be greater than zero")
	}
	if raw.HTTPWriteTimeoutSeconds <= 0 {
		add("HTTP_WRITE_TIMEOUT_SECONDS must be greater than zero")
	}
	if raw.HTTPIdleTimeoutSeconds <= 0 {
		add("HTTP_IDLE_TIMEOUT_SECONDS must be greater than zero")
	}
	if raw.HTTPMaxHeaderBytes <= 0 || raw.HTTPMaxHeaderBytes > 8<<20 {
		add("HTTP_MAX_HEADER_BYTES must be between 1 and 8388608")
	}

	if err := validateDatabaseURL(raw.DatabaseURL, appEnv); err != nil {
		add("DATABASE_URL: %v", err)
	}
	requireTLS := appEnv == EnvProduction
	switch strings.ToLower(strings.TrimSpace(raw.DBRequireTLS)) {
	case "":
	case "true", "1", "yes":
		requireTLS = true
	case "false", "0", "no":
		requireTLS = false
	default:
		add("DB_REQUIRE_TLS must be true or false (got %q)", raw.DBRequireTLS)
	}
	if requireTLS && strings.TrimSpace(raw.DatabaseURL) != "" {
		if err := requireDatabaseTLS(raw.DatabaseURL); err != nil {
			add("DATABASE_URL: %v (set sslmode=require or verify-full, or DB_REQUIRE_TLS=false for a private network)", err)
		}
	}
	if raw.DBMaxConns <= 0 {
		add("DB_MAX_CONNS must be greater than zero")
	}
	if raw.DBMinConns < 0 || raw.DBMinConns > raw.DBMaxConns {
		add("DB_MIN_CONNS must be between 0 and DB_MAX_CONNS")
	}
	if raw.DBConnectTimeoutSeconds <= 0 {
		add("DB_CONNECT_TIMEOUT_SECONDS must be greater than zero")
	}
	if raw.DBPingTimeoutSeconds <= 0 {
		add("DB_PING_TIMEOUT_SECONDS must be greater than zero")
	}

	if err := validateRedisURL(raw.RedisURL); err != nil {
		add("REDIS_URL: %v", err)
	}
	if raw.RedisTimeoutSeconds <= 0 {
		add("REDIS_TIMEOUT_SECONDS must be greater than zero")
	}

	if err := validateHTTPURL(raw.OneClickBaseURL); err != nil {
		add("ONECLICK_BASE_URL: %v", err)
	}
	if raw.OneClickTimeoutMS <= 0 || raw.OneClickTimeoutMS > 60000 {
		add("ONECLICK_TIMEOUT_MS must be between 1 and 60000")
	}
	if raw.OneClickQuoteDeadlineSeconds < 60 || raw.OneClickQuoteDeadlineSeconds > 600 {
		add("ONECLICK_QUOTE_DEADLINE_SECONDS must be between 60 and 600")
	}
	if raw.OneClickMaxResponseBytes <= 0 || raw.OneClickMaxResponseBytes > 10<<20 {
		add("ONECLICK_MAX_RESPONSE_BYTES must be between 1 and 10485760")
	}

	quoteMode, err := parseQuoteMode(raw.SwapQuoteMode)
	if err != nil {
		add("SWAP_QUOTE_MODE: %v", err)
	}
	marketMode, err := parseMarketDataMode(raw.MarketDataMode)
	if err != nil {
		add("MARKET_DATA_MODE: %v", err)
	}
	if raw.EnableLiveExecution && quoteMode != QuoteModeLive {
		add("ENABLE_LIVE_EXECUTION requires SWAP_QUOTE_MODE=live")
	}
	if raw.EnableLiveExecution && strings.TrimSpace(raw.OneClickJWT) == "" {
		add("ENABLE_LIVE_EXECUTION requires ONECLICK_JWT")
	}
	if raw.MarketDataTTLSeconds <= 0 {
		add("MARKET_DATA_TTL_SECONDS must be greater than zero")
	}
	if raw.MarketDataTimeoutMS <= 0 || raw.MarketDataTimeoutMS > 60000 {
		add("MARKET_DATA_TIMEOUT_MS must be between 1 and 60000")
	}
	if raw.AssetCatalogueTTLSeconds <= 0 {
		add("ASSET_CATALOGUE_TTL_SECONDS must be greater than zero")
	}
	if raw.SwapStatusPollSeconds < 1 || raw.SwapStatusPollSeconds > 60 {
		add("SWAP_STATUS_POLL_SECONDS must be between 1 and 60")
	}
	if raw.SwapStatusMaxAgeMinutes <= 0 {
		add("SWAP_STATUS_MAX_AGE_MINUTES must be greater than zero")
	}
	if raw.SwapStatusPollConcurrency < 1 || raw.SwapStatusPollConcurrency > 100 {
		add("SWAP_STATUS_POLL_CONCURRENCY must be between 1 and 100")
	}

	if raw.AuthSessionTTLMinutes < 5 || raw.AuthSessionTTLMinutes > 43200 {
		add("AUTH_SESSION_TTL_MINUTES must be between 5 and 43200")
	}
	corsOrigins, err := parseOrigins(raw.CORSAllowedOrigins)
	if err != nil {
		add("CORS_ALLOWED_ORIGINS: %v", err)
	}
	trustedProxies, err := parsePrefixes(raw.TrustedProxyCIDRs)
	if err != nil {
		add("TRUSTED_PROXY_CIDRS: %v", err)
	}
	if raw.HTTPMaxBodyBytes < 1024 || raw.HTTPMaxBodyBytes > 10<<20 {
		add("HTTP_MAX_BODY_BYTES must be between 1024 and 10485760")
	}
	if raw.RateLimitRPS < 1 || raw.RateLimitRPS > 10000 {
		add("RATE_LIMIT_RPS must be between 1 and 10000")
	}
	if raw.RateLimitBurst < raw.RateLimitRPS || raw.RateLimitBurst > 20000 {
		add("RATE_LIMIT_BURST must be between RATE_LIMIT_RPS and 20000")
	}

	logLevel, err := parseLogLevel(raw.LogLevel)
	if err != nil {
		add("LOG_LEVEL: %v", err)
	}
	logFormat := strings.ToLower(strings.TrimSpace(raw.LogFormat))
	if logFormat != "json" && logFormat != "text" {
		add("LOG_FORMAT must be json or text (got %q)", raw.LogFormat)
	}

	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}

	return &Config{
		AppEnv: appEnv,
		HTTP: HTTPConfig{
			Addr:              httpAddr,
			ReadHeaderTimeout: time.Duration(raw.HTTPReadHeaderTimeoutSeconds) * time.Second,
			ReadTimeout:       time.Duration(raw.HTTPReadTimeoutSeconds) * time.Second,
			WriteTimeout:      time.Duration(raw.HTTPWriteTimeoutSeconds) * time.Second,
			IdleTimeout:       time.Duration(raw.HTTPIdleTimeoutSeconds) * time.Second,
			MaxHeaderBytes:    raw.HTTPMaxHeaderBytes,
		},
		Worker: WorkerConfig{
			PollInterval:    time.Duration(raw.SwapStatusPollSeconds) * time.Second,
			StatusMaxAge:    time.Duration(raw.SwapStatusMaxAgeMinutes) * time.Minute,
			PollConcurrency: raw.SwapStatusPollConcurrency,
		},
		Database: DatabaseConfig{
			URL:            strings.TrimSpace(raw.DatabaseURL),
			MaxConns:       raw.DBMaxConns,
			MinConns:       raw.DBMinConns,
			ConnectTimeout: time.Duration(raw.DBConnectTimeoutSeconds) * time.Second,
			PingTimeout:    time.Duration(raw.DBPingTimeoutSeconds) * time.Second,
			RequireTLS:     requireTLS,
			PgBouncer:      raw.DBPgBouncer,
		},
		Redis: RedisConfig{
			URL:     strings.TrimSpace(raw.RedisURL),
			Timeout: time.Duration(raw.RedisTimeoutSeconds) * time.Second,
		},
		OneClick: OneClickConfig{
			BaseURL:          strings.TrimRight(strings.TrimSpace(raw.OneClickBaseURL), "/"),
			JWT:              strings.TrimSpace(raw.OneClickJWT),
			Timeout:          time.Duration(raw.OneClickTimeoutMS) * time.Millisecond,
			QuoteDeadline:    time.Duration(raw.OneClickQuoteDeadlineSeconds) * time.Second,
			MaxResponseBytes: raw.OneClickMaxResponseBytes,
			QuotePublicKey:   strings.TrimSpace(raw.OneClickQuotePublicKey),
		},
		Execution: ExecutionConfig{
			EnableLiveExecution: raw.EnableLiveExecution,
			QuoteMode:           quoteMode,
		},
		MarketData: MarketDataConfig{
			Mode:    marketMode,
			TTL:     time.Duration(raw.MarketDataTTLSeconds) * time.Second,
			Timeout: time.Duration(raw.MarketDataTimeoutMS) * time.Millisecond,
		},
		Catalogue: CatalogueConfig{
			TTL: time.Duration(raw.AssetCatalogueTTLSeconds) * time.Second,
		},
		Security: SecurityConfig{
			AuthRequired:       raw.AuthRequired,
			SessionTTL:         time.Duration(raw.AuthSessionTTLMinutes) * time.Minute,
			CORSAllowedOrigins: corsOrigins,
			TrustedProxies:     trustedProxies,
			MaxBodyBytes:       raw.HTTPMaxBodyBytes,
			RateLimitRPS:       float64(raw.RateLimitRPS),
			RateLimitBurst:     raw.RateLimitBurst,
			MetricsToken:       strings.TrimSpace(raw.MetricsToken),
			AdminToken:         strings.TrimSpace(raw.AdminToken),
			WorkerMetricsAddr:  strings.TrimSpace(raw.WorkerMetricsAddr),
		},
		Log: LogConfig{
			Level:  logLevel,
			Format: logFormat,
		},
	}, nil
}

// parseOrigins parses a comma-separated CORS allowlist. Wildcards are
// rejected: a credentialed API must never answer Access-Control-Allow-Origin: *.
func parseOrigins(value string) ([]string, error) {
	var origins []string
	for _, part := range strings.Split(value, ",") {
		origin := strings.TrimRight(strings.TrimSpace(part), "/")
		if origin == "" {
			continue
		}
		if strings.Contains(origin, "*") {
			return nil, errors.New("wildcard origins are not allowed")
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" {
			return nil, fmt.Errorf("invalid origin %q (want scheme://host[:port])", origin)
		}
		origins = append(origins, strings.ToLower(origin))
	}
	return origins, nil
}

// parsePrefixes parses a comma-separated list of CIDRs or bare IPs.
func parsePrefixes(value string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			addr, err := netip.ParseAddr(part)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q", part)
			}
			prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", part)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func parseQuoteMode(value string) (QuoteMode, error) {
	switch QuoteMode(strings.ToLower(strings.TrimSpace(value))) {
	case QuoteModeDemo:
		return QuoteModeDemo, nil
	case QuoteModeLive:
		return QuoteModeLive, nil
	default:
		return "", fmt.Errorf("must be demo or live (got %q)", value)
	}
}

func parseMarketDataMode(value string) (MarketDataMode, error) {
	switch MarketDataMode(strings.ToLower(strings.TrimSpace(value))) {
	case MarketDataFixture:
		return MarketDataFixture, nil
	case MarketDataLive:
		return MarketDataLive, nil
	default:
		return "", fmt.Errorf("must be fixture or live (got %q)", value)
	}
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("must be debug, info, warn, or error (got %q)", value)
	}
}

func validateHostPort(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("must be host:port: %w", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("port must be between 1 and 65535 (got %q)", port)
	}
	return nil
}

func validateHTTPURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("must not be empty")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("must use http or https (got %q)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("must include a host")
	}
	if parsed.User != nil {
		return errors.New("must not include credentials")
	}
	return nil
}

func validateRedisURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("must not be empty")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme != "redis" && parsed.Scheme != "rediss" {
		return fmt.Errorf("must use redis or rediss (got %q)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("must include a host")
	}
	return nil
}

func validateDatabaseURL(value string, appEnv Environment) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if appEnv == EnvLocal {
			return nil
		}
		return errors.New("is required outside local")
	}
	if strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return err
		}
		if parsed.Host == "" {
			return errors.New("must include a host")
		}
		return nil
	}
	if strings.Contains(value, "host=") {
		return nil
	}
	return errors.New("must be a postgres:// URL or a host= DSN")
}

// requireDatabaseTLS accepts only sslmode values that refuse plaintext.
func requireDatabaseTLS(value string) error {
	mode := ""
	if strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return err
		}
		mode = parsed.Query().Get("sslmode")
	} else {
		for _, field := range strings.Fields(value) {
			if key, val, ok := strings.Cut(field, "="); ok && key == "sslmode" {
				mode = strings.Trim(val, "'")
			}
		}
	}
	switch strings.ToLower(mode) {
	case "require", "verify-ca", "verify-full":
		return nil
	case "":
		return errors.New("sslmode is not set; plaintext fallback is possible")
	default:
		return fmt.Errorf("sslmode=%s allows unencrypted connections", mode)
	}
}

// String implements fmt.Stringer and never includes secrets or database
// passwords.
func (c Config) String() string {
	return fmt.Sprintf(
		"config{app_env=%s http_addr=%s database=%s redis=%s oneclick_base_url=%s oneclick_jwt=%s live_execution=%t quote_mode=%s market_data_mode=%s log_level=%s}",
		c.AppEnv,
		c.HTTP.Addr,
		redactURL(c.Database.URL),
		redactURL(c.Redis.URL),
		c.OneClick.BaseURL,
		redactSecret(c.OneClick.JWT),
		c.Execution.EnableLiveExecution,
		c.Execution.QuoteMode,
		c.MarketData.Mode,
		c.Log.Level,
	)
}

// LogValue implements slog.LogValuer so logging a Config is safe by default.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("app_env", string(c.AppEnv)),
		slog.String("http_addr", c.HTTP.Addr),
		slog.String("database", redactURL(c.Database.URL)),
		slog.String("redis", redactURL(c.Redis.URL)),
		slog.String("oneclick_base_url", c.OneClick.BaseURL),
		slog.String("oneclick_jwt", redactSecret(c.OneClick.JWT)),
		slog.Bool("live_execution", c.Execution.EnableLiveExecution),
		slog.String("quote_mode", string(c.Execution.QuoteMode)),
		slog.String("market_data_mode", string(c.MarketData.Mode)),
		slog.String("log_level", c.Log.Level.String()),
	)
}

func redactSecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "[REDACTED]"
}

func redactURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		// A DSN such as "host=db password=secret" has no URL scheme. Never
		// risk printing credentials for values we cannot parse safely.
		return "[REDACTED]"
	}
	parsed.User = nil
	return parsed.String()
}
