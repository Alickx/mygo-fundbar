package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// View builds the window from the app's state.
func (a *App) View(c *ui.Context) {
	t := theme(c)
	c.SetTheme(t)

	// Work a menu asked for, done before the frame is built.
	if a.pendingRemove >= 0 {
		a.Remove(a.pendingRemove)
		a.pendingRemove = -1
	}
	a.resolveWanted()

	c.OnShortcut(ui.Cmd, ui.KeyW, a.Hide)
	c.OnShortcut(ui.Cmd, ui.KeyQ, quit)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.refresh)
	if c.Shortcut(0, ui.KeyEscape) && a.query != "" {
		a.query, a.hits = "", nil
	}

	// The sidebar is frosted glass: what is behind the window shows through
	// it, and the content keeps a background of its own.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}

	split := ui.Split(c, &a.sidebarWidth,
		func() { a.sidebar(c, t, vibrant) },
		func() { a.main(c, t) })
	split.Fill().SplitReset(348)
	// The panes keep a usable width, whatever the divider is dragged to.
	a.sidebarWidth = min(max(a.sidebarWidth, 268), 460)
}

// theme is the window's look: the desktop's, with a softer canvas and cards.
func theme(c *ui.Context) *ui.Theme {
	base := ui.LightTheme()
	if c.Theme().Dark {
		base = ui.DarkTheme()
	}
	t := *base
	if t.Dark {
		t.Background = ui.Hex("#131317")
		t.Surface = ui.Hex("#1c1c22")
		t.SurfaceHover = ui.Hex("#26262f")
		t.SurfacePressed = ui.Hex("#30303a")
		t.Border = ui.Hex("#2c2c36")
		t.Text = ui.Hex("#f2f2f6")
		// Secondary text stays legible over a frosted sidebar.
		t.TextMuted = ui.Hex("#9b9bab")
	} else {
		t.Background = ui.Hex("#f6f6f8")
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#eeeef3")
		t.SurfacePressed = ui.Hex("#e2e2ea")
		t.Border = ui.Hex("#e2e2e9")
		t.Text = ui.Hex("#191920")
		t.TextMuted = ui.Hex("#6e6e7c")
	}
	t.Radius = 8
	return &t
}

// up colors a number by its sign: red as the funds rise, green as they fall.
func up(t *ui.Theme, v float64) ui.Color {
	switch {
	case v > 0:
		if t.Dark {
			return ui.Hex("#ff6b6b")
		}
		return ui.Hex("#d93b3a")
	case v < 0:
		if t.Dark {
			return ui.Hex("#4ad07a")
		}
		return ui.Hex("#0a8f4f")
	}
	return t.TextMuted
}

// card is the app's card: a surface with a hairline border, which a fund's
// parts sit on.
func card(c *ui.Context, t *ui.Theme) ui.Element {
	return ui.Column(c).Padding(13, 15).Radius(14).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04))
}

// pill writes a short label in a rounded chip.
func pill(c *ui.Context, text string, fg, bg ui.Color) ui.Element {
	return ui.Text(c, text).FontSize(11).FontWeight(500).NoWrap().
		TextColor(fg).Padding(1.5, 7).Radius(999).Background(bg).
		Border(1, fg.Alpha(0.20))
}

