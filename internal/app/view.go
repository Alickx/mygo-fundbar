package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
)

// View builds the window from the app's state.
func (a *App) View(c *ui.Context) {
	t := theme(c)
	c.SetTheme(t)

	// Work a menu asked for, done before the frame is built.
	a.applyPending()
	a.resolveWanted()

	c.OnShortcut(ui.Cmd, ui.KeyW, a.Hide)
	c.OnShortcut(ui.Cmd, ui.KeyQ, quit)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.refresh)
	if c.Shortcut(0, ui.KeyEscape) {
		switch {
		case a.groupEditing:
			a.cancelGroupEdit()
		case a.query != "":
			a.query, a.hits = "", nil
		}
	}

	// The sidebar is frosted glass: what is behind the window shows through
	// it, and the content keeps a background of its own.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}

	// Clamp before building the split so even the first frame of a restored
	// window leaves room for the detail pane. Input is applied after a frame.
	w, _ := c.Size()
	limit := max(float32(268), min(float32(460), w-553))
	if a.sidebarWidth == 0 {
		a.sidebarWidth = 348
	}
	a.sidebarWidth = min(max(a.sidebarWidth, 268), limit)
	split := ui.Split(c, &a.sidebarWidth,
		func() { a.sidebar(c, t, vibrant) },
		func() { a.main(c, t) })
	split.Fill().SplitReset(348)
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

		ui.Column(c).Padding(0, 14, 8, 14).Children(func() {
			a.groupBar(c, t, a.sidebarWidth-28)
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
						// Codes alone ask to be followed together, without a
						// search for each one's name first.
						if codes, ok := batchCodes(a.query); ok {
							a.batchResults(c, t, codes)
							return
						}
						a.searchResults(c, t)
					})
				})
				return
			}
			if len(a.funds) == 0 {
				emptyTitle, emptyNote := "还没有自选基金", "在上面搜索代码或名称"
				if a.group != holdingGroup {
					emptyTitle = "「" + a.groupName(a.group) + "」还没有基金"
					emptyNote = "搜索代码或名称，加入这个分组"
				}
				ui.Column(c).Fill().Center().Gap(6).Padding(20).Children(func() {
					ui.Text(c, emptyTitle).FontSize(13).FontWeight(500).
						SingleLine().Ellipsis("…").Tooltip(emptyTitle)
					ui.Text(c, emptyNote).FontSize(11.5).TextColor(t.TextMuted)
				})
				return
			}
			a.list.Key = func(i int) any { return a.funds[i].Code }
			a.list.Label = func(i int) string { return a.funds[i].Name }
			// The look-through leaves every row unmarked: the list chooses a
			// scratch row, so that a click on any row, even the fund it would
			// otherwise show chosen, lands somewhere the next frame can read.
			a.list.Selected = &a.sel
			if a.portfolio {
				a.list.Selected = &a.listSel
			}
			a.list.Reorder = a.reorder
			// A chosen row is painted in the list's accent before the row
			// builds. In the dark theme that accent under the row's rounded
			// corners reads as a blue ring, so the list takes a neutral
			// accent of its own: the row's background sits on a like color.
			listTheme := *t
			if listTheme.Dark {
				listTheme.Accent = t.SurfaceHover
				listTheme.InactiveAccent = t.SurfaceHover
			}
			ui.Themed(c, &listTheme).Grow(1).Background(ui.Transparent).Children(func() {
				ui.List(c, &a.list, len(a.funds)).Grow(1).Padding(0, 8).Gap(2).
					Rows(func(row ui.ListRow) { a.fundRow(row, t) })
			})
		})

		ui.Row(c).Padding(6, 14, 10, 14).AlignItems(ui.Center).Gap(8).Children(func() {
			if a.busy {
				ui.Spinner(c).Size(11, 11)
			}
			note := a.updatedText()
			if a.flashNow() != "" {
				note = a.flashNow()
			}
			if a.failure != "" {
				note = a.failure
			}
			ui.Text(c, note).FontSize(11).TextColor(t.TextMuted).
				FontFeatures("tnum").SingleLine().Ellipsis("…").Grow(1).Basis(0).
				Tooltip(note)
			if ui.Button(c, "设置").Shrink(0).Label("设置").
				Tooltip("通用设置，包括登录时启动").Clicked() {
				a.OpenSettings()
			}
		})
	})
}

