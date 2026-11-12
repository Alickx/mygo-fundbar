package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The window offers the download and then the restart, in the app itself:
// there is no window of the updater's to dismiss.
func TestUpdateStripDownloadsThenRestarts(t *testing.T) {
	a := preview(t)
	a.upd = updateState{
		step:    updateFound,
		version: "0.4.1",
		notes:   "修复了估值的一处误差。",
	}
	tt := ui.NewTesterWith(a.View, 1080, 840, ui.TesterOptions{})
	tt.Frame()

	for _, text := range []string{"新版本 0.4.1", "下载更新", "忽略", "更新说明"} {
		requireBox(t, tt, text)
	}
	if _, ok := tt.Find("安装并重启"); ok {
		t.Error("the strip offers a restart before the download")
	}

	a.upd.step, a.upd.progress, a.upd.notesShown = updateReady, 1, true
	tt.Frame()
	for _, text := range []string{"已下载完成，重启应用后生效", "安装并重启", "收起说明"} {
		requireBox(t, tt, text)
	}
	if _, ok := tt.Find("下载更新"); ok {
		t.Error("the strip offers the download once it is done")
	}

	// With nothing found, the strip takes no room at all.
	a.upd = updateState{}
	tt.Frame()
	if _, ok := tt.Find("新版本 0.4.1"); ok {
		t.Error("the strip stays once there is nothing to offer")
	}
}

// An ignored version stays hidden until the version after it, unless the
// user asks; and a download that waits for a restart survives a check.
func TestUpdateCheckRespectsIgnoredAndReady(t *testing.T) {
	a := preview(t)
	up := &mygo.Update{Version: "0.5.0", Notes: "更新说明"}

	a.foundUpdate(false, up, nil)
	if a.upd.step != updateFound || a.upd.version != "0.5.0" {
		t.Fatalf("a check found nothing: %+v", a.upd)
	}

	a.IgnoreUpdate()
	if a.skippedVersion != "0.5.0" || a.upd.step != updateNone {
		t.Fatalf("ignoring left %+v and skipped %q", a.upd, a.skippedVersion)
	}
	a.foundUpdate(false, up, nil)
	if a.upd.step != updateNone {
		t.Error("the ignored version came back on its own")
	}
	a.foundUpdate(true, up, nil)
	if a.upd.step != updateFound {
		t.Error("a check the user asked for hides an ignored version")
	}

	a.upd = updateState{step: updateReady, version: "0.5.0"}
	a.foundUpdate(false, up, nil)
	if a.upd.step != updateReady {
		t.Error("a check dropped the version that waits for a restart")
	}
}

// The app looks for a newer version on its own schedule: checkEvery after
// the last check that finished, and not on one long sleep, which macOS
// pauses while the Mac sleeps.
func TestUpdateDue(t *testing.T) {
	a := preview(t)
	if !a.updateDue(checkEvery) {
		t.Error("no check is due before the first one")
	}

	a.foundUpdate(false, nil, nil) // up to date, so nothing to show
	if a.checkedAt.IsZero() {
		t.Error("a check that finished did not say when")
	}
	if a.updateDue(checkEvery) {
		t.Error("another check is due right after one")
	}

	a.checkedAt = now().Add(-checkEvery - time.Second)
	if !a.updateDue(checkEvery) {
		t.Error("the check that checkEvery passed did not come due")
	}
	if a.updateDue(activationEvery) {
		t.Error("a check the window asks for came due sooner than its gap")
	}

	// One at a time: the answer on its way is what the app waits for.
	a.checking = true
	if a.updateDue(0) {
		t.Error("a second check started while one was running")
	}
	a.checking = false

	// A download in hand is newer than any manifest the app could read.
	for _, step := range []updateStep{updateFetching, updateReady} {
		a.checkedAt = time.Time{}
		a.upd = updateState{step: step}
		if a.updateDue(checkEvery) {
			t.Errorf("a check started while step %d was in hand", step)
		}
	}
}

// The status line says where the update is.
func TestUpdateStatus(t *testing.T) {
	tests := []struct {
		state updateState
		want  string
	}{
		{updateState{step: updateFetching, progress: 0.42}, "正在下载 42%"},
		{updateState{step: updateReady}, "已下载完成，重启应用后生效"},
		{updateState{step: updateFound, err: "下载失败：没有网络"}, "下载失败：没有网络"},
		{updateState{step: updateNone}, ""},
	}
	for _, tt := range tests {
		if got := tt.state.status(); got != tt.want {
			t.Errorf("%+v says %q, want %q", tt.state, got, tt.want)
		}
	}
}
