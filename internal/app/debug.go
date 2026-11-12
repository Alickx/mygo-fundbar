package app

import (
	"math"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/source"
	"fundbar/internal/store"
)

// The hooks below draw parts of the app without the network or a window,
// as tools/shot does.

// PreviewSearch fills the search field and its results.
func (a *App) PreviewSearch(query string, hits []source.Hit) {
	a.query = query
	a.hits = hits
}

// PreviewBatch fills the search field with codes alone, which the sidebar
// answers with its one-row batch add instead of a search.
func (a *App) PreviewBatch(query string) {
	a.query = query
	a.hits = nil
	a.searching = false
}

// PreviewSettings draws the settings window.
func (a *App) PreviewSettings(c *ui.Context) { a.settingsView(c) }

// PreviewEdit unfolds the fields of the shares held.
func (a *App) PreviewEdit(on bool) { a.editing = on }

// PreviewPortfolio shows the holding group's look-through, as the summary
// bar's button does. It is state only: a preview must not write the user's
// files.
func (a *App) PreviewPortfolio(on bool) {
	a.portfolio = on
	a.listSel = -1
}

// PreviewPeriod selects a chart period, as the control does.
func (a *App) PreviewPeriod(period int) {
	if period >= 0 && period < len(periods) {
		a.period = period
		a.pick = -1
	}
}

// PreviewCompare draws funds beside the chosen one, as the compare menu
// does. The comparison is session state, so a preview never writes it.
func (a *App) PreviewCompare(codes ...string) {
	a.compare = nil
	chosen := ""
	if f := a.Selected(); f != nil {
		chosen = f.Code
	}
	for _, code := range codes {
		if code == "" || code == chosen || a.inCompare(code) || len(a.compare) >= maxCompare {
			continue
		}
		if f := a.byCode(code); f != nil {
			a.compare = append(a.compare, f.Code)
		}
	}
}

// PreviewGroup shows a group the user made, making it when it is new.
func (a *App) PreviewGroup(name string) {
	for _, g := range a.groups {
		if g.Name == name {
			a.SelectGroup(g.ID)
			return
		}
	}
	a.beginNewGroup()
	a.groupDraft = name
	a.CommitGroup()
}

// PreviewGroupEdit unfolds the field of a new group.
func (a *App) PreviewGroupEdit() {
	a.beginNewGroup()
	a.groupDraft = "关注"
}

// PreviewLogin sets the login item a preview shows.
func (a *App) PreviewLogin(on bool) { a.previewLogin = on }

// PreviewTrayText sets the menu bar item's mode, as the settings do.
func (a *App) PreviewTrayText(mode string) { a.trayText = mode }

// PreviewGlobalShortcut shows the shortcut switch on or off, without claiming
// the keys a preview must not take.
func (a *App) PreviewGlobalShortcut(on bool) { a.globalShortcut = on }

// PreviewIndices shows or hides the panel's benchmarks, as the settings do.
func (a *App) PreviewIndices(on bool) { a.showIndices = on }

// PreviewMarketIndices fills the panel's benchmarks with a trading day's
// prices, which the fixture's cache does not hold.
func (a *App) PreviewMarketIndices() {
	a.indices = []model.Quote{
		{Symbol: "sh000001", Price: 3680.64, Pct: 0.46},
		{Symbol: "sh000300", Price: 4075.71, Pct: 0.41},
		{Symbol: "sz399006", Price: 2572.07, Pct: 3.11},
	}
}

// PreviewAlerts turns the notification rules on, as the settings show them.
func (a *App) PreviewAlerts(move bool, movePct float64, settled bool) {
	a.alerts = store.Alerts{Move: move, MovePct: movePct, Settled: settled}
	a.alertSeeded = false
}

// PreviewAlertDenied shows the permission the user took away.
func (a *App) PreviewAlertDenied() { a.alertErr = deniedAlert }

// PreviewTargets sets the price targets of a fund, for the holding card.
func (a *App) PreviewTargets(code string, takeProfit, stopLoss float64) {
	if f := a.byCode(code); f != nil {
		f.TakeProfit, f.StopLoss = takeProfit, stopLoss
	}
}