// groupBar switches the list between the holding group and the groups the
// user made, and makes and renames them. The window and the preview of the
// menu bar item draw the same bar.
//
// room is how wide the bar may be: the groups that fit show as chips, and
// the rest wait in a menu at their end, so that the group the windows list
// stays in sight however many groups there are.
func (a *App) groupBar(c *ui.Context, t *ui.Theme, room float32) {
	shown, rest := a.groupFit(c, room)
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(groupAddGap).AlignItems(ui.Center).Children(func() {
			ui.ScrollHorizontal(c).Grow(1).Basis(0).Children(func() {
				ui.Row(c).Gap(groupChipGap).AlignItems(ui.Center).Children(func() {
					for _, id := range shown {
						a.groupChip(c, t, id)
					}
					if len(rest) > 0 {
						a.groupMore(c, t, rest)
					}
				})
			})
			add := ui.Row(c.Key("group-new")).Padding(3, 8).Radius(999).
				Border(1, t.Border).Background(t.SurfaceHover).Shrink(0).
				Cursor(ui.CursorPointer).Tooltip("新建分组")
			if add.Hovered() {
				add.Background(t.SurfacePressed)
			}
			add.Children(func() {
				ui.Text(c, "＋").FontSize(groupChipFont).FontWeight(600).
					TextColor(t.TextMuted).NoWrap()
			})
			if add.Clicked() {
				a.beginNewGroup()
			}
		})
		if a.groupEditing {
			ui.Column(c).Gap(4).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					in := ui.TextInput(c, &a.groupDraft).Placeholder("分组名称").
						AutoFocus().Grow(1).Basis(0).MinWidth(0)
					if in.Submitted() {
						a.CommitGroup()
					}
					if ui.PrimaryButton(c, "确定").Shrink(0).Clicked() {
						a.CommitGroup()
					}
					if ui.Button(c, "取消").Shrink(0).Clicked() {
						a.cancelGroupEdit()
					}
				})
				if a.groupErr != "" {
					ui.Text(c, a.groupErr).FontSize(10.5).TextColor(t.Danger).
						NoWrap().SingleLine().Ellipsis("…")
				}
			})
		}
	})
}

// The group bar's sizes, which decide how many chips fit in its room.
const (
	groupChipFont = 11.5
	groupChipGap  = 5
	groupAddGap   = 6
	// groupChipExtra is a chip's padding and hairline border, beside its
	// text; groupAddExtra is the same for the button that makes a group.
	groupChipExtra = 20
	groupAddExtra  = 18
)

