package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// Store defines the repository operations required by the swap service.
// *repository.Store satisfies this interface.
type Store interface {
	SaveQuote(ctx context.Context, quote domain.Quote) (domain.Quote, error)
	GetQuote(ctx context.Context, id string) (domain.Quote, error)
	CreateSwap(ctx context.Context, swap domain.Swap, idempotencyKey string) (domain.Swap, bool, error)
	GetSwapByID(ctx context.Context, id string) (domain.Swap, error)
	UpdateSwapState(
		ctx context.Context,
		swapID string,
		oldState, newState domain.State,
		providerStatus domain.ProviderStatus,
		payload map[string]any,
	) (domain.Swap, domain.Event, error)
	RecordTransaction(
		ctx context.Context,
		swapID, chain, txHash string,
		role domain.TransactionRole,
	) (domain.Deposit, bool, error)
	ListSwaps(ctx context.Context, limit int, cursor string) ([]domain.Swap, string, error)
	ListSwapsBySession(ctx context.Context, sessionID string, limit int, cursor string) ([]domain.Swap, string, error)
	GetAssetByID(ctx context.Context, id string) (domain.Asset, error)
	ListAssets(ctx context.Context, chain string) ([]domain.Asset, error)
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
	Logger              *slog.Logger
	Now                 func() time.Time
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
	logger              *slog.Logger
	now                 func() time.Time
	bgWg                sync.WaitGroup
}

// Wait waits for in-flight background tasks (such as provider deposit notifications) to finish, or until ctx expires.
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
		logger:              logger,
		now:                 now,
	}, nil
}
