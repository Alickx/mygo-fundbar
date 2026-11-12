package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
)

func day(s string) time.Time { return parseDay(s) }

func TestCompareSeriesAlignsAtItsOwnFirstValue(t *testing.T) {
	older := &Fund{Code: "A", Name: "老基金", Navs: []model.Nav{
		{Date: "2025-12-20", Unit: 2.0},
		{Date: "2026-01-02", Unit: 1.0},
		{Date: "2026-01-06", Unit: 1.1},
	}}
	younger := &Fund{Code: "B", Name: "新基金", Navs: []model.Nav{
		{Date: "2026-01-05", Unit: 3.0},
		{Date: "2026-01-08", Unit: 3.3},
	}}
	gone := &Fund{Code: "C", Name: "窗口前", Navs: []model.Nav{
		{Date: "2025-12-01", Unit: 1.0},
	}}
	lines := compareSeries(day("2026-01-01"), day("2026-01-10"),
		[]*Fund{older, younger, gone})
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want the two funds with values in the window", len(lines))
	}
	if lines[0].Dates[0].Format("2006-01-02") != "2026-01-02" || lines[0].Pcts[0] != 0 {
		t.Fatalf("older starts at %v/%v, want its first value at 0%%",
			lines[0].Dates[0], lines[0].Pcts[0])
	}
	if got := lines[0].Pcts[len(lines[0].Pcts)-1]; abs(got-10) > 1e-9 {
		t.Fatalf("older change = %v, want 10", got)
	}
	// The younger fund starts later, at 0%, rather than being measured from
	// a value it never had.
	if lines[1].Dates[0].Format("2006-01-02") != "2026-01-05" || lines[1].Pcts[0] != 0 {
		t.Fatalf("younger starts at %v/%v, want its own first value at 0%%",
			lines[1].Dates[0], lines[1].Pcts[0])
	}
	if got := lines[1].Pcts[len(lines[1].Pcts)-1]; abs(got-10) > 1e-9 {
		t.Fatalf("younger change = %v, want 10", got)
	}
}

func TestCompareSeriesIgnoresADividendDrop(t *testing.T) {
	f := &Fund{Code: "D", Name: "分红基金", Navs: []model.Nav{
		{Date: "2026-01-02", Unit: 1.2},
		// The unit value fell by a twelfth, but the fund paid a dividend and
		// published a gain of one percent.
		{Date: "2026-01-05", Unit: 1.1, Pct: 1.0},
		{Date: "2026-01-06", Unit: 1.155, Pct: 5.0},
	}}
	lines := compareSeries(day("2026-01-01"), day("2026-01-10"), []*Fund{f})
	if len(lines) != 1 {
		t.Fatalf("lines = %d", len(lines))
	}
	if got := lines[0].Pcts[1]; got <= 0 {
		t.Fatalf("the dividend dented the line: %v", got)
	}
	if got := lines[0].Pcts[2]; abs(got-6.05) > 1e-9 {
		t.Fatalf("change after the dividend = %v, want 6.05", got)
	}
}

func TestCompareToggleKeepsThree(t *testing.T) {
	a := compareApp(t)
	for i := 1; i < len(a.all); i++ {
		a.toggleCompare(a.all[i].Code)
	}
	if len(a.compare) != maxCompare {
		t.Fatalf("compare = %v, want three", a.compare)
	}
	// The chosen fund is never compared with itself.
	a.clearCompare()
	a.toggleCompare(a.all[0].Code)
	if len(a.compare) != 0 {
		t.Fatalf("chose the selected fund as a comparison: %v", a.compare)
	}
	// Choosing a fund already there takes it out again.
	a.toggleCompare("008270")
	a.toggleCompare("008270")
	if len(a.compare) != 0 {
		t.Fatalf("a second choice did not remove the fund: %v", a.compare)
	}
}

