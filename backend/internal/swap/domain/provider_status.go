package domain

import "strings"

// ProviderStatus is the raw status string returned by a swap provider.
type ProviderStatus string

const (
	ProviderStatusPendingDeposit ProviderStatus = "PENDING_DEPOSIT"
	ProviderStatusKnownDepositTx ProviderStatus = "KNOWN_DEPOSIT_TX"
	ProviderStatusIncomplete     ProviderStatus = "INCOMPLETE_DEPOSIT"
	ProviderStatusProcessing     ProviderStatus = "PROCESSING"
	ProviderStatusSuccess        ProviderStatus = "SUCCESS"
	ProviderStatusRefunded       ProviderStatus = "REFUNDED"
	ProviderStatusFailed         ProviderStatus = "FAILED"
)

var providerStatuses = map[ProviderStatus]struct{}{
	ProviderStatusPendingDeposit: {},
	ProviderStatusKnownDepositTx: {},
	ProviderStatusIncomplete:     {},
	ProviderStatusProcessing:     {},
	ProviderStatusSuccess:        {},
	ProviderStatusRefunded:       {},
	ProviderStatusFailed:         {},
}

var terminalProviderStatuses = map[ProviderStatus]struct{}{
	ProviderStatusSuccess:  {},
	ProviderStatusRefunded: {},
	ProviderStatusFailed:   {},
}

// Normalize trims surrounding whitespace from a provider status.
func (s ProviderStatus) Normalize() ProviderStatus {
	return ProviderStatus(strings.TrimSpace(string(s)))
}

// Valid reports whether the status is one the backend understands.
func (s ProviderStatus) Valid() bool {
	_, ok := providerStatuses[s.Normalize()]
	return ok
}

// Terminal reports whether the provider considers the swap finished.
func (s ProviderStatus) Terminal() bool {
	_, ok := terminalProviderStatuses[s.Normalize()]
	return ok
}

// MapProviderStatus maps a raw provider status to an internal lifecycle state.
//
// PENDING_DEPOSIT maps to AWAITING_USER_DEPOSIT unless the backend already has
// a source transaction hash, in which case the swap is DEPOSIT_SUBMITTED.
// Unknown statuses return a typed error and must never be treated as success.
func MapProviderStatus(status ProviderStatus, hasSourceTransaction bool) (State, error) {
	switch status.Normalize() {
	case ProviderStatusPendingDeposit:
		if hasSourceTransaction {
			return StateDepositSubmitted, nil
		}
		return StateAwaitingUserDeposit, nil
	case ProviderStatusKnownDepositTx:
		return StateDepositSubmitted, nil
	case ProviderStatusIncomplete:
		return StateIncompleteDeposit, nil
	case ProviderStatusProcessing:
		return StateProcessing, nil
	case ProviderStatusSuccess:
		return StateSucceeded, nil
	case ProviderStatusRefunded:
		return StateRefunded, nil
	case ProviderStatusFailed:
		return StateFailed, nil
	default:
		return "", Newf(CodeUnknownProviderStatus, "Unrecognized provider status %q.", status.Normalize())
	}
}

// MapProviderStatusWithSource is a convenience wrapper for swaps that already
// recorded a source transaction.
func MapProviderStatusWithSource(status ProviderStatus, sourceTxHash string) (State, error) {
	return MapProviderStatus(status, strings.TrimSpace(sourceTxHash) != "")
}
