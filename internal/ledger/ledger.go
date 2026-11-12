// Package ledger records a fund's transactions and works out the position they
// leave behind: the shares held, what they cost, and what has been realised.
// It reads no files and no clock, so every sum can be tried with rows of its
// own.
package ledger

import (
	"math"
	"sort"
	"strconv"
	"time"
)

// Kind is what a transaction did.
type Kind string

const (
	// Open is the position the user held before they started keeping the
	// ledger. It takes no date: it always comes first.
	Open Kind = "open"
	// Buy adds shares for money.
	Buy Kind = "buy"
	// Sell takes shares back for money.
	Sell Kind = "sell"
	// Dividend is cash the fund paid out, which keeps the shares as they were.
	Dividend Kind = "dividend"
	// Reinvest adds shares the fund paid as a dividend. A split is recorded
	// the same way: the arithmetic is the same.
	Reinvest Kind = "reinvest"
)

// Txn is one row of a fund's ledger. The fields a kind does not use are left
// at zero.
type Txn struct {
	// ID is unique within a fund, and stays with its row as the ledger is
	// edited: a removed id is never given to another row.
	ID int `json:"id"`
	// Date is the day the transaction took effect, "2006-01-02". Only Open
	// may leave it empty.
	Date string `json:"date,omitempty"`
	Kind Kind   `json:"kind"`
	// Shares is the number of shares an open, buy, sell or reinvest moved;
	// Price is 成交净值 for a buy or sell and 成本价 for an open.
	Shares float64 `json:"shares,omitempty"`
	Price  float64 `json:"price,omitempty"`
	// Fee is the money the broker charged, in yuan, on a buy or sell.
	Fee float64 `json:"fee,omitempty"`
	// Amount is the money a cash dividend paid, in yuan.
	Amount float64 `json:"amount,omitempty"`
}

// Position is what the ledger leaves the user holding.
type Position struct {
	// Shares is the number held, in units of the fund, and Basis the money
	// paid for those shares, fees included.
	Shares float64
	Basis  float64
	// Cost is Basis per share, and zero without shares.
	Cost float64
	// Realized is the profit sales have taken, Dividends the cash the fund
	// paid, Invested all the money put in, and Fees all the broker charged.
	Realized, Dividends, Invested, Fees float64
	// CostUnknown says the position began with an opening row whose price was
	// never filled in, so Cost says nothing about what it cost.
	CostUnknown bool
}

// Step is one row's own result: what a sale realised, and the position after
// the row. A later list shows these beside the rows.
type Step struct {
	Txn      Txn
	Realized float64
	Shares   float64
	Cost     float64
}

// Problem says what is wrong with a row, or with the order the rows make.
type Problem int

const (
	BadShares Problem = iota
	BadPrice
	BadFee
	BadAmount
	BadDate
	Oversell
	TwoOpenings
	UnknownKind
)

// String names a problem, for an error's text.
func (p Problem) String() string {
	switch p {
	case BadShares:
		return "bad shares"
	case BadPrice:
		return "bad price"
	case BadFee:
		return "bad fee"
	case BadAmount:
		return "bad amount"
	case BadDate:
		return "bad date"
	case Oversell:
		return "sale beyond the shares held"
	case TwoOpenings:
		return "more than one opening row"
	case UnknownKind:
		return "unknown kind"
	}
	return "problem"
}

// Error is a Problem, with the row it was found on.
type Error struct {
	Problem Problem
	ID      int
	Date    string
	// Held is how many shares the position held when a sale asked for more
	// than that.
	Held float64
}

// Error writes the problem and the row that has it.
func (e *Error) Error() string {
	s := "ledger: " + e.Problem.String()
	if e.ID != 0 {
		s += " (id " + strconv.Itoa(e.ID) + ")"
	}
	if e.Date != "" {
		s += " at " + e.Date
	}
	if e.Problem == Oversell {
		s += ", held " + strconv.FormatFloat(e.Held, 'f', -1, 64)
	}
	return s
}

// dust is how few shares are still the whole position after a rounding: a
// count this small is zero, so that a sale of what a series of roundings
// added up to leaves nothing behind.
const dust = 5e-5

// cent is the value below which a count of shares is swept rather than left
// in the position: what a buyer would call a rounding.
const cent = 0.01

// validDate reports that a date is a real day written the one way the ledger
// sorts and compares: "2006-01-02".
func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}

