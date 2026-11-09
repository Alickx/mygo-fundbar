package app

import (
	"fmt"
	"strings"
	"time"

	"fundbar/internal/model"

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

	ui.Row(c).Fill().Children(func() {
		a.sidebar(c, t)
		a.main(c, t)
	})
}

// theme is the window's look: the desktop's, with a softer canvas and cards.
func theme(c *ui.Context) *ui.Theme {
	base := ui.LightTheme()
	if c.Theme().Dark {
		base = ui.DarkTheme()
	}
	t := *base
	if t.Dark {
		t.Background = ui.Hex("#111114")
		t.Surface = ui.Hex("#1b1b21")
		t.SurfaceHover = ui.Hex("#26262f")
		t.SurfacePressed = ui.Hex("#2f2f3a")
		t.Border = ui.Hex("#2a2a34")
		t.Text = ui.Hex("#f4f4f7")
		t.TextMuted = ui.Hex("#8c8c9a")
	} else {
		t.Background = ui.Hex("#f4f4f7")
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#f1f1f5")
		t.SurfacePressed = ui.Hex("#e7e7ee")
		t.Border = ui.Hex("#e3e3ea")
		t.Text = ui.Hex("#1a1a21")
		t.TextMuted = ui.Hex("#8b8b99")
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
		return ui.Hex("#e0403f")
	case v < 0:
		if t.Dark {
			return ui.Hex("#4ad07a")
		}
		return ui.Hex("#0f9d58")
	}
	return t.TextMuted
}

// sidebar is the list of funds the user follows, and the search above it.
func (a *App) sidebar(c *ui.Context, t *ui.Theme) {
	tb := c.TitleBar()
	left := tb.Left + 12
	if left < 80 {
		left = 80 // clear of the window controls
	}
	ui.Column(c).Width(352).FillHeight().Background(t.Surface).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		// The window's title bar, drawn by the view, beside the controls.
		ui.Row(c).Height(46).Padding(0, 14, 0, left).Gap(8).
			AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, "基金估值").FontSize(13).FontWeight(600)
			ui.Spacer(c)
			if n := len(a.funds); n > 0 {
				ui.Textf(c, "%d 只", n).FontSize(11).TextColor(t.TextMuted).FontFeatures("tnum")
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
				ui.Column(c).Fill().Center().Gap(5).Padding(20).Children(func() {
					ui.Text(c, "还没有自选基金").FontSize(12.5).Bold()
					ui.Text(c, "在上面搜索代码或名称").FontSize(11).TextColor(t.TextMuted)
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

		ui.Row(c).Padding(6, 16, 10, 16).AlignItems(ui.Center).Gap(6).Children(func() {
			if a.busy {
				ui.Spinner(c).Size(11, 11)
			}
			note := a.updatedText()
			if a.failure != "" {
				note = a.failure
			}
			ui.Text(c, note).FontSize(10.5).TextColor(t.TextMuted).
				FontFeatures("tnum").NoWrap()
		})
	})
}

