package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fundbar/internal/ledger"
)

// bundleSource writes a data directory of a manual fund, a ledgered fund and a
// second group, and returns it.
func bundleSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data := &Data{
		Funds: []Entry{
			{Code: "161725", Type: "指数型-股票", Shares: 12000, Cost: 1.1501,
				Groups: []string{""}},
			{Code: "005827", Type: "混合型-偏股", Shares: 999, Cost: 9.99,
				Groups: []string{"", "g1"}},
		},
		Groups:      []Group{{ID: "g1", Name: "关注"}},
		ActiveGroup: "g1",
		Selected:    "161725",
		Period:      2,
	}
	if err := data.Save(filepath.Join(dir, "funds.json")); err != nil {
		t.Fatal(err)
	}
	if err := SaveLedger(dir, "005827", bundleRows()); err != nil {
		t.Fatal(err)
	}
	return dir
}

// bundleRows is a fund's transaction history, as the app keeps it.
func bundleRows() []ledger.Txn {
	return []ledger.Txn{
		{ID: 1, Kind: ledger.Open, Shares: 8000, Price: 1.62},
		{ID: 2, Date: "2026-01-02", Kind: ledger.Buy, Shares: 1000, Price: 1.60, Fee: 2.4},
		{ID: 3, Date: "2026-01-03", Kind: ledger.Dividend, Amount: 120},
	}
}

func TestBundleRoundTripWritesTheSameFundsAndLedgers(t *testing.T) {
	src := bundleSource(t)
	at := time.Date(2026, 11, 12, 4, 15, 0, 0, time.UTC)

	b, err := BuildBundle(src, at)
	if err != nil {
		t.Fatal(err)
	}
	if b.App != BundleApp || b.Version != BundleVersion {
		t.Fatalf("bundle = %+v, want the app and version", b)
	}
	if got := b.Exported; got != at.Format(time.RFC3339) {
		t.Fatalf("exported = %q, want %q", got, at.Format(time.RFC3339))
	}
	if len(b.Ledgers) != 1 || len(b.Ledgers["005827"]) != 3 {
		t.Fatalf("ledgers = %+v, want only the fund that keeps rows", b.Ledgers)
	}
	if got := b.FundCount(); got != 2 {
		t.Fatalf("fund count = %d, want 2", got)
	}

	path := filepath.Join(t.TempDir(), "backup.json")
	if err := SaveBundle(path, b); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBundle(path)
	if err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := ApplyBundle(dest, got); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(src, "funds.json"))
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile(filepath.Join(dest, "funds.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(have, want) {
		t.Fatalf("funds.json changed:\n%s\nwant:\n%s", have, want)
	}
	rows, err := LoadLedger(dest, "005827")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, bundleRows()) {
		t.Fatalf("ledger = %+v, want %+v", rows, bundleRows())
	}
}

func TestBundleJSONShape(t *testing.T) {
	src := bundleSource(t)
	b, err := BuildBundle(src, time.Date(2026, 11, 12, 4, 15, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"app", "version", "exported", "funds", "ledgers"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("the bundle has no %q: %s", key, body)
		}
	}
	var ledgers map[string][]ledger.Txn
	if err := json.Unmarshal(fields["ledgers"], &ledgers); err != nil {
		t.Fatal(err)
	}
	if len(ledgers["005827"]) != 3 {
		t.Fatalf("ledgers = %v", ledgers)
	}
}