// PreviewLedger gives a fund sample rows for the ledger card, or opens its
// form, as tools/shot draws it. It keeps the rows in memory only: unlike
// PreviewGroup, a preview must not write the user's files.
func (a *App) PreviewLedger(code, mode string) {
	f := a.byCode(code)
	if f == nil {
		return
	}
	// A fund that already keeps rows previews them; one that does not gets
	// the sample.
	txns := append([]ledger.Txn(nil), f.Txns...)
	if len(txns) == 0 {
		txns = sampleLedger(f.Navs)
	}
	if mode == "ledger-error" {
		// A sale beyond the shares held is the load-time problem the warning
		// line exists for.
		if last, ok := navFromEnd(f.Navs, 0); ok {
			txns = append(txns, ledger.Txn{
				Date: last.Date, Kind: ledger.Sell, Shares: 999999, Price: last.Unit,
			})
		}
	}
	a.previewLedger(f, txns)
	a.editing = true
	switch mode {
	case "ledger-add":
		a.beginTxn(f)
	case "ledger-edit":
		for _, t := range f.Txns {
			if t.Kind == ledger.Buy {
				a.beginEditTxn(f, t.ID)
				break
			}
		}
	}
}

// previewLedger derives a fund's position from rows without saving them.
func (a *App) previewLedger(f *Fund, txns []ledger.Txn) {
	next := ledger.NextID(txns)
	for i := range txns {
		if txns[i].ID == 0 {
			txns[i].ID = next
			next++
		}
	}
	pos, steps, err := ledger.Replay(txns)
	f.Txns, f.pos, f.steps, f.ledgerErr = txns, pos, steps, err
	f.Shares, f.Cost = pos.Shares, pos.Cost
}

// sampleLedger is a fund's example rows, priced from its own values when it
// has them: an opening position, two buys, a dividend and a sale, with fees
// of a realistic 0.15%.
func sampleLedger(navs []model.Nav) []ledger.Txn {
	if len(navs) == 0 {
		return []ledger.Txn{
			ledger.Opening(12000, 0.62),
			{ID: 2, Date: "2026-08-14", Kind: ledger.Buy,
				Shares: 1000, Price: 0.6012, Fee: 0.90},
			{ID: 3, Date: "2026-09-18", Kind: ledger.Buy,
				Shares: 2000, Price: 0.5890, Fee: 1.77},
			{ID: 4, Date: "2026-10-08", Kind: ledger.Dividend, Amount: 220},
			{ID: 5, Date: "2026-10-28", Kind: ledger.Sell,
				Shares: 1500, Price: 0.6350, Fee: 1.43},
		}
	}
	first := navs[0]
	buy1, _ := navFromEnd(navs, 120)
	buy2, _ := navFromEnd(navs, 80)
	div, _ := navFromEnd(navs, 45)
	sale, _ := navFromEnd(navs, 12)
	fee := func(shares, price float64) float64 {
		return math.Round(shares*price*0.0015*100) / 100
	}
	return []ledger.Txn{
		ledger.Opening(12000, first.Unit),
		{ID: 2, Date: buy1.Date, Kind: ledger.Buy,
			Shares: 1000, Price: buy1.Unit, Fee: fee(1000, buy1.Unit)},
		{ID: 3, Date: buy2.Date, Kind: ledger.Buy,
			Shares: 2000, Price: buy2.Unit, Fee: fee(2000, buy2.Unit)},
		{ID: 4, Date: div.Date, Kind: ledger.Dividend, Amount: 220},
		{ID: 5, Date: sale.Date, Kind: ledger.Sell,
			Shares: 1500, Price: sale.Unit, Fee: fee(1500, sale.Unit)},
	}
}

// navFromEnd is the nav that many values back from the newest, or the oldest
// when the history is shorter.
func navFromEnd(navs []model.Nav, back int) (model.Nav, bool) {
	if len(navs) == 0 {
		return model.Nav{}, false
	}
	i := len(navs) - 1 - back
	if i < 0 {
		i = 0
	}
	return navs[i], true
}

// PreviewUpdate shows the strip of a newer version, as a check found one.
func (a *App) PreviewUpdate(version, notes, step string, notesShown bool) {
	s := updateFound
	switch step {
	case "fetching":
		s = updateFetching
	case "ready":
		s = updateReady
	}
	a.upd = updateState{step: s, version: version, notes: notes, notesShown: notesShown}
	if s == updateFetching {
		a.upd.progress = 0.42
	}
}
