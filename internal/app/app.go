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
	win  *mygo.Window
	tray *mygo.Tray
	// panel is the preview the menu bar item opened, or nil while it is
	// closed: a popover goes as the user clicks elsewhere.
	panel *mygo.Window

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
	// navView is the history the chart card shows: the line, or a list of
	// each day's value.
	navView int
	query   string

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
	// editing unfolds the fields of the shares held.
	editing bool
	// settings is the settings window, while it is open.
	settings     *mygo.Window
	settingsErr  string
	previewLogin bool

	hits      []source.Hit
	searching bool
	searchID  int

	busy    bool
	updated time.Time
	status  string
	failure string

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
var cst = time.FixedZone("CST", 8*60*60)

func now() time.Time { return time.Now().In(cst) }

// New makes the app, whose files live in dir.
func New(dir string) *App {
	return &App{
		src:           source.New(),
		dir:           dir,
		config:        filepath.Join(dir, "funds.json"),
		pendingRemove: -1,
		pendingLeave:  -1,
		wantJoin:      -1,
		pick:          -1,
		selCode:       map[string]string{},
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
	a.all = nil
	a.funds = nil
	a.sel = -1
	data := store.Load(a.config)
	a.groups = data.Groups
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
	for _, e := range data.Funds {
		groups := e.Groups
		if len(groups) == 0 {
			groups = []string{""}
		}
		f := &Fund{Code: e.Code, Type: e.Type, Shares: e.Shares, Cost: e.Cost, Groups: groups}
		if snap, ok := store.LoadSnapshot(a.dir, e.Code); ok {
			a.fill(f, snap)
		}
		a.all = append(a.all, f)
	}
	a.period = data.Period
	a.navView = data.NavView
	a.sidebarWidth = data.SidebarWidth
	if a.sidebarWidth == 0 {
		a.sidebarWidth = 348
	}
	a.rebuildList()
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
	a.syncSelection()
	data := &store.Data{
		Period: a.period, NavView: a.navView, SidebarWidth: a.sidebarWidth,
		Groups: a.groups, ActiveGroup: a.group, Selected: a.selCode[""],
	}
	for _, f := range a.all {
		data.Funds = append(data.Funds, store.Entry{
			Code: f.Code, Type: f.Type, Shares: f.Shares, Cost: f.Cost,
			Groups: f.Groups,
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
		quotes, profiles, qerr := a.priceAndProfile(ctx, symbols, profileSymbols)

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
			}
			a.busy = false
			a.updated = time.Now()
			a.status = ""
			a.failure = ""
			if qerr != nil {
				a.failure = "行情获取失败：" + qerr.Error()
			}
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
func tradingNow() bool {
	t := now()
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
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
	f := &Fund{Code: hit.Code, Name: hit.Name, Type: hit.Type, Groups: []string{a.group}}
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
	a.all = dropFund(a.all, f)
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
	var s Summary
	var value float64
	for _, f := range funds {
		if held && f.Shares > 0 {
			s.Holding = true
			s.Value += f.Value()
			s.Profit += f.TodayProfit()
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

// window title text of the menu bar item, kept as wide as it can be so that
// the icon does not move as the numbers change.
func (a *App) trayTitle() string {
	s := a.Summary()
	if len(a.all) == 0 || len(a.inGroup(holdingGroup)) == 0 {
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
