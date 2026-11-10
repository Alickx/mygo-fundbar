package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
)

// preview builds an app of two funds with data of their own, without the
// network.
func preview(t *testing.T) *App {
	t.Helper()
	a := New(t.TempDir())
	yesterday := now().AddDate(0, 0, -1).Format("2006-01-02")
	today := now().Format("2006-01-02")
	a.funds = []*Fund{
		{
			Code: "161725", Name: "招商中证白酒指数(LOF)A", Type: "指数型-股票",
			Shares: 12000, Cost: 0.62, Loaded: true, Report: "2026-09-30",
			StockPosition: 85.7, IndustryReport: "2026-09-30",
			Industries: []model.Industry{
				{Name: "制造业", Weight: 94.47},
				{Name: "金融业", Weight: 0.01},
			},
			Navs: []model.Nav{
				{Date: yesterday, Unit: 0.5896, Acc: 2.3057, Pct: -0.51},
				{Date: today, Unit: 0.5859, Acc: 2.3020, Pct: -0.63},
			},
			Holdings: []model.Holding{
				{Symbol: "sh600519", Code: "600519", Name: "贵州茅台", Weight: 18.54},
				{Symbol: "sz000858", Code: "000858", Name: "五粮液", Weight: 9.05},
			},
			Profiles: map[string]model.StockProfile{
				"sh600519": {Industry: "白酒Ⅱ",
					Concepts: []string{"白酒", "超级品牌"}},
				"sz000858": {Industry: "白酒Ⅱ"},
			},
		},
		{
			Code: "005827", Name: "易方达蓝筹精选混合", Type: "混合型-偏股",
			Shares: 8000, Cost: 1.62, Loaded: true,
			Navs: []model.Nav{{Date: today, Unit: 1.3993, Acc: 1.3993, Pct: -0.21}},
		},
	}
	a.funds[0].Quotes = map[string]model.Quote{
		"sh600519": {Symbol: "sh600519", Pct: -0.55, At: time.Now()},
		"sz000858": {Symbol: "sz000858", Pct: -1.06, At: time.Now()},
	}
	a.estimate(a.funds[0], nil)
	a.sel = 0
	a.period = 0
	return a
}

func TestViewBuilds(t *testing.T) {
	a := preview(t)
	tester := ui.NewTester(a.View, 1080, 780)
	tester.Frame()
	tester.Frame()
	if tester.Image() == nil {
		t.Fatal("expected a frame")
	}
}

func TestEstimatesFromQuotes(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	if !f.HasEst {
		t.Fatal("expected an estimate")
	}
	want := (-0.55*18.54 + -1.06*9.05) / (18.54 + 9.05)
	if diff := f.Est.Pct - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("estimate = %v, want %v", f.Est.Pct, want)
	}
	// Today's value is published, so the app shows it, not the estimate.
	if f.UsingEstimate() {
		t.Fatal("expected the published value")
	}
	if got := f.ChangePct(); got != -0.63 {
		t.Fatalf("change = %v, want -0.63", got)
	}
	if got, want := f.TodayProfit(), 12000*(0.5859-0.5896); abs(got-want) > 1e-9 {
		t.Fatalf("profit = %v, want %v", got, want)
	}
}

func TestEstimateIsNewerThanThePublishedValue(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	// Make the fund's value a day older than its prices, as it is while the
	// markets are open.
	f.Navs = f.Navs[:1]
	f.QuoteAt = now()
	if !f.UsingEstimate() {
		t.Fatal("expected the estimate to be the newest number")
	}
	want := 0.5896 * (1 + f.Est.Pct/100)
	if diff := f.EstNav() - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("estimated value = %v, want %v", f.EstNav(), want)
	}
}

func TestSummaryAddsUpHoldings(t *testing.T) {
	a := preview(t)
	s := a.Summary()
	if !s.Holding {
		t.Fatal("expected holdings")
	}
	if len(a.funds) != 2 {
		t.Fatalf("funds = %d", len(a.funds))
	}
	if s.Value <= 0 {
		t.Fatalf("value = %v", s.Value)
	}
}

func TestChoosingAFund(t *testing.T) {
	a := preview(t)
	tester := ui.NewTester(a.View, 1080, 780)
	tester.Frame()
	if err := tester.Click("易方达蓝筹精选混合"); err != nil {
		t.Skipf("the list row is not clickable in this render: %v", err)
	}
	tester.Frame()
	if a.sel != 1 {
		t.Fatalf("selected %d, want 1", a.sel)
	}
}

