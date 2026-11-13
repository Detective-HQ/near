package service

import (
	"context"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// RecordDepositParams contains the user-reported source-chain deposit data.
type RecordDepositParams struct {
	SwapID    string `json:"swapId"`
	TxHash    string `json:"txHash"`
	ChainID   string `json:"chainId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// RecordDepositResult is the outcome of recording a deposit transaction.
type RecordDepositResult struct {
	Swap      domain.Swap    `json:"swap"`
	Deposit   domain.Deposit `json:"deposit"`
	Duplicate bool           `json:"duplicate"`
}

// RecordDeposit processes a wallet-reported source transaction.
//
//   - The caller must own the swap (when it has an owner).
//   - The hash is validated for the origin chain's family, and an EVM chainId,
//     when supplied, must match the origin chain.
//   - Recording the transaction and advancing AWAITING_USER_DEPOSIT ->
//     DEPOSIT_SUBMITTED happen in one store transaction. Replaying the same
//     hash returns Duplicate=true; a different hash is a CONFLICT; a hash
//     already attached to another swap is a CONFLICT.
//   - A deposit is accepted even if the quote deadline has passed while the
//     swap is still awaiting it: the funds are already on chain, and the
//     provider decides between settlement and refund. Rejecting the hash would
//     only lose track of real money.
//   - The provider is then notified asynchronously; a failed notification does
//     not roll anything back because the poller is the source of truth.
func (s *Service) RecordDeposit(ctx context.Context, params RecordDepositParams) (RecordDepositResult, error) {
	swapID := strings.TrimSpace(params.SwapID)
	if swapID == "" {
		return RecordDepositResult{}, domain.New(domain.CodeValidationFailed, "Swap ID is required.")
	}
	if strings.TrimSpace(params.TxHash) == "" {
		return RecordDepositResult{}, domain.New(domain.CodeInvalidTxHash, "Transaction hash is required.")
	}

	swap, err := s.store.GetSwapByID(ctx, swapID)
	if err != nil {
		return RecordDepositResult{}, err
	}
	if err := checkOwner(swap, params.SessionID); err != nil {
		return RecordDepositResult{}, err
	}

	normalizedHash, err := ValidateTxHash(s.registry, swap.OriginChain, params.TxHash)
	if err != nil {
		return RecordDepositResult{}, err
	}
	if chainID := strings.TrimSpace(params.ChainID); chainID != "" {
		if chain, ok := s.registry.Get(swap.OriginChain); ok && chain.ChainID != "" && chain.ChainID != chainID {
			return RecordDepositResult{}, domain.Newf(domain.CodeWrongNetwork,
				"The deposit was reported on chain %s but this swap expects chain %s.", chainID, chain.ChainID)
		}
	}

	if swap.SourceTxHash == "" && swap.State.Terminal() {
		return RecordDepositResult{}, domain.Newf(domain.CodeInvalidSwapState,
			"Cannot record a deposit for swap %s in terminal state %s.", swap.ID, swap.State)
	}

	updated, deposit, duplicate, err := s.store.RecordDeposit(ctx, swap.ID, swap.OriginChain, normalizedHash,
		domain.ProviderStatusKnownDepositTx,
		map[string]any{"txHash": normalizedHash, "chainKey": swap.OriginChain, "source": "wallet_report"})
	if err != nil {
		return RecordDepositResult{}, err
	}
	if duplicate || updated.Simulated {
		return RecordDepositResult{Swap: updated, Deposit: deposit, Duplicate: duplicate}, nil
	}

	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		bgCtx, cancel := context.WithTimeout(context.Background(), s.quoteDeadline)
		defer cancel()
		_, submitErr := s.provider.SubmitDepositTx(bgCtx, providers.DepositRequest{
			TxHash:         normalizedHash,
			DepositAddress: updated.DepositAddress,
			DepositMemo:    updated.DepositMemo,
		})
		s.observe(submitErr)
		if submitErr != nil {
			s.logger.Warn("provider deposit notification failed; the poller will recover",
				"swap_id", updated.ID, "error", submitErr)
		}
	}()

	return RecordDepositResult{Swap: updated, Deposit: deposit}, nil
}

// checkOwner enforces that a swap with an owner is only touched by that owner.
func checkOwner(swap domain.Swap, sessionID string) error {
	if swap.SessionID != "" && !strings.EqualFold(swap.SessionID, strings.TrimSpace(sessionID)) {
		return domain.New(domain.CodeForbidden, "You do not have access to this swap.")
	}
	return nil
}
