package app

import (
	"testing"

	"fundbar/internal/model"
)

// insightFund is a held fund with one value, so that its weight is
// shares × unit over the portfolio. Navs are enough to make EstNav the
// published value.
func insightFund(code, name string, shares, unit float64, holdings []model.Holding, industries []model.Industry) *Fund {
	return &Fund{
		Code: code, Name: name, Shares: shares,
		Groups:     []string{holdingGroup},
		Navs:       []model.Nav{{Date: "2026-11-11", Unit: unit}},
		Holdings:   holdings,
		Industries: industries,
	}
}

func near(a, b float64) bool { return abs(a-b) < 1e-9 }

func TestPortfolioWeightsUseTheValueNow(t *testing.T) {
	funds := []*Fund{
		insightFund("000001", "基金A", 6000, 1, nil, nil), // 6000
		insightFund("000002", "基金B", 3000, 1, nil, nil), // 3000
		insightFund("000003", "基金C", 1000, 1, nil, nil), // 1000
	}
	in := portfolioInsight(funds)
	if in.Count != 3 || !near(in.Total, 10000) {
		t.Fatalf("count = %d, total = %v", in.Count, in.Total)
	}
	if len(in.Funds) != 3 {
		t.Fatalf("funds = %+v", in.Funds)
	}
	want := []struct {
		code string
		w    float64
	}{{"000001", 60}, {"000002", 30}, {"000003", 10}}
	var sum float64
	for i, w := range want {
		if in.Funds[i].Code != w.code || !near(in.Funds[i].Weight, w.w) {
			t.Fatalf("fund %d = %+v, want %s at %v%%", i, in.Funds[i], w.code, w.w)
		}
		sum += in.Funds[i].Weight
	}
	if !near(sum, 100) {
		t.Fatalf("weights sum to %v, want 100", sum)
	}
}

func TestPortfolioFundTiesKeepTheCodeOrder(t *testing.T) {
	funds := []*Fund{
		insightFund("000002", "基金B", 1000, 1, nil, nil),
		insightFund("000001", "基金A", 1000, 1, nil, nil),
	}
	in := portfolioInsight(funds)
	if in.Funds[0].Code != "000001" || in.Funds[1].Code != "000002" {
		t.Fatalf("tied funds = %+v, want them by code", in.Funds)
	}
}

func TestPortfolioAddsIndustriesAndStocksByFundWeight(t *testing.T) {
	a := insightFund("000001", "基金A", 6000, 1, []model.Holding{
		{Symbol: "sh600001", Name: "甲", Weight: 10},
		{Symbol: "sh600002", Name: "乙", Weight: 5},
	}, []model.Industry{
		{Name: "制造业", Weight: 50},
		{Name: "金融业", Weight: 20},
	})
	b := insightFund("000002", "基金B", 4000, 1, []model.Holding{
		{Symbol: "sh600001", Name: "甲", Weight: 8},
		{Symbol: "sh600003", Name: "丙", Weight: 7},
	}, []model.Industry{
		{Name: "制造业", Weight: 10},
		{Name: "通讯", Weight: 30},
	})
	a.Quotes = map[string]model.Quote{
		"sh600001": {Symbol: "sh600001", Pct: 1.2},
	}
	b.Quotes = map[string]model.Quote{
		"sh600001": {Symbol: "sh600001", Pct: -0.8},
	}

	in := portfolioInsight([]*Fund{a, b})
	if !near(in.IndustryTotal, 34+12+12) {
		t.Fatalf("industry total = %v, want 58", in.IndustryTotal)
	}
	if len(in.Industries) != 3 || in.Industries[0].Name != "制造业" ||
		!near(in.Industries[0].Weight, 34) {
		t.Fatalf("industries = %+v", in.Industries)
	}
	if !near(in.StockTotal, 9.2+3+2.8) || in.StockCount != 3 {
		t.Fatalf("stock total = %v over %d", in.StockTotal, in.StockCount)
	}
	top := in.Stocks[0]
	if top.Symbol != "sh600001" || !near(top.Weight, 9.2) || top.Funds != 2 {
		t.Fatalf("top stock = %+v", top)
	}
	// The price comes from any fund holding it; the first one in the
	// list's order answers.
	if !top.Quoted || !near(top.Pct, 1.2) {
		t.Fatalf("quote = %+v", top)
	}
	if !top.Quoted || top.Funds != 2 {
		t.Fatalf("stock is held by %d funds", top.Funds)
	}
	// Largest first: 9.2, 3.0, 2.8.
	if !near(in.Stocks[1].Weight, 3) || !near(in.Stocks[2].Weight, 2.8) {
		t.Fatalf("stock order = %+v", in.Stocks)
	}
}

func TestPortfolioStockTiesKeepTheSymbolOrder(t *testing.T) {
	f := insightFund("000001", "基金A", 1000, 1, []model.Holding{
		{Symbol: "sh600002", Name: "同名", Weight: 5},
		{Symbol: "sh600001", Name: "同名", Weight: 5},
	}, nil)
	in := portfolioInsight([]*Fund{f})
	if len(in.Stocks) != 2 {
		t.Fatalf("stocks = %+v", in.Stocks)
	}
	if in.Stocks[0].Symbol != "sh600001" || in.Stocks[1].Symbol != "sh600002" {
		t.Fatalf("tied stocks = %+v, want them by symbol", in.Stocks)
	}
}