// groupFit puts as many groups as fit in room into chips, and the rest, in
// order, into the menu: the group the windows list is always one of the
// chips.
func (a *App) groupFit(c *ui.Context, room float32) (shown, rest []string) {
	ids := make([]string, 0, len(a.groups)+1)
	ids = append(ids, holdingGroup)
	for _, g := range a.groups {
		ids = append(ids, g.ID)
	}
	width := make(map[string]float32, len(ids))
	for _, id := range ids {
		width[id] = chipWidth(c, a.groupName(id), groupChipFont, 500, groupChipExtra)
	}
	room -= chipWidth(c, "＋", groupChipFont, 600, groupAddExtra) + groupAddGap
	if room < 0 {
		room = 0
	}

	fit := func(ids []string, room float32) []string {
		var out []string
		used := float32(0)
		for i, id := range ids {
			need := width[id]
			if i > 0 {
				need += groupChipGap
			}
			if used+need > room {
				break
			}
			used += need
			out = append(out, id)
		}
		return out
	}
	shown = fit(ids, room)
	if len(shown) == len(ids) {
		return shown, nil
	}

	// The rest wait in a menu, which takes a chip of its own.
	room -= chipWidth(c, "⋯", groupChipFont, 600, groupChipExtra) + groupChipGap
	if room < 0 {
		room = 0
	}
	shown = fit(ids, room)
	if len(shown) == 0 {
		shown = []string{holdingGroup}
	}
	if !groupShown(shown, a.group) {
		// Room for the group the windows list, taken from the end of the
		// chips before it.
		used := float32(0)
		for i, id := range shown {
			used += width[id]
			if i > 0 {
				used += groupChipGap
			}
		}
		for len(shown) > 0 && used+width[a.group]+groupChipGap > room {
			last := len(shown) - 1
			used -= width[shown[last]]
			if last > 0 {
				used -= groupChipGap
			}
			shown = shown[:last]
		}
		shown = append(shown, a.group)
	}
	seen := make(map[string]bool, len(shown))
	for _, id := range shown {
		seen[id] = true
	}
	for _, id := range ids {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	return shown, rest
}

// groupShown reports that the chips show a group.
func groupShown(shown []string, id string) bool {
	for _, g := range shown {
		if g == id {
			return true
		}
	}
	return false
}

// chipWidth is how wide a chip of text is drawn, with extra for its padding.
func chipWidth(c *ui.Context, text string, size float32, weight int, extra float32) float32 {
	w, _ := c.MeasureText(0, ui.Span{Text: text, Size: size, Weight: weight})
	return w + extra
}

// groupMore is the menu of the groups the bar has no room for: a click lists
// one, and the group the windows list renames and goes away here too.
func (a *App) groupMore(c *ui.Context, t *ui.Theme, rest []string) {
	more := ui.Row(c.Key("group-more")).Padding(3, 9).Radius(999).
		Border(1, t.Border).Cursor(ui.CursorPointer).Shrink(0).
		Tooltip("更多分组")
	if more.Hovered() {
		more.Background(t.SurfaceHover)
	}
	more.Children(func() {
		ui.Text(c, "⋯").FontSize(groupChipFont).FontWeight(600).
			TextColor(t.TextMuted).NoWrap()
	})
	more.PopoverMenuButton(func(m *ui.Menu) {
		m.MaxWidth(300)
		for _, id := range rest {
			gid := id
			it := m.Item(a.groupName(gid)).Radio(gid == a.group)
			if long := a.groupName(gid); len([]rune(long)) > 16 {
				it.Tooltip(long)
			}
			if it.Chosen() {
				a.wantGroup, a.wantGroupSet = gid, true
			}
		}
		if a.group != holdingGroup {
			name := a.groupName(a.group)
			m.Separator()
			if m.Item("重命名「" + name + "」").Chosen() {
				a.wantRename = a.group
			}
			if m.Item("删除「" + name + "」").Danger().Chosen() {
				a.wantDeleteGroup = a.group
			}
		}
	})
}

// groupChip is one group of the bar: a click lists it, and a group the user
// made renames and goes away from its context menu.
func (a *App) groupChip(c *ui.Context, t *ui.Theme, id string) {
	chosen := id == a.group
	fg, bg, edge := t.TextMuted, ui.Transparent, ui.Transparent
	if chosen {
		fg = t.Accent
		bg = t.Accent.Alpha(0.14)
		edge = t.Accent.Alpha(0.22)
	}
	r := ui.Row(c.Key("group-chip-"+id)).Padding(3, 9).Radius(999).
		Background(bg).Border(1, edge).Cursor(ui.CursorPointer).
		AlignItems(ui.Center).Gap(4)
	if !chosen && r.Hovered() {
		r.Background(t.SurfaceHover)
	}
	r.Children(func() {
		ui.Text(c, a.groupName(id)).FontSize(groupChipFont).FontWeight(500).
			TextColor(fg).NoWrap()
	})
	if id != holdingGroup {
		r.ContextMenu(func(m *ui.Menu) {
			if m.Item("重命名分组").Chosen() {
				a.wantRename = id
			}
			if m.Item("删除分组").Chosen() {
				a.wantDeleteGroup = id
			}
		})
	}
	if r.Clicked() {
		a.SelectGroup(id)
	}
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
		f := a.byCode(h.Code)
		followed := f != nil && f.In(a.group)
		r := ui.Row(c.Key(h.Code)).Padding(8, 10).Radius(9).AlignItems(ui.Center).Gap(8)
		if r.Hovered() {
			r.Background(t.SurfaceHover)
		}
		r.Transition(ui.ElementTransition{Duration: 120 * time.Millisecond})
		r.Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, h.Name).FontSize(12.5).SingleLine().Ellipsis("…").Tooltip(h.Name)
				sub := h.Code
				if h.Type != "" {
					sub += " · " + h.Type
				}
				ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			})
			mark := "＋"
			if followed {
				mark = "已添加"
			}
			ui.Text(c, mark).FontSize(12).TextColor(t.Accent).NoWrap().Shrink(0)
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
	chosen := !a.portfolio && row.Index == a.sel

	change := f.ChangePct()
	tint := up(t, change)

	// A soft tint and a hairline edge, so that the chosen fund says so
	// without shouting over the numbers beside it. The tint is opaque: the
	// list paints the chosen row in the accent color behind it, and the
	// inner row's corners must cover it exactly rather than show it as a
	// blue ring.
	bg, edge := ui.Transparent, ui.Transparent
	tileBg := tint.Alpha(0.13)
	if chosen {
		if t.Dark {
			// The accent is too bright against the dark surface: the choice
			// is a neutral lift, not a blue ring around the corners.
			bg = t.SurfaceHover
			edge = t.Text.Alpha(0.12)
		} else {
			bg = t.Surface.Mix(t.Accent, 0.10)
			edge = t.Accent.Alpha(0.18)
		}
		tileBg = t.Surface.Mix(tint, 0.16)
	}

	r := ui.Row(c).Padding(8, 9).Radius(t.Radius).Gap(10).AlignItems(ui.Center).
		Background(bg).Border(1, edge).FillWidth()
	r.Transition(ui.ElementTransition{Duration: 140 * time.Millisecond})
	r.ContextMenu(func(m *ui.Menu) {
		// Removing a fund takes its transactions with it, which the menu
		// says where the choice is made rather than after it.
		remove := "移出自选"
		if n := len(f.Txns); n > 0 {
			remove = fmt.Sprintf("移出自选（连同 %d 笔交易记录）", n)
		}
		if m.Item(remove).Chosen() {
			a.removeAt(row.Index)
		}
		// A fund of two groups leaves the one the window lists, and stays in
		// the other; one that is only here leaves altogether, above.
		if len(f.Groups) > 1 && m.Item("移出「"+a.groupName(a.group)+"」").Chosen() {
			a.leaveAt(row.Index)
		}
		a.joinMenu(m, f, row.Index)
	})
	r.Children(func() {
		// The day's change, in a tile of its own color.
		ui.Box(c).Size(50, 42).Shrink(0).Radius(9).Background(tileBg).Center().Children(func() {
			text := "—"
			if f.Loaded || f.HasEst {
				text = pct(change)
			}
			ui.Text(c, text).FontSize(11.5).FontWeight(600).TextColor(tint).
				FontFeatures("tnum").NoWrap()
		})

		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			name := f.Name
			if name == "" {
				name = f.Code
			}
			ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine().
				Ellipsis("…").TextColor(t.Text).Tooltip(name)
			ui.Row(c).Gap(5).NoWrap().Children(func() {
				ui.Text(c, f.Code).FontSize(11).TextColor(t.TextMuted).
					FontFeatures("tnum")
				if f.Err != "" {
					ui.Text(c, "· 读取失败").FontSize(11).TextColor(t.Danger).NoWrap()
				}
				if f.UsingEstimate() && f.Est.Cover < minCover {
					ui.Textf(c, "· 覆盖 %.1f%%", f.Est.Cover).FontSize(11).
						TextColor(t.TextMuted).NoWrap()
				}
			})
		})

		ui.Column(c).AlignItems(ui.End).Shrink(0).Gap(2).Children(func() {
			if a.holds(f) {
				ui.Text(c, signedMoney(f.TodayProfit())).FontSize(12).
					FontWeight(600).TextColor(up(t, f.TodayProfit())).
					FontFeatures("tnum").NoWrap()
				ui.Text(c, "市值 "+money(f.Value())).FontSize(11).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			} else {
				ui.Textf(c, "%.4f", f.EstNav()).FontSize(12).FontWeight(500).
					TextColor(t.Text).FontFeatures("tnum").NoWrap()
				note := "未持有"
				if a.group != holdingGroup {
					note = "关注"
				}
				ui.Text(c, note).FontSize(11).TextColor(t.TextMuted).NoWrap()
			}
		})
	})
}

