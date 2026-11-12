package store

import (
	"testing"
	"time"
)

func TestEstimatesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// A missing file is an empty record, not an error.
	if got := LoadEstimates(dir); len(got) != 0 {
		t.Fatalf("missing file loaded %v", got)
	}

	zero := 0.0
	want := Estimates{
		"007760": {
			{Date: "2026-11-10", Est: 0.42, Cover: 63.4},
			{Date: "2026-11-11", Est: -0.31, Cover: 63.1, Actual: &zero},
		},
	}
	if err := SaveEstimates(dir, want); err != nil {
		t.Fatal(err)
	}
	got := LoadEstimates(dir)
	if len(got) != 1 || len(got["007760"]) != 2 {
		t.Fatalf("loaded %v", got)
	}
	first := got["007760"][0]
	if first.Date != "2026-11-10" || first.Est != 0.42 || first.Actual != nil {
		t.Fatalf("open record = %+v", first)
	}
	// A published 0.00% stays a finished record rather than reading blank.
	second := got["007760"][1]
	if second.Actual == nil || *second.Actual != 0 {
		t.Fatalf("a published zero lost its pointer: %+v", second)
	}
}

func TestAddRecordKeepsOrderAndCap(t *testing.T) {
	e := Estimates{}
	// The record arrives out of order and still lands in date order.
	e.AddRecord("007760", EstimateRecord{Date: "2026-01-03", Est: 3})
	e.AddRecord("007760", EstimateRecord{Date: "2026-01-01", Est: 1})
	e.AddRecord("007760", EstimateRecord{Date: "2026-01-02", Est: 2})
	recs := e["007760"]
	if len(recs) != 3 || recs[0].Est != 1 || recs[1].Est != 2 || recs[2].Est != 3 {
		t.Fatalf("order = %+v", recs)
	}

	// A second record for the same day updates the estimate but keeps its
	// place, and changes nothing when it says the same thing.
	if !e.AddRecord("007760", EstimateRecord{Date: "2026-01-02", Est: 2.5}) {
		t.Fatal("expected the new estimate to change the record")
	}
	if e.AddRecord("007760", EstimateRecord{Date: "2026-01-02", Est: 2.5}) {
		t.Fatal("expected a repeated record to change nothing")
	}
	if recs := e["007760"]; recs[1].Est != 2.5 {
		t.Fatalf("record = %+v", recs[1])
	}

	// Only the newest 60 days are kept.
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < maxEstimateRecords+3; i++ {
		e.AddRecord("007760", EstimateRecord{
			Date: start.AddDate(0, 0, i).Format("2006-01-02"), Est: float64(i),
		})
	}
	recs = e["007760"]
	if len(recs) != maxEstimateRecords {
		t.Fatalf("kept %d records, want %d", len(recs), maxEstimateRecords)
	}
	newest := start.AddDate(0, 0, maxEstimateRecords+2).Format("2006-01-02")
	if recs[len(recs)-1].Date != newest {
		t.Fatalf("newest record = %+v, want %s", recs[len(recs)-1], newest)
	}
}

func TestAddRecordKeepsAKnownActual(t *testing.T) {
	zero := 0.0
	e := Estimates{"007760": {{Date: "2026-11-11", Est: 0.5, Actual: &zero}}}
	if e.AddRecord("007760", EstimateRecord{Date: "2026-11-11", Est: 0.9}) {
		t.Fatal("expected a known actual to be left alone")
	}
	rec := e["007760"][0]
	if rec.Est != 0.5 || rec.Actual == nil || *rec.Actual != 0 {
		t.Fatalf("record was overwritten: %+v", rec)
	}
}

func TestPruneDropsFundsNoLongerFollowed(t *testing.T) {
	e := Estimates{
		"007760": {{Date: "2026-11-11", Est: 1}},
		"008270": {{Date: "2026-11-11", Est: 2}},
	}
	if !e.Prune(map[string]bool{"007760": true}) {
		t.Fatal("expected a prune")
	}
	if _, ok := e["008270"]; ok {
		t.Fatal("expected the unfollowed fund to go")
	}
	if len(e["007760"]) != 1 {
		t.Fatal("expected the followed fund to stay")
	}
	if e.Prune(map[string]bool{"007760": true}) {
		t.Fatal("expected a second prune to change nothing")
	}
}
