package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// panelTester frames the preview the menu bar item opens.
func panelTester(a *App) *ui.Tester {
	return ui.NewTester(a.Panel, 320, 520)
}

func TestPreviewListsTheFunds(t *testing.T) {
	a := preview(t)
	tt := panelTester(a)
	tt.Frame()
	want := []string{
		"基金估值",
		"招商中证白酒指数(LOF)A", "161725",
		"易方达蓝筹精选混合", "005827",
		pct(a.Summary().Pct),
		pct(a.funds[0].ChangePct()),
		pct(a.funds[1].ChangePct()),
		signedMoney(a.funds[0].TodayProfit()),
		"刷新", "打开主面板",
	}
	for _, text := range want {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
}

func TestPreviewOpensTheWindowOnAFund(t *testing.T) {
	a := preview(t)
	tt := panelTester(a)
	tt.Frame()
	if err := tt.Click("易方达蓝筹精选混合"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.want != "005827" {
		t.Fatalf("want %q, expected the fund the row stands for", a.want)
	}
}

func TestPreviewKeepsTheRestForTheWindow(t *testing.T) {
	a := preview(t)
	for i := 0; i < panelRows; i++ {
		a.funds = append(a.funds, &Fund{Code: fmt.Sprintf("1000%02d", i)})
	}
	tt := panelTester(a)
	tt.Frame()
	if !tt.HasText(fmt.Sprintf("还有 %d 只，在主面板查看", len(a.funds)-panelRows)) {
		t.Fatalf("missing the note about the rest in %v", tt.Texts())
	}
}

func TestPreviewSaysWhenNothingIsFollowed(t *testing.T) {
	a := preview(t)
	a.funds = nil
	tt := panelTester(a)
	tt.Frame()
	for _, text := range []string{"还没有自选基金", "打开主面板"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
}