func TestBundleValidationRefusesWhatItCannotRead(t *testing.T) {
	good := `{"app":"FundBar","version":1,"exported":"2026-11-12T04:15:00Z",` +
		`"funds":{"funds":[{"code":"161725","shares":12000,"cost":1}]},"ledgers":{}}`
	if _, err := ParseBundle([]byte(good)); err != nil {
		t.Fatalf("a good bundle was refused: %v", err)
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{"another app",
			`{"app":"Other","version":1,"funds":{"funds":[]},"ledgers":{}}`,
			"不是 FundBar 的备份"},
		{"a newer version",
			`{"app":"FundBar","version":2,"funds":{"funds":[]},"ledgers":{}}`,
			"更新的 FundBar 版本"},
		{"garbage", `{"app":`, "无法解析"},
		{"funds that do not parse",
			`{"app":"FundBar","version":1,"funds":{"funds":"no"},"ledgers":{}}`,
			"自选数据无法读取"},
		{"a ledger row that is invalid",
			`{"app":"FundBar","version":1,"funds":{"funds":[]},` +
				`"ledgers":{"161725":[{"kind":"buy","shares":-1,"price":1,"date":"2026-01-02"}]}}`,
			"交易记录有误"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A refused bundle writes nothing: the target keeps its file.
			dest := t.TempDir()
			sentinel := filepath.Join(dest, "funds.json")
			if err := os.WriteFile(sentinel, []byte(`{"funds":[]}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := ParseBundle([]byte(c.body)); err == nil {
				t.Fatal("the bundle was accepted")
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %q, want it to say %q", err, c.want)
			}
			body, err := os.ReadFile(sentinel)
			if err != nil || string(body) != `{"funds":[]}` {
				t.Fatalf("the target changed: %q, %v", body, err)
			}
		})
	}

	// The app name and version are sentinels, so a caller can tell them apart
	// without reading the Chinese.
	if _, err := ParseBundle([]byte(cases[0].body)); !errors.Is(err, ErrNotFundBar) {
		t.Fatalf("wrong app error = %v, want ErrNotFundBar", err)
	}
	if _, err := ParseBundle([]byte(cases[1].body)); !errors.Is(err, ErrNewerBundle) {
		t.Fatalf("newer version error = %v, want ErrNewerBundle", err)
	}
}

func TestApplyBundleDropsALedgerTheBundleLacks(t *testing.T) {
	b := &Bundle{
		App: BundleApp, Version: BundleVersion,
		Funds:   json.RawMessage(`{"funds":[{"code":"161725","groups":[""]}]}`),
		Ledgers: map[string][]ledger.Txn{"161725": bundleRows()},
	}
	dest := t.TempDir()
	if err := SaveLedger(dest, "005827", bundleRows()); err != nil {
		t.Fatal(err)
	}
	if err := SaveLedger(dest, "161725", []ledger.Txn{{ID: 1, Kind: ledger.Open, Shares: 1, Price: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyBundle(dest, b); err != nil {
		t.Fatal(err)
	}
	if rows, err := LoadLedger(dest, "005827"); rows != nil || err != nil {
		t.Fatalf("the ledger the bundle lacks stayed: %v, %v", rows, err)
	}
	rows, err := LoadLedger(dest, "161725")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, bundleRows()) {
		t.Fatalf("ledger = %+v, want the bundle's", rows)
	}
}

func TestBackupBeforeImportCanRestoreTheState(t *testing.T) {
	src := bundleSource(t)
	// An extra fund's ledger, which the data file does not follow, is still
	// left alone by the backup: only the followed funds are in it.
	at := time.Date(2026, 11, 12, 4, 15, 0, 0, time.UTC)
	path, err := BackupBeforeImport(src, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(path); got != "导入前 20261112-041500.json" {
		t.Fatalf("backup name = %q", got)
	}
	if filepath.Dir(path) != filepath.Join(src, "backups") {
		t.Fatalf("backup path = %q", path)
	}

	fundsBefore, err := os.ReadFile(filepath.Join(src, "funds.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Wreck the state an import replaced by mistake.
	if err := os.WriteFile(filepath.Join(src, "funds.json"), []byte(`{"funds":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	DropLedger(src, "005827")
	if err := SaveLedger(src, "005827", []ledger.Txn{{ID: 1, Kind: ledger.Open, Shares: 1, Price: 1}}); err != nil {
		t.Fatal(err)
	}

	b, err := ReadBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyBundle(src, b); err != nil {
		t.Fatal(err)
	}
	fundsAfter, err := os.ReadFile(filepath.Join(src, "funds.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fundsAfter, fundsBefore) {
		t.Fatalf("restored funds.json:\n%s\nwant:\n%s", fundsAfter, fundsBefore)
	}
	rows, err := LoadLedger(src, "005827")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, bundleRows()) {
		t.Fatalf("restored ledger = %+v, want %+v", rows, bundleRows())
	}
}