func TestCompareMenuChoosesAndClears(t *testing.T) {
	a := compareApp(t)
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	if err := tt.Click("对比"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); len(menu) < 4 {
		t.Fatalf("compare menu = %v, want the other funds", menu)
	}
	if err := tt.ChooseMenuItem("大成睿享混合C"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.inCompare("008270") {
		t.Fatalf("the menu did not choose the fund: %v", a.compare)
	}
	if !tt.HasText("对比 1") {
		t.Fatalf("the button does not count the comparison: %v", tt.Texts())
	}

	if err := tt.Click("对比 1"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("海富通欣利混合C"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.inCompare("011555") || !tt.HasText("对比 2") {
		t.Fatalf("second choice = %v, texts %v", a.compare, tt.Texts())
	}

	if err := tt.Click("对比 2"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("清除对比"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.compare) != 0 || tt.HasText("对比 2") {
		t.Fatalf("清除对比 left %v", a.compare)
	}
}

func TestCompareLegendNamesEachFundAndChange(t *testing.T) {
	a := compareApp(t)
	a.compare = []string{"008270", "011555"}
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	window := windowOf(a.Selected(), a.period)
	lines := compareSeries(parseDay(window[0].Date),
		parseDay(window[len(window)-1].Date),
		[]*Fund{a.Selected(), a.byCode("008270"), a.byCode("011555")})
	if len(lines) != 3 {
		t.Fatalf("lines = %d", len(lines))
	}
	for _, line := range lines {
		if !tt.HasText(line.Name) {
			t.Fatalf("the legend does not name %q: %v", line.Name, tt.Texts())
		}
		if got := pct(line.Pcts[len(line.Pcts)-1]); !tt.HasText(got) {
			t.Fatalf("the legend does not show %s: %v", got, tt.Texts())
		}
	}
}

func TestCompareDropsARemovedFund(t *testing.T) {
	a := compareApp(t)
	a.compare = []string{"008270", "011555"}
	row := -1
	for i, f := range a.funds {
		if f.Code == "008270" {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("the compared fund is not in the list")
	}
	a.Remove(row)
	if a.inCompare("008270") || !a.inCompare("011555") {
		t.Fatalf("remove left %v", a.compare)
	}
}

func TestChoosingAComparedFundTakesItOut(t *testing.T) {
	a := compareApp(t)
	a.compare = []string{"008270", "011555"}
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	// The list sets the chosen row directly; choose does the same for a
	// test that does not have to find the row.
	a.choose("011555")
	tt.Frame()
	if a.inCompare("011555") {
		t.Fatalf("the chosen fund stayed in the comparison: %v", a.compare)
	}
	if !a.inCompare("008270") {
		t.Fatalf("the comparison was not kept: %v", a.compare)
	}
}

func TestCompareLegendAndButtonFitTheWindow(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			for _, size := range []struct{ w, h int }{{900, 600}, {1080, 840}} {
				a := compareApp(t)
				a.compare = []string{"008270", "011555", "021528"}
				tt := ui.NewTesterWith(a.View, size.w, size.h,
					ui.TesterOptions{Dark: dark})
				tt.Frame()

				inside := func(text string) ui.Rect {
					t.Helper()
					r := requireBox(t, tt, text)
					if r.X < a.sidebarWidth || r.X+r.W > float32(size.w)+0.5 ||
						r.Y < 0 || r.Y+r.H > float32(size.h)+0.5 {
						t.Errorf("%dx%d %q is outside the pane: %+v",
							size.w, size.h, text, r)
					}
					if r.H > 32 {
						t.Errorf("%dx%d %q wraps vertically: %+v",
							size.w, size.h, text, r)
					}
					return r
				}
				inside("对比 3")
				for _, line := range compareSeries(
					parseDay(windowOf(a.Selected(), a.period)[0].Date),
					parseDay(lastDate(windowOf(a.Selected(), a.period))),
					[]*Fund{a.Selected(), a.byCode("008270"),
						a.byCode("011555"), a.byCode("021528")}) {
					// The names also sit in the sidebar, so the legend's own
					// line is checked through its change, which only it shows.
					if !tt.HasText(line.Name) {
						t.Fatalf("the legend lost %q", line.Name)
					}
					inside(pct(line.Pcts[len(line.Pcts)-1]))
				}
				// Without a comparison the info line still holds the button.
				a.clearCompare()
				tt.Frame()
				inside("对比")
				if !tt.HasText("近6月净值变化") {
					t.Fatalf("the info line went missing: %v", tt.Texts())
				}
			}
		})
	}
}

// lastDate is a window's newest date.
func lastDate(navs []model.Nav) string {
	if len(navs) == 0 {
		return ""
	}
	return navs[len(navs)-1].Date
}

// compareApp holds funds with long weekly histories: the chosen one, three
// friends, a young fund and one outside the holding group. The names and
// codes are the fixture's, so the screenshots and the tests read alike.
func compareApp(t *testing.T) *App {
	t.Helper()
	end := day("2026-11-11")
	navs := func(first time.Time, unit float64, step float64) []model.Nav {
		var out []model.Nav
		for i := 0; first.AddDate(0, 0, i*5).Before(end) || first.AddDate(0, 0, i*5).Equal(end); i++ {
			out = append(out, model.Nav{
				Date: first.AddDate(0, 0, i*5).Format("2006-01-02"),
				Unit: unit + float64(i)*step,
			})
		}
		return out
	}
	mk := func(code, name string, shares float64, groups []string,
		first time.Time, unit, step float64) *Fund {
		return &Fund{Code: code, Name: name, Shares: shares, Cost: unit,
			Groups: groups, Loaded: true, Navs: navs(first, unit, step)}
	}
	holding := []string{holdingGroup}
	funds := []*Fund{
		mk("007760", "景顺长城红利", 12000, holding, day("2025-01-01"), 1.0, 0.001),
		mk("008270", "大成睿享混合C", 5000, holding, day("2025-01-01"), 2.0, 0.002),
		mk("011555", "海富通欣利混合C", 3000, holding, day("2025-01-01"), 1.5, -0.001),
		mk("021528", "财通成长优选混合C", 0, []string{"g1"}, day("2025-01-01"), 3.0, 0.004),
		mk("028678", "诺安进取回报混合C", 0, holding, day("2026-08-14"), 2.2, -0.005),
	}
	a := New(t.TempDir())
	a.all = funds
	a.sidebarWidth = 348
	a.period = 2
	a.selCode[holdingGroup] = "007760"
	a.rebuildList()
	return a
}
