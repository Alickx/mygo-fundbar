// Package app is the fund bar: the funds the user follows, what the network
// says about them, and the windows and menu bar item that show them.
package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/calendar"
	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/source"
	"fundbar/internal/store"
)

// Fund is a fund the window shows, with what the network last said about it.
type Fund struct {
	Code   string
	Name   string
	Type   string
	Shares float64
	Cost   float64
	// Txns are the fund's transactions, and pos, steps and ledgerErr what
	// replaying them says: the position the shares mirror, each row's own
	// result, and the first problem the rows have. ledgerErr also carries why
	// the rows could not be read at all (store.ErrLedgerUnreadable). A fund
	// with no rows keeps its position typed by hand.
	Txns      []ledger.Txn
	pos       ledger.Position
	steps     []ledger.Step
	ledgerErr error
	// TakeProfit and StopLoss are the cumulative return, in percent, at which
	// the app announces the fund. Zero leaves that side off.
	TakeProfit float64
	StopLoss   float64
	// sharesText, costText, takeText and stopText are the holding form's
	// fields as typed, which the frame after the edit opens fills in from
	// the values.
	sharesText, costText string
	takeText, stopText   string
	editText             bool
	// Groups are the groups the fund is listed in: "" is the holding group,
	// the main list of funds the user holds or follows, and any other id is
	// a group the user made to watch funds without holding them.
	Groups []string

	// Navs is the fund's net asset value each day, oldest first.
	Navs           []model.Nav
	Holdings       []model.Holding
	Report         string
	StockPosition  float64
	Quotes         map[string]model.Quote
	Profiles       map[string]model.StockProfile
	Industries     []model.Industry
	IndustryReport string

	Est    model.Estimate
	HasEst bool
	// QuoteAt is when the stocks of the estimate were last priced.
	QuoteAt time.Time
	// Err is why the fund could not be read, if it could not.
	Err    string
	Loaded bool
}

// In reports that the fund is listed in a group.
func (f *Fund) In(group string) bool {
	for _, g := range f.Groups {
		if g == group {
			return true
		}
	}
	return false
}

// LastNav is the newest net asset value the fund published.
func (f *Fund) LastNav() (model.Nav, bool) {
	if len(f.Navs) == 0 {
		return model.Nav{}, false
	}
	return f.Navs[len(f.Navs)-1], true
}

// UsingEstimate reports that the holdings' prices are newer than the fund's
// published net asset value, so that the estimate is the freshest number the
// app has. Between the close and the evening's publication, the estimate is
// the day's change; after it, the published value is.
func (f *Fund) UsingEstimate() bool {
	if !f.HasEst || f.QuoteAt.IsZero() {
		return false
	}
	last, ok := f.LastNav()
	if !ok {
		return false
	}
	return f.QuoteAt.In(cst).Format("2006-01-02") > last.Date
}

// EstNav is the fund's net asset value, as the estimate gives it while the
// estimate is newer, and as published otherwise.
func (f *Fund) EstNav() float64 {
	last, ok := f.LastNav()
	if !ok {
		return 0
	}
	if f.UsingEstimate() {
		return last.Unit * (1 + f.Est.Pct/100)
	}
	return last.Unit
}

// ChangePct is how much the fund moved in the newest day the app knows of.
func (f *Fund) ChangePct() float64 {
	if f.UsingEstimate() {
		return f.Est.Pct
	}
	if last, ok := f.LastNav(); ok {
		return last.Pct
	}
	return 0
}

// Today is today's date where the funds trade.
func (f *Fund) Today() string { return now().Format("2006-01-02") }

// Settled reports that the funds' value for today is published, so that what
// shows is the value itself rather than an estimate.
func (f *Fund) Settled() bool {
	last, ok := f.LastNav()
	return ok && last.Date == f.Today()
}

// Stale reports that the prices behind the estimate are not today's, as in
// the morning, at the weekend, or on a holiday.
func (f *Fund) Stale() bool {
	if !f.HasEst || f.QuoteAt.IsZero() {
		return true
	}
	return f.QuoteAt.In(cst).Format("2006-01-02") != f.Today()
}

// Value is what the user's shares are worth.
func (f *Fund) Value() float64 { return f.Shares * f.EstNav() }

// todayShares is how many shares earned the change of the day the app shows.
// A fund with a ledger held them before that day began: a share bought on
// the day did not earn its change, and one sold that day still did. A
// position typed by hand has no days, and is taken as held throughout.
func (f *Fund) todayShares() float64 {
	if !f.ledgered() {
		return f.Shares
	}
	day := ""
	if f.UsingEstimate() {
		day = f.QuoteAt.In(cst).Format("2006-01-02")
	} else if last, ok := f.LastNav(); ok {
		day = last.Date
	}
	return ledger.SharesBefore(f.Txns, day)
}

// TodayProfit is what the user's shares gained in the newest day the app
// knows of, in money: today's estimate while the markets are open, and the
// published change of the last trading day once its value is out.
func (f *Fund) TodayProfit() float64 {
	shares := f.todayShares()
	if shares == 0 {
		return 0
	}
	if f.UsingEstimate() {
		last, _ := f.LastNav()
		return shares * (f.EstNav() - last.Unit)
	}
	if n := len(f.Navs); n >= 2 {
		prev, last := f.Navs[n-2], f.Navs[n-1]
		return shares * prev.Unit * model.DayReturn(prev, last) / 100
	}
	return 0
}

// todayBase is what the shares that earned the day's change were worth when
// the day began: what the day's gain is a percent of. It is not the value
// now less the gain, which a purchase or a sale during the day would bend.
func (f *Fund) todayBase() float64 {
	shares := f.todayShares()
	if shares == 0 {
		return 0
	}
	if f.UsingEstimate() {
		last, _ := f.LastNav()
		return shares * last.Unit
	}
	switch n := len(f.Navs); {
	case n >= 2:
		return shares * f.Navs[n-2].Unit
	case n == 1:
		// One value says nothing of a change: the shares count as unmoved.
		return shares * f.Navs[0].Unit
	}
	return 0
}

