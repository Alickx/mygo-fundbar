package app

import (
	"testing"

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
