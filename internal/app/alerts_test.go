package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/store"
)

// alertAt is a weekday morning the market trades, which the rules can be
// tried at without a clock.
func alertAt(hour int) time.Time {
	return time.Date(2026, 11, 11, hour, 0, 0, 0, cst)
}

// estimateFund is a fund whose estimate is the day's: its prices are from at,
// and its published value is the day before.
func estimateFund(code, name string, change float64, at time.Time) *Fund {
	day := at.In(cst)
	return &Fund{
		Code: code, Name: name, Groups: []string{holdingGroup},
		HasEst: true, Est: model.Estimate{Pct: change}, QuoteAt: day, Loaded: true,
		Navs: []model.Nav{{Date: day.AddDate(0, 0, -1).Format("2006-01-02"), Unit: 1}},
	}
}

// navFund is a fund whose value for day is published, or whose last value is
// older when day is empty.
func navFund(code, name string, shares, cost float64, at time.Time, published bool) *Fund {
	day := at.In(cst)
	last := model.Nav{Date: day.AddDate(0, 0, -1).Format("2006-01-02"), Unit: 1, Pct: 1}
	if published {
		last = model.Nav{Date: day.Format("2006-01-02"), Unit: 1.01, Pct: 1}
	}
	return &Fund{
		Code: code, Name: name, Shares: shares, Cost: cost,
		Groups: []string{holdingGroup}, Loaded: true,
		Navs: []model.Nav{
			{Date: day.AddDate(0, 0, -2).Format("2006-01-02"), Unit: 1, Pct: 0},
			last,
		},
	}
}

func TestMoveThresholdAnnouncesOnceADay(t *testing.T) {
	at := alertAt(10)
	f := estimateFund("007760", "基金A", 2.4, at)
	cfg := AlertConfig{Move: true, MovePct: 2}

	notes, state := evaluate(cfg, store.AlertState{}, []*Fund{f}, at)
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want one", notes)
	}
	if !strings.Contains(notes[0].Body, "基金A +2.40%") {
		t.Fatalf("body = %q", notes[0].Body)
	}
	if notes[0].Fund != "007760" {
		t.Fatalf("note is about %q, want the one fund", notes[0].Fund)
	}

	again, state := evaluate(cfg, state, []*Fund{f}, at)
	if len(again) != 0 {
		t.Fatalf("a second look announced %+v", again)
	}

	// The next trading date arms the rule again.
	next := at.AddDate(0, 0, 1)
	f.QuoteAt = next
	if notes, _ := evaluate(cfg, state, []*Fund{f}, next); len(notes) != 1 {
		t.Fatalf("the new day announced %+v, want one", notes)
	}
}

func TestMoveAnnouncesNewFundsAsOneNote(t *testing.T) {
	at := alertAt(10)
	funds := []*Fund{
		estimateFund("007760", "基金A", 2.5, at),
		estimateFund("008270", "基金B", -2.5, at),
		estimateFund("011555", "基金C", 1.2, at),
	}
	notes, state := evaluate(AlertConfig{Move: true, MovePct: 2}, store.AlertState{}, funds, at)
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want one", notes)
	}
	if !strings.Contains(notes[0].Body, "基金A +2.50%") ||
		!strings.Contains(notes[0].Body, "基金B -2.50%") {
		t.Fatalf("body = %q", notes[0].Body)
	}
	if strings.Contains(notes[0].Body, "基金C") {
		t.Fatalf("below the threshold but in the note: %q", notes[0].Body)
	}
	if notes[0].Fund != "" {
		t.Fatalf("a note about two funds opens %q", notes[0].Fund)
	}
	if len(state.Move) != 2 {
		t.Fatalf("marks = %v, want two funds", state.Move)
	}
}

func TestMoveIgnoresAStaleEstimate(t *testing.T) {
	at := alertAt(10)
	f := estimateFund("007760", "基金A", 5, at)
	// The fund still shows an estimate, but its prices are yesterday's.
	f.QuoteAt = at.AddDate(0, 0, -1)
	f.Navs[0].Date = at.AddDate(0, 0, -2).Format("2006-01-02")
	if notes, _ := evaluate(AlertConfig{Move: true, MovePct: 2}, store.AlertState{}, []*Fund{f}, at); len(notes) != 0 {
		t.Fatalf("stale estimate announced %+v", notes)
	}
}