// main is the summary and the detail of the chosen fund.
func (a *App) main(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Background(t.Background).Children(func() {
		a.summaryBar(c, t)
		ui.Divider(c)
		if a.upd.step != updateNone {
			a.updateStrip(c, t)
			ui.Divider(c)
		}
		if a.portfolio {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(18, 24, 24, 24).Gap(12).Children(func() {
					a.portfolioView(c, t)
				})
			})
			return
		}
		f := a.Selected()
		if f == nil {
			title, note := "在左侧添加一只基金", "搜索代码或名称，就能看到实时估值与今日收益"
			if a.group != holdingGroup {
				title = "「" + a.groupName(a.group) + "」还没有基金"
				note = "搜索代码或名称，加入这个分组；这里只看涨跌，不记录持仓"
			}
			ui.Column(c).Grow(1).Center().Gap(8).Padding(20).Children(func() {
				ui.Text(c, title).FontSize(17).FontWeight(600).
					SingleLine().Ellipsis("…").Tooltip(title)
				ui.Text(c, note).FontSize(12).TextColor(t.TextMuted).
					TextAlign(ui.Center)
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(18, 24, 24, 24).Gap(12).Children(func() {
				a.detailTitle(c, t, f)
				a.stats(c, t, f)
				if a.group == holdingGroup {
					a.holdingSection(c, t, f)
				}
				a.chartCard(c, t, f)
				a.industriesCard(c, t, f)
				a.holdingsCard(c, t, f)
				ui.Text(c, "估值由基金季报的重仓股及其实时涨跌推算，仅供参考；"+
					"实际净值以基金公司公布为准。").FontSize(10.5).TextColor(t.TextMuted)
			})
		})
	})
}

