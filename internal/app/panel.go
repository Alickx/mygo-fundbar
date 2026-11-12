package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
)

// panelRows is how many funds the preview lists; the rest wait in the
// window, as the preview stays small.
const panelRows = 8

// panelIndexSize is the text size of the benchmarks row, panelIndexGap the
// space between its parts, and panelIndicesHeight what the row adds to the
// flyout before it measures itself: the line and the gap above it.
const (
	panelIndexSize     = 10.5
	panelIndexGap      = 4
	panelIndicesHeight = 24
)

// panelRadius is how round the preview's corners are, close to the
// system's popover, which rounds the material behind them.
const panelRadius = 12

// Panel builds the preview the menu bar item opens: the funds the user
// follows, how today went, and the way to the window. It says what one
// click can read; the window keeps the details.
func (a *App) Panel(c *ui.Context) {
	t, glass := panelTheme(c)
	c.SetTheme(t)
	c.Root().Background(ui.Transparent)
	a.applyPending()
	if c.Shortcut(0, ui.KeyEscape) && a.groupEditing {
		a.cancelGroupEdit()
	}

	// The system draws a popover's material, corners and edge; elsewhere
	// the content draws its own. The popover's material takes its color
	// from what lies behind the window, which can wash the numbers grey:
	// over it the content keeps a glass of the app's own, which the
	// material still shows through.
	box := ui.Column(c).FillWidth()
	if glass.A > 0 {
		box.Background(glass).Radius(panelRadius)
	} else {
		box.Background(t.Surface).Radius(panelRadius).Border(1, t.Border)
	}
	box.Padding(12).Gap(9).Children(func() {
		a.panelHead(c, t)
		w, _ := c.Size()
		a.groupBar(c, t, w-24)
		ui.Divider(c)

		// A group lists other funds, and often a different number of
		// them: the list slides in from the side of the bar the group is
		// on, one page taking the place of another. The window takes its
		// new height at once — text that moved across the frames of an
		// animated height would blur, as the window scaled it — and the
		// fade of the slide covers the change.
		list := ui.Column(c).Gap(1)
		list.Children(func() {
			a.panelList(c, t)
		})
		if !a.panelSeeded {
			// The first frame of a preview shows a group, not a change of
			// groups: it takes the one it lists, and does not slide.
			a.panelSeeded = true
			a.panelShown = a.group
		} else if a.group != a.panelShown {
			a.panelDir = a.groupSide(a.panelShown, a.group)
			a.panelShown = a.group
			a.panelMoved = c.Now()
		}
		if into := a.panelInto(c); into < 1 {
			ease := ui.EaseOut(into)
			list.Translate(a.panelDir*(1-ease)*panelSlide, 0).
				Opacity(0.3 + 0.7*ease)
		}
		a.panelFoot(c, t)
	})
}

// panelSlide is how far, in DIPs, the list of a new group slides in, and
// panelSlideTime how long it takes.
const (
	panelSlide     = 20
	panelSlideTime = 200 * time.Millisecond
)

// panelList is the funds of the group, or what there is to say when it has
// none.
func (a *App) panelList(c *ui.Context, t *ui.Theme) {
	if len(a.funds) == 0 {
		ui.Column(c).Gap(3).Padding(4, 2).Children(func() {
			if a.group == holdingGroup {
				ui.Text(c, "还没有自选基金").FontSize(13).FontWeight(500)
				ui.Text(c, "在主面板里搜索代码或名称添加").FontSize(11).
					TextColor(t.TextMuted)
				return
			}
			ui.Text(c, "「"+a.groupName(a.group)+"」还没有基金").
				FontSize(13).FontWeight(500).SingleLine().Ellipsis("…").
				Tooltip(a.groupName(a.group))
			ui.Text(c, "在主面板里搜索代码或名称加入").
				FontSize(11).TextColor(t.TextMuted)
		})
		return
	}
	for i, f := range a.funds {
		if i >= panelRows {
			break
		}
		a.panelRow(c, t, f)
	}
	if more := len(a.funds) - panelRows; more > 0 {
		ui.Textf(c, "还有 %d 只，在主面板查看", more).FontSize(11).
			TextColor(t.TextMuted).Padding(8, 8, 2, 8)
	}
}