// sidebar is the list of funds the user follows, and the search above it.
func (a *App) sidebar(c *ui.Context, t *ui.Theme, vibrant bool) {
	tb := c.TitleBar()
	left := tb.Left + 12
	if left < 82 {
		left = 82 // clear of the window controls
	}
	col := ui.Column(c).FillHeight().BorderWidth(0, 1, 0, 0).
		BorderColor(t.Border)
	if !vibrant {
		col.Background(t.Surface)
	}
	col.Children(func() {
		// The window's title bar, drawn by the view, beside the controls.
		ui.Row(c).Height(46).Padding(0, 14, 0, left).Gap(8).
			AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, "基金估值").FontSize(13).FontWeight(600)
			ui.Spacer(c)
			if n := len(a.funds); n > 0 {
				ui.Textf(c, "%d 只", n).FontSize(11.5).TextColor(t.TextMuted).
					FontFeatures("tnum")
			}
		})

		ui.Column(c).Padding(0, 14, 10, 14).Children(func() {
			if f := ui.SearchField(c, &a.query).Placeholder("搜索代码或名称"); f.Changed() {
				a.searchDue = time.Now().Add(280 * time.Millisecond)
				c.After(280 * time.Millisecond)
			}
		})
		if !a.searchDue.IsZero() && time.Now().After(a.searchDue) {
			a.searchDue = time.Time{}
			a.search(a.query)
		}

		ui.Column(c).Grow(1).Padding(0, 6).Children(func() {
			if strings.TrimSpace(a.query) != "" {
				ui.Scroll(c).Fill().Children(func() {
					ui.Column(c).Gap(1).Padding(0, 4).Children(func() {
						a.searchResults(c, t)
					})
				})
				return
			}
			if len(a.funds) == 0 {
				ui.Column(c).Fill().Center().Gap(6).Padding(20).Children(func() {
					ui.Text(c, "还没有自选基金").FontSize(13).FontWeight(500)
					ui.Text(c, "在上面搜索代码或名称").FontSize(11.5).
						TextColor(t.TextMuted)
				})
				return
			}
			a.list.Key = func(i int) any { return a.funds[i].Code }
			a.list.Label = func(i int) string { return a.funds[i].Name }
			a.list.Selected = &a.sel
			a.list.Reorder = a.reorder
			ui.List(c, &a.list, len(a.funds)).Grow(1).Padding(0, 8).Gap(2).
				Rows(func(row ui.ListRow) { a.fundRow(row, t) })
		})

		ui.Row(c).Padding(6, 14, 10, 14).AlignItems(ui.Center).Gap(8).Children(func() {
			if a.busy {
				ui.Spinner(c).Size(11, 11)
			}
			note := a.updatedText()
			if a.failure != "" {
				note = a.failure
			}
			ui.Text(c, note).FontSize(11).TextColor(t.TextMuted).
				FontFeatures("tnum").NoWrap().Grow(1)
			if ui.Button(c, "设置").Label("设置").
				Tooltip("通用设置，包括登录时启动").Clicked() {
				a.OpenSettings()
			}
		})
	})
}

// searchResults are the funds the search found, which a click follows.
func (a *App) searchResults(c *ui.Context, t *ui.Theme) {
	if a.searching {
		ui.Row(c).Padding(8, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, "搜索中…").FontSize(12).TextColor(t.TextMuted)
		})
	} else if len(a.hits) == 0 {
		ui.Text(c, "没有找到基金").FontSize(12).TextColor(t.TextMuted).Padding(10, 14)
	}
	for _, h := range a.hits {
		h := h
		followed := a.byCode(h.Code) != nil
		r := ui.Row(c.Key(h.Code)).Padding(8, 10).Radius(9).AlignItems(ui.Center).Gap(8)
		if r.Hovered() {
			r.Background(t.SurfaceHover)
		}
		r.Transition(ui.ElementTransition{Duration: 120 * time.Millisecond})
		r.Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, h.Name).FontSize(12.5).NoWrap()
				sub := h.Code
				if h.Type != "" {
					sub += " · " + h.Type
				}
				ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).NoWrap()
			})
			mark := "＋"
			if followed {
				mark = "已添加"
			}
			ui.Text(c, mark).FontSize(12).TextColor(t.Accent)
		})
		if r.Clicked() {
			a.Add(h)
		}
	}
}