// TotalProfit is what the user's shares gained since they bought them: what
// the shares held are worth now, less what they cost, plus what sales took
// and dividends paid.
func (f *Fund) TotalProfit() float64 {
	if f.ledgered() {
		return f.Shares*f.EstNav() - f.pos.Basis + f.pos.Realized + f.pos.Dividends
	}
	if f.Shares == 0 {
		return 0
	}
	return f.Shares*f.EstNav() - f.Shares*f.Cost
}

// HoldingPct is the return on the shares held now, in percent: the current
// value against their average cost. It says nothing about sales already
// made; TotalPct does.
func (f *Fund) HoldingPct() float64 {
	// An opening position whose cost was never filled in leaves the average
	// cost too low to compare a price with.
	if f.Cost <= 0 || f.pos.CostUnknown {
		return 0
	}
	return (f.EstNav()/f.Cost - 1) * 100
}

// TotalPct is TotalProfit as a share of what the shares cost.
func (f *Fund) TotalPct() float64 {
	if f.ledgered() {
		// With an opening position of unknown cost, the money put in is not
		// all there to divide by.
		if f.pos.Invested <= 0 || f.pos.CostUnknown {
			return 0
		}
		return f.TotalProfit() / f.pos.Invested * 100
	}
	if f.Shares == 0 || f.Cost <= 0 {
		return 0
	}
	return (f.EstNav()/f.Cost - 1) * 100
}

// App is the state of the window and the menu bar item.
type App struct {
	win  *mygo.Window
	tray *mygo.Tray
	// panel is the preview the menu bar item opened, or nil while it is
	// closed: a popover goes as the user clicks elsewhere.
	panel *mygo.Window
	// panelShown is the group the preview listed last, and panelMoved when
	// it started listing another, which its list slides in from the right.
	// panelSeeded tells that the preview has drawn a frame since it opened.
	panelShown  string
	panelMoved  time.Time
	panelSeeded bool
	// panelDir is the side of the group bar the last group listed comes
	// from, which its list slides in from.
	panelDir float32

	src    *source.Client
	dir    string
	config string

	// all is every fund the user follows, in the order the user put them
	// in; funds is the active group's, which the list shows.
	all    []*Fund
	funds  []*Fund
	groups []store.Group
	// group is the active group: "" for the holding group, or the id of a
	// group the user made.
	group string
	// selCode is the chosen fund of each group, by group id.
	selCode map[string]string
	list    ui.ListState
	sel     int
	period  int
	pick    int
	// compare is the funds drawn beside the chosen one, by code. It is
	// session state: the chart's comparison is not saved, and choosing a
	// fund or removing one takes it out here.
	compare []string
	// navView is what the chart card shows: the line, a list of each day's
	// value, or the window's recovery from its deepest fall.
	navView int
	query   string
	// portfolio shows the holding group's look-through in place of a fund's
	// detail. It is session state: neither a launch nor a new window resumes
	// it, and nothing about it is saved. listSel is the row the list chooses
	// while the pane shows: the pane leaves no fund looking chosen, but a
	// click on any row, the chosen fund's included, says which fund to show
	// on the way out, and the next frame reads it here.
	portfolio bool
	listSel   int

	// groupEditing unfolds the field of a new or renamed group, whose id is
	// groupEditID ("" while the group is new).
	groupEditing bool
	groupEditID  string
	groupDraft   string
	groupErr     string
	// wantNewGroup, wantRename and wantDeleteGroup are what a menu asked
	// for, done before the next frame is built.
	wantNewGroup    bool
	wantRename      string
	wantDeleteGroup string
	// wantGroup is the group the more menu chose, "" being the holding
	// group, and wantGroupSet says it chose one.
	wantGroup    string
	wantGroupSet bool

	// sidebarWidth is how wide the user made the list of funds, and
	// savedSidebar what the file has.
	sidebarWidth float32
	savedSidebar float32
	// editing unfolds the holding card: the fields of the shares held, or
	// the ledger's rows and form.
	editing bool
	// tx is the transaction the ledger card edits, wantTxEdit and
	// wantTxDelete what a click or a menu asked for, done before the next
	// frame, and wantManual the fund a menu returns to a position typed by
	// hand. txNote is a line the card shows once, with txUndo's rows to put
	// back after a delete.
	tx           txForm
	wantTxEdit   txRef
	wantTxDelete txRef
	wantManual   string
	txNote       string
	txUndo       ledgerUndo
	// holdBodyH is the height the unfolded card took last frame, which the
	// card animates between.
	holdBodyH float32
	// settings is the settings window, while it is open. dataErr is why the
	// last backup, import or export failed, which it shows under the 数据
	// section.
	settings     *mygo.Window
	settingsErr  string
	dataErr      string
	previewLogin bool
	// saveFile, openFile and confirm are the native dialogs the backup and
	// the export ask with. They block until dismissed, so a caller runs them
	// off the main thread; a test replaces them, and the defaults do nothing
	// without a running app.
	saveFile func(mygo.SaveDialogOptions) (string, error)
	openFile func(mygo.OpenDialogOptions) ([]string, error)
	confirm  func(mygo.MessageOptions) (mygo.MessageResult, error)
	// writeImage puts a PNG on the clipboard for the share picture. It is a
	// field, as the dialogs are, so that a test sees the bytes; its default
	// does nothing until a real app is running.
	writeImage func([]byte) error
	// trayText is what the menu bar item shows beside the icon, one of the
	// trayMoney, trayPct and trayIcon modes; a preview of an older file
	// leaves it as the gain.
	trayText string
	// themeMode is the appearance the user chose: "light", "dark", or ""
	// for the system's. An older file names none, and follows the system.
	themeMode string
	// globalShortcut is whether the panel's shortcut is on. The two fields
	// below do the registering, so a test sees the calls without a running
	// app, as notify does; the defaults do nothing until the app is ready.
	globalShortcut     bool
	registerShortcut   func(acc string, fn func()) error
	unregisterShortcut func(acc string)
	// showIndices is the panel's row of market benchmarks, and indices are
	// what the last refresh read, kept in memory rather than in a fund's
	// cache because no fund owns them.
	showIndices bool
	indices     []model.Quote
	// lowCoverEstimate shows an estimate for funds whose disclosed holdings
	// cover less than minCover, as the settings switch asks.
	lowCoverEstimate bool

	hits      []source.Hit
	searching bool
	searchID  int

	// alerts are the notification rules the settings set, alertState what
	// they have announced so far, and notify what shows a note the rules
	// decided on; a test replaces notify.
	alerts     store.Alerts
	alertState store.AlertState
	notify     func(Note)
	// estimates is how close the app's estimates have been to the values the
	// funds published, kept beside the funds as the alert marks are.
	// estimatesDirty says they hold something the file does not yet: loading
	// settles open estimates from the cache without writing, and the next
	// refresh saves them.
	estimates      store.Estimates
	estimatesDirty bool
	// alertText is the move threshold as typed, and alertSeeded says it was
	// filled in from the setting for the window that is open.
	alertText   string
	alertSeeded bool
	// alertErr is why notifications do not arrive, such as a denied
	// permission, which the settings window shows.
	alertErr string

	busy    bool
	updated time.Time
	status  string
	failure string

	// upd is the newer version the app found and what it does with it, which
	// the strip under the summary shows; up downloads it. They change on the
	// main thread only.
	upd      updateState
	up       *mygo.Update
	watching bool
	// checkedAt is when the app last finished asking for a newer version,
	// and checking says one is in flight. They change on the main thread
	// only.
	checkedAt time.Time
	checking  bool
	// flash is a line the status row shows for a while, such as the answer
	// to a check the user asked for, and flashUntil when it goes.
	flash      string
	flashUntil time.Time
	// skippedVersion is the newer version the user ignored: the app offers
	// the one after it.
	skippedVersion string

	// want is a fund the menu bar item chose, which the next frame shows.
	want string
	// pendingRemove and pendingLeave are a fund the context menu takes out,
	// or takes out of the group, after the frame that built it; wantJoin is
	// a fund it adds to another group.
	pendingRemove int
	pendingLeave  int
	wantJoin      int
	wantJoinGroup string
	// searchDue is when the search awaits the end of typing.
	searchDue time.Time
}

