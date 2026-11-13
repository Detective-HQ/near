package domain

// ProviderHop is one audited step on the way to a provider-observed state.
type ProviderHop struct {
	To             State
	ProviderStatus ProviderStatus
}

// ProviderPath returns the shortest chain of legal transitions from the
// current state to a state the provider reports.
//
// A provider can legitimately skip states that the backend never observed:
// 1Click can settle a swap between two polls, so the backend may see
// AWAITING_USER_DEPOSIT followed directly by SUCCESS. The lifecycle graph does
// not allow skips, so instead of rejecting the observation (and leaving the
// swap stuck forever) the poller walks every intermediate state and writes one
// audit event per hop. The intermediate hops carry the provider status that
// would have produced them; the final hop carries the real observed status.
//
// The path never passes through a user-driven terminal state (CANCELLED,
// EXPIRED) and returns ok=false when no forward path exists.
func ProviderPath(from State, observed ProviderStatus, hasSourceTransaction bool) ([]ProviderHop, bool) {
	target, err := MapProviderStatus(observed, hasSourceTransaction)
	if err != nil || target == from || from.Terminal() {
		return nil, false
	}

	type node struct {
		state State
		prev  int
	}
	queue := []node{{state: from, prev: -1}}
	visited := map[State]bool{from: true}
	found := -1
	for index := 0; index < len(queue) && found < 0; index++ {
		current := queue[index].state
		// Iterate in the canonical state order so the chosen path is
		// deterministic when two shortest paths exist.
		for _, next := range allStates {
			if _, allowed := transitions[current][next]; !allowed || visited[next] {
				continue
			}
			if next != target && next.Terminal() {
				continue
			}
			visited[next] = true
			queue = append(queue, node{state: next, prev: index})
			if next == target {
				found = len(queue) - 1
				break
			}
		}
	}
	if found < 0 {
		return nil, false
	}

	var reversed []State
	for index := found; queue[index].prev >= 0; index = queue[index].prev {
		reversed = append(reversed, queue[index].state)
	}
	hops := make([]ProviderHop, 0, len(reversed))
	for index := len(reversed) - 1; index >= 0; index-- {
		state := reversed[index]
		status := observed.Normalize()
		if state != target {
			status = syntheticProviderStatus(state)
		}
		hops = append(hops, ProviderHop{To: state, ProviderStatus: status})
	}
	return hops, true
}

func syntheticProviderStatus(state State) ProviderStatus {
	switch state {
	case StateAwaitingUserDeposit:
		return ProviderStatusPendingDeposit
	case StateDepositSubmitted:
		return ProviderStatusKnownDepositTx
	case StateIncompleteDeposit:
		return ProviderStatusIncomplete
	case StateProcessing:
		return ProviderStatusProcessing
	default:
		return ""
	}
}