// fundRow is one fund of the list.
func (a *App) fundRow(row ui.ListRow, t *ui.Theme) {
	f := a.funds[row.Index]
	c := row.Context
	chosen := row.Index == a.sel

	change := f.ChangePct()
	tint := up(t, change)

	// A soft tint and a hairline edge, so that the chosen fund says so
	// without shouting over the numbers beside it. The tint is opaque: the
	// list paints the chosen row in the accent color behind it.
	bg, edge := ui.Transparent, ui.Transparent
	tileBg := tint.Alpha(0.13)
	if chosen {
		bg = t.Surface.Mix(t.Accent, 0.14)
		edge = t.Accent.Alpha(0.32)
		tileBg = t.Surface.Mix(tint, 0.16)
	}

	r := ui.Row(c).Padding(8, 9).Radius(10).Gap(10).AlignItems(ui.Center).
		Background(bg).Border(1, edge).FillWidth()
	r.Transition(ui.ElementTransition{Duration: 140 * time.Millisecond})
	r.ContextMenu(func(m *ui.Menu) {
		if m.Item("移出自选").Chosen() {
			a.removeAt(row.Index)
		}
	})
	r.Children(func() {
		// The day's change, in a tile of its own color.
		ui.Box(c).Size(50, 42).Radius(9).Background(tileBg).Center().Children(func() {
			text := "—"
			if f.Loaded || f.HasEst {
				text = pct(change)
			}
			ui.Text(c, text).FontSize(11.5).FontWeight(600).TextColor(tint).
				FontFeatures("tnum").NoWrap()
		})

		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			name := f.Name
			if name == "" {
				name = f.Code
			}
			ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine().
				Ellipsis("…")
			ui.Row(c).Gap(5).NoWrap().Children(func() {
				ui.Text(c, f.Code).FontSize(11).TextColor(t.TextMuted).
					FontFeatures("tnum")
				if f.Err != "" {
					ui.Text(c, "· 读取失败").FontSize(11).TextColor(t.Danger).NoWrap()
				}
			})
		})

		ui.Column(c).AlignItems(ui.End).Shrink(0).Gap(2).Children(func() {
			if f.Shares > 0 {
				ui.Text(c, signedMoney(f.TodayProfit())).FontSize(12).
					FontWeight(600).TextColor(up(t, f.TodayProfit())).
					FontFeatures("tnum").NoWrap()
				ui.Text(c, "市值 "+money(f.Value())).FontSize(11).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			} else {
				ui.Textf(c, "%.4f", f.EstNav()).FontSize(12).FontWeight(500).
					TextColor(t.Text).FontFeatures("tnum").NoWrap()
				ui.Text(c, "未持有").FontSize(11).TextColor(t.TextMuted).NoWrap()
			}
		})
	})
}

// main is the summary and the detail of the chosen fund.
func (a *App) main(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Background(t.Background).Children(func() {
		a.summaryBar(c, t)
		ui.Divider(c)
		f := a.Selected()
		if f == nil {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, "在左侧添加一只基金").FontSize(17).FontWeight(600)
				ui.Text(c, "搜索代码或名称，就能看到实时估值与今日收益").
					FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(18, 24, 24, 24).Gap(12).Children(func() {
				a.detailTitle(c, t, f)
				a.stats(c, t, f)
				a.holdingSection(c, t, f)
				a.chartCard(c, t, f)
				a.holdingsCard(c, t, f)
				ui.Text(c, "估值由基金季报的重仓股及其实时涨跌推算，仅供参考；"+
					"实际净值以基金公司公布为准。").FontSize(10.5).TextColor(t.TextMuted)
			})
		})
	})
}

