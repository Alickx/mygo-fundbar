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
	// The threshold field fills in from the setting each time the window
	// opens, rather than from whatever was typed last time.
	a.alertSeeded = false
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "设置",
		Width:     480,
		Height:    760,
		MinWidth:  420,
		MinHeight: 360,
		// A key of its own since the window grew: the state an earlier
		// version saved would open it at the old height, with the newer
		// sections out of sight until the user scrolled to them.
		StateKey: "settings-v2",
		Content:  ui.View(a.settingsView),
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
	if !a.alertSeeded {
		a.alertText = trim(a.alertConfig().MovePct, 2)
		a.alertSeeded = true
	}

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		// The sections scroll, so that a window left short, as one an earlier
		// version saved, still reaches all of them; the version line stays in
		// sight below.
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(20, 22, 14, 22).Gap(18).Children(func() {
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
						// What the menu bar item shows beside its icon. The
						// modes are kept as the choice's index, as the
						// segmented control works in indices.
						ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, "菜单栏显示").FontSize(13)
								ui.Text(c, "菜单栏图标旁显示收益、涨跌幅，或只留图标").
									FontSize(11.5).TextColor(t.TextMuted)
							})
							mode := trayModeIndex(a.trayText)
							if ui.Segmented(c, &mode, trayLabels...).Shrink(0).
								Label("菜单栏显示").Changed() {
								a.trayText = trayModeName(mode)
								a.save()
								a.updateTray()
							}
						})
						ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, "全局快捷键").FontSize(13)
								ui.Text(c, "在任何应用中按 ⌃⌥F 打开或收起小窗").
									FontSize(11.5).TextColor(t.TextMuted)
							})
							on := a.globalShortcut
							if ui.Switch(c, &on).Shrink(0).Changed() {
								a.applyGlobalShortcut(on)
								a.save()
							}
						})
						ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, "小窗显示大盘指数").FontSize(13)
								ui.Text(c, "小窗顶部显示上证、沪深300 和创业板的涨跌").
									FontSize(11.5).TextColor(t.TextMuted)
							})
							on := a.showIndices
							if ui.Switch(c, &on).Shrink(0).Changed() {
								a.showIndices = on
								a.save()
								a.redrawPanel()
							}
						})
						ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, "低覆盖也显示估值").FontSize(13)
								ui.Text(c, "重仓股覆盖不足 20% 时仍给出盘中估算，数字偏差可能更大").
									FontSize(11.5).TextColor(t.TextMuted)
							})
							on := a.lowCoverEstimate
							if ui.Switch(c, &on).Shrink(0).Changed() {
								a.setLowCoverEstimate(on)
							}
						})
					})
				})

				a.alertSettings(c, t)

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
						ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, "备份").FontSize(13)
								ui.Text(c, "自选、分组、持仓与交易记录存成一个文件，换电脑时带走").
									FontSize(11.5).TextColor(t.TextMuted)
							})
							ui.Row(c).Gap(8).Shrink(0).Children(func() {
								if ui.Button(c, "导出备份…").Shrink(0).Clicked() {
									a.ExportBackup()
								}
								if ui.Button(c, "导入备份…").Shrink(0).Clicked() {
									a.ImportBackup()
								}
							})
						})
						ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, "持仓表").FontSize(13)
								ui.Text(c, "当前持仓与收益，可用表格软件打开").
									FontSize(11.5).TextColor(t.TextMuted)
							})
							if ui.Button(c, "导出 CSV…").Shrink(0).Clicked() {
								a.ExportCSV()
							}
						})
					})
					if a.dataErr != "" {
						ui.Text(c, a.dataErr).FontSize(11.5).TextColor(t.Danger)
					}
				})

			})
		})

		ui.Column(c).Padding(0, 22, 20, 22).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Textf(c, "FundBar %s", a.version()).FontSize(12).
					FontWeight(500)
				ui.Text(c, "· MIT").FontSize(12).TextColor(t.TextMuted)
				if v := a.availableVersion(); v != "" {
					ui.Text(c, "新版本 "+v).FontSize(12).TextColor(t.Accent)
				}
				ui.Spacer(c)
				if ui.Button(c, "检查更新").Shrink(0).Clicked() {
					a.CheckUpdate()
				}
				ui.Link(c, "GitHub", "https://github.com/Alickx/mygo-fundbar").
					FontSize(12)
			})
			ui.Text(c, "数据来自天天基金与腾讯财经的公开接口；估值由季报重仓股推算，"+
				"仅供参考，实际净值以基金公司公布为准。").FontSize(11).
				TextColor(t.TextMuted)
		})
	})
}

// alertSettings draws the notification rules: what the app says without
// being asked, and the button that finds out whether the system allows it.
func (a *App) alertSettings(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, "提醒").FontSize(11.5).FontWeight(600).TextColor(t.TextMuted)
		box := card(c, t)
		box.Gap(12).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "盘中涨跌提醒").FontSize(13)
					ui.Text(c, "估值涨跌达到阈值时提醒，每只基金每天一次").
						FontSize(11.5).TextColor(t.TextMuted)
				})
				on := a.alerts.Move
				if ui.Switch(c, &on).Shrink(0).Changed() {
					a.alerts.Move = on
					if on && a.alerts.MovePct == 0 {
						a.alerts.MovePct = defaultMovePct
					}
					a.save()
				}
			})
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "涨跌幅阈值").FontSize(11.5).TextColor(t.TextMuted).Shrink(0)
				in := ui.TextInput(c, &a.alertText).Placeholder("2").Width(58).
					Shrink(0).Label("涨跌阈值输入").
					Tooltip("填 0.5 到 20 之间的百分数，默认 2")
				if in.Changed() {
					v := a.alerts.MovePct
					if readNumber(a.alertText, &v, 0.5, 20) && v != a.alerts.MovePct {
						a.alerts.MovePct = v
						a.save()
					}
				}
				ui.Text(c, "%").FontSize(12).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, "0.5 到 20，默认 2").FontSize(11).TextColor(t.TextMuted).
					SingleLine().Ellipsis("…").Grow(1).Basis(0)
			})
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "净值公布提醒").FontSize(13)
					ui.Text(c, "持有分组当天净值公布后提醒，22:00 后部分公布也会提醒").
						FontSize(11.5).TextColor(t.TextMuted)
				})
				on := a.alerts.Settled
				if ui.Switch(c, &on).Shrink(0).Changed() {
					a.alerts.Settled = on
					a.save()
				}
			})
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "测试通知").FontSize(13)
					ui.Text(c, "先发一条，确认系统允许 FundBar 提醒").
						FontSize(11.5).TextColor(t.TextMuted)
				})
				if ui.Button(c, "发送测试通知").Shrink(0).Clicked() {
					a.sendTestNotification()
				}
			})
			note := a.alertErr
			if note == "" && mygo.App.IsReady() && !mygo.NotificationsSupported() {
				// A development run shows no notifications at all, which the
				// user should learn here rather than from silence.
				note = unsupportedAlert
			}
			if note != "" {
				ui.Text(c, note).FontSize(11.5).TextColor(t.Danger)
			}
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
const previewVersion = "0.5.0"

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
	for _, f := range a.all {
		store.DropSnapshot(a.dir, f.Code)
		f.Loaded, f.HasEst = false, false
		f.Holdings, f.Quotes, f.Navs = nil, nil, nil
	}
	a.refresh()
}
