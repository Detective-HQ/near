package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// Store defines the persistence operations required by the swap service.
// *repository.Store (PostgreSQL) and *memstore.Store both satisfy it.
type Store interface {
	SaveQuote(ctx context.Context, quote domain.Quote) (domain.Quote, error)
	GetQuote(ctx context.Context, id string) (domain.Quote, error)
	GetQuotes(ctx context.Context, ids []string) (map[string]domain.Quote, error)
	CreateSwap(ctx context.Context, swap domain.Swap, idempotencyKey string) (domain.Swap, bool, error)
	GetSwapByID(ctx context.Context, id string) (domain.Swap, error)
	GetSwapByIdempotencyKey(ctx context.Context, key string) (domain.Swap, error)
	UpdateSwapState(
		ctx context.Context,
		swapID string,
		oldState, newState domain.State,
		providerStatus domain.ProviderStatus,
		payload map[string]any,
	) (domain.Swap, domain.Event, error)
	RecordDeposit(
		ctx context.Context,
		swapID, chain, txHash string,
		providerStatus domain.ProviderStatus,
		payload map[string]any,
	) (domain.Swap, domain.Deposit, bool, error)
	ListSwapsBySession(ctx context.Context, sessionID string, limit int, cursor string) ([]domain.Swap, string, error)
	ListEventsBySwapIDs(ctx context.Context, ids []string) (map[string][]domain.Event, error)
	ListTransactionsBySwapIDs(ctx context.Context, ids []string) (map[string][]domain.Deposit, error)
	GetAssetByID(ctx context.Context, id string) (domain.Asset, error)
	ListAssets(ctx context.Context, chain string) ([]domain.Asset, error)
	UpsertAssets(ctx context.Context, assets []domain.Asset, source string, observedAt time.Time) (int, error)
}

// ProviderObserver receives the outcome of every provider call. The risk
// engine implements it to track error rates and catalogue freshness.
type ProviderObserver interface {
	ObserveProviderCall(err error)
	ObserveCatalogueRefresh(at time.Time)
}

// Config tunes the orchestration service.
type Config struct {
	Store               Store
	Provider            providers.Adapter
	Registry            *registry.Registry
	QuoteMode           config.QuoteMode
	EnableLiveExecution bool
	QuoteDeadline       time.Duration
	StatusMaxAge        time.Duration
	// CatalogueTTL is how long the in-process asset catalogue is served before
	// it is reloaded from the store (and, when empty, from the provider).
	CatalogueTTL time.Duration
	Observer     ProviderObserver
	Logger       *slog.Logger
	Now          func() time.Time
}

// Service orchestrates quotes, swaps, deposits, cancellations, and status lookups.
type Service struct {
	store               Store
	provider            providers.Adapter
	registry            *registry.Registry
	quoteMode           config.QuoteMode
	enableLiveExecution bool
	quoteDeadline       time.Duration
	statusMaxAge        time.Duration
	catalogueTTL        time.Duration
	observer            ProviderObserver
	logger              *slog.Logger
	now                 func() time.Time
	bgWg                sync.WaitGroup

	catalogueMu     sync.RWMutex
	catalogue       *catalogueSnapshot
	catalogueFlight singleflight.Group
}

// Wait waits for in-flight background tasks (such as provider deposit
// notifications) to finish, or until ctx expires.
func (s *Service) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.bgWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// New creates an initialized swap service.
func New(cfg Config) (*Service, error) {
	if cfg.Store == nil {
		return nil, domain.New(domain.CodeInternal, "Store is required for swap service.")
	}
	if cfg.Provider == nil {
		return nil, domain.New(domain.CodeInternal, "Provider adapter is required for swap service.")
	}
	reg := cfg.Registry
	if reg == nil {
		var err error
		reg, err = registry.Load()
		if err != nil {
			return nil, domain.Wrap(domain.CodeInternal, "Could not load chain registry.", err)
		}
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	deadline := cfg.QuoteDeadline
	if deadline <= 0 {
		deadline = 3 * time.Minute
	}
	maxAge := cfg.StatusMaxAge
	if maxAge <= 0 {
		maxAge = 60 * time.Minute
	}
	catalogueTTL := cfg.CatalogueTTL
	if catalogueTTL <= 0 {
		catalogueTTL = 5 * time.Minute
	}
	mode := cfg.QuoteMode
	if mode == "" {
		mode = config.QuoteModeLive
	}

	return &Service{
		store:               cfg.Store,
		provider:            cfg.Provider,
		registry:            reg,
		quoteMode:           mode,
		enableLiveExecution: cfg.EnableLiveExecution,
		quoteDeadline:       deadline,
		statusMaxAge:        maxAge,
		catalogueTTL:        catalogueTTL,
		observer:            cfg.Observer,
		logger:              logger,
		now:                 now,
	}, nil
}

// QuoteMode reports whether the service serves demo or live quotes.
func (s *Service) QuoteMode() config.QuoteMode { return s.quoteMode }

// LiveExecutionEnabled reports the server execution gate.
func (s *Service) LiveExecutionEnabled() bool { return s.enableLiveExecution }

// Registry exposes the chain registry used for validation.
func (s *Service) Registry() *registry.Registry { return s.registry }

func (s *Service) observe(err error) {
	if s.observer != nil {
		s.observer.ObserveProviderCall(err)
	}
}
