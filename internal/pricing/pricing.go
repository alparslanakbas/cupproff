// Package pricing holds the verdict rules for one offer, the same rules the
// sister sites use (TR: PriceSummaryRefresher, DealsQueryService, TakipDibi).
// The database supplies the window figures (see OffersForCoffee); this
// package only decides, so the decisions are testable without a database.
package pricing

import "time"

// TrackingMinDays is the shortest history that lets a page say "lowest since
// <date>". We can't say "lowest ever": we know nothing before tracking began.
const TrackingMinDays = 30

// Summary is what the database knows about one offer. Prices are minor
// units; 0 means "none".
type Summary struct {
	Latest        int64     // current price
	Low30         int64     // lowest price in the window (after any 72 h gap)
	Usual         int64     // highest price seen on 7+ days in the window
	TrackedSince  time.Time // gap restart, else the first reading ever
	LowestTracked int64     // lowest price since TrackedSince
}

// Reference is the price a drop is measured against: the usual price, or the
// current price when there is no usual price yet or the price went up. So a
// new coffee, or one just raised, never shows a discount.
func (s Summary) Reference() int64 {
	return max(s.Usual, s.Latest)
}

// DropPercent is how far the current price sits below the reference,
// rounded down so 9.6% is never shown as 10%.
func (s Summary) DropPercent() int {
	ref := s.Reference()
	if ref <= 0 || s.Latest >= ref {
		return 0
	}
	return int((ref - s.Latest) * 100 / ref)
}

// LowestIn30Days: the current price is the window's lowest AND below the
// reference. The second half rules out a coffee that was simply never
// cheaper because it never changed, or one back at its usual price after a
// raise.
func (s Summary) LowestIn30Days() bool {
	return s.Low30 > 0 && s.Latest <= s.Low30 && s.Low30 < s.Reference()
}

// LowestSince returns the date tracking began when the current price is the
// lowest since then: the 30-day badge's conditions, at least TrackingMinDays
// of history, and nothing cheaper in all of it.
func (s Summary) LowestSince(now time.Time) (time.Time, bool) {
	if !s.LowestIn30Days() || s.TrackedSince.IsZero() || s.LowestTracked <= 0 {
		return time.Time{}, false
	}
	if s.TrackedSince.After(now.AddDate(0, 0, -TrackingMinDays)) || s.Latest > s.LowestTracked {
		return time.Time{}, false
	}
	return s.TrackedSince, true
}