func TestMoveThresholdZeroUsesTheDefault(t *testing.T) {
	at := alertAt(10)
	f := estimateFund("007760", "基金A", 2.1, at)
	if notes, _ := evaluate(AlertConfig{Move: true}, store.AlertState{}, []*Fund{f}, at); len(notes) != 1 {
		t.Fatalf("2.1%% with the default 2%% announced %+v", notes)
	}
}

func TestMoveNotesOfADayDoNotReplaceEachOther(t *testing.T) {
	at := alertAt(10)
	cfg := AlertConfig{Move: true, MovePct: 2}
	a := estimateFund("007760", "基金A", 2.5, at)
	b := estimateFund("008270", "基金B", 2.2, at)
	c := estimateFund("011555", "基金C", 1.0, at)
	d := estimateFund("021528", "基金D", 1.0, at)
	funds := []*Fund{a, b, c, d}

	first, state := evaluate(cfg, store.AlertState{}, funds, at)
	if len(first) != 1 {
		t.Fatalf("notes = %+v, want one", first)
	}
	// Two more funds reach the threshold later in the day.
	c.Est.Pct, d.Est.Pct = -3.0, 2.6
	second, _ := evaluate(cfg, state, funds, at)
	if len(second) != 1 {
		t.Fatalf("the later batch announced %+v, want one", second)
	}
	// A shared id would have the later note take the earlier one's place.
	if first[0].ID == second[0].ID {
		t.Fatalf("two batches share the id %q", first[0].ID)
	}
	if strings.Contains(second[0].Body, "基金A") || strings.Contains(second[0].Body, "基金B") {
		t.Fatalf("the later note repeats the earlier funds: %q", second[0].Body)
	}
}

func TestMoveBodyNamesTheLargestAndCountsTheRest(t *testing.T) {
	at := alertAt(10)
	funds := []*Fund{
		estimateFund("007760", "基金A", 2.1, at),
		estimateFund("008270", "基金B", 2.4, at),
		estimateFund("011555", "基金C", 2.2, at),
		estimateFund("021528", "基金D", 3.5, at),
		estimateFund("028678", "基金E", -2.9, at),
	}
	notes, _ := evaluate(AlertConfig{Move: true, MovePct: 2}, store.AlertState{}, funds, at)
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want one", notes)
	}
	body := notes[0].Body
	for _, want := range []string{"基金D +3.50%", "基金E -2.90%", "基金B +2.40%", "等 5 只"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q in it", body, want)
		}
	}
	if strings.Contains(body, "基金A") || strings.Contains(body, "基金C") {
		t.Fatalf("a smaller move is named beyond the three: %q", body)
	}
}

func TestNoteIDsSayWhatAClickOpens(t *testing.T) {
	at := alertAt(10)
	f := estimateFund("007760", "基金A", 2.5, at)
	f.Shares, f.Cost, f.TakeProfit = 1000, 0.5, 20
	notes, _ := evaluate(AlertConfig{Move: true, MovePct: 2}, store.AlertState{}, []*Fund{f}, at)
	if len(notes) != 2 {
		t.Fatalf("notes = %+v, want a move and a target", notes)
	}
	// A note about one fund names it, whichever rule spoke.
	for _, n := range notes {
		if code, ok := noteFund(n.ID); !ok || code != "007760" {
			t.Fatalf("noteFund(%q) = %q, %v, want the fund", n.ID, code, ok)
		}
	}
	// A note about several funds, the day's summary and the test note open
	// the window rather than a fund.
	for _, id := range []string{
		allNote + "move:2026-11-11:2", allNote + "settled:2026-11-11", allNote + "test", "",
	} {
		if code, ok := noteFund(id); ok {
			t.Fatalf("noteFund(%q) = %q, want no fund", id, code)
		}
	}
}

func TestSettledAnnouncesWhenAllArePublished(t *testing.T) {
	at := alertAt(20)
	funds := []*Fund{
		navFund("007760", "基金A", 1000, 1, at, true),
		navFund("008270", "基金B", 2000, 1, at, true),
	}
	notes, state := evaluate(AlertConfig{Settled: true}, store.AlertState{}, funds, at)
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want one", notes)
	}
	body := notes[0].Body
	if !strings.Contains(body, "2 只已全部公布") || !strings.Contains(body, "今日收益") {
		t.Fatalf("body = %q", body)
	}
	if again, _ := evaluate(AlertConfig{Settled: true}, state, funds, at); len(again) != 0 {
		t.Fatalf("settled announced twice: %+v", again)
	}
}