func TestPortfolioOverlapUsesTheSmallerWeight(t *testing.T) {
	a := insightFund("000001", "基金A", 1000, 1, []model.Holding{
		{Symbol: "sh600001", Name: "甲", Weight: 10},
		{Symbol: "sh600002", Name: "乙", Weight: 5},
		{Symbol: "sh600003", Name: "丙", Weight: 4},
	}, nil)
	b := insightFund("000002", "基金B", 1000, 1, []model.Holding{
		{Symbol: "sh600001", Name: "甲", Weight: 6},
		{Symbol: "sh600002", Name: "乙", Weight: 8},
		{Symbol: "sh600003", Name: "丙", Weight: 2},
		{Symbol: "sh600004", Name: "丁", Weight: 3},
	}, nil)
	c := insightFund("000003", "基金C", 1000, 1, []model.Holding{
		{Symbol: "sh600001", Name: "甲", Weight: 7},
	}, nil)
	// A fund without holdings counts as an asset of the portfolio, but adds
	// nothing to the stocks, industries or overlap.
	d := insightFund("000004", "基金D", 1000, 1, nil, nil)

	in := portfolioInsight([]*Fund{a, b, c, d})
	if in.Count != 4 || len(in.Funds) != 4 {
		t.Fatalf("count = %d, funds = %+v", in.Count, in.Funds)
	}
	if len(in.Overlaps) != 3 {
		t.Fatalf("overlaps = %+v, want the three pairs with holdings", in.Overlaps)
	}
	// A × B: min(10, 6) + min(5, 8) + min(4, 2) = 13, three stocks.
	ab := in.Overlaps[0]
	if ab.ACode != "000001" || ab.BCode != "000002" ||
		!near(ab.Weight, 13) || len(ab.Common) != 3 {
		t.Fatalf("A × B = %+v", ab)
	}
	// A × C: min(10, 7) = 7; B × C: min(6, 7) = 6.
	if in.Overlaps[1].ACode != "000001" || in.Overlaps[1].BCode != "000003" ||
		!near(in.Overlaps[1].Weight, 7) {
		t.Fatalf("A × C = %+v", in.Overlaps[1])
	}
	if in.Overlaps[2].ACode != "000002" || in.Overlaps[2].BCode != "000003" ||
		!near(in.Overlaps[2].Weight, 6) {
		t.Fatalf("B × C = %+v", in.Overlaps[2])
	}
}

func TestPortfolioOverlapMatchesAHoldingWithoutASymbolByName(t *testing.T) {
	a := insightFund("000001", "基金A", 1000, 1, []model.Holding{
		{Name: "同名股", Weight: 9},
	}, nil)
	b := insightFund("000002", "基金B", 1000, 1, []model.Holding{
		{Name: "同名股", Weight: 4},
	}, nil)
	in := portfolioInsight([]*Fund{a, b})
	if in.StockCount != 1 || in.Stocks[0].Funds != 2 {
		t.Fatalf("stocks = %+v", in.Stocks)
	}
	if len(in.Overlaps) != 1 || !near(in.Overlaps[0].Weight, 4) {
		t.Fatalf("overlaps = %+v", in.Overlaps)
	}
}

func TestPortfolioCountsOnlyHeldFundsOfTheHoldingGroup(t *testing.T) {
	held := insightFund("000001", "持有", 1000, 1, nil, nil)
	noShares := insightFund("000002", "未持有", 0, 1, nil, nil)
	elsewhere := insightFund("000003", "别的分组", 1000, 1, nil, nil)
	elsewhere.Groups = []string{"g1"}
	both := insightFund("000004", "两个分组", 1000, 1, nil, nil)
	both.Groups = []string{"", "g1"}

	in := portfolioInsight([]*Fund{held, noShares, elsewhere, both, nil})
	if in.Count != 2 {
		t.Fatalf("count = %d, want the two held funds of the holding group", in.Count)
	}
	if !near(in.Total, 2000) {
		t.Fatalf("total = %v, want 2000", in.Total)
	}
	for _, f := range in.Funds {
		if f.Code != "000001" && f.Code != "000004" {
			t.Fatalf("counted %s", f.Code)
		}
	}
}

func TestPortfolioWithOneFundHasNoOverlap(t *testing.T) {
	f := insightFund("000001", "基金A", 1000, 1,
		[]model.Holding{{Symbol: "sh600001", Name: "甲", Weight: 10}},
		[]model.Industry{{Name: "制造业", Weight: 40}})
	in := portfolioInsight([]*Fund{f})
	if !near(in.Funds[0].Weight, 100) {
		t.Fatalf("weight = %v", in.Funds[0].Weight)
	}
	if len(in.Overlaps) != 0 {
		t.Fatalf("one fund overlaps with %+v", in.Overlaps)
	}
	if !near(in.StockTotal, 10) || !near(in.IndustryTotal, 40) {
		t.Fatalf("totals = %v / %v", in.StockTotal, in.IndustryTotal)
	}
}

func TestPortfolioWithoutFundsIsEmpty(t *testing.T) {
	for _, funds := range [][]*Fund{nil, {}, {nil}} {
		in := portfolioInsight(funds)
		if in.Count != 0 || in.Total != 0 ||
			len(in.Funds) != 0 || len(in.Industries) != 0 ||
			len(in.Stocks) != 0 || len(in.Overlaps) != 0 {
			t.Fatalf("portfolioInsight(%v) = %+v", funds, in)
		}
	}
}
