package app

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo"
)

// The app updates itself in its own window: the strip under the summary
// names the newer version, downloads it when the user says so, and restarts
// into it. mygo.Updater does the work, so the archive is downloaded and
// checked against the key built into the app before it replaces anything;
// the updater plugin would show a window of its own instead, asking for a
// click the strip can take.
//
// The strip shows nothing in a build that cannot update itself, such as
// `go run .`.

// updateStep is what the strip offers.
type updateStep int

const (
	// updateNone shows nothing: no check yet, or none found a version.
	updateNone updateStep = iota
	// updateFound has a newer version, which the user may download.
	updateFound
	// updateFetching downloads it.
	updateFetching
	// updateReady downloaded it: the next launch runs it.
	updateReady
)

// updateState is what the strip shows. It changes on the main thread only.
type updateState struct {
	step     updateStep
	version  string
	notes    string
	err      string
	progress float64
	// notesShown unfolds the release notes.
	notesShown bool
}

// status is the line the strip shows beside the version.
func (u updateState) status() string {
	switch u.step {
	case updateFound:
		if u.err != "" {
			return u.err
		}
		return "下载后重启即可用上"
	case updateFetching:
		return fmt.Sprintf("正在下载 %d%%", int(u.progress*100))
	case updateReady:
		return "已下载完成，重启应用后生效"
	case updateNone:
		return ""
	}
	return ""
}

// firstCheck is how long after launch the app looks for a newer version,
// checkEvery how often it looks again while it runs, and activationEvery the
// shortest gap between the checks that bringing the app forward asks for.
const (
	firstCheck      = 10 * time.Second
	checkEvery      = 5 * time.Minute
	activationEvery = time.Hour
)

// watchTick is how often the app asks the clock whether checkEvery has
// passed, rather than sleeping checkEvery in one go: macOS stops the
// monotonic clock while it sleeps, and a timer on it wakes hours late after
// a night's sleep. The wall clock keeps counting, so each tick reads it.
const watchTick = 10 * time.Second

// watchUpdates looks for a newer version a moment after launch, and then
// every checkEvery for as long as the app runs.
func (a *App) watchUpdates() {
	if a.watching || !mygo.Updater.Enabled() {
		return
	}
	a.watching = true
	go func() {
		time.Sleep(firstCheck)
		for {
			a.checkWhenDue(checkEvery)
			time.Sleep(watchTick)
		}
	}()
}

// CheckUpdateOnActivate looks for a newer version because the user brought
// the window, or the app, forward: at most once activationEvery.
func (a *App) CheckUpdateOnActivate() {
	if !mygo.Updater.Enabled() {
		return // a build that cannot update itself only says so when asked.
	}
	a.checkWhenDue(activationEvery)
}

// checkWhenDue looks for a newer version if the last check is gap old. It
// reads the state on the main thread, where it lives.
func (a *App) checkWhenDue(gap time.Duration) {
	a.onMain(func() {
		if a.updateDue(gap) {
			a.checkUpdates(false)
		}
	})
}

// updateDue reports whether a check may start now: not while one runs or a
// download waits for a restart, and not before gap passed since the last
// check that finished.
func (a *App) updateDue(gap time.Duration) bool {
	if a.checking {
		return false // one at a time
	}
	switch a.upd.step {
	case updateFetching, updateReady:
		return false // a newer answer is no use beside the download in hand
	}
	return a.checkedAt.IsZero() || since(a.checkedAt) >= gap
}

// since reports how long ago t was, by the wall clock. time.Time itself
// measures with the monotonic clock where it can, and that clock stops
// while macOS sleeps: a Mac that slept the night would read its morning as
// the evening before, and wait another day for the check it owed.
func since(t time.Time) time.Duration { return time.Since(t.Round(0)) }

// CheckUpdate looks for a newer version because the user asked, and says
// what it found in the status row when there is nothing to offer.
func (a *App) CheckUpdate() {
	if !mygo.Updater.Enabled() {
		a.Flash("这个版本不能自行更新：请到 GitHub 下载新版本")
		return
	}
	a.onMain(func() {
		if a.checking {
			return // the answer on its way is the one to show.
		}
		a.checkUpdates(true)
	})
}

