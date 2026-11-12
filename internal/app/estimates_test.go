package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
	"fundbar/internal/store"
)

// estimatedFund is a fund whose estimate was made from prices of the given
// day.
func estimatedFund(quoteAt time.Time) *Fund {
	return &Fund{
		Code: "007760", HasEst: true, QuoteAt: quoteAt,
		Est: model.Estimate{Pct: 0.42, Cover: 63.4},
	}
}

func TestEstimateRecordWaitsForTheEvening(t *testing.T) {
	f := estimatedFund(time.Date(2026, 11, 11, 10, 0, 0, 0, cst))
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 11, day, hour, minute, 0, 0, cst)
	}
	for _, before := range []time.Time{
		at(11, 9, 0), at(11, 16, 14), at(10, 17, 0),
	} {
		if _, ok := estimateRecord(f, before); ok {
			t.Fatalf("recorded a provisional estimate at %v", before)
		}
	}
	r, ok := estimateRecord(f, at(11, 16, 15))
	if !ok {
		t.Fatal("expected the estimate to be final at 16:15")
	}
	if r.Date != "2026-11-11" || r.Est != 0.42 || r.Cover != 63.4 || r.Actual != nil {
		t.Fatalf("record = %+v", r)
	}
	// The next morning the quotes are still yesterday's, and the estimate is
	// final because its day is over.
	if r, ok := estimateRecord(f, at(12, 9, 0)); !ok || r.Date != "2026-11-11" {
		t.Fatalf("next morning record = %+v, %v", r, ok)
	}
	// A fund without an estimate, or without prices, has nothing to record.
	if _, ok := estimateRecord(&Fund{HasEst: true}, at(12, 9, 0)); ok {
		t.Fatal("expected no record without prices")
	}
	if _, ok := estimateRecord(&Fund{QuoteAt: f.QuoteAt}, at(12, 9, 0)); ok {
		t.Fatal("expected no record without an estimate")
	}
}

func TestRecordEstimatesKeepsTheFirstKnownActual(t *testing.T) {
	a := New(t.TempDir())
	f := estimatedFund(time.Date(2026, 11, 11, 10, 0, 0, 0, cst))
	a.all = []*Fund{f}

	before := time.Date(2026, 11, 11, 16, 14, 0, 0, cst)
	if a.recordEstimates(before) {
		t.Fatal("expected no record before the evening")
	}
	after := time.Date(2026, 11, 11, 16, 15, 0, 0, cst)
	if !a.recordEstimates(after) {
		t.Fatal("expected the evening's record")
	}
	if recs := a.estimates[f.Code]; len(recs) != 1 || recs[0].Est != 0.42 {
		t.Fatalf("records = %+v", recs)
	}

	// Once the published value is known, a later refresh must not replace
	// what the app estimated before it came out.
	actual := 0.30
	a.estimates[f.Code][0].Actual = &actual
	f.Est.Pct = 9
	if a.recordEstimates(after) {
		t.Fatal("expected a known actual to be left alone")
	}
	rec := a.estimates[f.Code][0]
	if rec.Est != 0.42 || rec.Actual == nil || *rec.Actual != actual {
		t.Fatalf("record was rewritten: %+v", rec)
	}
}

func TestRecordEstimatesPrunesUnfollowedFunds(t *testing.T) {
	a := New(t.TempDir())
	a.all = []*Fund{estimatedFund(time.Date(2026, 11, 11, 10, 0, 0, 0, cst))}
	a.estimates = store.Estimates{
		"000000": {{Date: "2026-11-10", Est: 1}},
	}
	if !a.recordEstimates(time.Date(2026, 11, 12, 9, 0, 0, 0, cst)) {
		t.Fatal("expected the record to change")
	}
	if _, ok := a.estimates["000000"]; ok {
		t.Fatal("expected the unfollowed fund to be pruned")
	}
}