func (a *App) summaryBar(c *ui.Context, t *ui.Theme) {
	s := a.Summary()
	ui.Row(c).Padding(16, 24, 14, 24).Gap(30).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "今日收益").FontSize(11.5).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).AlignItems(ui.Baseline).Children(func() {
				if s.Holding {
					ui.Text(c, signedMoney(s.Profit)).FontSize(25).FontWeight(600).
						TextColor(up(t, s.Profit)).FontFeatures("tnum").
						ContentTransition(digits)
					ui.Text(c, pct(s.Pct)).FontSize(12).TextColor(up(t, s.Pct)).
						FontFeatures("tnum").ContentTransition(digits)
				} else if len(a.funds) > 0 {
					ui.Text(c, pct(s.Pct)).FontSize(25).FontWeight(600).
						TextColor(up(t, s.Pct)).FontFeatures("tnum").
						ContentTransition(digits)
					ui.Text(c, "自选平均").FontSize(11.5).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "还没有自选基金").FontSize(14).TextColor(t.TextMuted)
				}
			})
		})
		if s.Holding {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "持仓市值").FontSize(11.5).TextColor(t.TextMuted)
				ui.Text(c, money(s.Value)).FontSize(15).FontWeight(600).
					FontFeatures("tnum")
			})
		}
		if len(a.funds) > 0 {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "涨跌分布").FontSize(11.5).TextColor(t.TextMuted)
				ui.Row(c).Gap(10).Children(func() {
					ui.Textf(c, "涨 %d", s.Up).FontSize(12.5).TextColor(up(t, 1)).
						FontFeatures("tnum")
					ui.Textf(c, "跌 %d", s.Down).FontSize(12.5).TextColor(up(t, -1)).
						FontFeatures("tnum")
				})
			})
		}
		ui.Spacer(c)
		ui.Column(c).AlignItems(ui.End).Gap(6).Children(func() {
			ui.Text(c, a.updatedText()).FontSize(11).TextColor(t.TextMuted).
				FontFeatures("tnum")
			if ui.Button(c, "刷新").Disabled(a.busy).Clicked() {
				a.refresh()
			}
		})
	})
}

func (a *App) updatedText() string {
	if a.busy {
		return "更新中…"
	}
	if a.updated.IsZero() {
		return "尚未更新"
	}
	if !tradingNow() {
		return "更新于 " + a.updated.In(cst).Format("01-02 15:04")
	}
	return "更新于 " + a.updated.In(cst).Format("15:04:05")
}

func (a *App) detailTitle(c *ui.Context, t *ui.Theme, f *Fund) {
	change := f.ChangePct()
	label, color := "已公布净值", t.TextMuted
	badgeBg := t.Text.Alpha(0.07)
	if f.UsingEstimate() {
		label, color, badgeBg = "盘中估算", t.Accent, t.Accent.Alpha(0.14)
	}
	ui.Row(c).AlignItems(ui.End).Children(func() {
		ui.Column(c).Grow(1).Gap(6).Children(func() {
			name := f.Name
			if name == "" {
				name = f.Code
			}
			ui.Text(c, name).FontSize(19).FontWeight(600).NoWrap()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, f.Code).FontSize(11).TextColor(t.TextMuted).
					FontFeatures("tnum").Padding(1, 6).Radius(5).
					Background(t.SurfaceHover)
				if f.Type != "" {
					ui.Text(c, f.Type).FontSize(11.5).TextColor(t.TextMuted).NoWrap()
				}
			})
		})
		ui.Column(c).AlignItems(ui.End).Gap(5).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "当日涨跌").FontSize(11.5).TextColor(t.TextMuted)
				pill(c, label, color, badgeBg)
			})
			ui.Text(c, pct(change)).FontSize(28).FontWeight(600).
				TextColor(up(t, change)).FontFeatures("tnum").
				ContentTransition(digits)
			ui.Text(c, a.valueLine(f)).FontSize(11).TextColor(t.TextMuted).
				FontFeatures("tnum")
		})
	})
}