func (a *App) summaryBar(c *ui.Context, t *ui.Theme) {
	s := a.GroupSummary(a.group)
	ui.Row(c).Padding(14, 24).Gap(18).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Wrap().Gap(18).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Shrink(0).Gap(3).Children(func() {
				label := "今日收益"
				if !s.Holding {
					label = "自选平均涨跌"
					if a.group != holdingGroup {
						label = "分组平均涨跌"
					}
				}
				ui.Text(c, label).FontSize(11.5).TextColor(t.TextMuted).NoWrap()
				ui.Row(c).Gap(8).AlignItems(ui.Baseline).Children(func() {
					if s.Holding {
						ui.Text(c, signedMoney(s.Profit)).FontSize(25).FontWeight(600).
							TextColor(up(t, s.Profit)).FontFeatures("tnum").NoWrap().
							ContentTransition(digits)
						ui.Text(c, pct(s.Pct)).FontSize(12).TextColor(up(t, s.Pct)).
							FontFeatures("tnum").NoWrap().ContentTransition(digits)
					} else if len(a.funds) > 0 {
						ui.Text(c, pct(s.Pct)).FontSize(25).FontWeight(600).
							TextColor(up(t, s.Pct)).FontFeatures("tnum").NoWrap().
							ContentTransition(digits)
					} else if a.group != holdingGroup {
						ui.Text(c, "分组还没有基金").FontSize(14).TextColor(t.TextMuted)
					} else {
						ui.Text(c, "还没有自选基金").FontSize(14).TextColor(t.TextMuted)
					}
				})
			})
			if s.Holding {
				ui.Column(c).Shrink(0).Gap(4).Children(func() {
					ui.Text(c, "持仓市值").FontSize(11.5).TextColor(t.TextMuted).NoWrap()
					ui.Text(c, money(s.Value)).FontSize(15).FontWeight(600).
						FontFeatures("tnum").NoWrap()
				})
			}
			if len(a.funds) > 0 {
				ui.Column(c).Shrink(0).Gap(4).Children(func() {
					ui.Text(c, "涨跌分布").FontSize(11.5).TextColor(t.TextMuted).NoWrap()
					ui.Row(c).Gap(10).Children(func() {
						ui.Textf(c, "涨 %d", s.Up).FontSize(12.5).TextColor(up(t, 1)).
							FontFeatures("tnum").NoWrap()
						ui.Textf(c, "跌 %d", s.Down).FontSize(12.5).TextColor(up(t, -1)).
							FontFeatures("tnum").NoWrap()
					})
				})
			}
		})
		ui.Column(c).Shrink(0).AlignItems(ui.End).Gap(6).Children(func() {
			note := a.freshNote()
			ui.Text(c, note).FontSize(11).TextColor(t.TextMuted).
				FontFeatures("tnum").NoWrap().Tooltip(a.updatedText())
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				// The look-through only exists where there is a portfolio to
				// look through: the holding group's funds with shares.
				if a.portfolioAvailable() {
					label := "组合透视"
					tip := "把各基金季报的持仓按市值加总，看看真正持有什么"
					if a.portfolio {
						label = "返回基金"
						tip = "回到所选基金的详情"
					}
					if ui.Button(c, label).Shrink(0).Tooltip(tip).Clicked() {
						a.togglePortfolio()
					}
				}
				// The picture is of the holding group, whatever group the
				// window lists: with none to draw, the choice is not offered,
				// and the look-through carries a share button of its own.
				if !a.portfolio && len(a.inGroup(holdingGroup)) > 0 {
					a.shareButton(c)
				}
				if ui.Button(c, "刷新").Shrink(0).Disabled(a.busy).Clicked() {
					a.refresh()
				}
			})
		})
	})
}

// updateStrip shows the newer version: what it is, how far its download
// got, and what the user can do about it.
func (a *App) updateStrip(c *ui.Context, t *ui.Theme) {
	u := a.upd
	ui.Column(c).Padding(10, 24, 10, 24).Gap(8).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "新版本 %s", u.version).FontSize(12.5).FontWeight(600).
				NoWrap().Shrink(0)
			ui.Text(c, u.status()).FontSize(11.5).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…").Grow(1).Basis(0).Tooltip(u.status())
			if u.notes != "" {
				label := "更新说明"
				if u.notesShown {
					label = "收起说明"
				}
				if ui.Button(c, label).Shrink(0).Clicked() {
					a.ToggleUpdateNotes()
				}
			}
			switch u.step {
			case updateFound:
				if ui.Button(c, "忽略").Shrink(0).
					Tooltip("这个版本不再提醒，下个版本照常提示").
					Clicked() {
					a.IgnoreUpdate()
				}
				if ui.PrimaryButton(c, "下载更新").Shrink(0).
					Tooltip("下载并校验签名，重启后生效").
					Clicked() {
					a.DownloadUpdate()
				}
			case updateReady:
				if ui.PrimaryButton(c, "安装并重启").Shrink(0).
					Tooltip("重启应用，用上刚下载的版本").
					Clicked() {
					a.RestartUpdate()
				}
			}
		})
		if u.step == updateFetching {
			ui.Progress(c, u.progress)
		}
		if u.notesShown && u.notes != "" {
			ui.Column(c).Padding(10, 12).Radius(10).Background(t.Surface).
				Border(1, t.Border).Children(func() {
				ui.Scroll(c).Height(72).Children(func() {
					ui.Text(c, u.notes).FontSize(11.5).TextColor(t.TextMuted)
				})
			})
		}
	})
}

