package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
	"fundbar/internal/store"
)

func TestTrayTitleFollowsTheMode(t *testing.T) {
	a := preview(t)
	s := a.Summary()
	if !s.Holding {
		t.Fatal("the fixture holds shares")
	}

	a.trayText = trayMoney
	if got, want := a.trayTitle(), padFigures(fmt.Sprintf("%+.2f", s.Profit), 7); got != want {
		t.Fatalf("money title = %q, want %q", got, want)
	}

	a.trayText = trayPct
	if got, want := a.trayTitle(), padFigures(pct(s.Pct), 7); got != want {
		t.Fatalf("percent title = %q, want %q", got, want)
	}

	a.trayText = trayIcon
	if got := a.trayTitle(); got != "" {
		t.Fatalf("icon-only title = %q, want none", got)
	}

	// Without shares the gain mode shows the funds' average, as it did
	// before the setting, and the percent mode says the same number.
	for _, f := range a.funds {
		f.Shares = 0
	}
	s = a.Summary()
	for _, mode := range []string{trayMoney, trayPct} {
		a.trayText = mode
		if got, want := a.trayTitle(), padFigures(pct(s.Pct), 7); got != want {
			t.Fatalf("%s title without shares = %q, want %q", mode, got, want)
		}
	}

	// Nothing followed: no text beside the icon in any mode.
	a.funds, a.all = nil, nil
	for _, mode := range trayModes {
		a.trayText = mode
		if got := a.trayTitle(); got != "" {
			t.Fatalf("%s title with no funds = %q, want none", mode, got)
		}
	}
}

func TestGlobalShortcutRegistersThroughTheSwitches(t *testing.T) {
	a := preview(t)
	var registered, unregistered string
	registers := 0
	a.registerShortcut = func(acc string, fn func()) error {
		registered = acc
		registers++
		if fn == nil {
			t.Fatal("the shortcut has no action to run")
		}
		return nil
	}
	a.unregisterShortcut = func(acc string) { unregistered = acc }

	a.applyGlobalShortcut(true)
	if registered != globalShortcutKeys || registers != 1 {
		t.Fatalf("registered %q %d times, want %q once", registered, registers, globalShortcutKeys)
	}
	if !a.globalShortcut {
		t.Fatal("the switch stayed off after registering")
	}

	a.applyGlobalShortcut(false)
	if unregistered != globalShortcutKeys {
		t.Fatalf("unregistered %q, want %q", unregistered, globalShortcutKeys)
	}
	if a.globalShortcut {
		t.Fatal("the switch stayed on after unregistering")
	}

	// A taken shortcut leaves the switch off and says why.
	a.registerShortcut = func(string, func()) error { return errors.New("shortcut is taken") }
	a.applyGlobalShortcut(true)
	if a.globalShortcut {
		t.Fatal("a failed registration left the switch on")
	}
	if !strings.Contains(a.settingsErr, "全局快捷键注册失败") {
		t.Fatalf("failure message = %q", a.settingsErr)
	}
}

func TestStartRegistersASavedShortcut(t *testing.T) {
	dir := t.TempDir()
	data := &store.Data{GlobalShortcut: true}
	if err := data.Save(filepath.Join(dir, "funds.json")); err != nil {
		t.Fatal(err)
	}
	a := New(dir)
	var registered string
	a.registerShortcut = func(acc string, fn func()) error {
		registered = acc
		return nil
	}
	a.Start()
	if registered != globalShortcutKeys {
		t.Fatalf("Start registered %q, want %q", registered, globalShortcutKeys)
	}
	if !a.globalShortcut {
		t.Fatal("Start left the setting off")
	}
}

