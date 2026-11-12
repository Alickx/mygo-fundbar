package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/source"
	"fundbar/internal/store"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// ledgerExample is the worked example the ledger tests use, as the app would
// keep it for one fund.
func ledgerExample() []ledger.Txn {
	return []ledger.Txn{
		{ID: 1, Kind: ledger.Open, Shares: 12000, Price: 0.62},
		{ID: 2, Date: "2026-01-02", Kind: ledger.Buy, Shares: 1000, Price: 0.6012, Fee: 0.9},
		{ID: 3, Date: "2026-01-03", Kind: ledger.Sell, Shares: 2000, Price: 0.64, Fee: 5},
		{ID: 4, Date: "2026-01-04", Kind: ledger.Dividend, Amount: 220},
		{ID: 5, Date: "2026-01-05", Kind: ledger.Reinvest, Shares: 350},
	}
}

// writeFunds puts a funds file in dir, with one fund in the given group.
func writeFunds(t *testing.T, dir string, entries ...store.Entry) {
	t.Helper()
	data := &store.Data{Selected: entries[0].Code, SidebarWidth: 348, Funds: entries}
	if err := data.Save(filepath.Join(dir, "funds.json")); err != nil {
		t.Fatal(err)
	}
}

// hasLedgerFile reports that the fund's ledger file is on disk, whatever it
// holds.
func hasLedgerFile(dir, code string) bool {
	_, err := os.Stat(filepath.Join(dir, "ledger", code+".json"))
	return err == nil
}

// shelvedRows is how many rows the fund's set-aside ledger holds, and zero
// when no ledger was set aside.
func shelvedRows(t *testing.T, dir, code string) int {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "ledger", code+".json.removed"))
	if err != nil {
		return 0
	}
	var rows []ledger.Txn
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("the set-aside ledger does not parse: %v", err)
	}
	return len(rows)
}

func TestLoadAdoptsTheLedger(t *testing.T) {
	dir := t.TempDir()
	writeFunds(t, dir, store.Entry{
		Code: "161725", Shares: 999, Cost: 9.99, Groups: []string{""},
	})
	if err := store.SaveLedger(dir, "161725", ledgerExample()); err != nil {
		t.Fatal(err)
	}

	a := New(dir)
	a.Load()
	if len(a.all) != 1 {
		t.Fatalf("funds = %d", len(a.all))
	}
	f := a.all[0]
	if !f.ledgered() || len(f.Txns) != 5 || len(f.steps) != 5 {
		t.Fatalf("ledgered = %v, txns = %d, steps = %d", f.ledgered(), len(f.Txns), len(f.steps))
	}
	if f.Shares != 11350 || f.Cost != 0.599547 {
		t.Fatalf("position = %v shares at %v; want the ledger's", f.Shares, f.Cost)
	}
	if f.ledgerErr != nil {
		t.Fatalf("ledger error = %v", f.ledgerErr)
	}

	// The funds file keeps the derived position as a mirror for older builds.
	a.save()
	data := store.Load(filepath.Join(dir, "funds.json"))
	if len(data.Funds) != 1 || data.Funds[0].Shares != 11350 || data.Funds[0].Cost != 0.599547 {
		t.Fatalf("entry = %+v, want the derived mirror", data.Funds)
	}
}

func TestManualFundLoadsAndSavesAsBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "funds.json")
	writeFunds(t, dir, store.Entry{
		Code: "161725", Type: "指数型-股票", Shares: 12000, Cost: 0.62,
		Groups: []string{""},
	})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	a := New(dir)
	a.Load()
	f := a.all[0]
	if f.ledgered() || len(f.Txns) != 0 {
		t.Fatal("a fund without a ledger should be manual")
	}
	if f.Shares != 12000 || f.Cost != 0.62 {
		t.Fatalf("position = %v at %v", f.Shares, f.Cost)
	}
	a.save()

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("the manual file changed:\n%s\nwant:\n%s", after, before)
	}
	// No ledger was written, and the file names no ledger field.
	if hasLedgerFile(dir, "161725") {
		t.Fatal("a manual fund should have no ledger file")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(after, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"txns", "ledger", "ledgerErr"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("funds.json names %q: %s", key, after)
		}
	}
}