// freshNote is how old the numbers are, in a word.
func (a *App) freshNote() string {
	switch {
	case a.busy:
		return "更新中…"
	case a.updated.IsZero():
		return "尚未更新"
	}
	return a.updated.In(cst).Format("15:04 更新")
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

// copyToClipboard puts text on the clipboard, and says what was copied, so
// that a click leaves a mark the user can see.
func copyToClipboard(c *ui.Context, what, text string) {
	c.WriteClipboard(text)
	c.Toast("已复制" + what + "：" + text)
}

func (a *App) detailTitle(c *ui.Context, t *ui.Theme, f *Fund) {
	change := f.ChangePct()
	label, color := "已公布净值", t.TextMuted
	badgeBg := t.Text.Alpha(0.07)
	if f.UsingEstimate() {
		label, color, badgeBg = "盘中估算", t.Accent, t.Accent.Alpha(0.14)
	}
	_, hasNav := f.LastNav()
	if !hasNav {
		label = "等待净值"
	}
	ui.Column(c).Gap(9).Children(func() {
		ui.Row(c).Gap(20).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(6).Children(func() {
				name := f.Name
				if name == "" {
					name = f.Code
				}
				// The name and the code copy themselves, as the user may be off
				// to look the fund up elsewhere.
				nameText := ui.Text(c, name).FontSize(19).FontWeight(600).
					Cursor(ui.CursorPointer).Tooltip("点击复制基金名称")
				if nameText.Clicked() {
					copyToClipboard(c, "名称", name)
				}
				ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
					codeText := ui.Text(c, f.Code).FontSize(11).TextColor(t.TextMuted).
						FontFeatures("tnum").Padding(1, 6).Radius(5).
						Background(t.SurfaceHover).Cursor(ui.CursorPointer).
						Tooltip("点击复制基金代码")
					if codeText.Clicked() {
						copyToClipboard(c, "代码", f.Code)
					}
					if f.Type != "" {
						pill(c, f.Type, t.TextMuted, t.Text.Alpha(0.06))
					}
					if a.group != holdingGroup {
						name := a.groupName(a.group)
						pill(c, name, t.Accent, t.Accent.Alpha(0.12)).
							Tooltip("这个分组只看涨跌，不记录持仓")
					}
				})
			})
			ui.Column(c).Shrink(0).AlignItems(ui.End).Gap(3).Children(func() {
				ui.Text(c, "当日涨跌").FontSize(11.5).TextColor(t.TextMuted).NoWrap()
				value := "—"
				if hasNav {
					value = pct(change)
				}
				ui.Text(c, value).FontSize(28).FontWeight(600).NoWrap().
					TextColor(up(t, change)).FontFeatures("tnum").
					ContentTransition(digits)
			})
		})
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			badge := pill(c, label, color, badgeBg)
			if !f.UsingEstimate() && f.Est.Cover > 0 && f.Est.Cover < minCover {
				// The day has an estimate, but too little of the fund is in
				// the disclosed holdings for it to be shown; say so rather
				// than leave the published value unexplained.
				badge.Tooltip(fmt.Sprintf(
					"重仓股覆盖 %.1f%%，不足 20%%，未显示盘中估算；可在设置里打开「低覆盖也显示估值」",
					f.Est.Cover))
			}
			if note := a.valueLine(f); note != "" {
				ui.Text(c, note).FontSize(11).TextColor(t.TextMuted).FontFeatures("tnum")
			}
		})
	})
}

