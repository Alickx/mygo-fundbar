package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/store"
)

// cardTester draws the holding card on its own, as the view tests do, and
// settles the unfold.
func cardTester(a *App, f *Fund, w, h int) *ui.Tester {
	build := func(c *ui.Context) {
		t := theme(c)
		c.SetTheme(t)
		a.applyPending()
		a.holdingSection(c, t, f)
	}
	tt := ui.NewTester(build, w, h)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	return tt
}

func TestNavOnPicksTheDayOrTheNext(t *testing.T) {
	navs := []model.Nav{
		{Date: "2026-10-09", Unit: 0.6012},
		{Date: "2026-10-12", Unit: 0.6050},
	}
	nav, note, ok := navOn(navs, "2026-10-12")
	if !ok || nav.Unit != 0.6050 || !strings.Contains(note, "当日净值 0.6050") {
		t.Fatalf("exact day = %+v, %q, %v", nav, note, ok)
	}

	// A weekend has no value of its own: the next one is the closest.
	nav, note, ok = navOn(navs, "2026-10-10")
	if !ok || nav.Date != "2026-10-12" ||
		!strings.Contains(note, "非交易日，按 2026-10-12 的净值 0.6050") {
		t.Fatalf("weekend = %+v, %q, %v", nav, note, ok)
	}

	// After the last value there is nothing to fill in yet.
	nav, note, ok = navOn(navs, "2026-10-20")
	if ok || nav.Unit != 0 || !strings.Contains(note, "尚未公布") {
		t.Fatalf("after the last = %+v, %q, %v", nav, note, ok)
	}

	if _, note, ok := navOn(nil, "2026-10-20"); ok || !strings.Contains(note, "尚未公布") {
		t.Fatalf("empty history = %q, %v", note, ok)
	}
}