// checkUpdates asks for the manifest, off the main thread, and hands what
// it found to the window. It is called on the main thread.
func (a *App) checkUpdates(user bool) {
	a.checking = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		up, err := mygo.Updater.Check(ctx)
		a.onMain(func() { a.foundUpdate(user, up, err) })
	}()
}

// foundUpdate takes the outcome of a check. A check the user asked for says
// what it found; one in the background stays quiet about a failure, and
// about a version the user ignored.
func (a *App) foundUpdate(user bool, up *mygo.Update, err error) {
	a.checking, a.checkedAt = false, now()
	if a.upd.step == updateReady {
		return // a version waits for a restart: keep it.
	}
	switch {
	case err != nil:
		a.upd = updateState{step: updateNone, notesShown: a.upd.notesShown}
		if user {
			a.notice("检查更新失败：" + err.Error())
		}
	case up == nil:
		a.upd = updateState{step: updateNone, notesShown: a.upd.notesShown}
		if user {
			a.notice("已是最新版本 " + a.version())
		}
	default:
		a.up = up
		if !user && up.Version == a.skippedVersion {
			a.upd = updateState{step: updateNone, notesShown: a.upd.notesShown}
			return
		}
		a.upd = updateState{
			step: updateFound, version: up.Version, notes: up.Notes,
			notesShown: a.upd.notesShown,
		}
		if user {
			a.notice("发现新版本 " + up.Version)
		}
	}
}

// DownloadUpdate downloads the version the strip names, and shows how far
// it got. Nothing replaces the running app: the new version runs at the
// next launch.
func (a *App) DownloadUpdate() {
	up := a.up
	if up == nil || a.upd.step != updateFound {
		return
	}
	a.upd.step, a.upd.progress, a.upd.err = updateFetching, 0, ""
	a.redraw()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		last := 0.0
		err := up.Install(ctx, func(done, total int64) {
			p := 0.0
			if total > 0 {
				p = float64(done) / float64(total)
			}
			// A frame per percent is enough, and cheaper than one per read.
			if p-last < 0.01 && p < 1 {
				return
			}
			last = p
			a.onMain(func() { a.upd.progress = p })
		})
		a.onMain(func() {
			switch {
			case err != nil:
				a.upd.step, a.upd.err = updateFound, "下载失败："+err.Error()
			default:
				a.upd.step, a.upd.progress = updateReady, 1
			}
		})
	}()
}

// RestartUpdate starts the app again, which runs the version it downloaded.
func (a *App) RestartUpdate() { mygo.App.Relaunch() }

// IgnoreUpdate stops the app from offering the version the strip names,
// until the one after it comes out.
func (a *App) IgnoreUpdate() {
	if a.upd.version != "" && a.upd.version != a.skippedVersion {
		a.skippedVersion = a.upd.version
		a.save()
	}
	a.upd = updateState{}
}

// ToggleUpdateNotes unfolds and folds the release notes of the update.
func (a *App) ToggleUpdateNotes() {
	a.upd.notesShown = !a.upd.notesShown
}

// availableVersion is the newer version the app found, or "".
func (a *App) availableVersion() string {
	switch a.upd.step {
	case updateFound, updateFetching, updateReady:
		return a.upd.version
	}
	return ""
}

// Flash shows a line in the status row for a few seconds, such as the
// answer to a check the user asked for.
func (a *App) Flash(msg string) {
	a.onMain(func() { a.notice(msg) })
}

// flashAlive is how long a notice stays.
const flashAlive = 6 * time.Second

// notice shows a line in the status row, on the main thread.
func (a *App) notice(msg string) {
	a.flash, a.flashUntil = msg, now().Add(flashAlive)
	time.AfterFunc(flashAlive, func() {
		a.onMain(func() {
			if !now().Before(a.flashUntil) {
				a.flash = ""
			}
		})
	})
}

// flashNow is the line the status row shows for a while, or "".
func (a *App) flashNow() string {
	if a.flash == "" || !now().Before(a.flashUntil) {
		return ""
	}
	return a.flash
}

// onMain runs fn on the main thread, where the window builds its user
// interface, and redraws it after. A window-less app, as the tests and the
// screenshots of the views make, runs it at once.
func (a *App) onMain(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}