// valueLine is the fund's own value under the day's change: the estimate
// while it is the newest number, and the published values otherwise.
func (a *App) valueLine(f *Fund) string {
	last, ok := f.LastNav()
	if f.UsingEstimate() {
		// An estimate shown over too few disclosed holdings says how much of
		// the fund it stands on.
		cover := ""
		if f.Est.Cover < minCover {
			cover = fmt.Sprintf(" · 重仓股覆盖 %.1f%%", f.Est.Cover)
		}
		if a.holds(f) {
			at := ""
			if !f.QuoteAt.IsZero() {
				at = " · " + f.QuoteAt.In(cst).Format("15:04")
			}
			return fmt.Sprintf("估算净值 %.4f", f.EstNav()) + at + cover
		}
		return f.QuoteAt.In(cst).Format("01-02 15:04") + cover
	}
	if !ok {
		return ""
	}
	if !a.holds(f) {
		return "净值日 " + last.Date
	}
	line := fmt.Sprintf("单位 %.4f", last.Unit)
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

// statCard is one measure of the fund. tip is the longer story a click on it
// tells, when the card has one.
type statCard struct {
	label string
	value string
	note  string
	color ui.Color
	tip   string
}

func (a *App) stats(c *ui.Context, t *ui.Theme, f *Fund) {
	last, hasLast := f.LastNav()
	var cards []statCard
	if a.holds(f) {
		cards = []statCard{
			{"今日收益", signedMoney(f.TodayProfit()), profitNote(f),
				up(t, f.TodayProfit()), ""},
			{"累计收益", signedMoney(f.TotalProfit()), pct(f.TotalPct()),
				up(t, f.TotalProfit()), profitTip(f)},
			{"持仓市值", money(f.Value()),
				fmt.Sprintf("%s 份", sharesText(f.Shares, 4)), t.Text, ""},
		}
	} else {
		// Without shares, the fund's own numbers are what the user came for.
		if hasLast {
			cards = append(cards, statCard{"单位净值",
				fmt.Sprintf("%.4f", last.Unit), last.Date, up(t, last.Pct), ""})
		}
		if f.UsingEstimate() {
			cards = append(cards, statCard{"估算净值",
				fmt.Sprintf("%.4f", f.EstNav()), "盘中估算", up(t, f.Est.Pct), ""})
		} else if hasLast && last.Acc > 0 {
			cards = append(cards, statCard{"累计净值",
				fmt.Sprintf("%.4f", last.Acc), last.Date, t.Text, ""})
		}
	}
	if len(cards) == 0 {
		return
	}
	ui.Row(c).Gap(10).Wrap().Children(func() {
		for _, card := range cards {
			col := ui.Column(c).Grow(1).Basis(0).MinWidth(128).Gap(3).
				Padding(11, 13).Radius(12).Background(t.Surface).
				Border(1, t.Border)
			if card.tip != "" {
				col.Tooltip(card.tip)
			}
			col.Children(func() {
				ui.Text(c, card.label).FontSize(11.5).TextColor(t.TextMuted)
				valueTip := card.value
				if card.tip != "" {
					valueTip = card.tip
				}
				ui.Text(c, card.value).FontSize(17).FontWeight(600).
					TextColor(card.color).FontFeatures("tnum").SingleLine().Ellipsis("…").Tooltip(valueTip).
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
		ui.Row(c).AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			ui.Text(c, "重仓股").FontSize(13).FontWeight(600).NoWrap().Shrink(0)
			if f.Report != "" {
				ui.Text(c, "报告期 "+f.Report).FontSize(11.5).TextColor(t.TextMuted).
					FontFeatures("tnum").NoWrap()
			}
			ui.Spacer(c)
			if f.StockPosition > 0 {
				ui.Textf(c, "股票仓位 %.1f%%", f.StockPosition).FontSize(11.5).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			}
			if f.HasEst {
				ui.Textf(c, "覆盖 %.1f%%", f.Est.Cover).FontSize(11.5).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
					Tooltip("前十大重仓股合计占基金净值的比例，越高估值越准")
			}
			// The record says how far the estimate has been off lately, so a
			// careful reader can weigh the number above it.
			switch acc := summarizeEstimateAccuracy(a.estimates[f.Code]); {
			case acc.Finished >= 3:
				ui.Textf(c, "估值偏差 ±%.2f%%", acc.Mean).FontSize(11.5).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
					Tooltip(acc.tip())
			case acc.Finished > 0:
				ui.Text(c, "估值偏差记录中").FontSize(11.5).
					TextColor(t.TextMuted).NoWrap().
					Tooltip("至少 3 个交易日的已公布净值之后，才会给出平均偏差")
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
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
						ui.Row(c).Gap(5).NoWrap().AlignItems(ui.Center).Children(func() {
							ui.Text(c, h.Name).FontSize(12).SingleLine().Ellipsis("…").
								Tooltip(h.Name).Shrink(1).MinWidth(0)
							marketTag(c, t, h)
						})
						p := f.Profiles[h.Symbol]
						if line := stockProfileLine(p); line != "" {
							detail := ui.Text(c, line).FontSize(10.5).
								TextColor(t.TextMuted).SingleLine().Ellipsis("…")
							if len(p.Concepts) > 0 {
								detail.Tooltip(line + "\n" + strings.Join(p.Concepts, " · "))
							} else {
								detail.Tooltip(line)
							}
						}
					})
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

// marketTag marks a holding that trades abroad, beside its name: another
// exchange than the fund's own matters when the user compares the stock's
// price. A holding of the mainland's exchanges needs no mark.
func marketTag(c *ui.Context, t *ui.Theme, h model.Holding) {
	label, fg := "", t.Accent
	switch h.Market() {
	case "hk":
		label = "港股"
	case "us":
		label, fg = "美股", t.Warning
	default:
		return
	}
	pill(c, label, fg, fg.Alpha(0.12)).Shrink(0)
}

// industriesCard shows the fund's allocation by industry, and how much of it
// each industry takes.
func (a *App) industriesCard(c *ui.Context, t *ui.Theme, f *Fund) {
	if len(f.Industries) == 0 {
		return
	}
	ui.Column(c).Gap(9).Children(func() {
		shown := make([]model.Industry, 0, len(f.Industries))
		for _, ind := range f.Industries {
			if ind.Weight >= 0.1 {
				shown = append(shown, ind)
			}
		}
		if len(shown) == 0 {
			shown = f.Industries
		}
		more := 0
		if len(shown) > 6 {
			more = len(shown) - 6
			shown = shown[:6]
		}
		ui.Row(c).AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			ui.Text(c, "行业配置").FontSize(13).FontWeight(600).NoWrap().Shrink(0)
			if f.IndustryReport != "" {
				ui.Text(c, "报告期 "+f.IndustryReport).FontSize(11.5).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			}
			ui.Spacer(c)
			if more > 0 {
				ui.Textf(c, "另有 %d 个较小行业", more).FontSize(11.5).
					TextColor(t.TextMuted).NoWrap()
			}
		})
		body := card(c, t)
		body.Gap(9).Children(func() {
			maxWeight := shown[0].Weight
			for _, ind := range shown {
				value := 0.0
				if maxWeight > 0 {
					value = ind.Weight / maxWeight
				}
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, ind.Name).FontSize(11.5).Grow(1).Basis(0).
						SingleLine().Ellipsis("…").Tooltip(ind.Name)
					ui.Progress(c, math.Max(0, math.Min(1, value))).
						Width(118).Shrink(0)
					ui.Textf(c, "%.1f%%", ind.Weight).FontSize(11.5).
						TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
						Width(52).TextAlign(ui.End)
				})
			}
		})
	})
}

// stockProfileLine names a stock's industry, when it has one.
func stockProfileLine(p model.StockProfile) string {
	return p.Industry
}

// holdingSection shows the shares the user holds, and their price: the two
// numbers typed by hand, or the transactions that work both out.
func (a *App) holdingSection(c *ui.Context, t *ui.Theme, f *Fund) {
	if !a.editing {
		// The fields fill in from the values as the edit opens.
		f.editText = false
	}
	if a.tx.open && a.tx.code != f.Code {
		// A draft belongs to the fund it was started for.
		a.cancelTxn()
	}
	body := card(c, t)
	body.Padding(8, 15).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "我的持仓").FontSize(12.5).FontWeight(600).NoWrap().Shrink(0)
			note := holdingNote(f, a.editing)
			ui.Text(c, note).FontSize(11.5).TextColor(t.TextMuted).FontFeatures("tnum").
				Grow(1).Basis(0).SingleLine().Ellipsis("…").Tooltip(note)
			switch {
			case a.editing:
				if ui.PrimaryButton(c, "完成").Shrink(0).Clicked() {
					a.editing = false
					a.cancelTxn()
				}
			case f.ledgered():
				if ui.Button(c, "交易记录").Shrink(0).Clicked() {
					a.editing = true
				}
			default:
				label := "录入持仓"
				if f.Shares > 0 {
					label = "编辑持仓"
				}
				if ui.Button(c, label).Shrink(0).Clicked() {
					a.editing = true
				}
			}
		})
		a.holdingBody(c, t, f)
	})
	a.txToast(c)
}

