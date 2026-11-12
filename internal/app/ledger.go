package app

import (
	"errors"

	"fundbar/internal/ledger"
	"fundbar/internal/store"
)

// ledgered reports that the fund's position comes from its transactions
// rather than from the two numbers typed by hand.
func (f *Fund) ledgered() bool { return len(f.Txns) > 0 }

// adoptLedger gives a fund the rows its ledger file holds, and the position
// they make. Load and Add both go through it: a file that is there is the
// fund's own history, whether an earlier run wrote it or the user put it
// back by hand after removing the fund, and it is not thrown away for being
// found.
func (a *App) adoptLedger(f *Fund) {
	txns, err := store.LoadLedger(a.dir, f.Code)
	switch {
	case err != nil:
		// The rows could not be read. The fund keeps the position it has,
		// and why stays on it, for its card to say.
		f.ledgerErr = err
	case len(txns) > 0:
		// One valid row is enough: the ledger is the truth, and the shares
		// and cost in funds.json only mirror it for older builds.
		f.Txns = txns
		f.pos, f.steps, f.ledgerErr = ledger.Replay(txns)
		f.Shares, f.Cost = f.pos.Shares, f.pos.Cost
	}
}

// setLedger makes txns the fund's ledger and derives the position from them.
// The rows are written before the fund takes them, and before funds.json is
// saved: a write that fails then leaves the windows showing what the file
// still holds, rather than a position the next launch would not find, and a
// write torn between the two files keeps the rows, which are the truth,
// rather than only the mirror. It reports a write that failed.
func (a *App) setLedger(f *Fund, txns []ledger.Txn) error {
	// Removing everything but the opening row returns the fund to a position
	// typed by hand, with exactly the numbers the opening carried.
	if op, ok := ledger.OnlyOpening(txns); ok {
		store.DropLedger(a.dir, f.Code)
		f.Txns, f.steps = nil, nil
		f.pos = ledger.Position{}
		f.ledgerErr = nil
		f.Shares, f.Cost = op.Shares, op.Price
		a.changed()
		return nil
	}
	if len(txns) == 0 {
		store.DropLedger(a.dir, f.Code)
		f.Txns, f.steps = nil, nil
		f.pos = ledger.Position{}
		f.ledgerErr = nil
		f.Shares, f.Cost = 0, 0
		a.changed()
		return nil
	}

	// An opening row made from a manual position is made without an id. Give
	// it, and any other row that has none, one now, so that every row on disk
	// can be edited and removed by id.
	next := ledger.NextID(txns)
	stored := append([]ledger.Txn(nil), txns...)
	for i := range stored {
		if stored[i].ID == 0 {
			stored[i].ID = next
			next++
		}
	}

	if errors.Is(f.ledgerErr, store.ErrLedgerUnreadable) {
		// A file that could not be read may still be where it was, as when
		// the fund was taken up after the app started: it is moved aside
		// rather than written over, for the rows a person may yet recover.
		_, _ = store.SetAsideLedger(a.dir, f.Code)
	}
	pos, steps, problem := ledger.Replay(stored)
	if err := store.SaveLedger(a.dir, f.Code, stored); err != nil {
		a.failure = "交易记录保存失败：" + err.Error()
		return err
	}
	f.Txns, f.pos, f.steps = stored, pos, steps
	f.Shares, f.Cost = pos.Shares, pos.Cost
	f.ledgerErr = problem
	a.changed()
	return nil
}