// cst is the time zone the funds trade in.
var cst = calendar.CST

func now() time.Time { return time.Now().In(cst) }

// configName is the file that keeps the settings and the funds the user
// follows, in the app's data directory.
const configName = "funds.json"

// StartupTheme is the appearance the saved setting asks for, read before the
// app's windows exist so that it starts in it rather than switching to it
// after the first frame.
func StartupTheme(dir string) mygo.ThemeSource {
	return themeSource(store.Load(filepath.Join(dir, configName)).Theme)
}

// New makes the app, whose files live in dir.
func New(dir string) *App {
	a := &App{
		src:           source.New(),
		dir:           dir,
		config:        filepath.Join(dir, configName),
		pendingRemove: -1,
		pendingLeave:  -1,
		wantJoin:      -1,
		pick:          -1,
		listSel:       -1,
		selCode:       map[string]string{},
		estimates:     store.Estimates{},
		trayText:      trayMoney,
		showIndices:   true,
	}
	a.notify = a.deliver
	// The dialogs belong to a running app only; a preview or a test has no
	// native window to attach them to, and gets no answer.
	a.saveFile = func(o mygo.SaveDialogOptions) (string, error) {
		if !mygo.App.IsReady() {
			return "", nil
		}
		return mygo.Dialog.Save(o)
	}
	a.openFile = func(o mygo.OpenDialogOptions) ([]string, error) {
		if !mygo.App.IsReady() {
			return nil, nil
		}
		return mygo.Dialog.Open(o)
	}
	a.confirm = func(o mygo.MessageOptions) (mygo.MessageResult, error) {
		if !mygo.App.IsReady() {
			return mygo.MessageResult{Button: -1}, nil
		}
		return mygo.Dialog.Message(o)
	}
	a.writeImage = func(data []byte) error {
		if !mygo.App.IsReady() {
			return nil
		}
		return mygo.Clipboard.WriteImage(data)
	}
	// The real shortcut is claimed only by a running app; a test replaces
	// these two before calling Start or the switch, and sees each call.
	a.registerShortcut = func(acc string, fn func()) error {
		if !mygo.App.IsReady() {
			return nil
		}
		return mygo.GlobalShortcut.Register(acc, fn)
	}
	a.unregisterShortcut = func(acc string) {
		if mygo.App.IsReady() {
			mygo.GlobalShortcut.Unregister(acc)
		}
	}
	return a
}

// Start reads the funds the user follows and the cache of what was read
// about them, and starts asking the network.
func (a *App) Start() {
	a.Load()
	a.setAsideUnreadable()
	if a.globalShortcut {
		// The saved setting is on: claim the keys again for this run.
		a.applyGlobalShortcut(true)
	}
	a.refresh()
	if a.win != nil {
		// A test or a preview has no window to bring the answers back to.
		go a.loop()
	}
	a.watchUpdates()
	a.watchNotifications()
}

