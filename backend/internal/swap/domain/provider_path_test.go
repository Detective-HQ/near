package domain

import "testing"

func TestProviderPathWalksSkippedStates(t *testing.T) {
	cases := []struct {
		name      string
		from      State
		status    ProviderStatus
		hasSource bool
		want      []State
	}{
		{"awaiting to success", StateAwaitingUserDeposit, ProviderStatusSuccess, true,
			[]State{StateDepositSubmitted, StateProcessing, StateSucceeded}},
		{"submitted to success", StateDepositSubmitted, ProviderStatusSuccess, true,
			[]State{StateProcessing, StateSucceeded}},
		{"awaiting to processing", StateAwaitingUserDeposit, ProviderStatusProcessing, false,
			[]State{StateDepositSubmitted, StateProcessing}},
		{"awaiting to refunded", StateAwaitingUserDeposit, ProviderStatusRefunded, false,
			[]State{StateDepositSubmitted, StateRefunded}},
		{"direct edge", StateProcessing, ProviderStatusSuccess, true,
			[]State{StateSucceeded}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hops, ok := ProviderPath(tc.from, tc.status, tc.hasSource)
			if !ok {
				t.Fatalf("no path")
			}
			if len(hops) != len(tc.want) {
				t.Fatalf("hops = %+v, want %v", hops, tc.want)
			}
			state := tc.from
			for i, hop := range hops {
				if hop.To != tc.want[i] {
					t.Fatalf("hop %d = %s, want %s", i, hop.To, tc.want[i])
				}
				if err := CanTransition(state, hop.To); err != nil {
					t.Fatalf("hop %d illegal: %v", i, err)
				}
				state = hop.To
			}
			if last := hops[len(hops)-1]; last.ProviderStatus != tc.status {
				t.Fatalf("final status = %s, want %s", last.ProviderStatus, tc.status)
			}
		})
	}
}

func TestProviderPathRejectsRegressionAndTerminal(t *testing.T) {
	if _, ok := ProviderPath(StateProcessing, ProviderStatusPendingDeposit, true); ok {
		t.Fatal("regression must not produce a path")
	}
	if _, ok := ProviderPath(StateSucceeded, ProviderStatusRefunded, true); ok {
		t.Fatal("terminal swaps must not move")
	}
	if _, ok := ProviderPath(StateProcessing, ProviderStatus("WAT"), true); ok {
		t.Fatal("unknown status must not produce a path")
	}
}
