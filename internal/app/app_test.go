package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/store"
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
	for _, f := range a.funds {
		f.Groups = []string{holdingGroup}
	}
	a.all = a.funds
	a.sel = 0
	a.period = 0
	a.rebuildList()
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
	// The holdings' change is scaled by the fund's stock position: the rest
	// of the fund is not stocks, and does not move with them.
	want := (-0.55*18.54 + -1.06*9.05) / (18.54 + 9.05)
	want *= f.StockPosition / 100
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

// The day's percent is measured against what the shares that earned it were
// worth when the day began: a buy or a sale during the day changes the value
// but not the percent.
func TestSummaryPctIsTheGainOnTheSharesThatEarnedIt(t *testing.T) {
	at := now()
	today := at.Format("2006-01-02")
	yesterday := at.AddDate(0, 0, -1).Format("2006-01-02")
	navs := []model.Nav{
		{Date: yesterday, Unit: 1},
		{Date: today, Unit: 1.1, Pct: 10},
	}
	fund := func(txns []ledger.Txn) *Fund {
		t.Helper()
		pos, _, err := ledger.Replay(txns)
		if err != nil {
			t.Fatal(err)
		}
		return &Fund{
			Code: "161725", Groups: []string{holdingGroup}, Loaded: true,
			Txns: txns, pos: pos, Shares: pos.Shares, Cost: pos.Cost, Navs: navs,
		}
	}
	summary := func(txns []ledger.Txn) Summary {
		a := New(t.TempDir())
		a.all = []*Fund{fund(txns)}
		a.rebuildList()
		return a.Summary()
	}

	// 1000 shares were held before the day; 1000 more were bought during it.
	bought := summary([]ledger.Txn{
		ledger.Opening(1000, 1),
		{ID: 2, Date: today, Kind: ledger.Buy, Shares: 1000, Price: 1.1},
	})
	if !bought.Holding || !closeTo(bought.Profit, 100) || !closeTo(bought.Pct, 10) {
		t.Fatalf("after a buy: %+v, want the 100 the first 1000 shares earned and 10%%",
			bought)
	}

	// All 1000 shares were sold during the day: what is held now is nothing,
	// but the shares earned the change before they went.
	sold := summary([]ledger.Txn{
		ledger.Opening(1000, 1),
		{ID: 2, Date: today, Kind: ledger.Sell, Shares: 1000, Price: 1.1},
	})
	if !sold.Holding || !closeTo(sold.Profit, 100) || !closeTo(sold.Pct, 10) {
		t.Fatalf("after a sale: %+v, want the day's gain still counted", sold)
	}
}