// setAsideUnreadable moves the ledger files that did not parse out of the
// way, once, as the app starts: recording a transaction would otherwise
// write a new ledger over rows a person may still recover. Load itself
// leaves them where they are, because a preview loads too and must not move
// the user's files.
func (a *App) setAsideUnreadable() {
	for _, f := range a.all {
		if !errors.Is(f.ledgerErr, store.ErrLedgerUnreadable) {
			continue
		}
		if _, err := store.SetAsideLedger(a.dir, f.Code); err != nil {
			a.failure = "交易记录读取失败：" + f.Code + "（" + err.Error() + "）"
		}
	}
}

// Load reads the funds the user follows and the cache of what was read
// about them, without asking the network.
func (a *App) Load() {
	a.all = nil
	a.funds = nil
	a.sel = -1
	data := store.Load(a.config)
	a.groups = data.Groups
	a.alerts = store.Alerts{}
	if data.Alerts != nil {
		a.alerts = *data.Alerts
	}
	a.alertState = *store.LoadAlerts(a.dir)
	a.estimates = store.LoadEstimates(a.dir)
	a.selCode = map[string]string{}
	if data.Selected != "" {
		a.selCode[""] = data.Selected
	}
	for id, code := range data.GroupSelected {
		if id != "" && code != "" {
			a.selCode[id] = code
		}
	}
	a.group = data.ActiveGroup
	if a.group != "" && !a.hasGroup(a.group) {
		a.group = ""
	}
	// The setting decides what a cached fund's estimate is worth as it is
	// filled in below, so it is read before the funds.
	a.lowCoverEstimate = data.LowCoverEstimate
	for _, e := range data.Funds {
		groups := e.Groups
		if len(groups) == 0 {
			groups = []string{""}
		}
		f := &Fund{
			Code: e.Code, Type: e.Type, TakeProfit: e.TakeProfit,
			StopLoss: e.StopLoss, Groups: groups,
		}
		f.Shares, f.Cost = e.Shares, e.Cost
		a.adoptLedger(f)
		if f.ledgerErr != nil && !f.ledgered() {
			// The status line's word goes with the next refresh; the fund's
			// own card keeps saying it.
			a.failure = "交易记录读取失败：" + e.Code
		}
		if snap, ok := store.LoadSnapshot(a.dir, e.Code); ok {
			a.fill(f, snap)
			// The cached values can settle an estimate that was still open
			// when the app last closed. Loading writes no file, as the
			// screenshot tool relies on: the next refresh saves it.
			if a.fillActuals(f) {
				a.estimatesDirty = true
			}
		}
		a.all = append(a.all, f)
	}
	a.period = data.Period
	a.navView = data.NavView
	a.sidebarWidth = data.SidebarWidth
	a.skippedVersion = data.SkippedVersion
	// The three settings an older file does not name fall back to what the
	// app did before them: the gain, no shortcut, and the benchmarks shown.
	a.trayText = data.TrayText
	if a.trayText == "" {
		a.trayText = trayMoney
	}
	a.themeMode = parseTheme(data.Theme)
	a.globalShortcut = data.GlobalShortcut
	a.showIndices = !data.HideIndices
	if a.sidebarWidth == 0 {
		a.sidebarWidth = 348
	}
	a.rebuildList()
	// An import can bring another appearance with it; the app switches once
	// everything else is in place. A preview or a test has no app loop to
	// switch, and draws the theme the system gives it.
	if mygo.App.IsReady() {
		mygo.Theme.SetSource(themeSource(a.themeMode))
	}
}

func (a *App) fill(f *Fund, snap *store.Snapshot) {
	f.Name = snap.Fund.Name
	f.Navs = snap.Fund.Navs
	f.StockPosition = snap.Fund.StockPosition
	f.Holdings = snap.Holdings
	f.Report = snap.Report
	f.Industries = snap.Fund.Industries
	f.IndustryReport = snap.Fund.IndustryReport
	f.Profiles = snap.Profiles
	f.Loaded = true
	a.estimate(f, snap.Quotes)
}

// minCover is the share of the fund the disclosed holdings have to cover for
// its estimate to be shown by default: below it, the lot says too little of
// what the fund owns. The settings can show the estimate anyway, marked with
// its coverage.
const minCover = 20.0

// estimate works the fund's change today out of its holdings and their
// prices, keyed by symbol.
func (a *App) estimate(f *Fund, quotes map[string]model.Quote) {
	if quotes != nil {
		f.Quotes = quotes
	}
	f.Est, f.HasEst = model.EstimateFund(f.Holdings, f.Quotes)
	if f.HasEst {
		// The holdings are the fund's stocks; bonds and cash do not move with
		// them, so the holdings' change comes down to the stock position.
		f.Est = model.ScaleEstimate(f.Est, f.StockPosition)
	}
	// A fund that holds an exchange-traded fund, or few stocks, cannot be
	// estimated from its holdings.
	if f.HasEst && f.Est.Cover < minCover && !a.lowCoverEstimate {
		f.HasEst = false
	}
	f.QuoteAt = latestQuote(f.Quotes, f.Holdings)
}

// setLowCoverEstimate turns on or off the estimate for thinly disclosed
// funds, and works out the loaded funds again with the new rule.
func (a *App) setLowCoverEstimate(on bool) {
	a.lowCoverEstimate = on
	for _, f := range a.all {
		a.estimate(f, nil)
	}
	a.save()
	a.redraw()
}

