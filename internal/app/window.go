package app

import (
	"github.com/egoist/mygo"
)

// Attach gives the app its window and its item in the menu bar.
func (a *App) Attach(win *mygo.Window, tray *mygo.Tray) {
	a.win = win
	a.tray = tray
	a.updateTray()
}

// Show brings the window to the front, on the fund chosen.
func (a *App) Show() {
	if a.win == nil {
		return
	}
	a.win.Show()
	a.win.Focus()
	a.win.Invalidate()
}

// ShowFund brings the window to the front, showing a fund.
func (a *App) ShowFund(code string) {
	a.want = code
	a.Show()
}

// Hide takes the window away, keeping the app in the menu bar.
func (a *App) Hide() {
	if a.win != nil {
		a.win.Hide()
	}
}

func (a *App) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// quit stops the app.
func quit() { mygo.App.Quit() }

// changed keeps the funds the user edited, and says their gain again in the
// menu bar.
func (a *App) changed() {
	a.save()
	a.updateTray()
}

// removeAt takes a fund out of the list after the frame that asked.
func (a *App) removeAt(i int) { a.pendingRemove = i }

// resolveWanted shows the fund the menu bar item chose.
func (a *App) resolveWanted() {
	if a.want == "" {
		return
	}
	code := a.want
	a.want = ""
	for i, f := range a.funds {
		if f.Code == code {
			a.sel = i
			a.pick = -1
			return
		}
	}
}

// TrayMenu is the menu of the item in the menu bar: the funds the user
// follows, and what the app does.
func (a *App) TrayMenu() *mygo.Menu {
	s := a.Summary()
	var items []*mygo.MenuItem

	headline := "还没有自选基金"
	switch {
	case s.Holding:
		headline = "今日收益 " + signedMoney(s.Profit) + "（" + pct(s.Pct) + "）"
	case len(a.funds) > 0:
		headline = "自选平均 " + pct(s.Pct)
	}
	items = append(items, &mygo.MenuItem{Label: headline, Disabled: true})

	if len(a.funds) > 0 {
		items = append(items, mygo.Separator())
		for i, f := range a.funds {
			if i >= 12 {
				break
			}
			f := f
			code := f.Code
			label := f.Name
			if label == "" {
				label = f.Code
			}
			sub := f.Code + "  " + pct(f.ChangePct())
			if f.Shares > 0 {
				sub += "  今日 " + signedMoney(f.TodayProfit())
			}
			items = append(items, &mygo.MenuItem{
				Label:    truncate(label, 20),
				Subtitle: sub,
				Click: func(*mygo.MenuItem, *mygo.Window) {
					a.ShowFund(code)
				},
			})
		}
	}

	items = append(items, mygo.Separator(),
		&mygo.MenuItem{Label: "刷新", Accelerator: "CmdOrCtrl+R",
			Click: func(*mygo.MenuItem, *mygo.Window) { a.refresh() }},
		&mygo.MenuItem{Label: "打开面板", Accelerator: "CmdOrCtrl+Shift+O",
			Click: func(*mygo.MenuItem, *mygo.Window) { a.Show() }},
		&mygo.MenuItem{Label: "设置…",
			Click: func(*mygo.MenuItem, *mygo.Window) { a.OpenSettings() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: "退出", Role: mygo.RoleQuit},
	)
	return mygo.NewMenu(items)
}

// truncate shortens a label to n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
