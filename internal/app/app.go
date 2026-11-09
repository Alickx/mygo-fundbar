// Package app is the fund bar: the funds the user follows, what the network
// says about them, and the windows and menu bar item that show them.
package app

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/egoist/mygo/ui"

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

	// Navs is the fund's net asset value each day, oldest first.
	Navs          []model.Nav
	Holdings      []model.Holding
	Report        string
	StockPosition float64
	Quotes        map[string]model.Quote

	Est    model.Estimate
	HasEst bool
	// QuoteAt is when the stocks of the estimate were last priced.
	QuoteAt time.Time
	// Err is why the fund could not be read, if it could not.
	Err    string
	Loaded bool
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

// TodayProfit is what the user's shares gained in the newest day the app
// knows of, in money: today's estimate while the markets are open, and the
// published change of the last trading day once its value is out.
func (f *Fund) TodayProfit() float64 {
	if f.Shares == 0 {
		return 0
	}
	if f.UsingEstimate() {
		last, _ := f.LastNav()
		return f.Shares * (f.EstNav() - last.Unit)
	}
	if n := len(f.Navs); n >= 2 {
		return f.Shares * (f.Navs[n-1].Unit - f.Navs[n-2].Unit)
	}
	return 0
}

// TotalProfit is what the user's shares gained since they bought them.
func (f *Fund) TotalProfit() float64 {
	if f.Shares == 0 {
		return 0
	}
	return f.Shares*f.EstNav() - f.Shares*f.Cost
}

// TotalPct is TotalProfit as a share of what the shares cost.
func (f *Fund) TotalPct() float64 {
	if f.Shares == 0 || f.Cost <= 0 {
		return 0
	}
	return (f.EstNav()/f.Cost - 1) * 100
}

// App is the state of the window and the menu bar item.
type App struct {
	win    *mygo.Window
	tray   *mygo.Tray
	src    *source.Client
	dir    string
	config string

	funds  []*Fund
	list   ui.ListState
	sel    int
	period int
	pick   int
	query  string

	hits      []source.Hit
	searching bool
	searchID  int

	busy    bool
	updated time.Time
	status  string
	failure string

	// want is a fund the menu bar item chose, which the next frame shows.
	want string
	// pendingRemove is a fund the context menu takes out, after the frame
	// that built it.
	pendingRemove int
	// searchDue is when the search awaits the end of typing.
	searchDue time.Time
}

// cst is the time zone the funds trade in.
var cst = time.FixedZone("CST", 8*60*60)

func now() time.Time { return time.Now().In(cst) }

// New makes the app, whose files live in dir.
func New(dir string) *App {
	return &App{
		src:           source.New(),
		dir:           dir,
		config:        filepath.Join(dir, "funds.json"),
		pendingRemove: -1,
		pick:          -1,
	}
}

// Start reads the funds the user follows and the cache of what was read
// about them, and starts asking the network.
func (a *App) Start() {
	a.Load()
	a.refresh()
	go a.loop()
}

// Load reads the funds the user follows and the cache of what was read
// about them, without asking the network.
func (a *App) Load() {
	a.funds = nil
	a.sel = 0
	data := store.Load(a.config)
	for _, e := range data.Funds {
		f := &Fund{Code: e.Code, Type: e.Type, Shares: e.Shares, Cost: e.Cost}
		if snap, ok := store.LoadSnapshot(a.dir, e.Code); ok {
			a.fill(f, snap)
		}
		a.funds = append(a.funds, f)
		if e.Code == data.Selected {
			a.sel = len(a.funds) - 1
		}
	}
	a.period = data.Period
	if len(a.funds) == 0 {
		a.sel = -1
	} else if a.sel >= len(a.funds) {
		a.sel = 0
	}
}

func (a *App) fill(f *Fund, snap *store.Snapshot) {
	f.Name = snap.Fund.Name
	f.Navs = snap.Fund.Navs
	f.StockPosition = snap.Fund.StockPosition
	f.Holdings = snap.Holdings
	f.Report = snap.Report
	f.Loaded = true
	a.estimate(f, snap.Quotes)
}

// estimate works the fund's change today out of its holdings and their
// prices, keyed by symbol.
func (a *App) estimate(f *Fund, quotes map[string]model.Quote) {
	if quotes != nil {
		f.Quotes = quotes
	}
	f.Est, f.HasEst = model.EstimateFund(f.Holdings, f.Quotes)
	// A fund that holds an exchange-traded fund, or few stocks, cannot be
	// estimated from its holdings.
	if f.HasEst && f.Est.Cover < 20 {
		f.HasEst = false
	}
	f.QuoteAt = latestQuote(f.Quotes, f.Holdings)
}

// save writes the funds the user follows.
func (a *App) save() {
	data := &store.Data{Period: a.period}
	for _, f := range a.funds {
		data.Funds = append(data.Funds, store.Entry{
			Code: f.Code, Type: f.Type, Shares: f.Shares, Cost: f.Cost,
		})
	}
	if a.sel >= 0 && a.sel < len(a.funds) {
		data.Selected = a.funds[a.sel].Code
	}
	if err := data.Save(a.config); err != nil {
		a.failure = "保存失败：" + err.Error()
	}
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
	if a.busy || len(a.funds) == 0 {
		return
	}
	a.busy = true
	codes := make([]string, 0, len(a.funds))
	types := make(map[string]string, len(a.funds))
	cached := make(map[string]*store.Snapshot, len(a.funds))
	for _, f := range a.funds {
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

		// One request prices every stock of every fund.
		var symbols []string
		seen := map[string]bool{}
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
			}
		}
		quotes, qerr := a.src.Quotes(ctx, symbols)

		a.win.Update(func() {
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
				a.fill(f, r.snap)
				f.Err = ""
				store.SaveSnapshot(a.dir, f.Code, r.snap)
			}
			a.busy = false
			a.updated = time.Now()
			a.status = ""
			a.failure = ""
			if qerr != nil {
				a.failure = "行情获取失败：" + qerr.Error()
			}
			a.updateTray()
		})
	}()
}

