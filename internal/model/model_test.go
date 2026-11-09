package model

import "testing"

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
	if c := est.Confidence(); c < 0.7 || c > 0.8 {
		t.Fatalf("confidence = %v, want 0.75", c)
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

func TestConfidenceIsCapped(t *testing.T) {
	if c := (Estimate{Cover: 200}).Confidence(); c != 1 {
		t.Fatalf("confidence = %v, want 1", c)
	}
	if c := (Estimate{}).Confidence(); c != 0 {
		t.Fatalf("confidence = %v, want 0", c)
	}
}
