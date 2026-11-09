package app

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// Exercise the window's actual layout and input, including a restored wide
// sidebar. Bounds catch clipped controls that a successful render alone misses.
func TestControlsSurviveWindowAndSidebarResize(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			a := preview(t)
			a.sidebarWidth = 460
			a.funds[0].Name = "景顺长城沪港深红利成长低波指数 C"
			a.updated = now()
			tt := ui.NewTesterWith(a.View, 1080, 840, ui.TesterOptions{Dark: dark})
			tt.Frame()
			tt.SetSize(900, 600)
			tt.Frame()

			check := func() {
				t.Helper()
				for _, text := range []string{"刷新", "涨跌分布", "净值走势", "近1月", "近1年", "编辑持仓"} {
					r := requireBox(t, tt, text)
					if r.X < a.sidebarWidth || r.X+r.W > 900 || r.Y < 0 || r.Y+r.H > 600 {
						t.Errorf("%s is clipped: %+v", text, r)
					}
					if r.H > 32 || r.W < float32(utf8.RuneCountInString(text))*8 {
						t.Errorf("%s is squeezed or wraps vertically: %+v", text, r)
					}
				}
			}
			check()

			// Try dragging the divider far beyond the detail pane's safe width.
			x := a.sidebarWidth
			tt.Press(x, 300)
			tt.Move(700, 300)
			tt.Release(700, 300)
			tt.Frame()
			check()
			if err := tt.Click("近1年"); err != nil {
				t.Fatal(err)
			}
			if a.period != 3 {
				t.Fatal("period control was visible but could not be selected")
			}
		})
	}
}

func TestHoldingEditorReturnsSpaceToChart(t *testing.T) {
	a := preview(t)
	a.sidebarWidth = 460
	tt := ui.NewTester(a.View, 900, 780)
	tt.Frame()
	before := requireBox(t, tt, "净值走势")
	if err := tt.Click("编辑持仓"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	shares := requireBox(t, tt, "持有份额输入")
	cost := requireBox(t, tt, "成本价输入")
	if shares.W < 150 || cost.W < 150 || shares.X+shares.W > cost.X || cost.X+cost.W > 876 {
		t.Fatalf("holding inputs overlap or are clipped: shares=%+v cost=%+v", shares, cost)
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	after := requireBox(t, tt, "净值走势")
	if a.editing || abs(float64(after.Y-before.Y)) > 0.5 {
		t.Fatalf("collapsed editor still takes space: before=%+v after=%+v", before, after)
	}
}

func TestMissingNavDoesNotClaimPublishedValue(t *testing.T) {
	a := New(t.TempDir())
	a.funds = []*Fund{{Code: "000001", Name: "等待加载的基金"}}
	tt := ui.NewTester(a.View, 900, 600)
	tt.Frame()
	if !tt.HasText("等待净值") || tt.HasText("已公布净值") {
		t.Fatal("missing data must not look like a published NAV")
	}
}

func requireBox(t *testing.T, tt *ui.Tester, text string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(text)
	if !ok || r.W <= 0 || r.H <= 0 {
		t.Fatalf("missing visible element %q: %+v", text, r)
	}
	return r
}