// searchResults are the funds the search found, which a click follows.
func (a *App) searchResults(c *ui.Context, t *ui.Theme) {
	if a.searching {
		ui.Row(c).Padding(8, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, "搜索中…").FontSize(11.5).TextColor(t.TextMuted)
		})
	} else if len(a.hits) == 0 {
		ui.Text(c, "没有找到基金").FontSize(11.5).TextColor(t.TextMuted).Padding(10, 14)
	}
	for _, h := range a.hits {
		h := h
		followed := a.byCode(h.Code) != nil
		r := ui.Row(c.Key(h.Code)).Padding(8, 10).Radius(9).AlignItems(ui.Center).Gap(8)
		if r.Hovered() {
			r.Background(t.SurfaceHover)
		}
		r.Children(func() {
			ui.Column(c).Grow(1).Gap(1).Children(func() {
				ui.Text(c, h.Name).FontSize(12).NoWrap()
				sub := h.Code
				if h.Type != "" {
					sub += " · " + h.Type
				}
				ui.Text(c, sub).FontSize(10.5).TextColor(t.TextMuted).NoWrap()
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

	// Text on the chosen row's accent, and the theme's on the others.
	main, muted := t.Text, t.TextMuted
	bg := ui.Transparent
	if chosen {
		bg = t.Accent
		main, muted = t.AccentText, t.AccentText.Alpha(0.72)
	}

	r := ui.Row(c).Padding(8, 9).Radius(10).Gap(10).AlignItems(ui.Center).
		Background(bg).FillWidth()
	r.ContextMenu(func(m *ui.Menu) {
		if m.Item("移出自选").Chosen() {
			a.removeAt(row.Index)
		}
	})
	r.Children(func() {
		// The day's change, in a tile of its own color.
		tile := ui.Box(c).Size(50, 42).Radius(10).Center()
		if chosen {
			tile.Background(t.AccentText.Alpha(0.16))
		} else {
			tile.Background(tint.Alpha(0.14))
		}
		tile.Children(func() {
			color := tint
			if chosen {
				color = t.AccentText
			}
			text := "—"
			if f.Loaded || f.HasEst {
				text = pct(change)
			}
			ui.Text(c, text).FontSize(11.5).FontWeight(600).TextColor(color).
				FontFeatures("tnum").NoWrap()
		})

		ui.Column(c).Grow(1).Gap(1).Children(func() {
			name := f.Name
			if name == "" {
				name = f.Code
			}
			ui.Text(c, name).FontSize(12.5).FontWeight(500).TextColor(main).NoWrap()
			ui.Row(c).Gap(5).NoWrap().Children(func() {
				ui.Text(c, f.Code).FontSize(10.5).TextColor(muted).FontFeatures("tnum")
				if f.Err != "" {
					ui.Text(c, "· 读取失败").FontSize(10.5).TextColor(t.Danger).NoWrap()
				}
			})
		})

		ui.Column(c).AlignItems(ui.End).Gap(1).Children(func() {
			if f.Shares > 0 {
				profit, profitMuted := up(t, f.TodayProfit()), muted
				if chosen {
					profit = t.AccentText
				}
				ui.Text(c, signedMoney(f.TodayProfit())).FontSize(11.5).
					FontWeight(600).TextColor(profit).FontFeatures("tnum").NoWrap()
				ui.Text(c, "市值 "+money(f.Value())).FontSize(10).
					TextColor(profitMuted).FontFeatures("tnum").NoWrap()
			} else {
				ui.Textf(c, "%.4f", f.EstNav()).FontSize(11.5).
					TextColor(main).FontFeatures("tnum").NoWrap()
				ui.Text(c, "未持有").FontSize(10).TextColor(muted).NoWrap()
			}
		})
	})
}

// main is the summary and the detail of the chosen fund.
func (a *App) main(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		a.summaryBar(c, t)
		ui.Divider(c)
		f := a.Selected()
		if f == nil {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, "在左侧添加一只基金").FontSize(17).Bold()
				ui.Text(c, "搜索代码或名称，就能看到实时估值与今日收益").
					FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(18, 24, 24, 24).Gap(12).Children(func() {
				a.detailTitle(c, t, f)
				a.stats(c, t, f)
				a.chartCard(c, t, f)
				a.holdingsCard(c, t, f)
				a.holdingEditor(c, t, f)
				ui.Text(c, "估值由基金季报的重仓股及其实时涨跌推算，仅供参考；"+
					"实际净值以基金公司公布为准。").FontSize(10.5).TextColor(t.TextMuted)
			})
		})
	})
}

