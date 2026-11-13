package memstore

import (
	"context"
	"math/big"
	"testing"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

func TestQuoteEvictionKeepsReferencedQuotesAndBoundsMemory(t *testing.T) {
	store := New()
	ctx := context.Background()
	save := func(id string) {
		if _, err := store.SaveQuote(ctx, domain.Quote{ID: id, AmountInAtomic: big.NewInt(1), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	save("q-referenced")
	if _, _, err := store.CreateSwap(ctx, domain.Swap{ID: "s1", QuoteID: "q-referenced", State: domain.StateAwaitingUserDeposit}, "key-1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxQuotes+500; i++ {
		save("q-" + string(rune('a'+i%26)) + "-" + time.Now().Format("150405.000000000") + "-" + big.NewInt(int64(i)).String())
	}
	if len(store.quotes) > maxQuotes {
		t.Fatalf("quotes = %d, exceeds cap %d", len(store.quotes), maxQuotes)
	}
	if _, err := store.GetQuote(ctx, "q-referenced"); err != nil {
		t.Fatalf("referenced quote was evicted: %v", err)
	}
}
