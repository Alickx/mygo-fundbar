package app

import (
	"bytes"
	"image"
	"image/png"
	"os"

	"github.com/egoist/mygo"

	"github.com/egoist/mygo/ui"
)

// The share picture is a poster of the day, not a screenshot: 720 DIP at
// scale 2 gives a 1440-pixel-wide PNG that reads in a chat window, and the
// dark look is the picture's own whatever the system appearance.
const (
	shareWidth = 720
	shareScale = 2
	// shareFunds is how many of the holding group's funds the picture lists
	// before the rest wait behind a count.
	shareFunds = 10
)

// changeDay is the day the fund's ChangePct belongs to: the estimate's prices
// while those are newer than the published value, and the newest published
// value's date otherwise. It is "" for a fund with neither.
func changeDay(f *Fund) string {
	if f.UsingEstimate() && !f.QuoteAt.IsZero() {
		return f.QuoteAt.In(cst).Format("2006-01-02")
	}
	if last, ok := f.LastNav(); ok {
		return last.Date
	}
	return ""
}

// shareDay is the date the picture's numbers belong to: the newest day the
// holding group's funds know of, from an estimate's prices or the published
// value it stands on.
func shareDay(funds []*Fund) string {
	day := ""
	for _, f := range funds {
		if d := changeDay(f); d > day {
			day = d
		}
	}
	if day == "" {
		day = now().Format("2006-01-02")
	}
	return day
}

// shareDetail is the word the detail title would use for the day's numbers:
// an estimate while any fund's prices are newer than its published value,
// and the published values otherwise.
func shareDetail(funds []*Fund) string {
	for _, f := range funds {
		if f.UsingEstimate() {
			return "盘中估算"
		}
	}
	return "已公布净值"
}

// posterTheme is the picture's dark look. It is made here rather than read
// from the window so that the poster is the same on a light desktop.
func posterTheme() *ui.Theme {
	base := ui.DarkTheme()
	t := *base
	t.Background = ui.Hex("#0e0e14")
	t.Surface = ui.Hex("#17171f")
	t.SurfaceHover = ui.Hex("#20202a")
	t.Border = ui.Hex("#2b2b37")
	t.Text = ui.Hex("#f4f4fa")
	t.TextMuted = ui.Hex("#a0a0b2")
	t.Accent = ui.Hex("#5b9dff")
	t.Radius = 10
	return &t
}

// shareView draws the picture's content. money leaves the amounts out by
// default: the poster says how the funds moved, and adding what the user
// owns is a choice, made where the picture is copied or saved.
func (a *App) shareView(c *ui.Context, money bool) {
	t := posterTheme()
	c.SetTheme(t)
	// Only the poster paints, so the frame below it stays transparent and
	// the render can be cut to the content's height.
	c.Root().Background(ui.Transparent)

	held := a.inGroup(holdingGroup)
	s := a.Summary()
	day := shareDay(held)
	detail := shareDetail(held)

	ui.Column(c).FillWidth().Padding(44, 40).Background(t.Background).
		Gap(22).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "基金估值").FontSize(17).FontWeight(600).
				TextColor(t.Text).NoWrap()
			ui.Spacer(c)
			ui.Text(c, day).FontSize(13).TextColor(t.TextMuted).
				FontFeatures("tnum").NoWrap()
		})
		ui.Column(c).Gap(4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "今日涨跌").FontSize(12.5).
					TextColor(t.TextMuted).NoWrap()
				fg, bg := t.TextMuted, t.Text.Alpha(0.08)
				if detail == "盘中估算" {
					fg, bg = t.Accent, t.Accent.Alpha(0.14)
				}
				pill(c, detail, fg, bg)
			})
			ui.Text(c, pct(s.Pct)).FontSize(68).FontWeight(700).
				TextColor(up(t, s.Pct)).FontFeatures("tnum").NoWrap()
			if money && s.Holding {
				ui.Text(c, "今日收益 "+signedMoney(s.Profit)+" 元").
					FontSize(17).FontWeight(500).TextColor(up(t, s.Profit)).
					FontFeatures("tnum").NoWrap()
			}
		})
		ui.Divider(c)
		ui.Column(c).Gap(11).Children(func() {
			shown := held
			if len(shown) > shareFunds {
				shown = shown[:shareFunds]
			}
			// Some funds may have no estimate today and stand on the day
			// before, which in the morning is the usual case. When they do,
			// every row says its day, so that the column reads as one and no
			// row looks as if its date were forgotten.
			mixed := false
			for _, f := range shown {
				if d := changeDay(f); d != "" && d != day {
					mixed = true
					break
				}
			}
			for _, f := range shown {
				change := f.ChangePct()
				tint := up(t, change)
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).
					Children(func() {
						ui.Text(c, fundName(f)).FontSize(15).TextColor(t.Text).
							Grow(1).Basis(0).MinWidth(0).SingleLine().
							Ellipsis("…").Tooltip(fundName(f))
						if money && f.Shares > 0 {
							ui.Text(c, signedMoney(f.TodayProfit())+" 元").
								FontSize(13).TextColor(up(t, f.TodayProfit())).
								FontFeatures("tnum").NoWrap().Shrink(0)
						}
						if mixed {
							label := ""
							if d := changeDay(f); len(d) >= 10 {
								label = d[5:]
							}
							ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).
								FontFeatures("tnum").NoWrap().Shrink(0)
						}
						sharePill(c, pct(change), tint).Shrink(0)
					})
			}
			if more := len(held) - len(shown); more > 0 {
				ui.Textf(c, "另有 %d 只", more).FontSize(12).
					TextColor(t.TextMuted).NoWrap()
			}
		})
		ui.Text(c, "估值仅供参考，以基金公司公布为准 · FundBar").
			FontSize(11.5).TextColor(t.TextMuted).NoWrap()
	})
}