// valueLine is the fund's own value under the day's change: the estimate
// while it is the newest number, and the published values otherwise.
func (a *App) valueLine(f *Fund) string {
	last, ok := f.LastNav()
	if f.UsingEstimate() {
		at := ""
		if !f.QuoteAt.IsZero() {
			at = " · " + f.QuoteAt.In(cst).Format("15:04")
		}
		return fmt.Sprintf("估算净值 %.4f", f.EstNav()) + at
	}
	if !ok {
		return ""
	}
	line := fmt.Sprintf("单位净值 %.4f", last.Unit)
	if last.Acc > 0 {
		line += fmt.Sprintf(" · 累计 %.4f", last.Acc)
	}
	if last.Date == f.Today() {
		return line + " · 今日已公布"
	}
	return line + " · " + last.Date
}

// digits rolls the numbers of a text as they change, as a counter does.
var digits = ui.ContentTransition{Numeric: true}

// statCard is one measure of the fund.
type statCard struct {
	label string
	value string
	note  string
	color ui.Color
}

func (a *App) stats(c *ui.Context, t *ui.Theme, f *Fund) {
	last, hasLast := f.LastNav()
	var cards []statCard
	if f.Shares > 0 {
		cards = []statCard{
			{"今日收益", signedMoney(f.TodayProfit()), profitNote(f),
				up(t, f.TodayProfit())},
			{"累计收益", signedMoney(f.TotalProfit()), pct(f.TotalPct()),
				up(t, f.TotalProfit())},
			{"持仓市值", money(f.Value()),
				fmt.Sprintf("%s 份", trim(f.Shares, 2)), t.Text},
		}
	} else {
		// Without shares, the fund's own numbers are what the user came for.
		if hasLast {
			cards = append(cards, statCard{"单位净值",
				fmt.Sprintf("%.4f", last.Unit), last.Date, up(t, last.Pct)})
		}
		if f.UsingEstimate() {
			cards = append(cards, statCard{"估算净值",
				fmt.Sprintf("%.4f", f.EstNav()), "盘中估算", up(t, f.Est.Pct)})
		} else if hasLast && last.Acc > 0 {
			cards = append(cards, statCard{"累计净值",
				fmt.Sprintf("%.4f", last.Acc), last.Date, t.Text})
		}
	}
	if len(cards) == 0 {
		return
	}
	ui.Row(c).Gap(10).Wrap().Children(func() {
		for _, card := range cards {
			ui.Column(c).Grow(1).Basis(0).MinWidth(128).Gap(3).
				Padding(11, 13).Radius(12).Background(t.Surface).
				Border(1, t.Border).Children(func() {
				ui.Text(c, card.label).FontSize(11.5).TextColor(t.TextMuted)
				ui.Text(c, card.value).FontSize(17).FontWeight(600).
					TextColor(card.color).FontFeatures("tnum").NoWrap().
					ContentTransition(digits)
				ui.Text(c, card.note).FontSize(11).TextColor(t.TextMuted).
					FontFeatures("tnum").NoWrap()
			})
		}
	})
}

// profitNote names the day the gain belongs to.
func profitNote(f *Fund) string {
	if f.UsingEstimate() {
		return "今日估算"
	}
	return "最新交易日"
}