func TestSettledAfterTenCountsTheRest(t *testing.T) {
	at := alertAt(22)
	funds := []*Fund{
		navFund("007760", "基金A", 1000, 1, at, true),
		navFund("008270", "基金B", 2000, 1, at, false),
	}
	notes, _ := evaluate(AlertConfig{Settled: true}, store.AlertState{}, funds, at)
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want one", notes)
	}
	if !strings.Contains(notes[0].Body, "已公布 1 只 · 待公布 1 只") {
		t.Fatalf("body = %q", notes[0].Body)
	}

	// Before 22:00 the part that is out is not worth saying.
	early := alertAt(20)
	notes, _ = evaluate(AlertConfig{Settled: true}, store.AlertState{}, funds, early)
	if len(notes) != 0 {
		t.Fatalf("before 22:00 announced %+v", notes)
	}
}

func TestSettledStaysQuietWithNothingPublished(t *testing.T) {
	at := alertAt(22)
	funds := []*Fund{
		navFund("007760", "基金A", 1000, 1, at, false),
		navFund("008270", "基金B", 2000, 1, at, false),
	}
	if notes, _ := evaluate(AlertConfig{Settled: true}, store.AlertState{}, funds, at); len(notes) != 0 {
		t.Fatalf("nothing published but announced %+v", notes)
	}
}

func TestSettledNeverOnAClosedDay(t *testing.T) {
	saturday := time.Date(2026, 11, 14, 22, 0, 0, 0, cst)
	funds := []*Fund{navFund("007760", "基金A", 1000, 1, saturday, true)}
	if notes, _ := evaluate(AlertConfig{Settled: true}, store.AlertState{}, funds, saturday); len(notes) != 0 {
		t.Fatalf("a closed day announced %+v", notes)
	}
}

func TestTargetEntersStaysLeavesAndReenters(t *testing.T) {
	at := alertAt(10)
	f := &Fund{
		Code: "007760", Name: "基金A", Shares: 1000, Cost: 1,
		TakeProfit: 20, Groups: []string{holdingGroup}, Loaded: true,
		Navs: []model.Nav{{Date: at.Format("2006-01-02"), Unit: 1.2}},
	}
	cfg := AlertConfig{}

	notes, state := evaluate(cfg, store.AlertState{}, []*Fund{f}, at)
	if len(notes) != 1 || notes[0].Title != "止盈提醒" {
		t.Fatalf("entering announced %+v", notes)
	}
	if !strings.Contains(notes[0].Body, "+20.00%") ||
		!strings.Contains(notes[0].Body, "止盈线 +20%") {
		t.Fatalf("body = %q", notes[0].Body)
	}

	// Staying in the zone is quiet.
	if notes, state = evaluate(cfg, state, []*Fund{f}, at); len(notes) != 0 {
		t.Fatalf("staying announced %+v", notes)
	}

	// Leaving re-arms the zone but not the day's announcement.
	f.Navs[0].Unit = 1.1
	if notes, state = evaluate(cfg, state, []*Fund{f}, at); len(notes) != 0 {
		t.Fatalf("leaving announced %+v", notes)
	}
	f.Navs[0].Unit = 1.3
	if notes, state = evaluate(cfg, state, []*Fund{f}, at); len(notes) != 0 {
		t.Fatalf("re-entering announced %+v, want the once-a-day cap", notes)
	}

	// The next day finds the fund still past its line, and says nothing: it
	// has not entered the zone again.
	next := at.AddDate(0, 0, 1)
	f.Navs[0].Date = next.Format("2006-01-02")
	if notes, state = evaluate(cfg, state, []*Fund{f}, next); len(notes) != 0 {
		t.Fatalf("a fund still in its zone announced %+v on a new day", notes)
	}

	// Leaving and coming back on the new day speaks once more.
	f.Navs[0].Unit = 1.1
	if notes, state = evaluate(cfg, state, []*Fund{f}, next); len(notes) != 0 {
		t.Fatalf("leaving announced %+v", notes)
	}
	f.Navs[0].Unit = 1.3
	if notes, _ = evaluate(cfg, state, []*Fund{f}, next); len(notes) != 1 {
		t.Fatalf("re-entering on a new day announced %+v, want one", notes)
	}
}