func TestCorruptLedgerFallsBackAndIsReported(t *testing.T) {
	dir := t.TempDir()
	writeFunds(t, dir, store.Entry{
		Code: "161725", Shares: 500, Cost: 1, Groups: []string{""},
	})
	path := filepath.Join(dir, "ledger", "161725.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"broken"`), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New(dir)
	a.Load()
	f := a.all[0]
	if !strings.Contains(a.failure, "161725") {
		t.Fatalf("failure = %q, want the fund named", a.failure)
	}
	if f.ledgered() || f.Shares != 500 || f.Cost != 1 {
		t.Fatalf("fund = %+v; want the manual position", f)
	}
	if !errors.Is(f.ledgerErr, store.ErrLedgerUnreadable) {
		t.Fatalf("ledger error = %v, want the unreadable file kept on the fund", f.ledgerErr)
	}
	// Load alone leaves the file where it is, as a preview must; the start of
	// a real run is what sets it aside.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Load should not move the corrupt file: %v", err)
	}
	a.setAsideUnreadable()
	if _, err := os.Stat(path + ".bad"); err != nil {
		t.Fatalf("the corrupt file should be kept beside it: %v", err)
	}
}

func TestLoadKeepsAReplayProblem(t *testing.T) {
	dir := t.TempDir()
	writeFunds(t, dir, store.Entry{
		Code: "161725", Shares: 999, Cost: 9.99, Groups: []string{""},
	})
	if err := store.SaveLedger(dir, "161725", []ledger.Txn{
		{ID: 1, Kind: ledger.Open, Shares: 1000, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: "split", Shares: 1000},
	}); err != nil {
		t.Fatal(err)
	}

	a := New(dir)
	a.Load()
	f := a.all[0]
	var problem *ledger.Error
	if !f.ledgered() || !errors.As(f.ledgerErr, &problem) || problem.Problem != ledger.UnknownKind {
		t.Fatalf("ledger error = %v, want the problem kept", f.ledgerErr)
	}
	// The lenient position is still used.
	if f.Shares != 1000 || f.Cost != 1 {
		t.Fatalf("position = %v at %v, want the rows that stood", f.Shares, f.Cost)
	}
}

func TestRemoveDropsTheLedger(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	a.all = []*Fund{{Code: "161725", Groups: []string{""}}}
	a.rebuildList()
	if err := store.SaveLedger(dir, "161725", ledgerExample()); err != nil {
		t.Fatal(err)
	}
	a.Remove(0)
	if hasLedgerFile(dir, "161725") {
		t.Fatal("removing a fund should drop its ledger")
	}
	// The rows are the one thing the network cannot give back, so they are
	// set aside rather than deleted.
	if shelvedRows(t, dir, "161725") != 5 {
		t.Fatalf("set aside rows = %d, want the five", shelvedRows(t, dir, "161725"))
	}
}

func TestDeleteGroupDropsTheLedger(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	a.groups = []store.Group{{ID: "g1", Name: "观察"}}
	a.all = []*Fund{{Code: "005827", Groups: []string{"g1"}}}
	a.group = "g1"
	a.selCode = map[string]string{}
	a.rebuildList()
	if err := store.SaveLedger(dir, "005827", ledgerExample()); err != nil {
		t.Fatal(err)
	}
	a.DeleteGroup("g1")
	if hasLedgerFile(dir, "005827") {
		t.Fatal("a fund dropped with its group should lose its ledger")
	}
	if shelvedRows(t, dir, "005827") != 5 {
		t.Fatal("a fund dropped with its group should leave its rows set aside")
	}
}

// A ledger that sold nothing reduces to the manual formulas: the shares held
// and their average cost say the same thing as the rows.
func TestLedgerTotalsReduceToTheManualFormulas(t *testing.T) {
	navs := []model.Nav{{Date: "2026-01-05", Unit: 3}}
	txns := []ledger.Txn{
		ledger.Opening(1000, 1.5),
		{ID: 2, Date: "2026-01-02", Kind: ledger.Buy, Shares: 1000, Price: 2.5},
	}
	pos, steps, err := ledger.Replay(txns)
	if err != nil {
		t.Fatal(err)
	}
	ledgerFund := &Fund{
		Txns: txns, pos: pos, steps: steps,
		Shares: pos.Shares, Cost: pos.Cost, Navs: navs,
	}
	manualFund := &Fund{Shares: 2000, Cost: 2, Navs: navs}

	if !closeTo(ledgerFund.TotalProfit(), manualFund.TotalProfit()) {
		t.Fatalf("ledger profit = %v, manual = %v", ledgerFund.TotalProfit(), manualFund.TotalProfit())
	}
	if !closeTo(ledgerFund.TotalPct(), manualFund.TotalPct()) {
		t.Fatalf("ledger pct = %v, manual = %v", ledgerFund.TotalPct(), manualFund.TotalPct())
	}
	if !closeTo(manualFund.TotalProfit(), 2000*3-2000*2) {
		t.Fatalf("manual profit = %v, want the old formula", manualFund.TotalProfit())
	}
	if !closeTo(manualFund.TotalPct(), (3.0/2-1)*100) {
		t.Fatalf("manual pct = %v, want the old formula", manualFund.TotalPct())
	}
}