// sharePill is a day's change in a tinted chip, as the list rows draw it.
func sharePill(c *ui.Context, text string, tint ui.Color) ui.Element {
	return ui.Text(c, text).FontSize(13).FontWeight(600).NoWrap().
		TextColor(tint).FontFeatures("tnum").Padding(3, 10).Radius(999).
		Background(tint.Alpha(0.16)).Border(1, tint.Alpha(0.24))
}

// shareImage renders the picture offscreen and cuts the transparent bottom
// away, so the PNG is exactly as tall as its content.
func (a *App) shareImage(money bool) *image.RGBA {
	// The frame is generous: the poster stays at the top and paints only
	// itself, so whatever is left below is transparent and trimmed.
	img := ui.Render(func(c *ui.Context) { a.shareView(c, money) },
		shareWidth, 1600, shareScale)
	return trimTransparent(img)
}

// SharePreview returns the share picture, for tools/shot to write without
// the clipboard or a dialog.
func (a *App) SharePreview(money bool) *image.RGBA { return a.shareImage(money) }

// trimTransparent cuts the rows below the last painted one.
func trimTransparent(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	height := b.Min.Y
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		row := false
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0 {
				row = true
				break
			}
		}
		if row {
			height = y + 1
			break
		}
	}
	if height <= b.Min.Y {
		return img
	}
	return img.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, height)).(*image.RGBA)
}

// sharePNG encodes the picture.
func (a *App) sharePNG(money bool) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, a.shareImage(money)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CopyShare puts the picture on the clipboard, percent only or with the
// user's amounts, and says so in the status row. It draws the picture on the
// thread that calls it — a menu's click, or a view's click put off with
// afterFrame so that no frame is being built — because the picture reads the
// funds, which only the main thread may touch.
func (a *App) CopyShare(money bool) {
	data, err := a.sharePNG(money)
	if err != nil {
		a.reportShare("生成分享图失败：" + err.Error())
		return
	}
	if err := a.writeImage(data); err != nil {
		a.reportShare("复制分享图失败：" + err.Error())
		return
	}
	a.reportShare("已复制分享图，可直接粘贴")
}

// SaveShare asks where to save the picture and writes it there. The picture
// is drawn first, on the calling thread, as CopyShare draws it; the dialog
// blocks until it is dismissed, so only the asking and the writing run off
// the main thread.
func (a *App) SaveShare() {
	data, err := a.sharePNG(false)
	if err != nil {
		a.reportShare("生成分享图失败：" + err.Error())
		return
	}
	name := "FundBar 今日收益 " + shareDay(a.inGroup(holdingGroup)) + ".png"
	if a.win == nil {
		a.saveShare(name, data)
		return
	}
	go a.saveShare(name, data)
}

func (a *App) saveShare(name string, data []byte) {
	path, err := a.saveFile(mygo.SaveDialogOptions{
		Title:       "保存分享图",
		ButtonLabel: "保存",
		DefaultPath: backupPath(name),
		Filters:     []mygo.FileFilter{{Name: "PNG", Extensions: []string{"png"}}},
	})
	if err != nil {
		a.reportShare("保存分享图失败：" + err.Error())
		return
	}
	if path == "" {
		// The user cancelled: nothing to do, and nothing to say.
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		a.reportShare("保存分享图失败：" + err.Error())
		return
	}
	a.reportShare("已保存分享图")
}

// reportShare says how a copy or a save went in the status row.
func (a *App) reportShare(msg string) {
	a.onMain(func() { a.notice(msg) })
}

// shareButton is the summary bar's share menu: the same choices the menu bar
// item's menu offers, where the window's user is already looking.
func (a *App) shareButton(c *ui.Context) {
	ui.PopoverMenuButton(c, "分享", func(m *ui.Menu) {
		if m.Item("复制图片（仅涨跌幅）").Chosen() {
			a.afterFrame(func() { a.CopyShare(false) })
		}
		if m.Item("复制图片（含金额）").Chosen() {
			a.afterFrame(func() { a.CopyShare(true) })
		}
		if m.Item("保存图片…").Chosen() {
			a.afterFrame(a.SaveShare)
		}
	}).Shrink(0).Tooltip("生成今日收益图，复制或保存")
}

// shareMenu is the tray's share submenu. It is disabled as a whole while
// the holding group is empty, and each item stays disabled too, so that no
// platform offers a choice it cannot fulfil.
func (a *App) shareMenu(empty bool) *mygo.MenuItem {
	items := []*mygo.MenuItem{
		{Label: "复制图片（仅涨跌幅）", Disabled: empty,
			Click: func(*mygo.MenuItem, *mygo.Window) { a.CopyShare(false) }},
		{Label: "复制图片（含金额）", Disabled: empty,
			Click: func(*mygo.MenuItem, *mygo.Window) { a.CopyShare(true) }},
		{Label: "保存图片…", Disabled: empty,
			Click: func(*mygo.MenuItem, *mygo.Window) { a.SaveShare() }},
	}
	return &mygo.MenuItem{Label: "分享今日收益", Submenu: items, Disabled: empty}
}
