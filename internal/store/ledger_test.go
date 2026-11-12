package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"fundbar/internal/ledger"
)

func TestLedgerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := []ledger.Txn{
		{ID: 1, Kind: ledger.Open, Shares: 12000, Price: 0.62},
		{ID: 2, Date: "2026-01-02", Kind: ledger.Buy, Shares: 1000, Price: 0.6012, Fee: 0.9},
		{ID: 3, Date: "2026-01-03", Kind: ledger.Sell, Shares: 2000, Price: 0.64, Fee: 5},
		{ID: 4, Date: "2026-01-04", Kind: ledger.Dividend, Amount: 220},
		{ID: 5, Date: "2026-01-05", Kind: ledger.Reinvest, Shares: 350},
	}
	if err := SaveLedger(dir, "161725", want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLedger(dir, "161725")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger = %+v, want %+v", got, want)
	}

	DropLedger(dir, "161725")
	if got, err := LoadLedger(dir, "161725"); got != nil || err != nil {
		t.Fatalf("ledger = %v, %v; want it gone", got, err)
	}
}

func TestMissingLedger(t *testing.T) {
	dir := t.TempDir()
	// A fund that keeps no ledger is not a problem to report.
	got, err := LoadLedger(dir, "161725")
	if got != nil || err != nil {
		t.Fatalf("ledger = %v, %v; want none", got, err)
	}
	// Reading a fund with no ledger does not make the directory.
	if _, err := os.Stat(filepath.Join(dir, "ledger")); !os.IsNotExist(err) {
		t.Fatalf("ledger directory should not exist, stat says %v", err)
	}
}

func TestUnparsableLedgerIsSetAside(t *testing.T) {
	dir := t.TempDir()
	path := ledgerPath(dir, "161725")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"not":"a list"`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadLedger(dir, "161725")
	if got != nil || !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("ledger = %v, %v; want the corrupt file reported", got, err)
	}
	// Reading is what a preview does too, so it reports the problem and
	// leaves the file where it is.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("reading should not move the corrupt file: %v", err)
	}
	if _, err := os.Stat(path + ".bad"); !os.IsNotExist(err) {
		t.Fatalf("reading should not set the file aside, stat says %v", err)
	}

	badPath, err := SetAsideLedger(dir, "161725")
	if err != nil {
		t.Fatalf("setting the corrupt file aside: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the corrupt file should be gone, stat says %v", err)
	}
	bad, err := os.ReadFile(badPath)
	if err != nil {
		t.Fatalf("the corrupt file should be kept beside it: %v", err)
	}
	if !reflect.DeepEqual(bad, body) {
		t.Fatalf(".bad = %q, want the original %q", bad, body)
	}

	// The next load finds no ledger, and a new one does not touch the bad
	// file.
	if got, err := LoadLedger(dir, "161725"); got != nil || err != nil {
		t.Fatalf("ledger = %v, %v; want none after the file was set aside", got, err)
	}
	if err := SaveLedger(dir, "161725", []ledger.Txn{
		{ID: 1, Kind: ledger.Open, Shares: 1, Price: 1},
	}); err != nil {
		t.Fatal(err)
	}
	bad, err = os.ReadFile(badPath)
	if err != nil || !reflect.DeepEqual(bad, body) {
		t.Fatalf(".bad was changed: %q, %v", bad, err)
	}
}

func TestShelvedLedgerKeepsTheRows(t *testing.T) {
	dir := t.TempDir()
	rows := []ledger.Txn{
		{ID: 1, Kind: ledger.Open, Shares: 12000, Price: 0.62},
		{ID: 2, Date: "2026-01-02", Kind: ledger.Buy, Shares: 1000, Price: 0.6012, Fee: 0.9},
	}
	if err := SaveLedger(dir, "161725", rows); err != nil {
		t.Fatal(err)
	}
	path := ledgerPath(dir, "161725")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	ShelveLedger(dir, "161725")
	// The fund keeps no ledger now, and the rows are beside where it was.
	if got, err := LoadLedger(dir, "161725"); got != nil || err != nil {
		t.Fatalf("ledger = %v, %v; want none after it was set aside", got, err)
	}
	kept, err := os.ReadFile(path + ".removed")
	if err != nil || !reflect.DeepEqual(kept, want) {
		t.Fatalf("set aside = %q, %v; want the rows as they were", kept, err)
	}

	// A ledger started afresh leaves what was set aside alone, and a fund
	// with no ledger has nothing to set aside.
	if err := SaveLedger(dir, "161725", rows[:1]); err != nil {
		t.Fatal(err)
	}
	if kept, err = os.ReadFile(path + ".removed"); err != nil || !reflect.DeepEqual(kept, want) {
		t.Fatalf("set aside changed: %q, %v", kept, err)
	}
	ShelveLedger(dir, "005827")
	if _, err := os.Stat(ledgerPath(dir, "005827") + ".removed"); !os.IsNotExist(err) {
		t.Fatalf("a fund with no ledger left a file behind, stat says %v", err)
	}
}

func TestLedgerThatCannotBeOpenedIsReportedAndLeftAlone(t *testing.T) {
	dir := t.TempDir()
	// A directory where the file should be cannot be read as one, and is not
	// a ledger that failed to parse: it is reported, and not renamed.
	path := ledgerPath(dir, "161725")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLedger(dir, "161725")
	if got != nil || err == nil || errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("ledger = %v, %v; want the read error itself", got, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("what could not be read should be left where it is: %v", err)
	}
}