func TestLedgerCardRecordsTheFirstTransaction(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	a.editing = true
	tt := cardTester(a, f, 760, 600)

	if !tt.HasText("记一笔交易") {
		t.Fatalf("the manual card does not invite the first row: %v", tt.Texts())
	}
	if err := tt.Click("记一笔交易"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !a.tx.open || a.tx.code != f.Code {
		t.Fatalf("the form did not open for the fund: %+v", a.tx)
	}
	// The price follows the draft's day until the user types one. The day is
	// pinned to the fixture's own last value first: the form takes the clock
	// when it opens, and a test that straddled midnight would ask for a day
	// the fixture has no value for.
	a.tx.date = parseDay(f.Navs[len(f.Navs)-1].Date)
	a.prefillNav(f)
	if a.tx.priceText != "0.5859" {
		t.Fatalf("price = %q, want the day's value", a.tx.priceText)
	}

	if err := tt.Click("份额输入"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("1000")
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()

	if !f.ledgered() || len(f.steps) != 2 {
		t.Fatalf("ledgered = %v, steps = %d; want the opening and the buy", f.ledgered(), len(f.steps))
	}
	if f.Shares != 13000 {
		t.Fatalf("shares = %v, want the opening and the buy", f.Shares)
	}
	for _, text := range []string{"期初", "买入"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
}

func TestLedgerCardRefusesAnOversell(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	a.previewLedger(f, ledgerExample())
	a.editing = true
	tt := cardTester(a, f, 760, 720)

	if err := tt.Click("记一笔"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	// The segmented control holds the kind; the list's own pill of the same
	// name is later in the tree.
	if err := tt.Click("卖出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tx.kind != 1 {
		t.Fatalf("kind = %d, want the sale", a.tx.kind)
	}
	if err := tt.Click("份额输入"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("999999")
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !strings.Contains(a.tx.err, "卖出份额超过当日可卖的") {
		t.Fatalf("error = %q, want the sale called too large", a.tx.err)
	}
	if !tt.HasText("卖出份额超过当日可卖的") {
		t.Fatalf("the problem is not shown: %v", tt.Texts())
	}
	if len(f.steps) != 5 {
		t.Fatalf("steps = %d, want the rows left alone", len(f.steps))
	}
}

func TestLedgerRowDeleteLeavesAnUndo(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	a.previewLedger(f, ledgerExample())
	a.editing = true
	tt := cardTester(a, f, 760, 720)

	// The buy is the only row of its kind while the form is folded, so the
	// click opens that row rather than a segment.
	if err := tt.Click("买入"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !a.tx.open || a.tx.id != 2 {
		t.Fatalf("the buy row did not open: %+v", a.tx)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(f.Txns) != 4 {
		t.Fatalf("rows = %d, want the buy removed", len(f.Txns))
	}
	if !tt.HasText("已删除这笔交易") || !tt.HasText("撤销") {
		t.Fatalf("the delete left no undo: %v", tt.Texts())
	}
	if err := tt.Click("撤销"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(f.Txns) != 5 {
		t.Fatalf("rows = %d, want the buy back", len(f.Txns))
	}
}

func TestABrokenLedgerCanBePutRight(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	a.previewLedger(f, []ledger.Txn{
		ledger.Opening(1000, 1),
		{ID: 2, Date: "2026-01-02", Kind: ledger.Sell, Shares: 2000, Price: 1},
	})
	if f.ledgerErr == nil {
		t.Fatal("the sample should begin with a problem")
	}
	a.editing = true
	tt := cardTester(a, f, 760, 720)
	if !tt.HasText("交易记录有误：卖出份额超过当日持有") {
		t.Fatalf("the warning is missing: %v", tt.Texts())
	}

	if err := tt.Click("卖出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !a.tx.open || a.tx.id != 2 {
		t.Fatalf("the bad sale did not open: %+v", a.tx)
	}
	if err := tt.Click("份额输入"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("500")
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if f.ledgerErr != nil || f.Shares != 500 {
		t.Fatalf("position = %v, error = %v; want the rows put right", f.Shares, f.ledgerErr)
	}
}

func TestLedgerOpeningRowEdits(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	a.previewLedger(f, ledgerExample())
	a.editing = true
	tt := cardTester(a, f, 760, 720)

	if err := tt.Click("期初"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !a.tx.open || a.tx.id != 1 {
		t.Fatalf("the opening row did not open: %+v", a.tx)
	}
	for _, text := range []string{"期初持仓", "成本价输入", "成本合计"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}

	if err := tt.Click("成本价输入"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("0.7")
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, row := range f.Txns {
		if row.ID == 1 && (row.Kind != ledger.Open || row.Price != 0.7) {
			t.Fatalf("opening row = %+v, want the new cost", row)
		}
	}
}

func TestBackToManualKeepsThePosition(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	a.previewLedger(f, ledgerExample())
	shares, cost := f.Shares, f.Cost
	if err := store.SaveLedger(a.dir, f.Code, ledgerExample()); err != nil {
		t.Fatal(err)
	}

	a.toManual(f)
	if f.ledgered() || len(f.Txns) != 0 || len(f.steps) != 0 {
		t.Fatalf("the rows stayed after the switch: %+v", f)
	}
	if f.Shares != shares || f.Cost != cost {
		t.Fatalf("position = %v at %v, want %v at %v", f.Shares, f.Cost, shares, cost)
	}
	// The rows that went are set aside, and the toast puts them back.
	if a.txUndo.code == "" || len(a.txUndo.txns) != 5 {
		t.Fatal("the switch should leave an undo")
	}
	if shelvedRows(t, a.dir, f.Code) != 5 {
		t.Fatal("the switch should set the rows aside")
	}
	a.undoTxn()
	if !f.ledgered() || len(f.Txns) != 5 || !hasLedgerFile(a.dir, f.Code) {
		t.Fatalf("the undo did not put the rows back: %+v", f)
	}
}

// A fund with no shares starts its ledger with the transaction itself: there
// is no position to write an opening row for.
func TestTheFirstRowFromZeroHasNoOpening(t *testing.T) {
	f := &Fund{Code: "161725"}
	a := New(t.TempDir())
	a.all = []*Fund{f}
	a.rebuildList()
	a.beginTxn(f)
	a.tx.sharesText = "1000"
	a.tx.priceText = "0.5"
	a.commitTxn(f)
	if len(f.steps) != 1 || f.steps[0].Txn.Kind != ledger.Buy {
		t.Fatalf("steps = %+v, want the buy alone", f.steps)
	}
}

func TestProfitTipNamesEachPart(t *testing.T) {
	pos, _, err := ledger.Replay(ledgerExample())
	if err != nil {
		t.Fatal(err)
	}
	f := &Fund{
		Txns: ledgerExample(), pos: pos, Shares: pos.Shares, Cost: pos.Cost,
		Navs: []model.Nav{{Date: "2026-01-06", Unit: 0.7}},
	}
	tip := profitTip(f)
	for _, want := range []string{
		"持有收益 ", "已实现收益 +", "现金分红 +220.00",
		"累计投入 ", "含手续费",
	} {
		if !strings.Contains(tip, want) {
			t.Fatalf("tip = %q, want %q", tip, want)
		}
	}
	if profitTip(&Fund{Shares: 1, Cost: 1}) != "" {
		t.Fatal("a manual fund has no ledger story to tell")
	}
}

func TestHoldingHeaderStates(t *testing.T) {
	a := preview(t)
	f := a.funds[0]

	f.Shares, f.Cost, f.Txns, f.steps = 0, 0, nil, nil
	tt := cardTester(a, f, 720, 220)
	if !tt.HasText("未录入") || !tt.HasText("录入持仓") {
		t.Fatalf("an empty manual fund reads %v", tt.Texts())
	}

	f.Shares, f.Cost = 12000, 0.62
	tt = cardTester(a, f, 720, 220)
	if !tt.HasText("12,000 份 · 成本 0.6200") || !tt.HasText("编辑持仓") {
		t.Fatalf("a manual holding reads %v", tt.Texts())
	}

	a.previewLedger(f, ledgerExample())
	tt = cardTester(a, f, 720, 220)
	if !tt.HasText("11,350 份 · 成本") || !tt.HasText("5 笔交易") || !tt.HasText("交易记录") {
		t.Fatalf("a ledger holding reads %v", tt.Texts())
	}

	a.previewLedger(f, []ledger.Txn{
		ledger.Opening(1000, 1),
		{ID: 2, Date: "2026-01-02", Kind: ledger.Sell, Shares: 1000, Price: 1.5},
		{ID: 3, Date: "2026-01-03", Kind: ledger.Dividend, Amount: 50},
	})
	tt = cardTester(a, f, 720, 220)
	if !tt.HasText("已清仓 · 累计收益 +550.00 · 3 笔交易") || !tt.HasText("交易记录") {
		t.Fatalf("a sold-out ledger reads %v", tt.Texts())
	}
}

// A file that could not be read falls back to the last saved position and says
// where the broken copy went.
func TestAnUnreadableLedgerIsExplained(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	f.Shares, f.Cost = 12000, 0.62
	f.ledgerErr = store.ErrLedgerUnreadable
	a.editing = true
	tt := cardTester(a, f, 760, 400)

	const want = "交易记录文件损坏，已另存为 .bad 文件；当前显示的是上次保存的份额与成本"
	if !tt.HasText(want) {
		t.Fatalf("the broken file is not explained: %v", tt.Texts())
	}
	if !tt.HasText("持有份额输入") {
		t.Fatalf("the saved position is not editable: %v", tt.Texts())
	}
}

// eightRows is a sound ledger long enough to push the form and the list over
// a short window.
func eightRows() []ledger.Txn {
	txns := []ledger.Txn{ledger.Opening(12000, 0.62)}
	dates := []string{
		"2026-01-05", "2026-02-05", "2026-03-05", "2026-04-05",
		"2026-05-05", "2026-06-05", "2026-07-05",
	}
	for i, date := range dates {
		txns = append(txns, ledger.Txn{
			ID: i + 2, Date: date, Kind: ledger.Buy,
			Shares: 500 + float64(i)*100, Price: 0.6 + float64(i)*0.01, Fee: 0.5,
		})
	}
	return txns
}

func TestLedgerFormFitsTheWindow(t *testing.T) {
	for _, size := range []struct{ w, h int }{{900, 600}, {1080, 840}} {
		for _, dark := range []bool{false, true} {
			name := fmt.Sprintf("%dx%d/%s", size.w, size.h,
				map[bool]string{false: "light", true: "dark"}[dark])
			t.Run(name, func(t *testing.T) {
				a := preview(t)
				f := a.funds[0]
				a.previewLedger(f, eightRows())
				a.editing = true
				a.beginTxn(f)
				tt := ui.NewTesterWith(a.View, size.w, size.h,
					ui.TesterOptions{Dark: dark})
				tt.Frame()
				tt.Advance(time.Second)
				tt.Frame()

				for _, text := range []string{
					"交易日期", "份额输入", "成交净值输入", "保存", "取消",
				} {
					r := requireBox(t, tt, text)
					if r.X < 0 || r.X+r.W > float32(size.w) ||
						r.Y < 0 || r.Y+r.H > float32(size.h) {
						t.Errorf("%s is clipped: %+v", text, r)
					}
					if r.H > 32 {
						t.Errorf("%s wraps vertically: %+v", text, r)
					}
				}
			})
		}
	}
}