// The target lines watch the shares held now: a sale that realised a gain
// must not keep a losing position inside its take-profit zone.
func TestTargetsWatchTheHoldingNotTheSales(t *testing.T) {
	at := alertAt(10)
	txns := []ledger.Txn{
		ledger.Opening(1000, 1),
		{ID: 2, Date: "2026-10-01", Kind: ledger.Sell, Shares: 500, Price: 2},
	}
	pos, steps, err := ledger.Replay(txns)
	if err != nil {
		t.Fatal(err)
	}
	f := &Fund{
		Code: "007760", Name: "基金A", Groups: []string{holdingGroup}, Loaded: true,
		Txns: txns, pos: pos, steps: steps, Shares: pos.Shares, Cost: pos.Cost,
		TakeProfit: 20,
		Navs:       []model.Nav{{Date: at.Format("2006-01-02"), Unit: 0.9}},
	}
	// The realised sale puts the cumulative return at +45%, while the shares
	// held now are 10% down.
	if got := f.TotalPct(); got < 20 {
		t.Fatalf("total return = %v, want the old rule to reach the line", got)
	}
	if notes, _ := evaluate(AlertConfig{}, store.AlertState{}, []*Fund{f}, at); len(notes) != 0 {
		t.Fatalf("a losing position triggered the take-profit line: %+v", notes)
	}

	// A real holding gain does reach it, and the note says the holding return.
	f.Navs[0].Unit = 1.3
	f.TakeProfit, f.StopLoss = 20, 0
	notes, _ := evaluate(AlertConfig{}, store.AlertState{}, []*Fund{f}, at)
	if len(notes) != 1 || !strings.Contains(notes[0].Body, "+30.00%") {
		t.Fatalf("holding gain announced %+v, want +30.00%%", notes)
	}
}

func TestTargetStopLossAndUnheldFunds(t *testing.T) {
	at := alertAt(10)
	f := &Fund{
		Code: "007760", Name: "基金A", Shares: 1000, Cost: 1,
		StopLoss: 10, Groups: []string{holdingGroup}, Loaded: true,
		Navs: []model.Nav{{Date: at.Format("2006-01-02"), Unit: 0.9}},
	}
	notes, _ := evaluate(AlertConfig{}, store.AlertState{}, []*Fund{f}, at)
	if len(notes) != 1 || notes[0].Title != "止损提醒" {
		t.Fatalf("stop loss announced %+v", notes)
	}
	if !strings.Contains(notes[0].Body, "止损线 -10%") {
		t.Fatalf("body = %q", notes[0].Body)
	}

	// Without shares there is no return to compare with a target.
	f.Shares = 0
	if notes, _ := evaluate(AlertConfig{}, store.AlertState{}, []*Fund{f}, at); len(notes) != 0 {
		t.Fatalf("an unheld fund announced %+v", notes)
	}
}

func TestRulesOffStaySilent(t *testing.T) {
	at := alertAt(22)
	funds := []*Fund{
		estimateFund("007760", "基金A", 5, at),
		navFund("008270", "基金B", 1000, 1, at, true),
	}
	if notes, _ := evaluate(AlertConfig{}, store.AlertState{}, funds, at); len(notes) != 0 {
		t.Fatalf("rules that are off announced %+v", notes)
	}
}

func TestSettingsWithAlertsBuilds(t *testing.T) {
	a := preview(t)
	a.alerts = store.Alerts{Move: true, MovePct: 2, Settled: true}
	a.alertErr = deniedAlert
	tt := ui.NewTester(a.PreviewSettings, 480, 620)
	tt.Frame()
	for _, text := range []string{
		"提醒", "盘中涨跌提醒", "涨跌幅阈值", "净值公布提醒",
		"发送测试通知", "通知权限被拒绝",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
}

func TestFundDetailShowsTheTargets(t *testing.T) {
	a := preview(t)
	a.alerts = store.Alerts{Move: true, MovePct: 2, Settled: true}
	f := a.funds[0]
	f.TakeProfit, f.StopLoss = 20, 8

	tt := ui.NewTester(a.View, 900, 600)
	tt.Frame()
	if !tt.HasText("止盈 +20% · 止损 -8%") {
		t.Fatalf("the collapsed line does not name the targets: %v", tt.Texts())
	}

	a.editing = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	for _, text := range []string{"止盈线（%）", "止损线（%）", "止盈输入", "止损输入"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
}