// finite reports that a number is a number, rather than a NaN or an infinity
// that arithmetic cannot carry.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Check validates the fields of one row, without looking at the rows around
// it. A date is needed on every row but an opening; a buy and a sell need a
// price; a dividend needs money; the shares a row moves must be more than
// zero. Fields a kind does not use may be zero or not, but a negative or
// unusable number is refused even there.
func Check(t Txn) error {
	switch t.Kind {
	case Open, Buy, Sell, Dividend, Reinvest:
	default:
		return &Error{Problem: UnknownKind, ID: t.ID, Date: t.Date}
	}
	if t.Date == "" {
		if t.Kind != Open {
			return &Error{Problem: BadDate, ID: t.ID}
		}
	} else if !validDate(t.Date) {
		return &Error{Problem: BadDate, ID: t.ID, Date: t.Date}
	}
	if !finite(t.Shares) || t.Shares < 0 {
		return &Error{Problem: BadShares, ID: t.ID, Date: t.Date}
	}
	if !finite(t.Price) || t.Price < 0 {
		return &Error{Problem: BadPrice, ID: t.ID, Date: t.Date}
	}
	if !finite(t.Fee) || t.Fee < 0 {
		return &Error{Problem: BadFee, ID: t.ID, Date: t.Date}
	}
	if !finite(t.Amount) || t.Amount < 0 {
		return &Error{Problem: BadAmount, ID: t.ID, Date: t.Date}
	}
	switch t.Kind {
	case Open, Buy, Sell, Reinvest:
		if t.Shares <= 0 {
			return &Error{Problem: BadShares, ID: t.ID, Date: t.Date}
		}
	}
	switch t.Kind {
	case Buy, Sell:
		if t.Price <= 0 {
			return &Error{Problem: BadPrice, ID: t.ID, Date: t.Date}
		}
	case Dividend:
		if t.Amount <= 0 {
			return &Error{Problem: BadAmount, ID: t.ID, Date: t.Date}
		}
	}
	return nil
}

// rank is where a kind comes on a day it shares with others: the dividend was
// paid first, then it was reinvested, then the day's sales and buys moved the
// shares that were left. The order does not depend on when the user typed the
// rows in.
func rank(k Kind) int {
	switch k {
	case Dividend:
		return 0
	case Reinvest:
		return 1
	case Sell:
		return 2
	case Buy:
		return 3
	}
	return 4
}

// Sort returns a copy of the rows in the ledger's canonical order: the opening
// row first, then by date, and on one day by kind and then by id.
func Sort(txns []Txn) []Txn {
	out := append([]Txn(nil), txns...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		aOpen, bOpen := a.Kind == Open, b.Kind == Open
		if aOpen != bOpen {
			return aOpen
		}
		if aOpen {
			return a.ID < b.ID
		}
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if ra, rb := rank(a.Kind), rank(b.Kind); ra != rb {
			return ra < rb
		}
		return a.ID < b.ID
	})
	return out
}

// NextID is an id no row uses, after the highest one. A removed id is not
// handed out again while a row with a higher one remains.
func NextID(txns []Txn) int {
	id := 0
	for _, t := range txns {
		if t.ID > id {
			id = t.ID
		}
	}
	return id + 1
}

// round4 and round6 take the precision the ledger keeps: shares to four
// decimals, a cost to six. Nothing is rounded to cents; the views round for
// display.
func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }
func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// costAfter is Basis per share as a step reports it.
func costAfter(pos Position) float64 {
	if pos.Shares == 0 {
		return 0
	}
	return round6(pos.Basis / pos.Shares)
}