// targetFields are a held fund's price targets: the holding return at which
// the app announces it. Empty leaves one side off.
func (a *App) targetFields(c *ui.Context, t *ui.Theme, f *Fund) {
	ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
		ui.Column(c).Gap(5).Grow(1).Basis(0).Children(func() {
			ui.Text(c, "止盈线（%）").FontSize(11.5).TextColor(t.TextMuted)
			if ui.TextInput(c, &f.takeText).Placeholder("不提醒").
				Label("止盈输入").
				Tooltip("持有收益率达到这个百分数时提醒，留空关闭").
				Changed() && readOptional(f.takeText, &f.TakeProfit, 0.1, 1000) {
				a.changed()
			}
		})
		ui.Column(c).Gap(5).Grow(1).Basis(0).Children(func() {
			ui.Text(c, "止损线（%）").FontSize(11.5).TextColor(t.TextMuted)
			if ui.TextInput(c, &f.stopText).Placeholder("不提醒").
				Label("止损输入").
				Tooltip("持有收益率跌到这个百分数时提醒，留空关闭").
				Changed() && readOptional(f.stopText, &f.StopLoss, 0.1, 1000) {
				a.changed()
			}
		})
	})
}

// targetLine names the price targets set on a fund, and "" when it has none.
func targetLine(f *Fund) string {
	var parts []string
	if f.TakeProfit > 0 {
		parts = append(parts, "止盈 +"+trim(f.TakeProfit, 2)+"%")
	}
	if f.StopLoss > 0 {
		parts = append(parts, "止损 -"+trim(f.StopLoss, 2)+"%")
	}
	return strings.Join(parts, " · ")
}

// targetText is a target as its field shows it: empty when the side is off.
func targetText(v float64) string {
	if v <= 0 {
		return ""
	}
	return trim(v, 2)
}

// readNumber reads a field's text as a number between lo and hi into v, and
// reports whether it did: text that is empty, not a number, or out of range
// leaves v as it was.
func readNumber(text string, v *float64, lo, hi float64) bool {
	s := strings.ReplaceAll(strings.TrimSpace(text), ",", "")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || n < lo || n > hi {
		return false
	}
	*v = n
	return true
}

// readOptional reads a field's text as a number between lo and hi into v,
// where an empty field turns the setting off.
func readOptional(text string, v *float64, lo, hi float64) bool {
	if strings.TrimSpace(text) == "" {
		*v = 0
		return true
	}
	return readNumber(text, v, lo, hi)
}

// trim writes a number with at most n decimals, and none that are zero.
func trim(v float64, n int) string {
	s := fmt.Sprintf("%.*f", n, v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// joinMenu adds the fund of a row to a group it is not in yet, from the menu
// of the list.
func (a *App) joinMenu(m *ui.Menu, f *Fund, row int) {
	missing := 0
	for _, g := range a.groups {
		if !f.In(g.ID) {
			missing++
		}
	}
	if !f.In(holdingGroup) {
		missing++
	}
	if missing == 0 {
		return
	}
	m.Submenu("加入分组", func(sub *ui.Menu) {
		if !f.In(holdingGroup) {
			if sub.Item("持有").Chosen() {
				a.joinAt(row, holdingGroup)
			}
		}
		for _, g := range a.groups {
			if f.In(g.ID) {
				continue
			}
			id := g.ID
			if sub.Item(g.Name).Chosen() {
				a.joinAt(row, id)
			}
		}
	})
}
