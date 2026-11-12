package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
	"fundbar/internal/store"
)

func TestPortfolioButtonNeedsHeldSharesInTheHoldingGroup(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.View, 1080, 840)
	tt.Frame()
	if !tt.HasText("组合透视") {
		t.Fatalf("no look-through button, got %v", tt.Texts())
	}

	// A made group lists funds without shares: there is no portfolio there.
	a.groups = []store.Group{{ID: "g1", Name: "关注"}}
	a.all[1].Groups = []string{"g1"}
	a.SelectGroup("g1")
	tt.Frame()
	if tt.HasText("组合透视") {
		t.Fatal("the look-through button showed in a made group")
	}

	// The holding group without any shares has nothing to add up either.
	a.SelectGroup(holdingGroup)
	for _, f := range a.all {
		f.Shares = 0
	}
	tt.Frame()
	if tt.HasText("组合透视") {
		t.Fatal("the look-through button showed without held shares")
	}
}

func TestPortfolioButtonShowsThePaneAndReturnsToTheFund(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.View, 1080, 1000)
	tt.Frame()
	if err := tt.Click("组合透视"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.portfolio {
		t.Fatal("the button did not show the look-through")
	}
	if !tt.HasText("返回基金") {
		t.Fatalf("the button does not offer the way back: %v", tt.Texts())
	}
	for _, text := range []string{"资产分布", "行业分布", "重仓股穿透"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in the pane", text)
		}
	}
	// The second fund reports no holdings, so the two funds share nothing.
	if !tt.HasText("持有的基金之间没有相同的重仓股") {
		t.Fatalf("expected the no-overlap note, got %v", tt.Texts())
	}
	// The list chooses a scratch row while the pane shows, so no fund looks
	// chosen.
	if a.list.Selected == nil || *a.list.Selected != -1 || a.listSel != -1 {
		t.Fatalf("a row looks chosen: selected = %v, scratch = %d",
			a.list.Selected, a.listSel)
	}

	if err := tt.Click("返回基金"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.portfolio {
		t.Fatal("返回基金 did not leave the look-through")
	}
	if !tt.HasText("净值走势") || tt.HasText("资产分布") {
		t.Fatalf("the fund's detail did not come back: %v", tt.Texts())
	}
}

func TestPortfolioSidebarRowLeavesThePane(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.View, 1080, 1000)
	tt.Frame()
	if err := tt.Click("组合透视"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	// Another fund in the sidebar leaves the pane for that fund.
	if err := tt.Click("易方达蓝筹精选混合"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.portfolio || a.sel != 1 {
		t.Fatalf("portfolio = %v, selected = %d", a.portfolio, a.sel)
	}

	// So does the fund that was chosen all along: while the pane showed it
	// was not marked, but a click on it is a choice like any other.
	if err := tt.Click("组合透视"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("易方达蓝筹精选混合"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.portfolio || a.sel != 1 {
		t.Fatalf("the same row did not leave the pane: portfolio = %v, selected = %d",
			a.portfolio, a.sel)
	}
}

func TestPortfolioLeavesWhenTheGroupOrTheSharesGo(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.View, 1080, 840)
	tt.Frame()
	if err := tt.Click("组合透视"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	// Without shares there is nothing to look through.
	for _, f := range a.all {
		f.Shares = 0
	}
	tt.Frame()
	if a.portfolio {
		t.Fatal("the pane stayed without held shares")
	}

	for _, f := range a.all {
		f.Shares = 1000
	}
	a.portfolio = true
	a.groups = []store.Group{{ID: "g1", Name: "关注"}}
	a.all[1].Groups = []string{"g1"}
	a.SelectGroup("g1")
	tt.Frame()
	if a.portfolio {
		t.Fatal("the pane stayed after another group was chosen")
	}
}

func TestPortfolioPaneFitsBothThemesAndSizes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			a := portfolioLayoutApp(t)
			a.portfolio = true
			for _, size := range []struct{ w, h int }{{900, 600}, {1080, 840}} {
				tt := ui.NewTesterWith(a.View, size.w, size.h, ui.TesterOptions{Dark: dark})
				tt.Frame()
				check := func(texts ...string) {
					t.Helper()
					for _, text := range texts {
						r := requireBox(t, tt, text)
						if r.X < a.sidebarWidth || r.X+r.W > float32(size.w)+0.5 {
							t.Errorf("%dx%d %q is outside the pane: %+v", size.w, size.h, text, r)
						}
					}
				}
				check("组合透视", "资产分布", "行业分布", "重仓股穿透",
					"组合占比", "持有基金")
				// The long industry name is ellipsised, so the bar and the
				// number after it keep their place.
				r := requireBox(t, tt, portfolioLongIndustry)
				if r.X < a.sidebarWidth || r.X+r.W > float32(size.w)+0.5 {
					t.Errorf("%dx%d long industry is outside the pane: %+v", size.w, size.h, r)
				}
				if r.W > float32(size.w)-a.sidebarWidth {
					t.Errorf("%dx%d long industry was not held to the pane: %+v", size.w, size.h, r)
				}
				// The last card waits below the fold at the smaller sizes.
				tt.Scroll(a.sidebarWidth+120, float32(size.h)*0.7, 0, 4000)
				tt.Frame()
				check("基金重合度")
			}
		})
	}
}

const portfolioLongIndustry = "一只名称非常非常非常长的行业分类用于检查省略号"

// portfolioLayoutApp holds three funds whose reports fill every card: enough
// industries for the more note, a shared stock for the overlap, and a name
// long enough to need an ellipsis.
func portfolioLayoutApp(t *testing.T) *App {
	t.Helper()
	nav := []model.Nav{{Date: "2026-11-11", Unit: 1}}
	holdings := func(prefix string, top float64) []model.Holding {
		out := make([]model.Holding, 0, 10)
		for i := 0; i < 10; i++ {
			out = append(out, model.Holding{
				Symbol: prefix + "00" + string(rune('1'+i)),
				Name:   "股票" + string(rune('A'+i)),
				Weight: top - float64(i),
			})
		}
		return out
	}
	industries := func(prefix string, n int) []model.Industry {
		out := make([]model.Industry, 0, n)
		out = append(out, model.Industry{Name: portfolioLongIndustry, Weight: 12})
		for i := 0; i < n-1; i++ {
			out = append(out, model.Industry{
				Name: prefix + "行业" + string(rune('A'+i)), Weight: 10 - float64(i),
			})
		}
		return out
	}
	a1 := insightFund("007760", "景顺长城沪港深红利成长低波指数C类份额", 12000, 1.2367,
		holdings("sh", 10), industries("制造", 8))
	a2 := insightFund("008270", "大成睿享混合C", 5000, 1.7032,
		holdings("sz", 9), industries("金融", 5))
	a3 := insightFund("011555", "海富通欣利混合C", 3000.5, 1.4336,
		holdings("bj", 8), industries("服务", 5))
	// One company in two funds: the stock row counts them and the overlap
	// card names the pair.
	a2.Holdings = append(a2.Holdings, model.Holding{
		Symbol: "sh001", Name: "股票A", Weight: 3,
	})
	a1.Quotes = map[string]model.Quote{
		"sh001": {Symbol: "sh001", Pct: 1.23},
	}
	a2.Quotes = map[string]model.Quote{
		"sh001": {Symbol: "sh001", Pct: 1.23},
	}
	for _, f := range []*Fund{a1, a2, a3} {
		f.Navs = nav
	}
	a := New(t.TempDir())
	a.all = []*Fund{a1, a2, a3}
	a.sidebarWidth = 348
	a.rebuildList()
	a.sel = 0
	return a
}