func (a *App) summaryBar(c *ui.Context, t *ui.Theme) {
	s := a.Summary()
	ui.Row(c).Padding(16, 24, 14, 24).Gap(30).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, "今日收益").FontSize(10.5).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).AlignItems(ui.Baseline).Children(func() {
				if s.Holding {
					ui.Text(c, signedMoney(s.Profit)).FontSize(25).FontWeight(600).
						TextColor(up(t, s.Profit)).FontFeatures("tnum")
					ui.Text(c, pct(s.Pct)).FontSize(12).TextColor(up(t, s.Pct)).
						FontFeatures("tnum")
				} else if len(a.funds) > 0 {
					ui.Text(c, pct(s.Pct)).FontSize(25).FontWeight(600).
						TextColor(up(t, s.Pct)).FontFeatures("tnum")
					ui.Text(c, "自选平均").FontSize(11).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "还没有自选基金").FontSize(14).TextColor(t.TextMuted)
				}
			})
		})
		if s.Holding {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "持仓市值").FontSize(10.5).TextColor(t.TextMuted)
				ui.Text(c, money(s.Value)).FontSize(15).FontWeight(600).FontFeatures("tnum")
			})
		}
		if len(a.funds) > 0 {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "涨跌分布").FontSize(10.5).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.Textf(c, "涨 %d", s.Up).FontSize(12).TextColor(up(t, 1)).
						FontFeatures("tnum")
					ui.Textf(c, "跌 %d", s.Down).FontSize(12).TextColor(up(t, -1)).
						FontFeatures("tnum")
				})
			})
		}
		ui.Spacer(c)
		ui.Column(c).AlignItems(ui.End).Gap(5).Children(func() {
			ui.Text(c, a.updatedText()).FontSize(10.5).TextColor(t.TextMuted).
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
	ui.Row(c).AlignItems(ui.End).Children(func() {
		ui.Column(c).Grow(1).Gap(5).Children(func() {
			name := f.Name
			if name == "" {
				name = f.Code
			}
			ui.Text(c, name).FontSize(19).FontWeight(600).NoWrap()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, f.Code).FontSize(10.5).TextColor(t.TextMuted).
					FontFeatures("tnum").Padding(1, 5).Radius(5).Background(t.SurfaceHover)
				if f.Type != "" {
					ui.Text(c, f.Type).FontSize(10.5).TextColor(t.TextMuted).NoWrap()
				}
			})
		})
		ui.Column(c).AlignItems(ui.End).Gap(3).Children(func() {
			ui.Text(c, pct(change)).FontSize(27).FontWeight(600).
				TextColor(up(t, change)).FontFeatures("tnum")
			ui.Text(c, a.estimateNote(f)).FontSize(10.5).TextColor(t.TextMuted).
				FontFeatures("tnum")
		})
	})
}

// estimateNote says where the number beside the fund's name comes from.
func (a *App) estimateNote(f *Fund) string {
	if f.UsingEstimate() {
		at := ""
		if !f.QuoteAt.IsZero() {
			at = " · " + f.QuoteAt.In(cst).Format("15:04")
		}
		return fmt.Sprintf("估算净值 %.4f", f.EstNav()) + at
	}
	last, ok := f.LastNav()
	if !ok {
		return ""
	}
	if last.Date == f.Today() {
		return "今日净值已公布"
	}
	return "净值日 " + last.Date
}

// statCard is one measure of the fund.
type statCard struct {
	label string
	value string
	note  string
	color ui.Color
}