func TestSearchResults(t *testing.T) {
	a := preview(t)
	a.PreviewSearch("白酒", nil)
	tester := ui.NewTester(a.View, 1080, 780)
	tester.Frame()
	if _, ok := tester.Find("没有找到基金"); !ok {
		t.Fatal("expected the empty search note")
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestNavListShowsEachTradingDay(t *testing.T) {
	a := preview(t)
	a.funds[0].Navs = []model.Nav{
		{Date: "2026-10-08", Unit: 0.6100, Acc: 2.3300, Pct: 0.5},
		{Date: "2026-10-09", Unit: 0.6050, Acc: 2.3250, Pct: -0.82},
		{Date: "2026-10-10", Unit: 0.6080, Acc: 2.3280, Pct: 0.50},
	}
	a.navView = 1
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	if !tt.HasText("历史净值") || !tt.HasText("日期") {
		t.Fatal("expected the list of each day's value")
	}
	for _, date := range []string{"2026-10-08", "2026-10-09", "2026-10-10"} {
		if !tt.HasText(date) {
			t.Fatalf("missing day %s in the history list", date)
		}
	}
	if err := tt.Click("走势"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.navView != 0 {
		t.Fatal("expected the line view")
	}
}

func TestIndustryAndStockProfileAreShown(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	for _, text := range []string{"行业配置", "制造业", "白酒Ⅱ"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q", text)
		}
	}
	// The stock's regional board is not shown: the industry is enough.
	if tt.HasText("贵州板块") {
		t.Fatal("expected no regional board")
	}
}

func TestDetailTitleCopiesTheNameAndCode(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	build := func(c *ui.Context) {
		t := theme(c)
		c.SetTheme(t)
		a.detailTitle(c, t, f)
	}
	tt := ui.NewTester(build, 760, 220)
	tt.Frame()

	if err := tt.Click(f.Code); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != f.Code {
		t.Fatalf("clipboard = %q, want the fund code %q", got, f.Code)
	}
	if !tt.HasText("已复制代码") {
		t.Fatalf("expected a note that the code was copied, got %v", tt.Texts())
	}

	if err := tt.Click(f.Name); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != f.Name {
		t.Fatalf("clipboard = %q, want the fund name %q", got, f.Name)
	}
}

func TestHoldingsMarkMarketsAbroad(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	f.Holdings = append([]model.Holding{
		{Symbol: "hk00700", Code: "00700", Name: "腾讯控股", Weight: 8.12},
		{Symbol: "usAAPL", Code: "AAPL", Name: "苹果", Weight: 4.21},
	}, f.Holdings...)
	f.Quotes = map[string]model.Quote{
		"hk00700": {Symbol: "hk00700", Pct: 1.24, At: time.Now()},
		"usAAPL":  {Symbol: "usAAPL", Pct: -0.42, At: time.Now()},
	}
	a.estimate(f, nil)
	tt := ui.NewTester(a.View, 1080, 1180)
	tt.Frame()
	for _, text := range []string{"腾讯控股", "港股", "苹果", "美股", "贵州茅台"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in the holdings, got %v", text, tt.Texts())
		}
	}
}

func TestMoneyAndPadding(t *testing.T) {
	if got := money(1234567.891); got != "1,234,567.89" {
		t.Fatalf("money = %q", got)
	}
	if got := money(-1200); got != "-1,200.00" {
		t.Fatalf("money = %q", got)
	}
	if got := signedMoney(128.4); got != "+128.40" {
		t.Fatalf("signedMoney = %q", got)
	}
	if got := pct(-1.234); got != "-1.23%" {
		t.Fatalf("pct = %q", got)
	}
	// Figure spaces keep the menu bar item's width steady.
	if got := padFigures("+9.00", 7); len([]rune(got)) != 7 || []rune(got)[0] != '\u2007' {
		t.Fatalf("padFigures = %q", got)
	}
	if got := padFigures("+1234.56", 7); got != "+1234.56" {
		t.Fatalf("padFigures = %q", got)
	}
}

// youngFund gives the first fund a history that starts days ago, as a fund
// issued recently has.
func youngFund(a *App, days int) *Fund {
	f := a.funds[0]
	last := now()
	f.Navs = nil
	for i := days; i >= 0; i-- {
		f.Navs = append(f.Navs, model.Nav{
			Date: last.AddDate(0, 0, -i).Format("2006-01-02"),
			Unit: 1 + float64(days-i)*0.001,
		})
	}
	return f
}

func TestYoungFundCallsTheLongerPeriodItsWholeHistory(t *testing.T) {
	a := preview(t)
	f := youngFund(a, 60)
	a.period = 2 // 近6月, longer than the fund's history
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	if !tt.HasText("成立以来净值变化") {
		t.Fatalf("expected the whole history to be named, got %v", tt.Texts())
	}
	if !tt.HasText("净值自 " + f.Navs[0].Date + " 起") {
		t.Fatalf("expected the first value's date, got %v", tt.Texts())
	}
	a.navView = 1
	tt.Frame()
	if !tt.HasText("成立以来 · 61 个交易日") {
		t.Fatalf("expected the list to count the whole history, got %v", tt.Texts())
	}
	// A period the fund fills keeps the period's own name.
	a.navView = 0
	a.period = 0
	tt.Frame()
	if tt.HasText("成立以来") {
		t.Fatal("a month of history is a month, not the fund's whole life")
	}
	if !tt.HasText("近1月净值变化") {
		t.Fatalf("expected the month's change, got %v", tt.Texts())
	}
}

func TestYoungFundDrawsTheWiderWindow(t *testing.T) {
	a := preview(t)
	f := youngFund(a, 60)
	// Every longer period holds the same values, but the window they are
	// drawn in grows, so the line takes less of the plot each time.
	if six, year := windowOf(f, 2), windowOf(f, 3); len(six) != len(year) {
		t.Fatalf("近6月 holds %d values, 近1年 %d", len(six), len(year))
	}
	start6, start12 := periodStart(f, 2), periodStart(f, 3)
	first := parseDay(f.Navs[0].Date)
	if !start12.Before(start6) || !start6.Before(first) {
		t.Fatalf("windows open at %v and %v, history at %v", start12, start6, first)
	}
}
