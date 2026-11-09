package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fundbar/internal/store"
)

// OpenSettings shows the settings window, making it the first time.
func (a *App) OpenSettings() {
	if a.settings != nil {
		a.settings.Show()
		a.settings.Focus()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "设置",
		Width:     480,
		Height:    430,
		MinWidth:  420,
		MinHeight: 360,
		StateKey:  "settings",
		Content:   ui.View(a.settingsView),
	})
	a.settings = w
	w.OnClose(func(*mygo.CloseEvent) { a.settings = nil })
	w.Show()
	w.Focus()
}

// settingsView draws the settings window.
func (a *App) settingsView(c *ui.Context) {
	t := theme(c)
	c.SetTheme(t)

	// The system keeps the login item, so it is what the switch shows.
	openAtLogin := a.loginItem()

	ui.Column(c).Fill().Background(t.Background).Padding(20, 22).Gap(18).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "通用").FontSize(11.5).FontWeight(600).TextColor(t.TextMuted)
			box := card(c, t)
			box.Gap(10).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
					ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
						ui.Text(c, "登录时自动启动").FontSize(13)
						ui.Text(c, "登录后直接待在菜单栏，不打开窗口").
							FontSize(11.5).TextColor(t.TextMuted)
					})
					if ui.Switch(c, &openAtLogin).Shrink(0).Changed() {
						a.openAtLogin(openAtLogin)
					}
				})
				if a.settingsErr != "" {
					ui.Text(c, a.settingsErr).FontSize(11.5).
						TextColor(t.Danger)
				}
			})
		})

		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "数据").FontSize(11.5).FontWeight(600).TextColor(t.TextMuted)
			box := card(c, t)
			box.Gap(10).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, "缓存").FontSize(13)
						ui.Text(c, "历史净值和重仓股存在本地，每次启动先读缓存").
							FontSize(11.5).TextColor(t.TextMuted)
					})
					if ui.Button(c, "清空并刷新").Shrink(0).Clicked() {
						a.clearCache()
					}
				})
				ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, "自选基金").FontSize(13)
						ui.Text(c, a.dir).FontSize(11.5).TextColor(t.TextMuted).
							SingleLine().Ellipsis("…").Tooltip(a.dir)
					})
					if ui.Button(c, "打开文件夹").Shrink(0).Clicked() {
						mygo.Shell.OpenPath(a.dir)
					}
				})
			})
		})

		ui.Spacer(c)

		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Textf(c, "FundBar %s", a.version()).FontSize(12).
					FontWeight(500)
				ui.Text(c, "· MIT").FontSize(12).TextColor(t.TextMuted)
				ui.Spacer(c)
				ui.Link(c, "GitHub", "https://github.com/Alickx/mygo-fundbar").
					FontSize(12)
			})
			ui.Text(c, "数据来自天天基金与腾讯财经的公开接口；估值由季报重仓股推算，"+
				"仅供参考，实际净值以基金公司公布为准。").FontSize(11).
				TextColor(t.TextMuted)
		})
	})
}

// loginItem reports whether the app starts at login, and false in a view
// drawn without a running app, as tools/shot draws one.
func (a *App) loginItem() bool {
	if !mygo.App.IsReady() {
		return a.previewLogin
	}
	return mygo.App.OpenAtLogin()
}

// version is the app's, or the one the previews show when drawn without a
// running app.
func (a *App) version() string {
	if !mygo.App.IsReady() {
		return previewVersion
	}
	return mygo.App.Version()
}

// previewVersion is what a window drawn without a running app shows, as
// tools/shot draws one; mygo.json holds the version of a build.
const previewVersion = "0.2.0"

// openAtLogin starts the app at login, or stops it, as the user asked.
func (a *App) openAtLogin(on bool) {
	if !mygo.App.IsReady() {
		a.previewLogin = on
		return
	}
	if err := mygo.App.SetOpenAtLogin(on); err != nil {
		a.settingsErr = "设置失败：" + err.Error()
		return
	}
	a.settingsErr = ""
}

// clearCache forgets what was read about the funds and asks again.
func (a *App) clearCache() {
	for _, f := range a.funds {
		store.DropSnapshot(a.dir, f.Code)
		f.Loaded, f.HasEst = false, false
		f.Holdings, f.Quotes, f.Navs = nil, nil, nil
	}
	a.refresh()
}