func TestLedgerTotalProfitAddsEverythingUp(t *testing.T) {
	pos, _, err := ledger.Replay(ledgerExample())
	if err != nil {
		t.Fatal(err)
	}
	f := &Fund{
		Txns: ledgerExample(), pos: pos, Shares: pos.Shares, Cost: pos.Cost,
		Navs: []model.Nav{{Date: "2026-01-06", Unit: 0.7}},
	}
	want := f.Shares*0.7 - pos.Basis + pos.Realized + pos.Dividends
	if !closeTo(f.TotalProfit(), want) {
		t.Fatalf("profit = %v, want the holding plus the sale plus the dividend %v", f.TotalProfit(), want)
	}
}

func TestHoldingPct(t *testing.T) {
	f := &Fund{
		Shares: 1000, Cost: 1.2,
		Navs: []model.Nav{{Date: "2026-01-05", Unit: 1.5}},
	}
	if got, want := f.HoldingPct(), (1.5/1.2-1)*100; !closeTo(got, want) {
		t.Fatalf("HoldingPct = %v, want %v", got, want)
	}
	// Without a cost there is no return to give.
	if got := (&Fund{Shares: 1000, Navs: f.Navs}).HoldingPct(); got != 0 {
		t.Fatalf("HoldingPct without a cost = %v, want 0", got)
	}
}

// An opening position whose cost was never filled in leaves the average cost
// too low to be one: a later buy alone would make the fund look many times
// up, and would reach any take-profit line.
func TestAnUnknownOpeningCostGivesNoReturn(t *testing.T) {
	txns := []ledger.Txn{
		ledger.Opening(9000, 0),
		{ID: 2, Date: "2026-01-02", Kind: ledger.Buy, Shares: 1000, Price: 1},
	}
	pos, _, err := ledger.Replay(txns)
	if err != nil {
		t.Fatal(err)
	}
	if !pos.CostUnknown || !closeTo(pos.Cost, 0.1) {
		t.Fatalf("position = %+v, want an unknown cost averaged over the buy alone", pos)
	}
	f := &Fund{
		Txns: txns, pos: pos, Shares: pos.Shares, Cost: pos.Cost,
		Navs: []model.Nav{{Date: "2026-01-05", Unit: 1}},
	}
	if got := f.HoldingPct(); got != 0 {
		t.Fatalf("HoldingPct = %v, want 0 rather than +900%%", got)
	}
	if got := f.TotalPct(); got != 0 {
		t.Fatalf("TotalPct = %v, want 0", got)
	}
}

// A share bought on the day whose NAV is shown did not earn that day's
// change; a share sold that day did. The manual path is unchanged.
func TestTodayProfitUsesTheSharesHeldBeforeTheDay(t *testing.T) {
	// One look at the clock: two calls could fall on either side of midnight
	// and name the same day twice.
	at := now()
	today := at.Format("2006-01-02")
	yesterday := at.AddDate(0, 0, -1).Format("2006-01-02")
	navs := []model.Nav{
		{Date: yesterday, Unit: 1.0, Pct: 10},
		{Date: today, Unit: 1.1, Pct: 10},
	}

	manual := &Fund{Shares: 1000, Navs: navs}
	if got := manual.TodayProfit(); !closeTo(got, 100) {
		t.Fatalf("manual profit = %v, want 100", got)
	}

	// 1000 shares were held before today; 1000 more were bought today.
	bought := []ledger.Txn{
		ledger.Opening(1000, 1),
		{ID: 2, Date: today, Kind: ledger.Buy, Shares: 1000, Price: 1.1},
	}
	pos, _, err := ledger.Replay(bought)
	if err != nil {
		t.Fatal(err)
	}
	f := &Fund{Txns: bought, pos: pos, Shares: pos.Shares, Cost: pos.Cost, Navs: navs}
	if f.Shares != 2000 {
		t.Fatalf("shares = %v, want both the old and the new", f.Shares)
	}
	if got := f.TodayProfit(); !closeTo(got, 100) {
		t.Fatalf("profit after a buy today = %v, want the day's gain on the 1000 held before it", got)
	}

	// 1000 shares were sold today, but all 2000 earned the change.
	sold := []ledger.Txn{
		ledger.Opening(2000, 1),
		{ID: 2, Date: today, Kind: ledger.Sell, Shares: 1000, Price: 1.1},
	}
	pos, _, err = ledger.Replay(sold)
	if err != nil {
		t.Fatal(err)
	}
	f = &Fund{Txns: sold, pos: pos, Shares: pos.Shares, Cost: pos.Cost, Navs: navs}
	if f.Shares != 1000 {
		t.Fatalf("shares = %v, want the sale gone", f.Shares)
	}
	if got := f.TodayProfit(); !closeTo(got, 200) {
		t.Fatalf("profit after a sale today = %v, want the day's gain on all 2000", got)
	}
}

