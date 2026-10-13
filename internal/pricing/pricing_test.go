package pricing

import (
	"testing"
	"time"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func TestNoUsualPriceMeansNoDiscount(t *testing.T) {
	s := Summary{Latest: 1800, Low30: 1800, Usual: 0}
	if s.DropPercent() != 0 || s.LowestIn30Days() {
		t.Fatalf("%+v: a coffee without 7 days of one price must show no drop and no badge", s)
	}
}

func TestRaisedPriceIsNotADrop(t *testing.T) {
	s := Summary{Latest: 2200, Low30: 2000, Usual: 2000}
	if s.Reference() != 2200 || s.DropPercent() != 0 || s.LowestIn30Days() {
		t.Fatalf("%+v: after a raise the reference is the new price", s)
	}
}

func TestRealDropAndLowestIn30Days(t *testing.T) {
	s := Summary{Latest: 1810, Low30: 1810, Usual: 2000}
	if got := s.DropPercent(); got != 9 {
		t.Errorf("DropPercent = %d, want 9 (9.5%% rounds down)", got)
	}
	if !s.LowestIn30Days() {
		t.Error("the window's lowest price below the usual price is the 30-day low")
	}
	// Cheaper earlier in the window: a drop, but not the 30-day low.
	if (Summary{Latest: 1900, Low30: 1810, Usual: 2000}).LowestIn30Days() {
		t.Error("a price above the window's low is not the 30-day low")
	}
}

func TestLowestSinceNeedsThirtyDaysAndTheLowestOfAllHistory(t *testing.T) {
	base := Summary{Latest: 1800, Low30: 1800, Usual: 2000, LowestTracked: 1800}

	long := base
	long.TrackedSince = now.AddDate(0, 0, -45)
	if since, ok := long.LowestSince(now); !ok || !since.Equal(long.TrackedSince) {
		t.Errorf("45 days tracked, lowest of all: want the badge since %v, got %v %v", long.TrackedSince, since, ok)
	}

	short := base
	short.TrackedSince = now.AddDate(0, 0, -20)
	if _, ok := short.LowestSince(now); ok {
		t.Error("20 days of history is too short to say 'lowest since'")
	}

	cheaperBefore := long
	cheaperBefore.LowestTracked = 1700
	if _, ok := cheaperBefore.LowestSince(now); ok {
		t.Error("it was cheaper earlier in the tracked history")
	}

	noBadge := long
	noBadge.Usual = 0 // the 30-day badge fails, so this tier fails too
	if _, ok := noBadge.LowestSince(now); ok {
		t.Error("the upper tier needs the 30-day badge's conditions")
	}
}
