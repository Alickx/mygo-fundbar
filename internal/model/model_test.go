package model

import (
	"math"
	"testing"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestEstimateFund(t *testing.T) {
	holdings := []Holding{
		{Symbol: "sh600519", Weight: 10},
		{Symbol: "sz000858", Weight: 30},
		{Symbol: "sh601318", Weight: 5}, // quoted now
		{Symbol: "hk00700", Weight: 20}, // no quote
	}
	quotes := map[string]Quote{
		"sh600519": {Pct: 2},
		"sz000858": {Pct: -1},
		"sh601318": {Pct: -5},
	}
	est, ok := EstimateFund(holdings, quotes)
	if !ok {
		t.Fatal("expected an estimate")
	}
	// (10*2 + 30*-1 + 5*-5) / 45 = -35/45
	want := -35.0 / 45.0
	if diff := est.Pct - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("pct = %v, want %v", est.Pct, want)
	}
	if est.Cover != 45 {
		t.Fatalf("cover = %v, want 45", est.Cover)
	}
	if est.Stocks != 3 {
		t.Fatalf("stocks = %v, want 3", est.Stocks)
	}
}

func TestHoldingMarket(t *testing.T) {
	cases := map[string]string{
		"sh600519": "",
		"sz000858": "",
		"hk00700":  "hk",
		"usAAPL":   "us",
		"":         "", // a foreign stock the data does not name
	}
	for symbol, want := range cases {
		if got := (Holding{Symbol: symbol}).Market(); got != want {
			t.Errorf("Market(%q) = %q, want %q", symbol, got, want)
		}
	}
}

// The holdings' change is scaled by the share of the fund that is stocks: the
// bonds and cash beside them do not move.
func TestScaleEstimate(t *testing.T) {
	e := Estimate{Pct: 2, Cover: 30, Stocks: 10}
	if got := ScaleEstimate(e, 50); !closeTo(got.Pct, 1) {
		t.Fatalf("half stocks = %v, want half the change", got.Pct)
	}
	if got := ScaleEstimate(e, 100); !closeTo(got.Pct, 2) {
		t.Fatalf("all stocks = %v, want the change as it was", got.Pct)
	}
	// An unknown or impossible stock position leaves the estimate alone.
	for _, stock := range []float64{0, -1, 101} {
		if got := ScaleEstimate(e, stock); got.Pct != 2 {
			t.Fatalf("stock position %v = %v, want the estimate unchanged", stock, got.Pct)
		}
	}
	if got := ScaleEstimate(e, 50); got.Cover != 30 || got.Stocks != 10 {
		t.Fatalf("coverage changed: %+v", got)
	}
}

func TestEstimateFundNeedsAQuote(t *testing.T) {
	if _, ok := EstimateFund([]Holding{{Symbol: "sh600519", Weight: 10}}, nil); ok {
		t.Fatal("expected no estimate without quotes")
	}
	if _, ok := EstimateFund(nil, map[string]Quote{"sh600519": {Pct: 1}}); ok {
		t.Fatal("expected no estimate without holdings")
	}
	// A holding of no weight says nothing about the fund.
	if _, ok := EstimateFund([]Holding{{Symbol: "sh600519"}}, map[string]Quote{
		"sh600519": {Pct: 1},
	}); ok {
		t.Fatal("expected no estimate from a holding of no weight")
	}
}