// save writes the funds the user follows.
func (a *App) save() {
	a.syncSelection()
	// The default mode writes nothing, so a file from before the setting
	// stays as it was until the user chooses another mode.
	trayText := a.trayText
	if trayText == trayMoney {
		trayText = ""
	}
	data := &store.Data{
		Period: a.period, NavView: a.navView, SidebarWidth: a.sidebarWidth,
		Groups: a.groups, ActiveGroup: a.group, Selected: a.selCode[""],
		SkippedVersion: a.skippedVersion,
		TrayText:       trayText, GlobalShortcut: a.globalShortcut,
		HideIndices: !a.showIndices, LowCoverEstimate: a.lowCoverEstimate,
		Theme: a.themeMode,
	}
	if a.alerts != (store.Alerts{}) {
		// The file names the rules once the user has set one, and not before,
		// so that it stays as it was for everyone who has none.
		data.Alerts = &a.alerts
	}
	for _, f := range a.all {
		shares, cost := f.Shares, f.Cost
		if f.ledgered() {
			// The entry keeps the position the ledger derives, so that a
			// build that knows nothing of ledgers still shows it.
			shares, cost = f.pos.Shares, f.pos.Cost
		}
		data.Funds = append(data.Funds, store.Entry{
			Code: f.Code, Type: f.Type, Shares: shares, Cost: cost,
			TakeProfit: f.TakeProfit, StopLoss: f.StopLoss, Groups: f.Groups,
		})
	}
	for id, code := range a.selCode {
		if id == "" || code == "" {
			continue
		}
		if data.GroupSelected == nil {
			data.GroupSelected = map[string]string{}
		}
		data.GroupSelected[id] = code
	}
	if err := data.Save(a.config); err != nil {
		a.failure = "保存失败：" + err.Error()
	}
}

// marketIndices are the benchmarks the panel shows, in its order, with the
// short name it has room for. The same Tencent endpoint that prices stocks
// answers for indices with the same fields.
var marketIndices = []struct {
	Symbol string
	Label  string
}{
	{"sh000001", "上证"},
	{"sh000300", "沪深300"},
	{"sz399006", "创业板"},
}

// indexSymbols is the symbols of the benchmarks, as the quotes request asks
// for them.
func indexSymbols() []string {
	out := make([]string, len(marketIndices))
	for i, m := range marketIndices {
		out[i] = m.Symbol
	}
	return out
}

// splitIndices takes the benchmarks out of a quote map, in the panel's order.
// They arrive in the same request as the funds' stocks, but belong to the app
// rather than to any fund: left in, they would be written into every fund's
// cache file.
func splitIndices(quotes map[string]model.Quote) []model.Quote {
	var out []model.Quote
	for _, m := range marketIndices {
		if q, ok := quotes[m.Symbol]; ok {
			out = append(out, q)
			delete(quotes, m.Symbol)
		}
	}
	return out
}

// indexLabel is the short name the panel gives a benchmark.
func indexLabel(symbol string) string {
	for _, m := range marketIndices {
		if m.Symbol == symbol {
			return m.Label
		}
	}
	return symbol
}

// Selected is the fund the detail pane shows, or nil.
func (a *App) Selected() *Fund {
	if a.sel < 0 || a.sel >= len(a.funds) {
		return nil
	}
	return a.funds[a.sel]
}

// refresh reads every fund again, in the background, and shows the outcome
// when it is in.
func (a *App) refresh() {
	if a.busy || len(a.all) == 0 {
		return
	}
	// A preview or a test draws the app without a window: there is nowhere to
	// bring the answer back to, and no network is asked.
	if a.win == nil {
		return
	}
	a.busy = true
	codes := make([]string, 0, len(a.all))
	types := make(map[string]string, len(a.all))
	cached := make(map[string]*store.Snapshot, len(a.all))
	for _, f := range a.all {
		codes = append(codes, f.Code)
		types[f.Code] = f.Type
		if snap, ok := store.LoadSnapshot(a.dir, f.Code); ok {
			cached[f.Code] = snap
		}
	}
	go func() {
		type result struct {
			code string
			snap *store.Snapshot
			err  error
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		results := make([]result, len(codes))
		var wg sync.WaitGroup
		slots := make(chan struct{}, 3)
		for i, code := range codes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				slots <- struct{}{}
				defer func() { <-slots }()
				r := result{code: code}
				r.snap = a.readFund(ctx, code, types[code], cached[code])
				results[i] = r
			}()
		}
		wg.Wait()

		// One request prices every stock of every fund, and one more reads
		// the industry of those whose profiles the cache does not hold yet.
		var symbols, profileSymbols []string
		seen := map[string]bool{}
		seenProfiles := map[string]bool{}
		for _, r := range results {
			if r.snap == nil {
				continue
			}
			for _, h := range r.snap.Holdings {
				if h.Symbol == "" {
					continue
				}
				if !seen[h.Symbol] {
					seen[h.Symbol] = true
					symbols = append(symbols, h.Symbol)
				}
				if _, ok := r.snap.Profiles[h.Symbol]; !ok && !seenProfiles[h.Symbol] {
					seenProfiles[h.Symbol] = true
					profileSymbols = append(profileSymbols, h.Symbol)
				}
			}
		}
		// The benchmarks ride along in the one request, and leave the map
		// before the funds take it over.
		for _, symbol := range indexSymbols() {
			if !seen[symbol] {
				seen[symbol] = true
				symbols = append(symbols, symbol)
			}
		}
		quotes, profiles, qerr := a.priceAndProfile(ctx, symbols, profileSymbols)
		indices := splitIndices(quotes)
		if qerr != nil {
			// A failed request leaves no row, as a preview shows no row
			// before the first refresh.
			indices = nil
		}

		a.win.Update(func() {
			a.indices = indices
			changed := false
			for _, r := range results {
				f := a.byCode(r.code)
				if f == nil {
					continue
				}
				if r.snap == nil {
					f.Err = "读取失败，请稍后重试"
					continue
				}
				r.snap.Quotes = quotes
				if len(profiles) > 0 {
					if r.snap.Profiles == nil {
						r.snap.Profiles = make(map[string]model.StockProfile, len(profiles))
					}
					for symbol, profile := range profiles {
						r.snap.Profiles[symbol] = profile
					}
				}
				if len(r.snap.Profiles) == 0 {
					r.snap.Profiles = cachedProfiles(cached[r.code])
				}
				a.fill(f, r.snap)
				f.Err = ""
				store.SaveSnapshot(a.dir, f.Code, r.snap)
				if a.fillActuals(f) {
					changed = true
				}
			}
			if a.recordEstimates(now()) {
				changed = true
			}
			a.busy = false
			a.updated = time.Now()
			a.status = ""
			a.failure = ""
			if qerr != nil {
				a.failure = "行情获取失败：" + qerr.Error()
			}
			if changed || a.estimatesDirty {
				a.estimatesDirty = false
				if err := store.SaveEstimates(a.dir, a.estimates); err != nil && a.failure == "" {
					a.failure = "保存估值记录失败：" + err.Error()
				}
			}
			a.runAlerts()
			a.updateTray()
			a.redrawPanel()
		})
	}()
}