func TestFillActualsUsesThePublishedChange(t *testing.T) {
	a := New(t.TempDir())
	f := &Fund{Code: "007760", Navs: []model.Nav{
		{Date: "2026-10-09", Unit: 1.2356},
		// A dividend day: the unit value dipped while the fund published +1.00%.
		{Date: "2026-10-12", Unit: 1.2352, Pct: 1.00},
	}}
	a.estimates = store.Estimates{f.Code: {
		{Date: "2026-10-12", Est: 0.8},
		{Date: "2026-10-13", Est: 0.1}, // not published yet, as a QDII's can lag
	}}
	if !a.fillActuals(f) {
		t.Fatal("expected the published day to settle")
	}
	rec := a.estimates[f.Code]
	if rec[0].Actual == nil || *rec[0].Actual != 1.00 {
		t.Fatalf("actual = %v, want the published 1.00", rec[0].Actual)
	}
	if rec[1].Actual != nil {
		t.Fatal("expected a day without a value to stay open")
	}
	if a.fillActuals(f) {
		// The second pass still has one open record, but no value for it.
		t.Fatal("expected no change once the known day is settled")
	}
}

func TestSummarizeEstimateAccuracy(t *testing.T) {
	actual := func(v float64) *float64 { return &v }
	recs := []store.EstimateRecord{
		{Est: 0.20, Actual: actual(0.30)},   // off 0.10, agrees
		{Est: -0.10, Actual: actual(0.005)}, // off 0.105, too small to count
		{Est: -0.20, Actual: actual(-0.25)}, // off 0.05, agrees
		{Est: 0.05, Actual: actual(-0.10)},  // off 0.15, disagrees
		{Est: 0.40},                         // still open, not counted
	}
	acc := summarizeEstimateAccuracy(recs)
	if acc.Finished != 4 || acc.Days != 4 {
		t.Fatalf("finished = %d, days = %d, want 4 and 4", acc.Finished, acc.Days)
	}
	if want := (0.10 + 0.105 + 0.05 + 0.15) / 4; abs(acc.Mean-want) > 1e-9 {
		t.Fatalf("mean = %v, want %v", acc.Mean, want)
	}
	if acc.Aligned != 2 {
		t.Fatalf("aligned = %d, want 2", acc.Aligned)
	}
	if tip := acc.tip(); tip != "最近 4 个交易日，盘中估值与当晚公布净值平均差 0.10 个百分点，方向一致 2 天。偏差越小，盘中估值越可信。" {
		t.Fatalf("tip = %q", tip)
	}

	// Only the newest 20 finished records count.
	recs = nil
	for i := 0; i < 25; i++ {
		recs = append(recs, store.EstimateRecord{Est: 0, Actual: actual(0.2)})
	}
	if acc := summarizeEstimateAccuracy(recs); acc.Finished != 25 || acc.Days != 20 {
		t.Fatalf("finished = %d, days = %d, want 25 and 20", acc.Finished, acc.Days)
	}
}

func TestAccuracyShowsOnlyOnceThreeRecordsAreFinished(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	f.Industries = nil
	actual := func(v float64) *float64 { return &v }
	two := []store.EstimateRecord{
		{Date: "2026-11-10", Est: 0.2, Actual: actual(0.1)},
		{Date: "2026-11-11", Est: 0.2, Actual: actual(0.1)},
	}
	a.estimates = store.Estimates{f.Code: two}
	tt := ui.NewTester(a.View, 900, 600)
	tt.Frame()
	if !tt.HasText("估值偏差记录中") || tt.HasText("估值偏差 ±") {
		t.Fatalf("expected the pending state, got %v", tt.Texts())
	}

	a.estimates[f.Code] = append(a.estimates[f.Code],
		store.EstimateRecord{Date: "2026-11-12", Est: 0.2, Actual: actual(0.1)})
	tt.Frame()
	if !tt.HasText("估值偏差 ±0.10%") {
		t.Fatalf("expected the accuracy figure, got %v", tt.Texts())
	}
}