func TestIndexQuotesLeaveTheFundsQuotes(t *testing.T) {
	a := preview(t)
	quotes := map[string]model.Quote{
		"sh600519": {Symbol: "sh600519", Price: 1500, Pct: -0.55},
		"sh000001": {Symbol: "sh000001", Price: 3680.64, Pct: 0.46},
		"sh000300": {Symbol: "sh000300", Price: 4075.71, Pct: 0.41},
		"sz399006": {Symbol: "sz399006", Price: 2572.07, Pct: 3.11},
	}
	indices := splitIndices(quotes)
	if len(indices) != 3 {
		t.Fatalf("indices = %+v, want three", indices)
	}
	if got := indexLabel(indices[0].Symbol); got != "上证" {
		t.Fatalf("first index = %q, want 上证", got)
	}
	if got := indexLabel(indices[2].Symbol); got != "创业板" {
		t.Fatalf("last index = %q, want 创业板", got)
	}
	for _, symbol := range indexSymbols() {
		if _, ok := quotes[symbol]; ok {
			t.Fatalf("the quote map still holds %s", symbol)
		}
	}

	// The fund takes the remaining quotes, and its cache keeps no index.
	f := a.funds[0]
	a.estimate(f, quotes)
	if _, ok := f.Quotes["sh600519"]; !ok {
		t.Fatal("the fund lost its own stock")
	}
	for _, symbol := range indexSymbols() {
		if _, ok := f.Quotes[symbol]; ok {
			t.Fatalf("the fund's quotes gained the index %s", symbol)
		}
	}
}

func TestPanelShowsTheMarketIndices(t *testing.T) {
	a := preview(t)
	a.PreviewMarketIndices()
	tt := panelTester(a)
	tt.Frame()
	for _, text := range []string{
		"上证", "沪深300", "创业板",
		pct(0.46), pct(0.41), pct(3.11),
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}

	// The setting off takes the row away.
	a.showIndices = false
	tt.Frame()
	for _, text := range []string{"上证", "沪深300", "创业板"} {
		if tt.HasText(text) {
			t.Fatalf("the row is still there with the setting off: %q", text)
		}
	}

	// No quotes is no row either, as before the first refresh.
	a.showIndices = true
	a.indices = nil
	tt.Frame()
	for _, text := range []string{"上证", "沪深300", "创业板"} {
		if tt.HasText(text) {
			t.Fatalf("an empty row is drawn: %q", text)
		}
	}
}

func TestIndexRowFitsThePanelAt320(t *testing.T) {
	a := preview(t)
	a.PreviewMarketIndices()
	tt := panelTester(a)
	tt.Frame()
	for _, text := range []string{
		"上证", "沪深300", "创业板",
		pct(0.46), pct(0.41), pct(3.11),
	} {
		r := requireBox(t, tt, text)
		if r.X < 0 || r.X+r.W > 320 || r.Y < 0 || r.Y+r.H > 520 {
			t.Errorf("%q is clipped: %+v", text, r)
		}
		// One line, rather than a wrapped row.
		if r.H > 20 {
			t.Errorf("%q wraps vertically: %+v", text, r)
		}
	}
}

func TestSettingsShowsTheNewGeneralRows(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.PreviewSettings, 480, 760)
	tt.Frame()
	for _, text := range []string{
		"菜单栏显示", "收益金额", "涨跌幅", "仅图标",
		"全局快捷键", "在任何应用中按 ⌃⌥F 打开或收起小窗",
		"小窗显示大盘指数",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
}

func TestSettingsShowsTheBackupAndCSVRows(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.PreviewSettings, 480, 1500)
	tt.Frame()
	for _, text := range []string{
		"数据",
		"备份", "自选、分组、持仓与交易记录存成一个文件，换电脑时带走",
		"导出备份…", "导入备份…",
		"持仓表", "当前持仓与收益，可用表格软件打开",
		"导出 CSV…",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
}

func TestSettingsDefaultAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "funds.json")
	old := `{"funds":[{"code":"007760","shares":1000,"cost":1}]}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(dir)
	a.Load()
	if a.trayText != trayMoney {
		t.Fatalf("an older file's tray text = %q, want the gain", a.trayText)
	}
	if a.globalShortcut {
		t.Fatal("an older file claims the shortcut")
	}
	if !a.showIndices {
		t.Fatal("an older file hides the benchmarks")
	}

	a.trayText = trayPct
	a.globalShortcut = true
	a.showIndices = false
	a.save()

	b := New(dir)
	b.Load()
	if b.trayText != trayPct {
		t.Fatalf("saved tray text = %q", b.trayText)
	}
	if !b.globalShortcut {
		t.Fatal("the saved shortcut was lost")
	}
	if b.showIndices {
		t.Fatal("the saved hidden benchmarks were lost")
	}

	// The file keeps the three settings under names an older build ignores.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["trayText"] != trayPct || fields["globalShortcut"] != true || fields["hideIndices"] != true {
		t.Fatalf("saved settings = %v", fields)
	}
}