// readFund reads a fund, from the cache when it is today's.
func (a *App) readFund(ctx context.Context, code, kind string, cached *store.Snapshot) *store.Snapshot {
	if cached != nil && time.Since(cached.At) < 20*time.Minute && fresh(cached) &&
		cached.Fund.IndustriesLoaded {
		return cached
	}
	fund, err := a.src.Fund(ctx, code)
	if err != nil {
		if cached != nil {
			return cached
		}
		return nil
	}
	holdings, report, err := a.src.Holdings(ctx, code)
	if err != nil && cached != nil && len(cached.Holdings) > 0 {
		holdings, report = cached.Holdings, cached.Report
	}
	if len(holdings) == 0 {
		// Nothing to estimate from, but the fund itself is worth showing.
		holdings, report = cachedHoldings(cached), cachedReport(cached)
	}
	industries, industryReport, err := a.src.Industries(ctx, code)
	if err == nil {
		fund.Industries = industries
		fund.IndustryReport = industryReport
		fund.IndustriesLoaded = true
	} else if cached != nil {
		fund.Industries = cached.Fund.Industries
		fund.IndustryReport = cached.Fund.IndustryReport
		fund.IndustriesLoaded = cached.Fund.IndustriesLoaded
	}
	return &store.Snapshot{
		At: time.Now(), Fund: fund, Holdings: holdings, Report: report,
		Profiles: cachedProfiles(cached),
	}
}

// priceAndProfile asks for the prices and profiles of every stock at once.
func (a *App) priceAndProfile(ctx context.Context, symbols, profileSymbols []string) (map[string]model.Quote, map[string]model.StockProfile, error) {
	var quotes map[string]model.Quote
	var profiles map[string]model.StockProfile
	var qerr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		quotes, qerr = a.src.Quotes(ctx, symbols)
	}()
	if len(profileSymbols) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			profiles, _ = a.src.Profiles(ctx, profileSymbols)
		}()
	}
	wg.Wait()
	return quotes, profiles, qerr
}

// fresh reports that a snapshot's net asset values are the newest the market
// has published.
func fresh(snap *store.Snapshot) bool { return freshAt(snap, now()) }

// freshAt is fresh at a given time.
func freshAt(snap *store.Snapshot, at time.Time) bool {
	if snap == nil || snap.Fund == nil || len(snap.Fund.Navs) == 0 {
		return false
	}
	last := snap.Fund.Navs[len(snap.Fund.Navs)-1].Date
	t := at.In(cst)
	if last == t.Format("2006-01-02") {
		return true
	}
	// A closed market publishes nothing, so the last value stays the newest.
	if !calendar.IsTradingDay(t) {
		return true
	}
	// Before the evening's publication, yesterday's value is the newest.
	return t.Hour() < 20
}

func cachedHoldings(s *store.Snapshot) []model.Holding {
	if s == nil {
		return nil
	}
	return s.Holdings
}

func cachedReport(s *store.Snapshot) string {
	if s == nil {
		return ""
	}
	return s.Report
}

func cachedProfiles(s *store.Snapshot) map[string]model.StockProfile {
	if s == nil {
		return nil
	}
	return s.Profiles
}

func latestQuote(quotes map[string]model.Quote, holdings []model.Holding) time.Time {
	var newest time.Time
	for _, h := range holdings {
		if q, ok := quotes[h.Symbol]; ok && q.At.After(newest) {
			newest = q.At
		}
	}
	return newest
}

func (a *App) byCode(code string) *Fund {
	for _, f := range a.all {
		if f.Code == code {
			return f
		}
	}
	return nil
}

// loop asks the network again while the funds trade, and rarely otherwise.
func (a *App) loop() {
	for {
		time.Sleep(10 * time.Second)
		a.win.Update(func() {
			if a.busy || a.win == nil {
				return
			}
			since := time.Since(a.updated)
			if tradingNow() {
				if since > 45*time.Second {
					a.refresh()
				}
				return
			}
			if since > 15*time.Minute {
				a.refresh()
			}
			if a.savedSidebar != a.sidebarWidth {
				a.save()
				a.savedSidebar = a.sidebarWidth
			}
		})
	}
}

// tradingNow reports whether the exchanges are open.
func tradingNow() bool { return tradingAt(now()) }