// Replay works the position out of the rows, in the canonical order, and
// gives each row's own result. It is total: a row it cannot use is skipped
// and the first problem is returned with the position the rows that stood
// before it left. Everything else is applied leniently, so the caller still
// has a position to show.
func Replay(txns []Txn) (Position, []Step, error) {
	var pos Position
	var steps []Step
	var first error
	opened := false

	note := func(err error) {
		if first == nil {
			first = err
		}
	}

	for _, t := range Sort(txns) {
		if err := Check(t); err != nil {
			note(err)
			steps = append(steps, Step{Txn: t, Shares: pos.Shares, Cost: costAfter(pos)})
			continue
		}

		realized := 0.0
		switch t.Kind {
		case Open:
			if opened {
				note(&Error{Problem: TwoOpenings, ID: t.ID, Date: t.Date})
				steps = append(steps, Step{Txn: t, Shares: pos.Shares, Cost: costAfter(pos)})
				continue
			}
			opened = true
			s := round4(t.Shares)
			pos.Shares += s
			pos.Basis += s * t.Price
			pos.Invested += s * t.Price
			if t.Price == 0 {
				pos.CostUnknown = true
			}
		case Buy:
			s := round4(t.Shares)
			pos.Shares += s
			pos.Basis += s*t.Price + t.Fee
			pos.Invested += s*t.Price + t.Fee
			pos.Fees += t.Fee
		case Sell:
			take := round4(t.Shares)
			held := pos.Shares
			if take-held > dust {
				note(&Error{Problem: Oversell, ID: t.ID, Date: t.Date, Held: held})
				// The row asks for more than there is: sell what there is, so
				// that the position stays one the user could hold.
				take = math.Max(held, 0)
			}
			var cost float64
			if pos.Shares-take < cent {
				// The sale leaves less than a hundredth of a share. That is a
				// rounding, not a holding: the whole basis goes with the
				// shares that leave.
				cost = pos.Basis
				pos.Shares = 0
			} else {
				cost = pos.Basis * take / pos.Shares
				pos.Shares -= take
			}
			pos.Basis -= cost
			realized = take*t.Price - t.Fee - cost
			pos.Realized += realized
			pos.Fees += t.Fee
		case Dividend:
			pos.Dividends += t.Amount
		case Reinvest:
			pos.Shares += round4(t.Shares)
		}
		// A position of no shares costs nothing, whatever rounding left
		// behind.
		if pos.Shares == 0 {
			pos.Basis = 0
		}
		steps = append(steps, Step{Txn: t, Realized: realized, Shares: pos.Shares, Cost: costAfter(pos)})
	}

	if math.Abs(pos.Shares) < dust {
		pos.Shares, pos.Basis = 0, 0
	} else {
		pos.Shares = round4(pos.Shares)
	}
	if pos.Shares == 0 {
		pos.Cost = 0
	} else {
		pos.Cost = round6(pos.Basis / pos.Shares)
	}
	return pos, steps, first
}

// Put adds a row, or replaces the row with the same id, and returns the rows
// in canonical order with the position they make. A row with no id is given
// the next one. A row the arithmetic cannot accept leaves the ledger as it
// was, with the problem returned.
func Put(txns []Txn, t Txn) ([]Txn, Position, error) {
	if err := Check(t); err != nil {
		pos, _, _ := Replay(txns)
		return Sort(txns), pos, err
	}

	out := append([]Txn(nil), txns...)
	next := NextID(out)
	// An opening row made from a manual position has no id yet. Give it one
	// now, so that every row on disk can be edited and removed by id.
	for i := range out {
		if out[i].ID == 0 {
			out[i].ID = next
			next++
		}
	}
	if t.ID == 0 {
		t.ID = next
		out = append(out, t)
	} else {
		found := false
		for i := range out {
			if out[i].ID == t.ID {
				out[i] = t
				found = true
				break
			}
		}
		if !found {
			out = append(out, t)
		}
	}

	pos, _, err := Replay(out)
	if err != nil {
		// A change that leaves the rows with a problem they did not have, such
		// as a sale beyond the shares held or a second opening, is refused as
		// a whole, so that a typo does not quietly change the position. Rows
		// that already had a problem, as a hand-edited file can, stay
		// editable, so that they can still be put right.
		if old, _, was := Replay(txns); was == nil {
			return Sort(txns), old, err
		}
	}
	return Sort(out), pos, err
}

// Remove takes a row out and returns the remaining rows and position. A
// removal that would give the rows a problem they did not have, as taking out
// a buy that a later sale depends on does, is refused as a whole.
func Remove(txns []Txn, id int) ([]Txn, Position, error) {
	out := make([]Txn, 0, len(txns))
	removed := false
	for _, t := range txns {
		if t.ID == id && !removed {
			removed = true
			continue
		}
		out = append(out, t)
	}
	if !removed {
		pos, _, _ := Replay(txns)
		return Sort(txns), pos, nil
	}

	pos, _, err := Replay(out)
	if err != nil {
		// As in Put: rows that were sound are left as they were.
		if old, _, was := Replay(txns); was == nil {
			return Sort(txns), old, err
		}
	}
	return Sort(out), pos, err
}

// Opening is the row that carries a position the user held before keeping the
// ledger. It has no date, so it always sorts first.
func Opening(shares, cost float64) Txn {
	return Txn{Kind: Open, Shares: shares, Price: cost}
}

// OnlyOpening reports that the rows are just the opening one. Removing
// everything else then returns the fund to a position typed by hand.
func OnlyOpening(txns []Txn) (Txn, bool) {
	if len(txns) != 1 || txns[0].Kind != Open {
		return Txn{}, false
	}
	return txns[0], true
}

// SharesBefore is how many shares were held at the close before date: the
// rows dated earlier, with the opening always among them. A share bought on
// the day itself did not earn that day's change, and one sold that day still
// did.
func SharesBefore(txns []Txn, date string) float64 {
	before := make([]Txn, 0, len(txns))
	for _, t := range txns {
		if t.Kind == Open || t.Date < date {
			before = append(before, t)
		}
	}
	pos, _, _ := Replay(before)
	return pos.Shares
}
