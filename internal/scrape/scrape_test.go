package scrape

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/market"
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

// fakeCurrencies keeps one store's answer for one market in memory, with its
// age in days, the way roaster_markets does.
type fakeCurrencies struct {
	answer  string
	ageDays int32
	saves   int
}

func (f *fakeCurrencies) PresentedCurrency(_ context.Context, p db.PresentedCurrencyParams) (string, error) {
	if f.answer == "" || f.ageDays >= p.MaxAgeDays {
		return "", pgx.ErrNoRows
	}
	return f.answer, nil
}

func (f *fakeCurrencies) SavePresentedCurrency(_ context.Context, p db.SavePresentedCurrencyParams) error {
	f.answer, f.ageDays = p.Currency, 0
	f.saves++
	return nil
}

// homePage answers every request with status (and, for 403, the header
// Cloudflare's challenge carries) and counts the requests.
func homePage(t *testing.T, status int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if status == http.StatusForbidden {
			w.Header().Set("Cf-Mitigated", "challenge")
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func presentedFor(t *testing.T, srv *httptest.Server, store currencyStore) (string, error) {
	t.Helper()
	r := &Runner{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	client := &shopify.Client{HTTP: srv.Client(), UserAgent: "test"}
	return r.presented(context.Background(), store, client, "harney", srv.URL, market.UK, nil)
}

// 14 Nov 2026: Harney's home page challenged the weekly check and the whole
// store failed, its US catalog too. A challenge with an earlier answer keeps
// that answer and saves it again, so the home page is asked again in a week.
func TestChallengedCurrencyCheckKeepsTheLastAnswer(t *testing.T) {
	srv, calls := homePage(t, http.StatusForbidden)
	store := &fakeCurrencies{answer: "USD", ageDays: 8} // older than the weekly check

	cur, err := presentedFor(t, srv, store)
	if err != nil || cur != "USD" {
		t.Fatalf("got %q, %v; want the last answer USD", cur, err)
	}
	if *calls != 1 || store.saves != 1 || store.ageDays != 0 {
		t.Fatalf("calls %d, saves %d, age %d; want one request and the answer saved again", *calls, store.saves, store.ageDays)
	}

	// Saved again: the next run reads it from the store, not the home page.
	if cur, err := presentedFor(t, srv, store); err != nil || cur != "USD" || *calls != 1 {
		t.Fatalf("next run: %q, %v, calls %d; want USD without a request", cur, err, *calls)
	}
}

// Without an earlier answer there is nothing to keep: the challenge goes up
// and counts for the breaker, as before.
func TestChallengedCurrencyCheckWithoutAnAnswerStillFails(t *testing.T) {
	srv, _ := homePage(t, http.StatusForbidden)
	store := &fakeCurrencies{}

	if _, err := presentedFor(t, srv, store); !errors.Is(err, shopify.ErrChallenged) || store.saves != 0 {
		t.Fatalf("err %v, saves %d; want ErrChallenged and nothing saved", err, store.saves)
	}
}

// A 429 is not a challenge: Shopify asks us to wait out Retry-After, so the
// store stops for today even with an answer on file.
func TestRateLimitedCurrencyCheckDoesNotFallBack(t *testing.T) {
	srv, _ := homePage(t, http.StatusTooManyRequests)
	store := &fakeCurrencies{answer: "USD", ageDays: 8}

	if _, err := presentedFor(t, srv, store); !errors.Is(err, shopify.ErrRateLimited) || store.saves != 0 {
		t.Fatalf("err %v, saves %d; want ErrRateLimited and nothing saved", err, store.saves)
	}
}
