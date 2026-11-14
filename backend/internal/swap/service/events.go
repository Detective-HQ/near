package service

import (
	"sync"

	"universal-defi-intelligence/internal/swap/domain"
)

// EventBus provides in-process publish/subscribe for swap lifecycle updates.
type EventBus struct {
	mu   sync.RWMutex
	subs map[string][]chan domain.Swap
}

// NewEventBus allocates an EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subs: make(map[string][]chan domain.Swap),
	}
}

// Subscribe returns a channel receiving updates for swapID, and an unsubscribe func.
func (b *EventBus) Subscribe(swapID string) (chan domain.Swap, func()) {
	ch := make(chan domain.Swap, 16)
	b.mu.Lock()
	b.subs[swapID] = append(b.subs[swapID], ch)
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		chs := b.subs[swapID]
		for i, c := range chs {
			if c == ch {
				b.subs[swapID] = append(chs[:i], chs[i+1:]...)
				break
			}
		}
		if len(b.subs[swapID]) == 0 {
			delete(b.subs, swapID)
		}
	}
	return ch, unsubscribe
}

// Publish broadcasts a swap update to all active subscribers.
func (b *EventBus) Publish(swap domain.Swap) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs[swap.ID] {
		select {
		case ch <- swap:
		default:
		}
	}
}