// A fund whose disclosed holdings cover little of it has no estimate by
// default. The settings can ask for one anyway, and the fund's line then says
// how much of it the estimate stands on.
func TestLowCoverEstimateFollowsTheSetting(t *testing.T) {
	a := preview(t)
	f := &Fund{
		Code: "011555", Name: "海富通欣利混合C", Loaded: true,
		// Half the fund is stocks, so the holdings' 2% change comes down to 1.
		StockPosition: 50,
		Holdings:      []model.Holding{{Symbol: "sh600030", Weight: 5}},
		Quotes: map[string]model.Quote{
			"sh600030": {Symbol: "sh600030", Pct: 2, At: now()},
		},
		Navs: []model.Nav{{
			Date: now().AddDate(0, 0, -1).Format("2006-01-02"), Unit: 1,
		}},
	}
	a.all = []*Fund{f}
	a.rebuildList()
	a.estimate(f, nil)
	if f.HasEst {
		t.Fatal("a fund of 5% coverage should have no estimate by default")
	}

	a.setLowCoverEstimate(true)
	if !f.HasEst || !closeTo(f.Est.Cover, 5) || !closeTo(f.Est.Pct, 1) {
		t.Fatalf("estimate = %+v, %v; want the low-coverage estimate, scaled to the stocks",
			f.Est, f.HasEst)
	}
	if note := a.valueLine(f); !strings.Contains(note, "重仓股覆盖 5.0%") {
		t.Fatalf("value line = %q, want the coverage said", note)
	}
	if data := store.Load(filepath.Join(a.dir, "funds.json")); !data.LowCoverEstimate {
		t.Fatal("the switch was not saved")
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

func TestTodayProfitUsesThePublishedChangeOnADistributionDay(t *testing.T) {
	f := &Fund{Shares: 12000, Navs: []model.Nav{
		{Date: "2026-10-09", Unit: 1.2356, Pct: 0.50},
		{Date: "2026-10-12", Unit: 1.2352, Pct: 1.00},
	}}
	// The unit value fell, but the fund paid a dividend: the gain is the
	// published +1.00%.
	if got := signedMoney(f.TodayProfit()); got != "+148.27" {
		t.Fatalf("profit on a distribution day = %s, want +148.27", got)
	}
	// An ordinary day keeps the unit values' difference.
	f.Navs = []model.Nav{
		{Date: "2026-11-10", Unit: 1.2447, Pct: -0.16},
		{Date: "2026-11-11", Unit: 1.2367, Pct: -0.64},
	}
	if got := signedMoney(f.TodayProfit()); got != "-96.00" {
		t.Fatalf("profit on an ordinary day = %s, want -96.00", got)
	}
}

func TestTradingHoursSkipHolidays(t *testing.T) {
	holiday := time.Date(2026, 10, 5, 10, 0, 0, 0, cst)
	if tradingAt(holiday) {
		t.Fatal("the market is closed on the National Day holiday")
	}
	ordinary := time.Date(2026, 11, 11, 10, 0, 0, 0, cst)
	if !tradingAt(ordinary) {
		t.Fatal("the market is open on an ordinary weekday morning")
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

func TestLongerPeriodsKeepTheirSavedIndices(t *testing.T) {
	want := []struct {
		label string
		days  int
	}{
		{"近1月", 30}, {"近3月", 92}, {"近6月", 183}, {"近1年", 365},
		{"今年来", 0}, {"近3年", 1095}, {"成立以来", 0},
	}
	if len(periods) != len(want) {
		t.Fatalf("periods = %d, want %d", len(periods), len(want))
	}
	for i, w := range want {
		if periods[i].label != w.label || periods[i].days != w.days {
			t.Errorf("period %d = %q/%d, want %q/%d", i, periods[i].label, periods[i].days, w.label, w.days)
		}
	}
}

func TestYearToDateOpensAtTheYearEndBase(t *testing.T) {
	f := &Fund{Navs: []model.Nav{
		{Date: "2025-12-30", Unit: 1.00},
		{Date: "2025-12-31", Unit: 1.01},
		{Date: "2026-01-02", Unit: 1.02},
		{Date: "2026-01-05", Unit: 1.00},
	}}
	start := periodStart(f, 4) // 今年来
	if got := start.Format("2006-01-02"); got != "2025-12-31" {
		t.Fatalf("year-to-date opens at %s, want the last value before January 1", got)
	}
	window := windowOf(f, 4)
	if len(window) != 3 || window[0].Date != "2025-12-31" {
		t.Fatalf("window = %+v", window)
	}
	// The base is the year-end value, so the first day's move is included.
	if got, want := model.PeriodChange(window), (1.00/1.01-1)*100; abs(got-want) > 1e-9 {
		t.Fatalf("year-to-date change = %v, want %v", got, want)
	}
}

func TestWholeHistoryIsEveryValue(t *testing.T) {
	f := youngFund(preview(t), 30)
	if start := periodStart(f, 6); start.Format("2006-01-02") != f.Navs[0].Date {
		t.Fatalf("成立以来 opens at %v, want the first value %s", start, f.Navs[0].Date)
	}
	if got := len(windowOf(f, 6)); got != len(f.Navs) {
		t.Fatalf("成立以来 holds %d values, want all %d", got, len(f.Navs))
	}
}

func TestNumberReadsTheHoldingFields(t *testing.T) {
	cases := []struct {
		text string
		want float64
		ok   bool
	}{
		{"1203.47", 1203.47, true},
		{" 1,203.5 ", 1203.5, true},
		{"0", 0, true},
		// Text that is not a number in range leaves the value as it was.
		{"", 7, false},
		{"0.00.", 7, false},
		{"-1", 7, false},
		{"1e11", 7, false},
	}
	for _, c := range cases {
		v := 7.0
		if ok := readNumber(c.text, &v, 0, 1e10); ok != c.ok || v != c.want {
			t.Errorf("readNumber(%q) = %v, %v; want %v, %v", c.text, v, ok, c.want, c.ok)
		}
	}
}

func TestHoldingFieldsTakeDecimals(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	f.Shares, f.Cost = 0, 0
	a.editing = true
	build := func(c *ui.Context) {
		t := theme(c)
		c.SetTheme(t)
		a.holdingSection(c, t, f)
	}
	tt := ui.NewTester(build, 760, 220)
	tt.Frame()
	tt.Frame()

	if err := tt.Click("持有份额输入"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("1203.47")
	tt.Frame()
	if f.Shares != 1203.47 {
		t.Fatalf("shares = %v, want 1203.47", f.Shares)
	}

	if err := tt.Click("成本价输入"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("1.2467")
	tt.Frame()
	if f.Cost != 1.2467 {
		t.Fatalf("cost = %v, want 1.2467", f.Cost)
	}
}
