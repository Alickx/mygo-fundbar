package app

import (
	"errors"
	"time"

	"github.com/egoist/mygo"

	"fundbar/internal/store"
)

// deniedAlert is what the settings window says when the user took the
// permission to notify away.
const deniedAlert = "通知权限被拒绝。请在「系统设置 → 通知 → FundBar」中允许通知，然后点「发送测试通知」重试。"

// unsupportedAlert is what it says when the app runs unpackaged, as `go run`
// starts it: the system shows the notifications of a packaged app only.
const unsupportedAlert = "当前运行方式不支持系统通知。macOS 只为打包后的应用显示通知，请使用安装包，或以 go tool mygo dev 运行。"

// watchNotifications opens what a clicked note is about. The system hands
// back the id of every note of the app the user clicks, also one that a run
// before this one showed, so the id is what says what to open.
func (a *App) watchNotifications() {
	mygo.App.OnNotificationClick(func(id string) {
		if code, ok := noteFund(id); ok && a.byCode(code) != nil {
			a.ShowFund(code)
			return
		}
		a.Show()
	})
}

// runAlerts decides whether the funds a refresh just filled are worth
// announcing, and shows what it decided. It runs on the main thread, at the
// end of the refresh.
func (a *App) runAlerts() {
	// A preview and a test draw the windows without a running app, and must
	// not post anything or mark it as said.
	if !mygo.App.IsReady() {
		return
	}
	notes, state := evaluate(a.alertConfig(), a.alertState, a.all, time.Now().In(cst))
	if !state.Equal(a.alertState) {
		// A refresh that announced nothing writes nothing.
		a.alertState = state
		if err := store.SaveAlerts(a.dir, &state); err != nil && a.failure == "" {
			a.failure = "保存提醒状态失败：" + err.Error()
		}
	}
	for _, n := range notes {
		if a.notify != nil {
			a.notify(n)
		}
	}
}

// alertConfig is the rules the settings hold, with the move threshold's
// default filled in.
func (a *App) alertConfig() AlertConfig {
	cfg := AlertConfig{Move: a.alerts.Move, MovePct: a.alerts.MovePct, Settled: a.alerts.Settled}
	if cfg.MovePct <= 0 {
		cfg.MovePct = defaultMovePct
	}
	return cfg
}

// deliver shows a note through the system. The first note an app shows waits
// for the user to say whether it may show any, so the wait is kept off the
// main thread, where it would hold up the frame or the refresh that asked
// for the note; the outcome comes back to the windows.
func (a *App) deliver(n Note) {
	if !mygo.App.IsReady() || a.win == nil {
		return
	}
	if !mygo.NotificationsSupported() {
		a.delivered(unsupportedAlert)
		return
	}
	notif := mygo.NewNotification(mygo.NotificationOptions{
		ID: n.ID, Title: n.Title, Body: n.Body,
	})
	go func() {
		err := notif.Show()
		a.win.Update(func() { a.delivered(alertError(err)) })
	}()
}

// alertError is what the settings window says about a note that did not
// arrive: what to do about a permission the user took away, or the error
// itself. A note that arrived leaves nothing to say.
func alertError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, mygo.ErrNotificationsDenied):
		return deniedAlert
	}
	return "发送通知失败：" + err.Error()
}

// delivered keeps what there is to say about the last note for the settings
// window, and shows it there.
func (a *App) delivered(msg string) {
	a.alertErr = msg
	if a.settings != nil {
		a.settings.Invalidate()
	}
}

// sendTestNotification shows a note whatever the switches say: the usual
// reason notifications do not arrive is a permission, and this is how the
// user finds out.
func (a *App) sendTestNotification() {
	if !mygo.App.IsReady() {
		return
	}
	if a.notify != nil {
		a.notify(Note{
			ID:    allNote + "test",
			Title: "FundBar 测试通知",
			Body:  "看到这条通知，说明提醒可以正常送达。",
		})
	}
}
