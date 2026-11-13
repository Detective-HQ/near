package domain

import (
	"fmt"
	"strings"
	"time"
)

// State is the provider-agnostic swap lifecycle state.
type State string

const (
	StateCreated             State = "CREATED"
	StateQuoting             State = "QUOTING"
	StateQuoteUnavailable    State = "QUOTE_UNAVAILABLE"
	StateQuoted              State = "QUOTED"
	StateAwaitingUserDeposit State = "AWAITING_USER_DEPOSIT"
	StateDepositSubmitted    State = "DEPOSIT_SUBMITTED"
	StateIncompleteDeposit   State = "INCOMPLETE_DEPOSIT"
	StateProcessing          State = "PROCESSING"
	StateSucceeded           State = "SUCCEEDED"
	StateRefunded            State = "REFUNDED"
	StateFailed              State = "FAILED"
	StateCancelled           State = "CANCELLED"
	StateExpired             State = "EXPIRED"
)

var allStates = []State{
	StateCreated,
	StateQuoting,
	StateQuoteUnavailable,
	StateQuoted,
	StateAwaitingUserDeposit,
	StateDepositSubmitted,
	StateIncompleteDeposit,
	StateProcessing,
	StateSucceeded,
	StateRefunded,
	StateFailed,
	StateCancelled,
	StateExpired,
}

var stateRanks = map[State]int{
	StateCreated:             0,
	StateQuoting:             1,
	StateQuoted:              2,
	StateAwaitingUserDeposit: 3,
	StateDepositSubmitted:    4,
	StateIncompleteDeposit:   4,
	StateProcessing:          5,
	StateQuoteUnavailable:    6,
	StateSucceeded:           6,
	StateRefunded:            6,
	StateFailed:              6,
	StateCancelled:           6,
	StateExpired:             6,
}

var terminalStates = map[State]struct{}{
	StateQuoteUnavailable: {},
	StateSucceeded:        {},
	StateRefunded:         {},
	StateFailed:           {},
	StateCancelled:        {},
	StateExpired:          {},
}

// transitions is the only allowed lifecycle graph. It deliberately omits
// shortcuts such as CREATED -> QUOTED or DEPOSIT_SUBMITTED -> SUCCEEDED.
var transitions = map[State]map[State]struct{}{
	StateCreated: stateSet(
		StateQuoting,
	),
	StateQuoting: stateSet(
		StateQuoted,
		StateQuoteUnavailable,
	),
	StateQuoted: stateSet(
		StateAwaitingUserDeposit,
		StateCancelled,
		StateExpired,
		StateFailed,
	),
	StateAwaitingUserDeposit: stateSet(
		StateDepositSubmitted,
		StateCancelled,
		StateExpired,
		StateFailed,
	),
	StateDepositSubmitted: stateSet(
		StateProcessing,
		StateIncompleteDeposit,
		StateRefunded,
		StateFailed,
	),
	StateIncompleteDeposit: stateSet(
		StateProcessing,
		StateRefunded,
		StateFailed,
	),
	StateProcessing: stateSet(
		StateSucceeded,
		StateRefunded,
		StateFailed,
	),
}

func stateSet(states ...State) map[State]struct{} {
	out := make(map[State]struct{}, len(states))
	for _, state := range states {
		out[state] = struct{}{}
	}
	return out
}

// AllStates returns a copy of the lifecycle states for tests and diagnostics.
func AllStates() []State {
	out := make([]State, len(allStates))
	copy(out, allStates)
	return out
}

// ParseState normalizes and validates a state string.
func ParseState(value string) (State, bool) {
	state := State(strings.ToUpper(strings.TrimSpace(value)))
	if _, ok := stateRanks[state]; !ok {
		return "", false
	}
	return state, true
}

// Valid reports whether the state is part of the lifecycle.
func (s State) Valid() bool {
	_, ok := stateRanks[s]
	return ok
}

// Terminal reports whether no further transition is allowed.
func (s State) Terminal() bool {
	_, ok := terminalStates[s]
	return ok
}

// Rank returns the monotonic lifecycle position.
func (s State) Rank() (int, bool) {
	rank, ok := stateRanks[s]
	return rank, ok
}

// String implements fmt.Stringer.
func (s State) String() string {
	return string(s)
}

// CanTransition validates a single edge in the lifecycle graph.
func CanTransition(from, to State) error {
	if !from.Valid() {
		return Newf(CodeInvalidSwapState, "Unknown source state %q.", from)
	}
	if !to.Valid() {
		return Newf(CodeInvalidSwapState, "Unknown destination state %q.", to)
	}
	if from == to {
		return Newf(CodeInvalidSwapState, "Swap is already in state %s.", from)
	}
	if from.Terminal() {
		return Newf(CodeTerminalState, "Swap is already in terminal state %s.", from)
	}
	if _, ok := transitions[from][to]; !ok {
		return Newf(CodeInvalidSwapState, "Cannot transition from %s to %s.", from, to)
	}
	return nil
}

