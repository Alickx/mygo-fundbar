package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
	"fundbar/internal/source"
	"fundbar/internal/store"
)

// quarterReports is a history of two quarters, newest first, of the fund
// preview builds: the newer one holds 贵州茅台, the older one 五粮液.
func quarterReports() []source.Report {
	return []source.Report{
		{Label: "2026年3季度", Date: "2026-09-30", Holdings: []model.Holding{
			{Symbol: "sh600519", Code: "600519", Name: "贵州茅台", Weight: 18.54},
		}},
		{Label: "2026年2季度", Date: "2026-06-30", Holdings: []model.Holding{
			{Symbol: "sz000858", Code: "000858", Name: "五粮液", Weight: 9.05},
		}},
	}
}

// snapshotOf is what a read of the fund returns, with the reports.
func snapshotOf(f *Fund, reports []source.Report) *store.Snapshot {
	return &store.Snapshot{
		Fund:     &source.Fund{Name: f.Name, Navs: f.Navs},
		Holdings: f.Holdings, Report: f.Report, Reports: reports,
	}
}

func TestHoldingsCardListsTheChosenQuarter(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	f.History = quarterReports()
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	for _, text := range []string{"2026年3季度", "贵州茅台", "今日涨跌", "覆盖"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}

	if err := tt.Click("2026年3季度"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	if len(menu) != 2 {
		t.Fatalf("quarter menu = %v, want both quarters", menu)
	}
	if err := tt.ChooseMenuItem("2026年2季度 · 2026-06-30"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.ReportIdx != 1 {
		t.Fatalf("the menu chose quarter %d, want 1", f.ReportIdx)
	}
	if !tt.HasText("2026年2季度") || !tt.HasText("五粮液") {
		t.Fatalf("the card does not list the older quarter: %v", tt.Texts())
	}
	if tt.HasText("贵州茅台") {
		t.Fatalf("the newest quarter's stock is still listed: %v", tt.Texts())
	}
	for _, text := range []string{"今日涨跌", "覆盖 27.6%", "估值偏差", "2026年3季度"} {
		if tt.HasText(text) {
			t.Fatalf("%q describes the newest quarter alone, but is shown", text)
		}
	}

	// The newest quarter is one menu item away again.
	if err := tt.Click("2026年2季度"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("2026年3季度 · 2026-09-30"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.ReportIdx != 0 || !tt.HasText("贵州茅台") || !tt.HasText("今日涨跌") {
		t.Fatalf("back to the newest quarter: idx %d, texts %v", f.ReportIdx, tt.Texts())
	}
}

func TestFillKeepsTheHistoryAcrossACache(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	reports := quarterReports()
	a.fill(f, snapshotOf(f, reports))
	if len(f.History) != 2 || f.ReportIdx != 0 {
		t.Fatalf("history = %d reports, quarter %d", len(f.History), f.ReportIdx)
	}

	// A cache carries no history: the one in memory stays, with the quarter
	// the user chose.
	f.ReportIdx = 1
	a.fill(f, snapshotOf(f, nil))
	if len(f.History) != 2 || f.ReportIdx != 1 {
		t.Fatalf("the cache wiped the history: %d reports, quarter %d",
			len(f.History), f.ReportIdx)
	}

	// The same reports read again leave the choice where it is.
	a.fill(f, snapshotOf(f, quarterReports()))
	if f.ReportIdx != 1 {
		t.Fatalf("the same reports reset the quarter to %d", f.ReportIdx)
	}

	// A new report opens the card at the newest quarter again.
	newest := append([]source.Report{
		{Label: "2026年4季度", Date: "2026-12-31", Holdings: reports[0].Holdings},
	}, reports...)
	a.fill(f, snapshotOf(f, newest))
	if len(f.History) != 3 || f.ReportIdx != 0 {
		t.Fatalf("a new report left %d reports, quarter %d",
			len(f.History), f.ReportIdx)
	}
}

func TestShownHoldingsFallBackToTheNewestQuarter(t *testing.T) {
	f := &Fund{
		Holdings: []model.Holding{{Name: "甲", Weight: 3}},
		Report:   "2026-09-30",
	}
	if f.ShowingHistory() || f.ShownReport() != "2026-09-30" ||
		f.ShownHoldings()[0].Name != "甲" {
		t.Fatalf("a fund without a history shows %q", f.ShownReport())
	}

	f.History = quarterReports()
	if f.ShowingHistory() || f.ShownReport() != "2026-09-30" {
		t.Fatal("the newest quarter is a history quarter")
	}
	f.ReportIdx = 7 // out of range, as a shortened history leaves it
	if f.ShowingHistory() {
		t.Fatal("an index past the history shows a quarter")
	}
	f.ReportIdx = 1
	if !f.ShowingHistory() || f.ShownReport() != "2026-06-30" ||
		f.ShownHoldings()[0].Name != "五粮液" {
		t.Fatalf("quarter 1 shows %q: %+v", f.ShownReport(), f.ShownHoldings())
	}
}
