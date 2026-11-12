package app

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/store"
)

func TestExportBackupThroughTheDialog(t *testing.T) {
	a := preview(t)
	a.save()
	if err := store.SaveLedger(a.dir, "161725", ledgerExample()); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "backup.json")
	var opts mygo.SaveDialogOptions
	a.saveFile = func(o mygo.SaveDialogOptions) (string, error) {
		opts = o
		return out, nil
	}

	a.ExportBackup()
	if !strings.Contains(opts.DefaultPath, "FundBar 备份 ") ||
		!strings.HasSuffix(opts.DefaultPath, ".json") {
		t.Fatalf("default path = %q, want a dated FundBar backup name", opts.DefaultPath)
	}
	b, err := store.ReadBundle(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.FundCount(); got != 2 {
		t.Fatalf("exported funds = %d, want 2", got)
	}
	if rows := b.Ledgers["161725"]; len(rows) != 5 {
		t.Fatalf("exported ledger = %+v, want the five rows", b.Ledgers)
	}
	if got := a.flashNow(); got != "已导出备份" {
		t.Fatalf("flash = %q, want 已导出备份", got)
	}

	// A cancelled dialog writes nothing.
	other := filepath.Join(t.TempDir(), "cancelled.json")
	a.saveFile = func(mygo.SaveDialogOptions) (string, error) { return "", nil }
	a.ExportBackup()
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("a cancelled export wrote a file: %v", err)
	}
}

// chooseImport answers the open dialog with a bundle file and the
// confirmation with a button.
func chooseImport(a *App, path string, button int) {
	a.openFile = func(mygo.OpenDialogOptions) ([]string, error) {
		return []string{path}, nil
	}
	a.confirm = func(mygo.MessageOptions) (mygo.MessageResult, error) {
		return mygo.MessageResult{Button: button}, nil
	}
}

func TestImportBackupNeedsTheConfirmation(t *testing.T) {
	bundlePath := writeImportBundle(t)
	a := preview(t)
	a.save()

	// Cancelling the open dialog changes nothing.
	a.openFile = func(mygo.OpenDialogOptions) ([]string, error) { return nil, nil }
	a.ImportBackup()
	if len(a.all) != 2 {
		t.Fatalf("a cancelled open changed the funds to %v", codes(a.all))
	}

	// Declining the confirmation changes nothing, and makes no backup: the
	// user did not ask to replace anything.
	chooseImport(a, bundlePath, 0)
	a.ImportBackup()
	if len(a.all) != 2 {
		t.Fatalf("a declined import changed the funds to %v", codes(a.all))
	}
	if _, err := os.Stat(filepath.Join(a.dir, "backups")); !os.IsNotExist(err) {
		t.Fatalf("a declined import made a backup: %v", err)
	}

	// A confirmed import replaces the funds, writes a ledger, and leaves the
	// pre-import state in backups/.
	chooseImport(a, bundlePath, 1)
	a.ImportBackup()
	if got := codes(a.all); len(got) != 1 || got[0] != "007760" {
		t.Fatalf("imported funds = %v, want 007760", got)
	}
	if a.group != "g1" || len(a.groups) != 1 || a.groups[0].Name != "关注" {
		t.Fatalf("imported groups = %+v, active = %q", a.groups, a.group)
	}
	rows, err := store.LoadLedger(a.dir, "007760")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, ledgerExample()) {
		t.Fatalf("imported ledger = %+v", rows)
	}
	entries, err := os.ReadDir(filepath.Join(a.dir, "backups"))
	if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".json") {
		t.Fatalf("backups = %v, %v", entries, err)
	}
	if got := a.flashNow(); got != "已导入 1 只基金" {
		t.Fatalf("flash = %q, want 已导入 1 只基金", got)
	}
}

func TestImportBackupRefusesANonBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not.json")
	if err := os.WriteFile(path, []byte(`{"app":"Other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := preview(t)
	a.save()
	chooseImport(a, path, 1)
	a.ImportBackup()
	if len(a.all) != 2 {
		t.Fatalf("a refused import changed the funds to %v", codes(a.all))
	}
	if !strings.Contains(a.dataErr, "不是 FundBar 的备份") {
		t.Fatalf("data error = %q", a.dataErr)
	}
}

// writeImportBundle writes a bundle of one ledgered fund in a made group and
// returns its path.
func writeImportBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data := &store.Data{
		Funds: []store.Entry{{Code: "007760", Type: "指数型-股票",
			Shares: 11350, Cost: 0.599547, Groups: []string{"", "g1"}}},
		Groups:      []store.Group{{ID: "g1", Name: "关注"}},
		ActiveGroup: "g1",
		Selected:    "007760",
	}
	if err := data.Save(filepath.Join(dir, "funds.json")); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLedger(dir, "007760", ledgerExample()); err != nil {
		t.Fatal(err)
	}
	b, err := store.BuildBundle(dir, now())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := store.SaveBundle(path, b); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCSVPositions(t *testing.T) {
	txns := ledgerExample()
	pos, steps, err := ledger.Replay(txns)
	if err != nil {
		t.Fatal(err)
	}
	navs := []model.Nav{{Date: "2026-01-05", Unit: 0.7, Pct: 1.2}}
	funds := []*Fund{
		{
			Code: "161725", Name: "某某, 白酒", Groups: []string{"", "g1"},
			Txns: txns, pos: pos, steps: steps, Shares: pos.Shares, Cost: pos.Cost,
			Navs: navs,
		},
		{
			Code: "005827", Name: "蓝筹", Groups: []string{""},
			Shares: 12000, Cost: 1.15, Navs: []model.Nav{{Date: "2026-01-05", Unit: 1.4}},
		},
		{
			Code: "021528", Name: "只看不买", Groups: []string{"g1"},
			Navs: []model.Nav{{Date: "2026-01-05", Unit: 1.1}},
		},
	}
	groupName := func(id string) string {
		if id == holdingGroup {
			return "持有"
		}
		return "关注"
	}
	body, err := positionsCSV(funds, groupName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("the CSV has no UTF-8 BOM: % x", body[:min(8, len(body))])
	}
	records, err := csv.NewReader(bytes.NewReader(body[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("CSV rows = %d, want a header and three funds", len(records))
	}
	if !reflect.DeepEqual(records[0], positionsHeader) {
		t.Fatalf("header = %v, want %v", records[0], positionsHeader)
	}

	// The ledgered fund: the position the rows derive, its name with a comma
	// quoted, and its row count.
	byCode := make(map[string][]string, 3)
	for _, r := range records[1:] {
		byCode[r[0]] = r
	}
	led := byCode["161725"]
	if led[1] != "某某, 白酒" || led[2] != "持有/关注" {
		t.Fatalf("ledgered row = %v", led)
	}
	if led[3] != trim(pos.Shares, 4) || led[11] != "5" {
		t.Fatalf("ledgered shares and rows = %q, %q", led[3], led[11])
	}
	manual := byCode["005827"]
	if manual[2] != "持有" || manual[3] != "12000" || manual[4] != "1.15" {
		t.Fatalf("manual row = %v", manual)
	}
	if manual[7] != trim(12000*1.4, 2) {
		t.Fatalf("value = %q", manual[7])
	}
	// A fund that holds no shares has empty holding cells but still says what
	// it is and where its value is.
	watch := byCode["021528"]
	if watch[2] != "关注" || watch[5] != "1.1" || watch[6] != "2026-01-05" {
		t.Fatalf("watch-only row = %v", watch)
	}
	for _, i := range []int{3, 4, 7, 8, 9, 10} {
		if watch[i] != "" {
			t.Fatalf("watch-only cell %d = %q, want empty", i, watch[i])
		}
	}
	if strings.Contains(string(body), "11,350") {
		t.Fatal("the CSV has thousands separators")
	}
}

func TestCSVEmptyFileHasTheHeader(t *testing.T) {
	body, err := positionsCSV(nil, func(string) string { return "持有" })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte("\uFEFF"+"代码,名称,分组")) {
		t.Fatalf("the empty sheet = %q", body)
	}
}