// TransitionInput describes one explicit lifecycle transition.
type TransitionInput struct {
	To                      State
	ProviderStatus          ProviderStatus
	Payload                 map[string]any
	VerifiedProviderOutcome bool
	Now                     time.Time
}

// Apply validates and applies one transition. It returns the updated swap and
// the audit event that must be written in the same database transaction.
//
// SUCCEEDED and REFUNDED are settlement states: they require
// VerifiedProviderOutcome=true. A client callback or a deposit hash alone can
// never finalize settlement.
func Apply(swap Swap, in TransitionInput) (Swap, Event, error) {
	if err := CanTransition(swap.State, in.To); err != nil {
		return swap, Event{}, err
	}
	if (in.To == StateSucceeded || in.To == StateRefunded) && !in.VerifiedProviderOutcome {
		return swap, Event{}, New(CodeInvalidSwapState, "Settlement requires a verified provider outcome.")
	}

	now := normalizeNow(in.Now)

	oldState := swap.State
	swap.State = in.To
	swap.ProviderStatus = in.ProviderStatus
	swap.RealSettlement = in.To == StateSucceeded && in.VerifiedProviderOutcome
	swap.UpdatedAt = now
	if swap.CreatedAt.IsZero() {
		swap.CreatedAt = now
	}

	event := Event{
		SwapID:         swap.ID,
		OldState:       oldState,
		NewState:       in.To,
		ProviderStatus: in.ProviderStatus,
		Payload:        clonePayload(in.Payload),
		CreatedAt:      now,
	}
	return swap, event, nil
}

// ProviderUpdateInput describes a provider status observation. It is separate
// from TransitionInput because a provider may report the same lifecycle state
// repeatedly (for example INCOMPLETE_DEPOSIT -> INCOMPLETE_DEPOSIT).
type ProviderUpdateInput struct {
	Status                  ProviderStatus
	HasSourceTransaction    bool
	Payload                 map[string]any
	VerifiedProviderOutcome bool
	Now                     time.Time
}

// ApplyProviderUpdate maps a provider status to a lifecycle state and records
// the observation.
//
// The boolean result reports whether the observation was recorded. A stale
// provider response that would move the swap backward is ignored and returns
// false with no error. A same-state observation is recorded as an audit event
// without changing the lifecycle state. Forward movements use Apply and keep
// all of its settlement safeguards.
func ApplyProviderUpdate(swap Swap, in ProviderUpdateInput) (Swap, Event, bool, error) {
	if !swap.State.Valid() {
		return swap, Event{}, false, Newf(CodeInvalidSwapState, "Unknown current state %q.", swap.State)
	}

	mapped, err := MapProviderStatus(in.Status, in.HasSourceTransaction)
	if err != nil {
		return swap, Event{}, false, err
	}

	if mapped == swap.State {
		now := normalizeNow(in.Now)
		swap.ProviderStatus = in.Status.Normalize()
		swap.UpdatedAt = now
		if swap.CreatedAt.IsZero() {
			swap.CreatedAt = now
		}
		event := Event{
			SwapID:         swap.ID,
			OldState:       swap.State,
			NewState:       swap.State,
			ProviderStatus: swap.ProviderStatus,
			Payload:        clonePayload(in.Payload),
			CreatedAt:      now,
		}
		return swap, event, true, nil
	}

	if err := CanTransition(swap.State, mapped); err != nil {
		currentRank, _ := swap.State.Rank()
		mappedRank, _ := mapped.Rank()
		if mappedRank <= currentRank {
			// Provider responses can arrive out of order or repeat after a
			// later state was observed. Never regress or rewrite terminal state.
			return swap, Event{}, false, nil
		}
		return swap, Event{}, false, err
	}

	updated, event, err := Apply(swap, TransitionInput{
		To:                      mapped,
		ProviderStatus:          in.Status,
		Payload:                 in.Payload,
		VerifiedProviderOutcome: in.VerifiedProviderOutcome,
		Now:                     in.Now,
	})
	if err != nil {
		return swap, Event{}, false, err
	}
	return updated, event, true, nil
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC()
}

// NewSwap creates a swap in CREATED with consistent timestamps.
func NewSwap(id, quoteID string, now time.Time) Swap {
	now = normalizeNow(now)
	return Swap{
		ID:        id,
		QuoteID:   quoteID,
		State:     StateCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// InitialEvent builds the initial audit entry. OldState is intentionally empty
// because there was no previous state.
func InitialEvent(swap Swap) Event {
	state := swap.State
	if state == "" {
		state = StateCreated
	}
	return Event{
		SwapID:         swap.ID,
		OldState:       "",
		NewState:       state,
		ProviderStatus: swap.ProviderStatus,
		CreatedAt:      swap.CreatedAt,
	}
}

func clonePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}

// String implements fmt.Stringer.
func (s State) GoString() string {
	return fmt.Sprintf("domain.State(%q)", string(s))
}