// groupSide is the side of the group bar that the list of group to comes
// from, 1 for the right and -1 for the left, as the bar orders the groups:
// a group to the right of the one listed takes its place from the right.
func (a *App) groupSide(from, to string) float32 {
	order := func(id string) int {
		if id == holdingGroup {
			return 0
		}
		for i, g := range a.groups {
			if g.ID == id {
				return i + 1
			}
		}
		return -1
	}
	if order(to) < order(from) {
		return -1
	}
	return 1
}

// panelInto is how far the slide of a new group's list has gone, from 0 to
// 1: 1 when the list has not moved, or has arrived.
func (a *App) panelInto(c *ui.Context) float32 {
	if a.panelMoved.IsZero() {
		return 1
	}
	into := float32(c.Now().Sub(a.panelMoved)) / float32(panelSlideTime)
	if into >= 1 {
		a.panelMoved = time.Time{}
		return 1
	}
	c.AnimationFrame()
	return max(into, 0)
}

// panelTheme is the preview's look: the window's, made to read on the
// glass the content keeps over the system's material (glass, transparent
// where the content draws its own background). Secondary text keeps more
// contrast than in the window, as the material is never as even as a
// surface.
func panelTheme(c *ui.Context) (t *ui.Theme, glass ui.Color) {
	base := *theme(c)
	t = &base
	if !c.Vibrancy() {
		return t, ui.Transparent
	}
	if t.Dark {
		// Numbers on a dark glass need the space around them to be theirs:
		// the material alone leaves them on whatever grey lies behind.
		glass = t.Background.Alpha(0.62)
		t.TextMuted = ui.Hex("#b6b6c4")
		t.Border = ui.Hex("#42424f")
		t.Surface = ui.Hex("#26262f")
		t.SurfaceHover = ui.Hex("#30303a")
		t.SurfacePressed = ui.Hex("#3a3a46")
	} else {
		glass = t.Background.Alpha(0.60)
		t.TextMuted = ui.Hex("#5f5f6d")
		t.Border = ui.Hex("#dcdce6")
	}
	return t, glass
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
			ui.Text(c, signedMoney(s.Profit)).FontSize(19).FontWeight(600).
				TextColor(up(t, s.Profit)).FontFeatures("tnum").NoWrap()
			ui.Text(c, pct(s.Pct)).FontSize(11.5).TextColor(up(t, s.Pct)).
				FontFeatures("tnum").NoWrap()
		case len(a.funds) > 0:
			label := "自选平均"
			if a.group != holdingGroup {
				label = "分组平均"
			}
			ui.Text(c, label).FontSize(11.5).TextColor(t.TextMuted).NoWrap()
			ui.Text(c, pct(s.Pct)).FontSize(19).FontWeight(600).
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
	a.panelIndices(c, t)
}

// panelIndices draws the benchmarks under the day's summary, when the last
// refresh read them and the setting shows them. The three names, prices and
// changes do not fit across 320 DIP, so the row drops the prices rather than
// wrap; a wider panel keeps them.
func (a *App) panelIndices(c *ui.Context, t *ui.Theme) {
	if !a.showIndices || len(a.indices) == 0 {
		return
	}
	avail := float32(0)
	if w, _ := c.Size(); w > 0 {
		avail = w - 24 // the panel box pads 12 DIP on each side
	}
	withPrices := indexRowWidth(c, a.indices, true) <= avail
	r := ui.Row(c).AlignItems(ui.Baseline).Gap(panelIndexGap)
	r.Children(func() {
		for i, q := range a.indices {
			if i > 0 {
				ui.Text(c, "·").FontSize(panelIndexSize).
					TextColor(t.TextMuted).NoWrap().Shrink(0)
			}
			ui.Text(c, indexLabel(q.Symbol)).FontSize(panelIndexSize).
				TextColor(t.TextMuted).NoWrap().Shrink(0)
			if withPrices {
				ui.Textf(c, "%.2f", q.Price).FontSize(panelIndexSize).
					FontFeatures("tnum").NoWrap().Shrink(0)
			}
			ui.Text(c, pct(q.Pct)).FontSize(panelIndexSize).
				TextColor(up(t, q.Pct)).FontFeatures("tnum").NoWrap().Shrink(0)
		}
	})
}