func (a *App) holdingsCard(c *ui.Context, t *ui.Theme, f *Fund) {
	ui.Column(c).Gap(9).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "重仓股").FontSize(13).FontWeight(600)
			if f.Report != "" {
				ui.Text(c, "报告期 "+f.Report).FontSize(11.5).TextColor(t.TextMuted).
					FontFeatures("tnum")
			}
			ui.Spacer(c)
			if f.StockPosition > 0 {
				ui.Textf(c, "股票仓位 %.1f%%", f.StockPosition).FontSize(11.5).
					TextColor(t.TextMuted).FontFeatures("tnum")
			}
			if f.HasEst {
				ui.Textf(c, "覆盖 %.1f%%", f.Est.Cover).FontSize(11.5).
					TextColor(t.TextMuted).FontFeatures("tnum").
					Tooltip("前十大重仓股合计占基金净值的比例，越高估值越准")
			}
		})
		body := card(c, t)
		body.Gap(4).Children(func() {
			if len(f.Holdings) == 0 {
				ui.Text(c, "还没有读到持仓，刷新试试").FontSize(11.5).
					TextColor(t.TextMuted)
				return
			}
			const numW = 76
			ui.Row(c).Padding(0, 7, 2, 7).Children(func() {
				ui.Text(c, "名称").FontSize(11).TextColor(t.TextMuted).Grow(1)
				ui.Text(c, "占净值比").FontSize(11).TextColor(t.TextMuted).
					Width(numW).TextAlign(ui.End)
				ui.Text(c, "今日涨跌").FontSize(11).TextColor(t.TextMuted).
					Width(numW).TextAlign(ui.End)
			})
			for i, h := range f.Holdings {
				q, quoted := f.Quotes[h.Symbol]
				change := "—"
				if quoted {
					change = pct(q.Pct)
				}
				row := ui.Row(c).Padding(3, 7).Radius(6).AlignItems(ui.Center)
				if i%2 == 1 {
					row.Background(t.Text.Alpha(0.035))
				}
				row.Children(func() {
					ui.Text(c, h.Name).FontSize(12).Grow(1).NoWrap()
					ui.Textf(c, "%.2f%%", h.Weight).FontSize(11.5).
						TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
						Width(numW).TextAlign(ui.End)
					ui.Text(c, change).FontSize(11.5).TextColor(up(t, q.Pct)).
						FontFeatures("tnum").NoWrap().Width(numW).TextAlign(ui.End)
				})
			}
		})
	})
}

// holdingSection shows the shares the user holds, and their price: read
// only until they ask to edit them.
func (a *App) holdingSection(c *ui.Context, t *ui.Theme, f *Fund) {
	holding := f.Shares > 0
	body := card(c, t)
	body.Padding(11, 15).Gap(11).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "我的持仓").FontSize(13).FontWeight(600)
			switch {
			case a.editing && holding:
				ui.Text(c, "改完点「完成」").FontSize(11.5).TextColor(t.TextMuted)
			case holding:
				ui.Textf(c, "%s 份 · 成本价 %.4f", trim(f.Shares, 2), f.Cost).
					FontSize(11.5).TextColor(t.TextMuted).FontFeatures("tnum")
			}
			ui.Spacer(c)
			if a.editing {
				if ui.PrimaryButton(c, "完成").Clicked() {
					a.editing = false
				}
			} else if ui.Button(c, map[bool]string{true: "编辑持仓", false: "录入持仓"}[holding]).
				Clicked() {
				a.editing = true
			}
		})

		// The fields unfold, and fold away, rather than appearing and
		// taking the room at once.
		target := float32(0)
		if a.editing {
			target = holdingFormHeight
		}
		form := ui.Box(c).ClipY()
		h := form.AnimateTo("holding-form", target, ui.Smooth)
		form.Height(h)
		if h > 1 {
			form.Children(func() {
				ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
					ui.Column(c).Gap(5).Width(200).Children(func() {
						ui.Text(c, "持有份额").FontSize(11.5).TextColor(t.TextMuted)
						if ui.NumberInput(c, &f.Shares, 0, 1e10, 100).
							Changed() {
							a.changed()
						}
					})
					ui.Column(c).Gap(5).Width(160).Children(func() {
						ui.Text(c, "成本价").FontSize(11.5).TextColor(t.TextMuted)
						if ui.NumberInput(c, &f.Cost, 0, 1e6, 0.0001).
							Changed() {
							a.changed()
						}
					})
				})
			})
		}
		if !holding && !a.editing {
			ui.Text(c, "填上持有份额和成本价，状态栏就会显示今天赚了多少").
				FontSize(11.5).TextColor(t.TextMuted)
		}
	})
}

// holdingFormHeight is the room the fields take, which the section opens to.
const holdingFormHeight = 62

// trim writes a number with at most n decimals, and none that are zero.
func trim(v float64, n int) string {
	s := fmt.Sprintf("%.*f", n, v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