// A day's true return is the unit values' ratio, without the drift of the
// two-decimal published change, until a dividend or split makes the two
// disagree.
func TestDayReturn(t *testing.T) {
	ordinary := Nav{Date: "2026-11-11", Unit: 1.2345}
	next := Nav{Date: "2026-11-12", Unit: 1.2467, Pct: 0.99}
	if DistributionDay(ordinary, next) {
		t.Fatal("a rounded unit value is not a distribution")
	}
	ratio := (1.2467/1.2345 - 1) * 100
	got := DayReturn(ordinary, next)
	if !closeTo(got, ratio) {
		t.Fatalf("ordinary return = %v, want the unit ratio %v", got, ratio)
	}
	// The published two-decimal figure carries a little drift.
	if got == next.Pct {
		t.Fatalf("ordinary return = %v, want the unit values' own ratio", got)
	}

	// Fund 007760 on 2026-10-12: the unit value moved -0.03% while the fund
	// paid a dividend and published +1.00%.
	before := Nav{Date: "2026-10-09", Unit: 1.2356}
	after := Nav{Date: "2026-10-12", Unit: 1.2352, Pct: 1.00}
	if !DistributionDay(before, after) {
		t.Fatal("expected a distribution day")
	}
	if got := DayReturn(before, after); got != 1.00 {
		t.Fatalf("distribution return = %v, want the published 1.00", got)
	}

	// Some history has no published change at all. A dividend then shows only
	// in the accumulated value: the unit fell by the payout and no further.
	paidBefore := Nav{Date: "2026-10-13", Unit: 1.2000, Acc: 2.0000}
	paidAfter := Nav{Date: "2026-10-14", Unit: 1.1000, Acc: 1.9500}
	if !DistributionDay(paidBefore, paidAfter) {
		t.Fatal("expected the accumulated value to show the dividend")
	}
	if got := DayReturn(paidBefore, paidAfter); !closeTo(got, (1.15/1.2-1)*100) {
		t.Fatalf("return on a dividend day without a published change = %v", got)
	}
	// Without the accumulated value there is nothing to tell a dividend from
	// a fall, and the unit ratio is all there is.
	plainAfter := Nav{Date: "2026-10-14", Unit: 1.1000}
	if DistributionDay(paidBefore, plainAfter) {
		t.Fatal("a fall with no accumulated value is not a distribution")
	}

	// Old history gives no published change for some days: the unit ratio
	// is all there is.
	old := Nav{Date: "2019-09-06", Unit: 1.0000}
	moved := Nav{Date: "2019-09-09", Unit: 1.0100}
	if DistributionDay(old, moved) {
		t.Fatal("a day without a published change is not a distribution")
	}
	if got := DayReturn(old, moved); !closeTo(got, 1.00) {
		t.Fatalf("return without a published change = %v, want 1.00", got)
	}
}

func TestPeriodChangeCompoundsDistributions(t *testing.T) {
	navs := []Nav{
		{Date: "2026-10-09", Unit: 1.2356},
		{Date: "2026-10-12", Unit: 1.2352, Pct: 1.00},
	}
	if got := PeriodChange(navs); !closeTo(got, 1.00) {
		t.Fatalf("period change = %v, want the published 1.00", got)
	}
	if got := PeriodChange(navs[:1]); got != 0 {
		t.Fatalf("a single day's period change = %v, want 0", got)
	}
}

// Without a distribution the compounding telescopes back into the unit
// values' own ratio, so ordinary windows keep the numbers they had.
func TestPeriodChangeTelescopes(t *testing.T) {
	navs := []Nav{
		{Date: "2026-05-12", Unit: 1.0000},
		{Date: "2026-05-13", Unit: 1.0100},
		{Date: "2026-05-14", Unit: 1.0050},
		{Date: "2026-05-15", Unit: 1.0200},
	}
	if HasDistribution(navs) {
		t.Fatal("expected no distribution day")
	}
	want := (1.0200/1.0000 - 1) * 100
	if got := PeriodChange(navs); !closeTo(got, want) {
		t.Fatalf("period change = %v, want the unit ratio %v", got, want)
	}
}

func TestMaxDrawdownUsesTheAdjustedSeries(t *testing.T) {
	// The middle day paid a dividend while the unit value moved down: on the
	// true series it is a gain, so the fall below starts at its value.
	navs := []Nav{
		{Date: "2026-10-09", Unit: 1.2356},
		{Date: "2026-10-12", Unit: 1.2352, Pct: 1.00},
		{Date: "2026-10-13", Unit: 1.2000, Pct: -2.85},
	}
	got, peak, low := MaxDrawdown(navs)
	if peak != "2026-10-12" || low != "2026-10-13" {
		t.Fatalf("drawdown runs %s -> %s, want 2026-10-12 -> 2026-10-13", peak, low)
	}
	want := -(1 - 1.2000/1.2352) * 100
	if !closeTo(got, want) {
		t.Fatalf("drawdown = %v, want %v", got, want)
	}
	if got > 0 {
		t.Fatalf("drawdown = %v, want a negative percent", got)
	}
}