// readFund reads a fund, from the cache when it is today's.
func (a *App) readFund(ctx context.Context, code, kind string, cached *store.Snapshot) *store.Snapshot {
	if cached != nil && time.Since(cached.At) < 20*time.Minute && fresh(cached) {
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
	return &store.Snapshot{At: time.Now(), Fund: fund, Holdings: holdings, Report: report}
}

// fresh reports that a snapshot's net asset values are today's, or that the
// market has not published today's yet and they are the newest.
func fresh(snap *store.Snapshot) bool {
	if snap == nil || snap.Fund == nil || len(snap.Fund.Navs) == 0 {
		return false
	}
	last := snap.Fund.Navs[len(snap.Fund.Navs)-1].Date
	today := now().Format("2006-01-02")
	if last == today {
		return true
	}
	// Before the evening's publication, yesterday's value is the newest.
	if h := now().Hour(); h < 20 && now().Weekday() != time.Saturday && now().Weekday() != time.Sunday {
		return true
	}
	return false
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
	for _, f := range a.funds {
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
		})
	}
}

// tradingNow reports whether the exchanges are open.
func tradingNow() bool {
	t := now()
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	m := t.Hour()*60 + t.Minute()
	return (m >= 9*60+15 && m <= 11*60+30) || (m >= 13*60 && m <= 15*60+5)
}

// Add follows a fund.
func (a *App) Add(hit source.Hit) {
	if a.byCode(hit.Code) != nil {
		a.query = ""
		a.hits = nil
		for i, f := range a.funds {
			if f.Code == hit.Code {
				a.sel = i
			}
		}
		return
	}
	f := &Fund{Code: hit.Code, Name: hit.Name, Type: hit.Type}
	a.funds = append(a.funds, f)
	a.sel = len(a.funds) - 1
	a.query = ""
	a.hits = nil
	a.pick = -1
	a.save()
	a.refresh()
}

// Remove stops following the fund at i.
func (a *App) Remove(i int) {
	if i < 0 || i >= len(a.funds) {
		return
	}
	store.DropSnapshot(a.dir, a.funds[i].Code)
	a.funds = append(a.funds[:i], a.funds[i+1:]...)
	if a.sel >= len(a.funds) {
		a.sel = len(a.funds) - 1
	}
	a.pick = -1
	a.save()
	a.updateTray()
	a.redraw()
}

// reorder moves rows of the list to the place the user dragged them to.
func (a *App) reorder(rows []int, to int) {
	if len(rows) == 0 || len(a.funds) == 0 {
		return
	}
	moving := make(map[int]bool, len(rows))
	var moved []*Fund
	for _, r := range rows {
		if r >= 0 && r < len(a.funds) {
			moving[r] = true
			moved = append(moved, a.funds[r])
		}
	}
	if len(moved) == 0 {
		return
	}
	chosen := a.Selected()
	var rest []*Fund
	for i, f := range a.funds {
		if !moving[i] {
			rest = append(rest, f)
		}
	}
	// `to` counts the rows before the place in the list as it was.
	at := to
	for r := range moving {
		if r < to {
			at--
		}
	}
	at = max(0, min(at, len(rest)))
	out := make([]*Fund, 0, len(a.funds))
	out = append(out, rest[:at]...)
	out = append(out, moved...)
	out = append(out, rest[at:]...)
	a.funds = out
	for i, f := range a.funds {
		if f == chosen {
			a.sel = i
		}
	}
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

// Summary adds up the funds the user follows.
func (a *App) Summary() Summary {
	var s Summary
	var cost, value float64
	for _, f := range a.funds {
		if f.Shares > 0 {
			s.Holding = true
			s.Value += f.Value()
			s.Profit += f.TodayProfit()
			cost += f.Shares * f.Cost
			value += f.Value()
		}
		switch {
		case f.ChangePct() > 0:
			s.Up++
		case f.ChangePct() < 0:
			s.Down++
		}
	}
	if s.Holding {
		last := value - s.Profit
		if last > 0 {
			s.Pct = s.Profit / last * 100
		}
	} else {
		// Without shares, the funds' average change says the same thing.
		var sum float64
		var n int
		for _, f := range a.funds {
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

// window title text of the menu bar item, kept as wide as it can be so that
// the icon does not move as the numbers change.
func (a *App) trayTitle() string {
	s := a.Summary()
	if len(a.funds) == 0 {
		return ""
	}
	var text string
	if s.Holding {
		text = fmt.Sprintf("%+.2f", s.Profit)
	} else {
		text = fmt.Sprintf("%+.2f%%", s.Pct)
	}
	return padFigures(text, 7)
}

func (a *App) trayTooltip() string {
	s := a.Summary()
	if len(a.funds) == 0 {
		return "基金估值：还没有自选基金"
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
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String() + "." + frac
	}
	return b.String() + "." + frac
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