// tradingAt reports whether the exchanges are open at t.
func tradingAt(t time.Time) bool {
	t = t.In(cst)
	if !calendar.IsTradingDay(t) {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	return (m >= 9*60+15 && m <= 11*60+30) || (m >= 13*60 && m <= 15*60+5)
}

// Add follows a fund, in the group the user is looking at.
func (a *App) Add(hit source.Hit) {
	a.query = ""
	a.hits = nil
	a.pick = -1
	if f := a.byCode(hit.Code); f != nil {
		if !f.In(a.group) {
			f.Groups = append(f.Groups, a.group)
			a.save()
			a.rebuildList()
			a.redraw()
		}
		a.choose(f.Code)
		return
	}
	if !store.ValidCode(hit.Code) {
		// A code names files in the data directory; anything else is not
		// followed there, wherever the hit came from.
		a.failure = "基金代码无效：" + hit.Code
		return
	}
	f := &Fund{Code: hit.Code, Name: hit.Name, Type: hit.Type, Groups: []string{a.group}}
	// A ledger file found for the code is this fund's history — one written
	// in an earlier run, or one the user put back by hand after removing the
	// fund — so it is taken up rather than thrown away.
	a.adoptLedger(f)
	a.all = append(a.all, f)
	a.selCode[a.group] = f.Code
	a.rebuildList()
	a.save()
	a.refresh()
}

// Remove stops following the fund at i, in every group.
func (a *App) Remove(i int) {
	f := a.visible(i)
	if f == nil {
		return
	}
	store.DropSnapshot(a.dir, f.Code)
	// The transactions are the one thing here the network cannot give back:
	// they are set aside rather than deleted, so that a fund removed by
	// mistake can have them put back by hand.
	store.ShelveLedger(a.dir, f.Code)
	a.all = dropFund(a.all, f)
	a.pruneEstimates()
	// A fund that is no longer followed cannot stay in the comparison.
	a.pruneCompare("")
	for id, code := range a.selCode {
		if code == f.Code {
			delete(a.selCode, id)
		}
	}
	a.rebuildList()
	a.pick = -1
	a.save()
	a.updateTray()
	a.redraw()
}

// LeaveGroup takes the fund at i out of the active group, and keeps it in
// the groups it is in besides that one. A fund that leaves its last group is
// no longer followed at all.
func (a *App) LeaveGroup(i int) {
	f := a.visible(i)
	if f == nil {
		return
	}
	groups := make([]string, 0, len(f.Groups))
	for _, g := range f.Groups {
		if g != a.group {
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 {
		a.Remove(i)
		return
	}
	f.Groups = groups
	delete(a.selCode, a.group)
	a.rebuildList()
	a.save()
	a.updateTray()
	a.redraw()
}

// reorder moves rows of the list to the place the user dragged them to. The
// rows of the other groups keep their order around the moved ones.
func (a *App) reorder(rows []int, to int) {
	if len(rows) == 0 || len(a.funds) == 0 {
		return
	}
	a.syncSelection()
	moving := make(map[*Fund]bool, len(rows))
	var moved []*Fund
	for _, r := range rows {
		if r < 0 || r >= len(a.funds) || moving[a.funds[r]] {
			continue
		}
		moving[a.funds[r]] = true
		moved = append(moved, a.funds[r])
	}
	if len(moved) == 0 {
		return
	}
	var rest []*Fund
	for _, f := range a.funds {
		if !moving[f] {
			rest = append(rest, f)
		}
	}
	// `to` counts the rows before the place in the list as it was.
	at := to
	for _, r := range rows {
		if r >= 0 && r < to {
			at--
		}
	}
	at = max(0, min(at, len(rest)))

	// Place the moved rows beside the row they now follow, or precede, in
	// the group, so that the funds of the other groups keep their own order.
	remaining := make([]*Fund, 0, len(a.all))
	for _, f := range a.all {
		if !moving[f] {
			remaining = append(remaining, f)
		}
	}
	pos := 0
	switch {
	case len(rest) == 0:
		// Every row of the group moved: the block goes where the first of
		// them was.
		for i, f := range a.all {
			if f == moved[0] {
				pos = min(i, len(remaining))
				break
			}
		}
	case at > 0:
		for i, f := range remaining {
			if f == rest[at-1] {
				pos = i + 1
				break
			}
		}
	default:
		for i, f := range remaining {
			if f == rest[0] {
				pos = i
				break
			}
		}
	}
	out := make([]*Fund, 0, len(a.all))
	out = append(out, remaining[:pos]...)
	out = append(out, moved...)
	out = append(out, remaining[pos:]...)
	a.all = out
	a.rebuildList()
	a.save()
}

// search asks the fund site for funds matching the query, once the user has
// stopped typing.
func (a *App) search(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		a.hits = nil
		a.searching = false
		return
	}
	if _, ok := batchCodes(q); ok {
		// A batch of codes needs no search: its row adds them all at once,
		// and a search already in flight no longer matters.
		a.searchID++
		a.hits = nil
		a.searching = false
		return
	}
	a.searchID++
	id := a.searchID
	a.searching = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		hits, err := a.src.Search(ctx, q)
		a.win.Update(func() {
			if id != a.searchID {
				return
			}
			a.searching = false
			if err != nil {
				a.failure = "搜索失败：" + err.Error()
				return
			}
			a.hits = hits
		})
	}()
}

// Summary is what the menu bar item and the top of the window say.
type Summary struct {
	Value   float64
	Profit  float64
	Pct     float64
	Holding bool
	Up      int
	Down    int
}

// Summary adds up the funds of the holding group: the menu bar item stays
// what it was, whatever group the windows show.
func (a *App) Summary() Summary { return a.summarize(a.inGroup(""), true) }

// GroupSummary adds up the funds of a group. Only the holding group counts
// the shares held: a made group says how its funds moved, and nothing of
// what the user owns.
func (a *App) GroupSummary(group string) Summary {
	return a.summarize(a.inGroup(group), group == "")
}

// summarize adds up the funds, counting their shares when asked to.
func (a *App) summarize(funds []*Fund, held bool) Summary {
	return summarizeFunds(funds, held)
}

// summarizeFunds adds up the funds, counting their shares when asked to. It
// is a function of its own so that the alert rules can say what the menu bar
// says without the app around them.
func summarizeFunds(funds []*Fund, held bool) Summary {
	var s Summary
	// base is what the shares that earned the day's change were worth when
	// the day began, which the day's percent is measured against.
	var base float64
	for _, f := range funds {
		// A fund sold out during the day holds nothing now, but its shares
		// earned the day's change before they went, so it still counts.
		began := f.todayBase()
		if held && (f.Shares > 0 || began > 0) {
			s.Holding = true
			s.Value += f.Value()
			s.Profit += f.TodayProfit()
			base += began
		}
		switch {
		case f.ChangePct() > 0:
			s.Up++
		case f.ChangePct() < 0:
			s.Down++
		}
	}
	if s.Holding {
		if base > 0 {
			s.Pct = s.Profit / base * 100
		}
	} else {
		// Without shares, the funds' average change says the same thing.
		var sum float64
		var n int
		for _, f := range funds {
			if f.Loaded || f.HasEst {
				sum += f.ChangePct()
				n++
			}
		}
		if n > 0 {
			s.Pct = sum / float64(n)
		}
	}
	return s
}

// The menu bar item's modes: the day's gain in money, as it always showed;
// the day's percent; or no text beside the icon for a shared screen.
const (
	trayMoney = "money"
	trayPct   = "pct"
	trayIcon  = "icon"
)

// trayModes and trayLabels are the modes and their names, in the order the
// settings offer them.
var (
	trayModes  = []string{trayMoney, trayPct, trayIcon}
	trayLabels = []string{"收益金额", "涨跌幅", "仅图标"}
)

// trayModeIndex is the index of a mode among the settings' choices, and
// trayModeName the mode an index names.
func trayModeIndex(mode string) int {
	for i, m := range trayModes {
		if m == mode {
			return i
		}
	}
	return 0
}

func trayModeName(i int) string {
	if i < 0 || i >= len(trayModes) {
		return trayMoney
	}
	return trayModes[i]
}

// window title text of the menu bar item, kept as wide as it can be so that
// the icon does not move as the numbers change.
func (a *App) trayTitle() string {
	s := a.Summary()
	if len(a.all) == 0 || len(a.inGroup(holdingGroup)) == 0 {
		return ""
	}
	switch a.trayText {
	case trayIcon:
		return ""
	case trayPct:
		// The holding's own percent when shares are held, the average
		// otherwise, which is what Summary already says.
		return padFigures(pct(s.Pct), 7)
	}
	if s.Holding {
		return padFigures(fmt.Sprintf("%+.2f", s.Profit), 7)
	}
	return padFigures(pct(s.Pct), 7)
}

// globalShortcutKeys is the accelerator the settings turn on, in the syntax
// the menus use. Ctrl and Alt are the control and option keys on a Mac.
const globalShortcutKeys = "Ctrl+Alt+F"

// shortcutErrPrefix opens what the settings window says when the shortcut
// is taken.
const shortcutErrPrefix = "全局快捷键注册失败："

// applyGlobalShortcut claims or releases the panel's keys, and keeps the
// setting and the window's message in step with what happened. Start and the
// settings switch call it; a test replaces registerShortcut and
// unregisterShortcut to see each call.
func (a *App) applyGlobalShortcut(on bool) {
	if !on {
		a.unregisterShortcut(globalShortcutKeys)
		a.globalShortcut = false
		if strings.HasPrefix(a.settingsErr, shortcutErrPrefix) {
			a.settingsErr = ""
		}
		return
	}
	if err := a.registerShortcut(globalShortcutKeys, a.TogglePanel); err != nil {
		// The shortcut belongs to another app or to this one already. The
		// switch goes back off, and the settings window says why.
		a.globalShortcut = false
		a.settingsErr = shortcutErrPrefix + err.Error()
		return
	}
	a.globalShortcut = true
	if strings.HasPrefix(a.settingsErr, shortcutErrPrefix) {
		a.settingsErr = ""
	}
}

func (a *App) trayTooltip() string {
	s := a.Summary()
	if len(a.inGroup(holdingGroup)) == 0 {
		if len(a.all) == 0 {
			return "基金估值：还没有自选基金"
		}
		return "基金估值：持有分组还没有基金"
	}
	if s.Holding {
		return fmt.Sprintf("今日收益 %+.2f 元（%+.2f%%）\n持仓市值 %.2f 元", s.Profit, s.Pct, s.Value)
	}
	return fmt.Sprintf("自选基金平均 %+.2f%%", s.Pct)
}

func (a *App) updateTray() {
	if a.tray == nil {
		return
	}
	a.tray.SetTitle(a.trayTitle())
	a.tray.SetToolTip(a.trayTooltip())
}

// padFigures pads text with figure spaces, which are as wide as a digit, so
// that the menu bar item keeps its width as the number changes.
func padFigures(s string, n int) string {
	if len([]rune(s)) >= n {
		return s
	}
	return strings.Repeat("\u2007", n-len([]rune(s))) + s
}

// money writes an amount with thousands separators and two decimals.
func money(v float64) string {
	neg := v < 0
	v = math.Abs(v)
	s := fmt.Sprintf("%.2f", v)
	whole, frac, _ := strings.Cut(s, ".")
	s = groupDigits(whole) + "." + frac
	if neg {
		return "-" + s
	}
	return s
}

// groupDigits puts thousands separators in a whole number.
func groupDigits(whole string) string {
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// signedMoney writes an amount with its sign.
func signedMoney(v float64) string {
	if v > 0 {
		return "+" + money(v)
	}
	return money(v)
}

// pct writes a change in percent with its sign.
func pct(v float64) string {
	return fmt.Sprintf("%+.2f%%", v)
}
