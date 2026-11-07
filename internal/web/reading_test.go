package web

import (
	"testing"
	"time"

	"github.com/alparslanakbas/cupproof/internal/pricing"
)

// The verdict sentences, word for word. They moved into the catalog when the
// site got other languages, and English must read exactly as it did before.
func TestReadingSentences(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	since := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	was := int64(2500)
	for _, c := range []struct {
		sum           pricing.Summary
		storeWas      *int64
		currency, own string
		want          string
	}{
		{pricing.Summary{Latest: 1800, Usual: 2000, OwnPrice: pricing.OwnNoDrop}, nil, "EUR", "SEK",
			"€18.00 is €2.00 under its usual €20.00, but the store's own price in Swedish kronor didn't drop: the difference is the exchange rate."},
		{pricing.Summary{Latest: 1800, Usual: 2000, OwnPrice: pricing.OwnUnknown}, nil, "EUR", "",
			"€18.00 is €2.00 under its usual €20.00, but we can't compare it with the store's own price, so it may be only the exchange rate."},
		{pricing.Summary{Latest: 1800, Usual: 2000, Low30: 1800, TrackedSince: since, LowestTracked: 1800}, nil, "USD", "",
			"$18.00 is $2.00 under its usual $20.00, and the lowest since Aug 1, 2026."},
		{pricing.Summary{Latest: 1800, Usual: 2000, Low30: 1800}, nil, "USD", "",
			"$18.00 is $2.00 under its usual $20.00, and the lowest price in the last 30 days."},
		{pricing.Summary{Latest: 1800, Usual: 2000, Low30: 1700}, nil, "GBP", "",
			"£18.00 is £2.00 under its usual £20.00."},
		{pricing.Summary{Latest: 1800}, nil, "USD", "", "Not enough history yet to judge."},
		{pricing.Summary{Latest: 1800, TrackedSince: since}, nil, "USD", "",
			"Not enough history yet to judge; tracked since Aug 1, 2026."},
		{pricing.Summary{Latest: 2200, Usual: 2000}, nil, "USD", "", "$22.00 is above its usual $20.00."},
		{pricing.Summary{Latest: 2000, Usual: 2000}, &was, "USD", "",
			"At its usual price of $20.00. The store calls it a sale, from $25.00."},
	} {
		if got := reading("en", c.sum, now, c.storeWas, c.currency, c.own); got != c.want {
			t.Errorf("reading = %q\n     want %q", got, c.want)
		}
	}
}

func TestCheckedAndCurrencyNames(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct{ got, want string }{
		{checked("en", now.Add(-time.Hour), now), "Checked today, 11:00 UTC"},
		{checked("en", now.Add(-24*time.Hour), now), "Checked yesterday, 12:00 UTC"},
		{checked("en", time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC), now), "Checked Nov 2"},
		{checked("en", time.Time{}, now), ""},
		{currencyName("en", "USD"), "US dollars"},
		{currencyName("en", "NOK"), "Norwegian kroner"},
		{currencyName("en", "CHF"), "CHF"},
	} {
		if c.got != c.want {
			t.Errorf("%q, want %q", c.got, c.want)
		}
	}
}