func TestRiskOnEmptyAndSingleWindows(t *testing.T) {
	for _, navs := range [][]Nav{nil, {{Date: "2026-11-11", Unit: 1.2}}} {
		if dd, _, _ := MaxDrawdown(navs); dd != 0 {
			t.Fatalf("drawdown = %v, want 0", dd)
		}
		if vol, n := AnnualizedVolatility(navs); vol != 0 || n > 0 {
			t.Fatalf("volatility = %v over %d returns, want nothing", vol, n)
		}
		if ann, days := AnnualizedReturn(navs); ann != 0 || days != 0 {
			t.Fatalf("annualized = %v over %d days, want nothing", ann, days)
		}
	}
}

func TestAnnualizedVolatilityIsTheSampleDeviation(t *testing.T) {
	// Returns of +1% and -1% around zero: the sample deviation is
	// √(2/(2-1)) = √2 percentage points.
	navs := []Nav{
		{Date: "2026-11-09", Unit: 1},
		{Date: "2026-11-10", Unit: 1.01},
		{Date: "2026-11-11", Unit: 1.01 * 0.99},
	}
	vol, n := AnnualizedVolatility(navs)
	if n != 2 {
		t.Fatalf("returns = %d, want 2", n)
	}
	want := math.Sqrt2 * math.Sqrt(250)
	if !closeTo(vol, want) {
		t.Fatalf("volatility = %v, want %v", vol, want)
	}
}

func TestAnnualizedReturnNeedsASpan(t *testing.T) {
	// A year to the day: the total change is already the yearly rate.
	navs := []Nav{
		{Date: "2025-11-11", Unit: 1},
		{Date: "2026-11-11", Unit: 1.10, Pct: 10},
	}
	got, days := AnnualizedReturn(navs)
	if days != 365 {
		t.Fatalf("days = %d, want 365", days)
	}
	if !closeTo(got, 10) {
		t.Fatalf("annualized = %v, want 10", got)
	}
	// A shorter window still says how long it is, so the caller can gate it.
	half := []Nav{
		{Date: "2026-05-11", Unit: 1},
		{Date: "2026-11-11", Unit: 1.05, Pct: 5},
	}
	if _, days := AnnualizedReturn(half); days != 184 {
		t.Fatalf("days = %d, want 184", days)
	}
}

func TestAdjusted(t *testing.T) {
	// An ordinary slice is its own back-adjusted series.
	ordinary := []Nav{
		{Date: "2026-05-12", Unit: 1.0000},
		{Date: "2026-05-13", Unit: 1.0200, Pct: 2.00},
		{Date: "2026-05-14", Unit: 1.0302, Pct: 1.00},
	}
	adj := Adjusted(ordinary)
	want := []float64{1.0000, 1.0200, 1.0302}
	for i := range want {
		if !closeTo(adj[i], want[i]) {
			t.Fatalf("adjusted[%d] = %v, want %v", i, adj[i], want[i])
		}
	}

	// A distribution is taken out of the earlier values, not the last one.
	distribution := []Nav{
		{Date: "2026-10-09", Unit: 1.2356},
		{Date: "2026-10-12", Unit: 1.2352, Pct: 1.00},
	}
	adj = Adjusted(distribution)
	if !closeTo(adj[1], 1.2352) || !closeTo(adj[0], 1.2352/1.01) {
		t.Fatalf("adjusted = %v, want the last value and the one before it divided by 1.01", adj)
	}
	if !HasDistribution(distribution) {
		t.Fatal("expected the distribution to be found")
	}
}