// Recording the first transaction on a manual position opens the ledger with
// that position, and deleting everything but the opening puts the two numbers
// back where they were.
func TestOnlyOpeningReturnsToManual(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	f := &Fund{Code: "161725", Shares: 12000, Cost: 0.62, Groups: []string{""}}
	a.all = []*Fund{f}
	a.rebuildList()

	txns, _, err := ledger.Put(
		[]ledger.Txn{ledger.Opening(f.Shares, f.Cost)},
		ledger.Txn{Date: "2026-01-02", Kind: ledger.Buy, Shares: 1000, Price: 0.5},
	)
	if err != nil {
		t.Fatal(err)
	}
	a.setLedger(f, txns)
	if !f.ledgered() || f.Shares != 13000 {
		t.Fatalf("fund = %+v, want the opened ledger", f)
	}
	if !hasLedgerFile(dir, "161725") {
		t.Fatal("the ledger should be on disk")
	}
	// The mirror follows the ledger into funds.json.
	data := store.Load(filepath.Join(dir, "funds.json"))
	if len(data.Funds) != 1 || data.Funds[0].Shares != f.pos.Shares ||
		data.Funds[0].Cost != f.pos.Cost {
		t.Fatalf("entry = %+v, want the ledger's mirror", data.Funds)
	}

	var buyID int
	for _, txn := range f.Txns {
		if txn.Kind == ledger.Buy {
			buyID = txn.ID
		}
	}
	txns, _, err = ledger.Remove(f.Txns, buyID)
	if err != nil {
		t.Fatal(err)
	}
	a.setLedger(f, txns)
	if f.ledgered() || len(f.Txns) != 0 {
		t.Fatal("only the opening should return the fund to manual")
	}
	if f.Shares != 12000 || f.Cost != 0.62 {
		t.Fatalf("position = %v at %v, want the numbers the opening carried", f.Shares, f.Cost)
	}
	if hasLedgerFile(dir, "161725") {
		t.Fatal("the ledger file should be gone")
	}
}

// A fund taken up again finds the ledger an earlier run left for its code, as
// when the user puts one back by hand after removing the fund.
func TestAddTakesUpALedgerFoundForTheCode(t *testing.T) {
	dir := t.TempDir()
	if err := store.SaveLedger(dir, "161725", ledgerExample()); err != nil {
		t.Fatal(err)
	}
	a := New(dir)
	a.Add(source.Hit{Code: "161725", Name: "招商中证白酒指数(LOF)A"})
	if len(a.all) != 1 {
		t.Fatalf("funds = %d", len(a.all))
	}
	f := a.all[0]
	if !f.ledgered() || len(f.Txns) != len(ledgerExample()) {
		t.Fatalf("ledgered = %v, rows = %d; want the file found for the code",
			f.ledgered(), len(f.Txns))
	}
}

// A ledger that cannot be written leaves the fund as the file still has it: a
// position shown now would be one the next launch would not find.
func TestASaveThatFailsKeepsThePositionAsItWas(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	f := &Fund{Code: "161725", Shares: 1000, Cost: 1, Groups: []string{""}}
	a.all = []*Fund{f}
	a.rebuildList()
	// A file where the ledger directory should be makes every write fail.
	if err := os.WriteFile(filepath.Join(dir, "ledger"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	txns := []ledger.Txn{
		ledger.Opening(1000, 1),
		{ID: 2, Date: "2026-01-02", Kind: ledger.Buy, Shares: 500, Price: 1.2},
	}
	if err := a.setLedger(f, txns); err == nil {
		t.Fatal("the write should have failed")
	}
	if f.ledgered() || f.Shares != 1000 || f.Cost != 1 {
		t.Fatalf("fund = %+v, want the position it had", f)
	}
}
