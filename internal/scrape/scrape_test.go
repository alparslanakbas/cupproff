package scrape

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alparslanakbas/cupproof/internal/shopify"
)

// Three refusals in a row stop the run; a store that answers, or fails for
// another reason, resets the count. Wrapped errors count (one() wraps them).
func TestBreakerStopsOnlyOnRefusalsInARow(t *testing.T) {
	limited := fmt.Errorf("en-us: %w", shopify.ErrRateLimited)
	challenged := fmt.Errorf("meta.json: %w", shopify.ErrChallenged)
	other := errors.New("HTTP 500")

	var b breaker
	steps := []struct {
		err  error
		stop bool
	}{
		{limited, false}, {challenged, false}, {nil, false}, // an answer resets
		{limited, false}, {other, false}, // another failure resets too
		{limited, false}, {limited, false}, {challenged, true},
	}
	for i, s := range steps {
		if got := b.record(s.err); got != s.stop {
			t.Fatalf("step %d (%v): stop = %v, want %v", i, s.err, got, s.stop)
		}
	}
}