func (a *App) stats(c *ui.Context, t *ui.Theme, f *Fund) {
	var cards []statCard
	if last, ok := f.LastNav(); ok {
		cards = append(cards, statCard{"单位净值", fmt.Sprintf("%.4f", last.Unit), last.Date, t.Text})
	}
	if f.UsingEstimate() {
		cards = append(cards, statCard{"估算净值", fmt.Sprintf("%.4f", f.EstNav()),
			"盘中估算", up(t, f.Est.Pct)})
	} else if last, ok := f.LastNav(); ok && last.Acc > 0 {
		cards = append(cards, statCard{"累计净值", fmt.Sprintf("%.4f", last.Acc),
			last.Date, t.Text})
	}
	if f.Shares > 0 {
		cards = append(cards,
			statCard{"持仓市值", money(f.Value()),
				fmt.Sprintf("%s 份", trim(f.Shares, 2)), t.Text},
			statCard{"今日收益", signedMoney(f.TodayProfit()), profitNote(f),
				up(t, f.TodayProfit())},
			statCard{"累计收益", signedMoney(f.TotalProfit()), pct(f.TotalPct()),
				up(t, f.TotalProfit())},
		)
	} else {
		cards = append(cards, statCard{"我的持仓", "未持有", "在下面填写份额", t.TextMuted})
	}
	ui.Row(c).Gap(10).Children(func() {
		for _, card := range cards {
			ui.Column(c).Grow(1).Gap(3).Padding(11, 13).Radius(12).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Text(c, card.label).FontSize(10.5).TextColor(t.TextMuted)
				ui.Text(c, card.value).FontSize(15).FontWeight(600).
					TextColor(card.color).FontFeatures("tnum").NoWrap()
				ui.Text(c, card.note).FontSize(10).TextColor(t.TextMuted).
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
	ui.Column(c).Gap(9).Padding(13, 15).Radius(14).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "重仓股").FontSize(12.5).FontWeight(600)
			if f.Report != "" {
				ui.Text(c, "报告期 "+f.Report).FontSize(10.5).TextColor(t.TextMuted).
					FontFeatures("tnum")
			}
			ui.Spacer(c)
			if f.StockPosition > 0 {
				ui.Textf(c, "股票仓位 %.1f%%", f.StockPosition).FontSize(10.5).
					TextColor(t.TextMuted).FontFeatures("tnum")
			}
			if f.HasEst {
				ui.Textf(c, "覆盖 %.1f%%", f.Est.Cover).FontSize(10.5).
					TextColor(t.TextMuted).FontFeatures("tnum").
					Tooltip("前十大重仓股合计占基金净值的比例，越高估值越准")
			}
		})
		if len(f.Holdings) == 0 {
			ui.Text(c, "还没有读到持仓，刷新试试").FontSize(11).TextColor(t.TextMuted)
			return
		}
		half := (len(f.Holdings) + 1) / 2
		ui.Row(c).Gap(26).Children(func() {
			a.holdingColumn(c, t, f, f.Holdings[:half])
			a.holdingColumn(c, t, f, f.Holdings[half:])
		})
	})
}

func (a *App) holdingColumn(c *ui.Context, t *ui.Theme, f *Fund, holdings []model.Holding) {
	ui.Column(c).Grow(1).Gap(5).Children(func() {
		for _, h := range holdings {
			q := f.Quotes[h.Symbol]
			change := "—"
			if h.Symbol != "" {
				if _, ok := f.Quotes[h.Symbol]; ok {
					change = pct(q.Pct)
				}
			}
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, h.Name).FontSize(11.5).Grow(1).NoWrap()
				ui.Textf(c, "%.2f%%", h.Weight).FontSize(10.5).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
				ui.Text(c, change).FontSize(11).TextColor(up(t, q.Pct)).
					FontFeatures("tnum").NoWrap().Width(54).TextAlign(ui.End)
			})
		}
	})
}

func (a *App) holdingEditor(c *ui.Context, t *ui.Theme, f *Fund) {
	ui.Column(c).Gap(10).Padding(13, 15).Radius(14).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "我的持仓").FontSize(12.5).FontWeight(600)
			ui.Spacer(c)
			ui.Text(c, "填写后，状态栏会显示今日收益").FontSize(10.5).TextColor(t.TextMuted)
		})
		ui.Row(c).Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Gap(4).Width(190).Children(func() {
				ui.Text(c, "持有份额").FontSize(10.5).TextColor(t.TextMuted)
				if ui.NumberInput(c, &f.Shares, 0, 1e10, 100).Changed() {
					a.changed()
				}
			})
			ui.Column(c).Gap(4).Width(150).Children(func() {
				ui.Text(c, "成本价").FontSize(10.5).TextColor(t.TextMuted)
				if ui.NumberInput(c, &f.Cost, 0, 1e6, 0.0001).Changed() {
					a.changed()
				}
			})
			ui.Spacer(c)
		})
	})
}

// trim writes a number with at most n decimals, and none that are zero.
func trim(v float64, n int) string {
	s := fmt.Sprintf("%.*f", n, v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
