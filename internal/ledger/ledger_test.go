package ledger

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// problemOf is the problem a Check or Replay error names, and false when the
// error is nil.
func problemOf(err error) (Problem, bool) {
	if err == nil {
		return 0, false
	}
	if e, ok := err.(*Error); ok {
		return e.Problem, true
	}
	return 0, true
}

// The worked example: an opening position, a buy with a fee, a sale, a cash
// dividend and a reinvestment. Every number below is what the ledger must
// make of them.
func TestWorkedExample(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 12000, Price: 0.62},
		{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 1000, Price: 0.6012, Fee: 0.9},
		{ID: 3, Date: "2026-01-03", Kind: Sell, Shares: 2000, Price: 0.64, Fee: 5},
		{ID: 4, Date: "2026-01-04", Kind: Dividend, Amount: 220},
		{ID: 5, Date: "2026-01-05", Kind: Reinvest, Shares: 350},
	}
	pos, steps, err := Replay(txns)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	want := Position{
		Shares: 11350, Basis: 6804.853846153846, Cost: 0.599547,
		Realized: 37.753846153846, Dividends: 220,
		Invested: 8042.1, Fees: 5.9,
	}
	if !closeTo(pos.Shares, want.Shares) || !closeTo(pos.Basis, want.Basis) ||
		!closeTo(pos.Cost, want.Cost) || !closeTo(pos.Realized, want.Realized) ||
		!closeTo(pos.Dividends, want.Dividends) || !closeTo(pos.Invested, want.Invested) ||
		!closeTo(pos.Fees, want.Fees) {
		t.Fatalf("position = %+v, want %+v", pos, want)
	}
	if pos.CostUnknown {
		t.Fatal("a position with a filled-in opening does not have an unknown cost")
	}
	if len(steps) != len(txns) {
		t.Fatalf("steps = %d, want one per row", len(steps))
	}
	// The buy shows the position it left, and the sale shows what it realised.
	if !closeTo(steps[1].Shares, 13000) || !closeTo(steps[1].Cost, 0.618623) {
		t.Fatalf("buy step = %+v", steps[1])
	}
	if !closeTo(steps[2].Realized, 37.753846153846) || !closeTo(steps[2].Shares, 11000) {
		t.Fatalf("sell step = %+v", steps[2])
	}
	if !closeTo(steps[4].Shares, 11350) || !closeTo(steps[4].Cost, 0.599547) {
		t.Fatalf("reinvest step = %+v", steps[4])
	}
}

// A sale takes the same cost per share from the basis, so the average cost
// does not move.
func TestSaleLeavesTheAverageCostAlone(t *testing.T) {
	before := []Txn{
		{ID: 1, Kind: Open, Shares: 12000, Price: 0.62},
		{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 1000, Price: 0.6012, Fee: 0.9},
	}
	pos, _, err := Replay(before)
	if err != nil {
		t.Fatal(err)
	}
	cost := pos.Cost

	after := append(append([]Txn(nil), before...),
		Txn{ID: 3, Date: "2026-01-03", Kind: Sell, Shares: 2000, Price: 0.64, Fee: 5})
	pos, _, err = Replay(after)
	if err != nil {
		t.Fatal(err)
	}
	if !closeTo(pos.Cost, cost) {
		t.Fatalf("cost after the sale = %v, want the same %v", pos.Cost, cost)
	}
	if !closeTo(pos.Cost, 0.618623) {
		t.Fatalf("cost = %v, want 0.618623", pos.Cost)
	}
	if !closeTo(pos.Realized, 2000*0.64-5-8042.1*2000/13000) {
		t.Fatalf("realised = %v", pos.Realized)
	}
}

func TestSaleExhaustingThePositionLeavesNothing(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 12000, Price: 0.62},
		{ID: 2, Date: "2026-01-02", Kind: Sell, Shares: 12000, Price: 0.7, Fee: 1},
	}
	pos, _, err := Replay(txns)
	if err != nil {
		t.Fatal(err)
	}
	if pos.Shares != 0 || pos.Basis != 0 || pos.Cost != 0 {
		t.Fatalf("position = %+v, want shares, basis and cost exactly zero", pos)
	}
	if !closeTo(pos.Realized, 12000*0.7-1-7440) || !closeTo(pos.Fees, 1) {
		t.Fatalf("realised = %v, fees = %v", pos.Realized, pos.Fees)
	}
	if !closeTo(pos.Invested, 7440) {
		t.Fatalf("invested = %v, want what was put in", pos.Invested)
	}
}