// indexRowWidth is how wide the benchmarks row takes with or without the
// prices, so the row can tell whether they fit before it draws them.
func indexRowWidth(c *ui.Context, quotes []model.Quote, prices bool) float32 {
	measure := func(s string) float32 {
		w, _ := c.MeasureText(0, ui.Span{Text: s, Size: panelIndexSize})
		return w
	}
	var w float32
	for i, q := range quotes {
		if i > 0 {
			w += panelIndexGap + measure("·") + panelIndexGap
		}
		w += measure(indexLabel(q.Symbol))
		if prices {
			w += panelIndexGap + measure(fmt.Sprintf("%.2f", q.Price))
		}
		w += panelIndexGap + measure(pct(q.Pct))
	}
	return w
}

// panelRow is one fund of the preview: the day's change, and a click that
// opens the window on it.
func (a *App) panelRow(c *ui.Context, t *ui.Theme, f *Fund) {
	change := f.ChangePct()
	tint := up(t, change)
	r := ui.Row(c.Key(f.Code)).Padding(5, 7).Radius(t.Radius).Gap(10).
		AlignItems(ui.Center).FillWidth().Cursor(ui.CursorPointer)
	if r.Hovered() {
		r.Background(t.SurfaceHover)
	}
	r.Transition(ui.ElementTransition{Duration: 120 * time.Millisecond})
	r.Children(func() {
		// The day's change keeps a pill of its own, tinted by its sign, with
		// an edge so that it reads on the glass as it does on a surface.
		ui.Box(c).Size(52, 32).Shrink(0).Radius(8).
			Background(tint.Alpha(0.16)).Border(1, tint.Alpha(0.20)).
			Center().Children(func() {
			text := "—"
			if f.Loaded || f.HasEst {
				text = pct(change)
			}
			ui.Text(c, text).FontSize(11.5).FontWeight(600).TextColor(tint).
				FontFeatures("tnum").NoWrap()
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
			name := f.Name
			if name == "" {
				name = f.Code
			}
			ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine().
				Ellipsis("…").Tooltip(name)
			sub := f.Code
			if f.Err != "" {
				sub += " · 读取失败"
			}
			ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…")
		})
		// The shares the user holds are what the preview is for; a fund
		// they only follow says so by leaving the space empty, and a made
		// group never shows them.
		if a.holds(f) {
			profit := f.TodayProfit()
			ui.Column(c).AlignItems(ui.End).Shrink(0).Gap(1).Children(func() {
				ui.Text(c, signedMoney(profit)).FontSize(12).FontWeight(600).
					TextColor(up(t, profit)).FontFeatures("tnum").NoWrap()
				ui.Text(c, "市值 "+money(f.Value())).FontSize(11).
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
		// Asking for newer numbers is the quiet action; the window is the
		// one a filled button points at.
		refresh := ui.ButtonBase(c).Padding(4, 8).Radius(t.Radius).
			Shrink(0).Cursor(ui.CursorPointer).Disabled(a.busy)
		if refresh.Hovered() && !a.busy {
			refresh.Background(t.SurfaceHover)
		}
		refresh.Children(func() {
			ui.Text(c, "刷新").FontSize(12).TextColor(t.TextMuted).NoWrap()
		})
		if refresh.Clicked() {
			a.refresh()
		}
		ui.Spacer(c)
		if ui.Button(c, "打开主面板").Shrink(0).Clicked() {
			a.afterFrame(a.Show)
		}
	})
}
