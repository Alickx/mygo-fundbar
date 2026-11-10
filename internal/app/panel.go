package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// panelRows is how many funds the preview lists; the rest wait in the
// window, as the preview stays small.
const panelRows = 8

// Panel builds the preview the menu bar item opens: the funds the user
// follows, how today went, and the way to the window. It says what one
// click can read; the window keeps the details.
func (a *App) Panel(c *ui.Context) {
	t := theme(c)
	c.SetTheme(t)
	c.Root().Background(ui.Transparent)
	a.applyPending()
	if c.Shortcut(0, ui.KeyEscape) && a.groupEditing {
		a.cancelGroupEdit()
	}

	// The system draws a popover's material, corners and edge; elsewhere
	// the content draws its own.
	box := ui.Column(c).FillWidth()
	if !c.Vibrancy() {
		box.Background(t.Surface).Radius(10).Border(1, t.Border)
	}
	box.Padding(12).Gap(9).Children(func() {
		a.panelHead(c, t)
		w, _ := c.Size()
		a.groupBar(c, t, w-24)
		ui.Divider(c)
		if len(a.funds) == 0 {
			ui.Column(c).Gap(3).Padding(4, 2).Children(func() {
				if a.group == holdingGroup {
					ui.Text(c, "还没有自选基金").FontSize(12.5).FontWeight(500)
					ui.Text(c, "在主面板里搜索代码或名称添加").FontSize(11).
						TextColor(t.TextMuted)
					return
				}
				ui.Text(c, "「"+a.groupName(a.group)+"」还没有基金").
					FontSize(12.5).FontWeight(500).SingleLine().Ellipsis("…").
					Tooltip(a.groupName(a.group))
				ui.Text(c, "在主面板里搜索代码或名称加入").
					FontSize(11).TextColor(t.TextMuted)
			})
		} else {
			ui.Column(c).Gap(1).Children(func() {
				for i, f := range a.funds {
					if i >= panelRows {
						break
					}
					a.panelRow(c, t, f)
				}
			})
			if more := len(a.funds) - panelRows; more > 0 {
				ui.Textf(c, "还有 %d 只，在主面板查看", more).FontSize(11).
					TextColor(t.TextMuted).Padding(2, 8)
			}
		}
		a.panelFoot(c, t)
	})
}

// panelHead is the day's gain, as the summary bar of the window says it.
func (a *App) panelHead(c *ui.Context, t *ui.Theme) {
	s := a.GroupSummary(a.group)
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "基金估值").FontSize(13).FontWeight(600).NoWrap().Shrink(0)
		ui.Spacer(c)
		ui.Text(c, a.freshNote()).FontSize(11).TextColor(t.TextMuted).
			FontFeatures("tnum").NoWrap().Tooltip(a.updatedText())
	})
	ui.Row(c).AlignItems(ui.Baseline).Gap(7).Children(func() {
		switch {
		case s.Holding:
			ui.Text(c, "今日收益").FontSize(11.5).TextColor(t.TextMuted).NoWrap()
			ui.Text(c, signedMoney(s.Profit)).FontSize(18).FontWeight(600).
				TextColor(up(t, s.Profit)).FontFeatures("tnum").NoWrap()
			ui.Text(c, pct(s.Pct)).FontSize(11.5).TextColor(up(t, s.Pct)).
				FontFeatures("tnum").NoWrap()
		case len(a.funds) > 0:
			label := "自选平均"
			if a.group != holdingGroup {
				label = "分组平均"
			}
			ui.Text(c, label).FontSize(11.5).TextColor(t.TextMuted).NoWrap()
			ui.Text(c, pct(s.Pct)).FontSize(18).FontWeight(600).
				TextColor(up(t, s.Pct)).FontFeatures("tnum").NoWrap()
		default:
			label := "还没有自选基金"
			if a.group != holdingGroup {
				label = "分组还没有基金"
			}
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		if len(a.funds) > 0 {
			ui.Textf(c, "涨 %d · 跌 %d", s.Up, s.Down).FontSize(11).
				TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
		}
	})
}

// panelRow is one fund of the preview: the day's change, and a click that
// opens the window on it.
func (a *App) panelRow(c *ui.Context, t *ui.Theme, f *Fund) {
	change := f.ChangePct()
	tint := up(t, change)
	r := ui.Row(c.Key(f.Code)).Padding(5, 7).Radius(t.Radius).Gap(9).
		AlignItems(ui.Center).FillWidth().Cursor(ui.CursorPointer)
	if r.Hovered() {
		r.Background(t.SurfaceHover)
	}
	r.Transition(ui.ElementTransition{Duration: 120 * time.Millisecond})
	r.Children(func() {
		ui.Box(c).Size(50, 32).Shrink(0).Radius(8).
			Background(tint.Alpha(0.13)).Center().Children(func() {
			text := "—"
			if f.Loaded || f.HasEst {
				text = pct(change)
			}
			ui.Text(c, text).FontSize(11).FontWeight(600).TextColor(tint).
				FontFeatures("tnum").NoWrap()
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
			name := f.Name
			if name == "" {
				name = f.Code
			}
			ui.Text(c, name).FontSize(12.5).FontWeight(500).SingleLine().
				Ellipsis("…").Tooltip(name)
			sub := f.Code
			if f.Err != "" {
				sub += " · 读取失败"
			}
			ui.Text(c, sub).FontSize(10.5).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…")
		})
		// The shares the user holds are what the preview is for; a fund
		// they only follow says so by leaving the space empty, and a made
		// group never shows them.
		if a.holds(f) {
			profit := f.TodayProfit()
			ui.Column(c).AlignItems(ui.End).Shrink(0).Gap(1).Children(func() {
				ui.Text(c, signedMoney(profit)).FontSize(11.5).FontWeight(600).
					TextColor(up(t, profit)).FontFeatures("tnum").NoWrap()
				ui.Text(c, "市值 "+money(f.Value())).FontSize(10.5).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			})
		}
	})
	if r.Clicked() {
		a.afterFrame(func() { a.ShowFund(f.Code) })
	}
}

// panelFoot is the way to the window, and the way to newer numbers.
func (a *App) panelFoot(c *ui.Context, t *ui.Theme) {
	ui.Divider(c)
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		if a.busy {
			ui.Spinner(c).Size(11, 11)
		}
		if ui.Button(c, "刷新").Shrink(0).Disabled(a.busy).Clicked() {
			a.refresh()
		}
		ui.Spacer(c)
		if ui.Button(c, "打开主面板").Shrink(0).Clicked() {
			a.afterFrame(a.Show)
		}
	})
}