// A dividend pays money whatever the shares are, including none at all.
func TestDividend(t *testing.T) {
	pos, _, err := Replay([]Txn{
		{ID: 1, Kind: Open, Shares: 1000, Price: 1.2},
		{ID: 2, Date: "2026-01-02", Kind: Dividend, Amount: 34.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !closeTo(pos.Dividends, 34.5) || !closeTo(pos.Shares, 1000) || !closeTo(pos.Basis, 1200) {
		t.Fatalf("position = %+v; a dividend moves neither shares nor basis", pos)
	}

	pos, _, err = Replay([]Txn{{ID: 1, Date: "2026-01-02", Kind: Dividend, Amount: 20}})
	if err != nil {
		t.Fatalf("a dividend with no shares: %v", err)
	}
	if !closeTo(pos.Dividends, 20) || pos.Shares != 0 {
		t.Fatalf("position = %+v, want the dividend alone", pos)
	}
}

func TestReinvestAddsZeroCostShares(t *testing.T) {
	pos, _, err := Replay([]Txn{
		{ID: 1, Kind: Open, Shares: 1000, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Reinvest, Shares: 500},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !closeTo(pos.Shares, 1500) || !closeTo(pos.Basis, 1000) {
		t.Fatalf("position = %+v, want 1500 shares on the same basis", pos)
	}
	if !closeTo(pos.Cost, round6(1000.0/1500)) {
		t.Fatalf("cost = %v, want the diluted average", pos.Cost)
	}
}

func TestOpeningWithoutAPriceHasAnUnknownCost(t *testing.T) {
	pos, _, err := Replay([]Txn{{ID: 1, Kind: Open, Shares: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	if !pos.CostUnknown {
		t.Fatal("expected the cost to be unknown")
	}
	if pos.Shares != 1000 || pos.Basis != 0 || pos.Cost != 0 || pos.Invested != 0 {
		t.Fatalf("position = %+v", pos)
	}
}

// The average cost is kept to six decimals; a round 0.62 stays exactly 0.62.
func TestOpeningCostIsExact(t *testing.T) {
	pos, _, err := Replay([]Txn{{ID: 1, Kind: Open, Shares: 12000, Price: 0.62}})
	if err != nil {
		t.Fatal(err)
	}
	if pos.Cost != 0.62 {
		t.Fatalf("cost = %v, want exactly 0.62", pos.Cost)
	}
	// A fee-less buy at its own price is that price again.
	pos, _, err = Replay([]Txn{
		{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 1000, Price: 0.6012},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pos.Cost != 0.6012 {
		t.Fatalf("cost = %v, want exactly 0.6012", pos.Cost)
	}
}

// One day's rows replay the same whatever order the user typed them in: the
// dividend, then the reinvestment, then the sale, then the buy.
func TestSameDayOrderIsIndependentOfEntryOrder(t *testing.T) {
	rows := []Txn{
		{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 50, Price: 2},
		{ID: 2, Date: "2026-01-02", Kind: Sell, Shares: 200, Price: 2},
		{ID: 3, Date: "2026-01-02", Kind: Reinvest, Shares: 100},
		{ID: 4, Date: "2026-01-02", Kind: Dividend, Amount: 100},
	}
	open := Txn{ID: 5, Kind: Open, Shares: 1000, Price: 1}

	forward := append([]Txn{open}, rows...)
	backward := []Txn{open}
	for i := len(rows) - 1; i >= 0; i-- {
		backward = append(backward, rows[i])
	}

	posA, stepsA, err := Replay(forward)
	if err != nil {
		t.Fatal(err)
	}
	posB, stepsB, err := Replay(backward)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(posA, posB) || !reflect.DeepEqual(stepsA, stepsB) {
		t.Fatalf("entry order changed the replay:\n%+v %+v\n%+v %+v", posA, stepsA, posB, stepsB)
	}

	kinds := []Kind{Dividend, Reinvest, Sell, Buy}
	for i, want := range kinds {
		if got := stepsA[i+1].Txn.Kind; got != want {
			t.Fatalf("row %d is %s, want %s", i, got, want)
		}
	}

	// The reinvestment came before the sale, so its shares diluted the sale's
	// cost: 1000 of basis over 1100 shares, 200 taken out.
	pos := posA
	if !closeTo(pos.Shares, 950) {
		t.Fatalf("shares = %v, want 950", pos.Shares)
	}
	if !closeTo(pos.Realized, 400-1000.0*200/1100) {
		t.Fatalf("realised = %v, want the diluted basis", pos.Realized)
	}
}

// A reinvested dividend is shares before a sale on the same day: the sale's
// cost is spread over the bigger holding.
func TestReinvestBeforeSaleDilutesIt(t *testing.T) {
	pos, _, err := Replay([]Txn{
		{ID: 1, Kind: Open, Shares: 1000, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Sell, Shares: 1000, Price: 2},
		{ID: 3, Date: "2026-01-02", Kind: Reinvest, Shares: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !closeTo(pos.Shares, 1000) || !closeTo(pos.Basis, 500) || !closeTo(pos.Cost, 0.5) {
		t.Fatalf("position = %+v, want half the basis left on 1000 shares", pos)
	}
	if !closeTo(pos.Realized, 1500) {
		t.Fatalf("realised = %v, want 2000 taken less the 500 diluted basis", pos.Realized)
	}
}

// A sale of more than is held is clamped to what is held, and the problem
// says how much that was.
func TestOversellIsClamped(t *testing.T) {
	pos, _, err := Replay([]Txn{
		{ID: 1, Kind: Open, Shares: 100, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Sell, Shares: 100.0001, Price: 2},
	})
	problem, ok := problemOf(err)
	if !ok || problem != Oversell {
		t.Fatalf("problem = %v, %v; want an oversell", problem, ok)
	}
	e := err.(*Error)
	if !closeTo(e.Held, 100) || e.ID != 2 || e.Date != "2026-01-02" {
		t.Fatalf("error = %+v", e)
	}
	// The clamped sale took the 100 shares there were, not the 100.0001 asked
	// for, so nothing comes from nowhere.
	if pos.Shares != 0 || pos.Basis != 0 {
		t.Fatalf("position = %+v, want the clamped sale to empty it", pos)
	}
	if !closeTo(pos.Realized, 100*2-100) {
		t.Fatalf("realised = %v, want only the shares that were held", pos.Realized)
	}

	// A sale that asks for a hair more than is held, under the tolerance the
	// arithmetic keeps, is the position itself rather than an oversell.
	pos, _, err = Replay([]Txn{
		{ID: 1, Kind: Open, Shares: 100, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Sell, Shares: 100.00004, Price: 2},
	})
	if err != nil {
		t.Fatalf("a sale within the dust tolerance: %v", err)
	}
	if pos.Shares != 0 || !closeTo(pos.Realized, 100) {
		t.Fatalf("position = %+v, want the sale of the shares there were", pos)
	}
}

// A sale that leaves less than a hundredth of a share sweeps the rounding
// away rather than leaving a holding too small to sell.
func TestDustSweep(t *testing.T) {
	pos, _, err := Replay([]Txn{
		{ID: 1, Kind: Open, Shares: 100.005, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Sell, Shares: 100, Price: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pos.Shares != 0 || pos.Basis != 0 || pos.Cost != 0 {
		t.Fatalf("position = %+v, want the dust swept away", pos)
	}
	if !closeTo(pos.Realized, 200-100.005) {
		t.Fatalf("realised = %v, want the whole basis taken", pos.Realized)
	}
}

// Shares are kept to four decimals: a series of roundings that adds up to 0.3
// is 0.3, and selling it leaves exactly nothing.
func TestRound4Dust(t *testing.T) {
	bought := []Txn{
		{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 0.1, Price: 1},
		{ID: 2, Date: "2026-01-03", Kind: Buy, Shares: 0.2, Price: 1},
	}
	pos, _, err := Replay(bought)
	if err != nil {
		t.Fatal(err)
	}
	if pos.Shares != 0.3 {
		t.Fatalf("shares = %v, want exactly 0.3", pos.Shares)
	}

	sold := append(append([]Txn(nil), bought...),
		Txn{ID: 3, Date: "2026-01-04", Kind: Sell, Shares: 0.3, Price: 1})
	pos, _, err = Replay(sold)
	if err != nil {
		t.Fatalf("selling exactly what was held: %v", err)
	}
	if pos.Shares != 0 || pos.Basis != 0 {
		t.Fatalf("position = %+v, want exactly zero", pos)
	}
	if !closeTo(pos.Realized, 0) {
		t.Fatalf("realised = %v, want nothing", pos.Realized)
	}
}

// Editing a buy in the middle changes what a later sale cost, and so what it
// realised.
func TestEditInTheMiddleShiftsALaterSale(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 1000, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 1000, Price: 1},
		{ID: 3, Date: "2026-01-03", Kind: Sell, Shares: 1000, Price: 2},
	}
	pos, _, err := Replay(txns)
	if err != nil {
		t.Fatal(err)
	}
	if !closeTo(pos.Realized, 1000) {
		t.Fatalf("realised = %v, want 2000 taken less the 1000 basis", pos.Realized)
	}

	// The same buy at 2 costs 1000 more, so the sale takes 1500 of basis.
	txns, pos, err = Put(txns, Txn{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 1000, Price: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !closeTo(pos.Realized, 500) {
		t.Fatalf("realised = %v, want 2000 less the 1500 diluted basis", pos.Realized)
	}
	if !closeTo(pos.Cost, 1.5) {
		t.Fatalf("cost = %v, want 1500 over the 1000 shares left", pos.Cost)
	}
}

// Removing a buy a later sale depends on is refused: the rows are left as
// they were and the oversell is reported.
func TestRemoveThatWouldOversellIsRejected(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 100, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 100, Price: 1},
		{ID: 3, Date: "2026-01-03", Kind: Sell, Shares: 150, Price: 2},
	}
	got, pos, err := Remove(txns, 2)
	problem, ok := problemOf(err)
	if !ok || problem != Oversell {
		t.Fatalf("problem = %v, %v; want an oversell", problem, ok)
	}
	if len(got) != len(txns) || !closeTo(pos.Shares, 50) {
		t.Fatalf("rows = %d, shares = %v; want the ledger left as it was", len(got), pos.Shares)
	}

	// Removing the sale itself goes through.
	got, pos, err = Remove(txns, 3)
	if err != nil {
		t.Fatalf("removing the sale: %v", err)
	}
	if len(got) != 2 || !closeTo(pos.Shares, 200) {
		t.Fatalf("rows = %d, shares = %v", len(got), pos.Shares)
	}
}

func TestTwoOpenings(t *testing.T) {
	pos, steps, err := Replay([]Txn{
		{ID: 1, Kind: Open, Shares: 100, Price: 1},
		{ID: 2, Kind: Open, Shares: 999, Price: 5},
	})
	problem, ok := problemOf(err)
	if !ok || problem != TwoOpenings {
		t.Fatalf("problem = %v, %v; want two openings", problem, ok)
	}
	if e := err.(*Error); e.ID != 2 {
		t.Fatalf("the second opening is the problem, got id %d", e.ID)
	}
	if !closeTo(pos.Shares, 100) || len(steps) != 2 {
		t.Fatalf("position = %+v, steps = %d; want the first opening alone", pos, len(steps))
	}
}

// A change that gives sound rows a problem is refused as a whole, whatever
// the problem is; rows that already had one stay editable.
func TestPutRefusesAProblemTheRowsDidNotHave(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 100, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 100, Price: 1},
	}
	got, pos, err := Put(txns, Txn{Kind: Open, Shares: 999, Price: 5})
	problem, ok := problemOf(err)
	if !ok || problem != TwoOpenings {
		t.Fatalf("problem = %v, %v; want two openings", problem, ok)
	}
	if len(got) != len(txns) || !closeTo(pos.Shares, 200) {
		t.Fatalf("rows = %d, shares = %v; want the ledger left as it was", len(got), pos.Shares)
	}

	// A file edited by hand can sell more than it holds. A buy dated before
	// the sale puts it right, and must not be refused for the problem that
	// was already there.
	broken := []Txn{
		{ID: 1, Kind: Open, Shares: 100, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Sell, Shares: 150, Price: 2},
	}
	if _, _, err := Replay(broken); err == nil {
		t.Fatal("the rows should start with an oversell")
	}
	got, _, err = Put(broken, Txn{Date: "2026-01-01", Kind: Buy, Shares: 100, Price: 1})
	if err != nil || len(got) != 3 {
		t.Fatalf("rows = %d, err = %v; want the repairing buy accepted", len(got), err)
	}
}

func TestCheckFieldRules(t *testing.T) {
	good := Txn{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 10, Price: 1, Fee: 0.1}
	cases := []struct {
		name string
		txn  Txn
		want Problem
		ok   bool
	}{
		{"open without a date", Txn{ID: 1, Kind: Open, Shares: 10, Price: 1}, 0, true},
		{"open with a date", Txn{ID: 1, Date: "2026-01-02", Kind: Open, Shares: 10}, 0, true},
		{"buy", good, 0, true},
		{"sell", Txn{ID: 1, Date: "2026-01-02", Kind: Sell, Shares: 10, Price: 1}, 0, true},
		{"dividend", Txn{ID: 1, Date: "2026-01-02", Kind: Dividend, Amount: 20}, 0, true},
		{"reinvest", Txn{ID: 1, Date: "2026-01-02", Kind: Reinvest, Shares: 10}, 0, true},
		{"unknown kind", Txn{ID: 1, Date: "2026-01-02", Kind: "split", Shares: 10}, UnknownKind, false},
		{"buy without a date", Txn{ID: 1, Kind: Buy, Shares: 10, Price: 1}, BadDate, false},
		{"date not padded", Txn{ID: 1, Date: "2026-1-2", Kind: Buy, Shares: 10, Price: 1}, BadDate, false},
		{"date not a day", Txn{ID: 1, Date: "2026-13-01", Kind: Buy, Shares: 10, Price: 1}, BadDate, false},
		{"open without shares", Txn{ID: 1, Kind: Open}, BadShares, false},
		{"buy without shares", Txn{ID: 1, Date: "2026-01-02", Kind: Buy, Price: 1}, BadShares, false},
		{"sell of negative shares", Txn{ID: 1, Date: "2026-01-02", Kind: Sell, Shares: -1, Price: 1}, BadShares, false},
		{"reinvest without shares", Txn{ID: 1, Date: "2026-01-02", Kind: Reinvest}, BadShares, false},
		{"buy without a price", Txn{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 10}, BadPrice, false},
		{"sell without a price", Txn{ID: 1, Date: "2026-01-02", Kind: Sell, Shares: 10}, BadPrice, false},
		{"negative opening price", Txn{ID: 1, Kind: Open, Shares: 10, Price: -1}, BadPrice, false},
		{"negative fee", Txn{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 10, Price: 1, Fee: -0.1}, BadFee, false},
		{"dividend without money", Txn{ID: 1, Date: "2026-01-02", Kind: Dividend}, BadAmount, false},
		{"negative dividend", Txn{ID: 1, Date: "2026-01-02", Kind: Dividend, Amount: -5}, BadAmount, false},
		{"NaN shares", Txn{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: math.NaN(), Price: 1}, BadShares, false},
		{"infinite price", Txn{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 1, Price: math.Inf(1)}, BadPrice, false},
		{"NaN fee", Txn{ID: 1, Date: "2026-01-02", Kind: Buy, Shares: 1, Price: 1, Fee: math.NaN()}, BadFee, false},
		{"infinite dividend", Txn{ID: 1, Date: "2026-01-02", Kind: Dividend, Amount: math.Inf(1)}, BadAmount, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Check(c.txn)
			got, ok := problemOf(err)
			if ok != !c.ok {
				t.Fatalf("Check = %v, want ok = %v", err, c.ok)
			}
			if !c.ok && got != c.want {
				t.Fatalf("problem = %v, want %v", got, c.want)
			}
		})
	}
}

func TestJSONNamesAndOmitempty(t *testing.T) {
	full := Txn{
		ID: 7, Date: "2026-01-02", Kind: Buy,
		Shares: 1000, Price: 0.6012, Fee: 0.9, Amount: 220,
	}
	body, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "date", "kind", "shares", "price", "fee", "amount"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("%s is missing from %s", key, body)
		}
	}

	sparse := Txn{ID: 1, Kind: Open, Shares: 12000, Price: 0.62}
	body, err = json.Marshal(sparse)
	if err != nil {
		t.Fatal(err)
	}
	var sparseFields map[string]any
	if err := json.Unmarshal(body, &sparseFields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"date", "fee", "amount"} {
		if _, ok := sparseFields[key]; ok {
			t.Fatalf("%s should be left out of %s", key, body)
		}
	}
	var back Txn
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, sparse) {
		t.Fatalf("round trip = %+v, want %+v", back, sparse)
	}
}

func TestNextIDAfterDeletions(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 10, Price: 1},
		{ID: 5, Date: "2026-01-02", Kind: Buy, Shares: 10, Price: 1},
	}
	if got := NextID(txns); got != 6 {
		t.Fatalf("NextID = %d, want 6", got)
	}
	// Deleting a middle id does not make the next row take it, or the length
	// of the list: ids are never counted again.
	got, _, err := Remove([]Txn{
		{ID: 1, Kind: Open, Shares: 10, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 1, Price: 1},
		{ID: 5, Date: "2026-01-03", Kind: Buy, Shares: 1, Price: 1},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if id := NextID(got); id != 6 {
		t.Fatalf("NextID after deletion = %d, want 6", id)
	}
	if got := NextID(nil); got != 1 {
		t.Fatalf("NextID of an empty ledger = %d, want 1", got)
	}
}

func TestPutAssignsAnIDToAnOpeningAndANewRow(t *testing.T) {
	txns, pos, err := Put([]Txn{Opening(1000, 1)}, Txn{
		Date: "2026-01-02", Kind: Buy, Shares: 500, Price: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != 2 || txns[0].ID == 0 || txns[1].ID == 0 || txns[0].ID == txns[1].ID {
		t.Fatalf("rows = %+v, want two rows with unique ids", txns)
	}
	if !closeTo(pos.Shares, 1500) {
		t.Fatalf("shares = %v", pos.Shares)
	}
}

func TestSharesBefore(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 1000, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 500, Price: 1},
		{ID: 3, Date: "2026-01-03", Kind: Sell, Shares: 200, Price: 1},
		{ID: 4, Date: "2026-01-03", Kind: Dividend, Amount: 10},
		{ID: 5, Date: "2026-01-04", Kind: Reinvest, Shares: 100},
	}
	cases := map[string]float64{
		"":           1000, // before the ledger begins: the opening alone
		"2026-01-01": 1000,
		"2026-01-02": 1000,
		"2026-01-03": 1500,
		"2026-01-04": 1300,
		"2026-01-05": 1400,
	}
	for date, want := range cases {
		if got := SharesBefore(txns, date); !closeTo(got, want) {
			t.Errorf("SharesBefore(%q) = %v, want %v", date, got, want)
		}
	}
}

func TestOnlyOpening(t *testing.T) {
	op := Opening(12000, 0.62)
	if got, ok := OnlyOpening([]Txn{op}); !ok || got.Shares != 12000 || got.Price != 0.62 {
		t.Fatalf("OnlyOpening = %+v, %v", got, ok)
	}
	if _, ok := OnlyOpening([]Txn{op, {ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 1, Price: 1}}); ok {
		t.Fatal("two rows are not only the opening")
	}
	if _, ok := OnlyOpening([]Txn{{ID: 2, Date: "2026-01-02", Kind: Buy, Shares: 1, Price: 1}}); ok {
		t.Fatal("a buy is not the opening")
	}
	if _, ok := OnlyOpening(nil); ok {
		t.Fatal("an empty ledger is not only the opening")
	}
}

// An unknown kind is reported and skipped, but kept in the list: the app
// leaves the row on disk for a build that knows what it means.
func TestUnknownKindIsKeptButSkipped(t *testing.T) {
	txns := []Txn{
		{ID: 1, Kind: Open, Shares: 1000, Price: 1},
		{ID: 2, Date: "2026-01-02", Kind: "split", Shares: 1000},
	}
	pos, steps, err := Replay(txns)
	problem, ok := problemOf(err)
	if !ok || problem != UnknownKind {
		t.Fatalf("problem = %v, %v; want an unknown kind", problem, ok)
	}
	if !closeTo(pos.Shares, 1000) || len(steps) != 2 {
		t.Fatalf("position = %+v, steps = %d", pos, len(steps))
	}
	if steps[1].Txn.Kind != "split" {
		t.Fatalf("the unknown row should still be listed: %+v", steps[1])
	}
}

func TestSortPutsTheOpeningFirstAndThenTheDay(t *testing.T) {
	txns := []Txn{
		{ID: 4, Date: "2026-01-02", Kind: Buy, Shares: 1, Price: 1},
		{ID: 3, Date: "2026-01-02", Kind: Sell, Shares: 1, Price: 1},
		{ID: 1, Kind: Open, Shares: 1, Price: 1},
		{ID: 2, Date: "2026-01-01", Kind: Buy, Shares: 1, Price: 1},
	}
	got := Sort(txns)
	want := []int{1, 2, 3, 4}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("order = %+v, want ids %v", got, want)
		}
	}
	// Sort copies: the caller's slice is left as it was.
	if txns[0].ID != 4 {
		t.Fatal("Sort should not reorder the slice it was given")
	}
}
