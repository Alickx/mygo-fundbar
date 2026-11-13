package app

import (
	"fmt"
	"math"
	"sort"
	"time"

	"fundbar/internal/ledger"
	"fundbar/internal/model"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// periodMode is how a period's start is found: a fixed number of days back
// from the newest value, the turn of its year, or the fund's own first value.
type periodMode int

const (
	periodDays periodMode = iota
	periodYearToDate
	periodAll
)

// periods are the stretches of history the chart shows, in the order the
// control lists them. The order is saved as an index, so new periods are
// appended rather than put beside their length.
var periods = []struct {
	label string
	short string
	days  int
	mode  periodMode
}{
	{"近1月", "1月", 30, periodDays},
	{"近3月", "3月", 92, periodDays},
	{"近6月", "6月", 183, periodDays},
	{"近1年", "1年", 365, periodDays},
	{"今年来", "今年", 0, periodYearToDate},
	{"近3年", "3年", 1095, periodDays},
	{"成立以来", "全部", 0, periodAll},
}

// periodStart is when the chosen period opens. A fund younger than the
// period has net asset values from later on, but the window the user asked
// for still starts here: see plot.
func periodStart(f *Fund, period int) time.Time {
	if len(f.Navs) == 0 {
		return time.Time{}
	}
	if period < 0 || period >= len(periods) {
		period = 0
	}
	last := parseDay(f.Navs[len(f.Navs)-1].Date)
	switch periods[period].mode {
	case periodAll:
		return parseDay(f.Navs[0].Date)
	case periodYearToDate:
		// Open at the last value before January 1, so that the first day's
		// move is part of the year rather than its base.
		jan1 := time.Date(last.Year(), 1, 1, 0, 0, 0, 0, cst)
		for i := len(f.Navs) - 1; i >= 0; i-- {
			if at := parseDay(f.Navs[i].Date); at.Before(jan1) {
				return at
			}
		}
		// A fund that started this year has no year-end value to open at, so
		// its whole history is all there is.
		return parseDay(f.Navs[0].Date)
	}
	return last.AddDate(0, 0, -periods[period].days)
}

// windowOf is the fund's net asset value over the chosen period.
func windowOf(f *Fund, period int) []model.Nav {
	if len(f.Navs) == 0 {
		return nil
	}
	return valuesFrom(f, periodStart(f, period))
}

// valuesFrom is the fund's net asset value from a day on.
func valuesFrom(f *Fund, from time.Time) []model.Nav {
	var out []model.Nav
	for _, n := range f.Navs {
		if !parseDay(n.Date).Before(from) {
			out = append(out, n)
		}
	}
	return out
}

// chartWindow is the values the chart draws and the day its axis opens at,
// with the window's name. A manager start, when given, replaces the chosen
// period with the current team's own term.
func chartWindow(f *Fund, period int, managerSince time.Time) (navs []model.Nav, from time.Time, partial bool, label string) {
	if !managerSince.IsZero() {
		return valuesFrom(f, managerSince), managerSince, false, "现任经理以来"
	}
	navs = windowOf(f, period)
	from = periodStart(f, period)
	partial = len(f.Navs) > 0 && parseDay(f.Navs[0].Date).After(from)
	label = periods[period].label
	if partial {
		label = "成立以来"
	}
	return navs, from, partial, label
}

func parseDay(date string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", date, cst)
	if err != nil {
		return time.Time{}
	}
	return t
}

// buyMark is a purchase placed on the net asset value line: the day whose
// value it stands on, and the rows made on it. Buys that fall on the same
// value share one mark, so a busy week reads as one dot rather than a stack.
type buyMark struct {
	at     int // index in the window's navs
	date   string
	count  int
	shares float64
	amount float64
}

// buyMarks places the fund's purchases on the window's values. A purchase on
// a day the fund did not publish takes the next value, as the ledger's form
// does; one after the newest value sits on the last point, where the line
// ends, until the fund publishes its day. Purchases before the window are
// left out: the period does not show them.
func buyMarks(navs []model.Nav, txns []ledger.Txn) []buyMark {
	if len(navs) == 0 {
		return nil
	}
	var marks []buyMark
	at := map[int]int{} // nav index → index in marks
	for _, t := range txns {
		if t.Kind != ledger.Buy || t.Date == "" || t.Date < navs[0].Date {
			continue
		}
		i := len(navs) - 1
		for j, nav := range navs {
			if nav.Date >= t.Date {
				i = j
				break
			}
		}
		amount := t.Shares*t.Price + t.Fee
		if k, ok := at[i]; ok {
			mark := &marks[k]
			mark.count++
			mark.shares += t.Shares
			mark.amount += amount
			if t.Date > mark.date {
				mark.date = t.Date
			}
			continue
		}
		at[i] = len(marks)
		marks = append(marks, buyMark{
			at: i, date: t.Date, count: 1, shares: t.Shares, amount: amount,
		})
	}
	sort.Slice(marks, func(i, j int) bool { return marks[i].at < marks[j].at })
	return marks
}

// buyTip tells the story of a mark: what was bought, for how much, and when.
func buyTip(m buyMark) string {
	shares := sharesText(m.shares, 4)
	if m.count == 1 {
		return m.date + " 买入 " + shares + " 份 · " + money(m.amount) + " 元"
	}
	return fmt.Sprintf("%d 笔买入（最近 %s）· 共 %s 份 · %s 元",
		m.count, m.date, shares, money(m.amount))
}

// chartCard shows the fund's net asset value over a period the user picks,
// as a line, as the value of each day, or as how it is recovering from its
// deepest fall.
func (a *App) chartCard(c *ui.Context, t *ui.Theme, f *Fund) {
	if a.period < 0 || a.period >= len(periods) {
		a.period = 0
	}
	if a.navView < 0 || a.navView > 2 {
		a.navView = 0
	}
	// A recent manager change offers its own window, but only while the user
	// chose it: the filter stays set as the chosen fund changes, and a fund
	// without the change keeps its period.
	managerSince, managerNames, managerMode := f.managerChange(now())
	managerMode = managerMode && a.managerOnly
	if !managerMode {
		managerSince, managerNames = time.Time{}, nil
	}
	navs, from, partial, label := chartWindow(f, a.period, managerSince)
	// A young fund has less history than the period asks for, so the longer
	// periods show the same days. Cast the span as the fund's whole history,
	// and keep the wider window in the plot, so it reads as a new fund rather
	// than a chart that did not change.
	//
	// The plot opens at the window's start when the fund is younger than it,
	// so the line takes only the tail of the plot; the manager window keeps
	// its own first day too, even when the fund publishes its first value a
	// day or two later.
	axisFrom := time.Time{}
	if partial || managerMode {
		axisFrom = from
	}
	change := model.PeriodChange(navs)
	distribution := model.HasDistribution(navs)
	if a.pick >= len(navs) {
		a.pick = -1
	}
	// The chosen fund and the removed ones leave the comparison before the
	// legend and the plot read it.
	a.pruneCompare(f.Code)
	comparing := a.navView == 0 && len(a.compare) > 0 && len(navs) > 0
	var lines []compareLine
	if comparing {
		funds := []*Fund{f}
		for _, code := range a.compare {
			if g := a.byCode(code); g != nil {
				funds = append(funds, g)
			}
		}
		lines = compareSeries(parseDay(navs[0].Date),
			parseDay(navs[len(navs)-1].Date), funds)
	}
	// The recovery view reads the whole window, and says how far the last
	// value still is from its high.
	recovery := model.RecoveryOf(navs)

	card(c, t).Gap(8).Children(func() {
		// The readout has its own line: selecting a date never pushes the
		// period controls out of the card or changes the plot's position.
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			title := "净值走势"
			switch a.navView {
			case 1:
				title = "历史净值"
			case 2:
				title = "净值修复"
			}
			ui.Text(c, title).FontSize(13).FontWeight(600).NoWrap().Shrink(0)
			ui.Spacer(c)
			ui.Segmented(c, &a.navView, "走势", "列表", "修复").Shrink(0).
				Tooltip("净值走势、历史净值与净值修复").
				OnChange(func() {
					a.pick = -1
					a.save()
				})
			a.periodControl(c, t, f, managerMode)
		})
		ui.Row(c).Height(18).AlignItems(ui.Center).Gap(8).Children(func() {
			if len(navs) == 0 {
				return
			}
			if comparing {
				// The legend replaces the single line: each fund's change over
				// the same window, in its line's color.
				a.compareLegend(c, t, lines)
				a.compareButton(c, t)
				return
			}
			if a.navView == 2 {
				a.recoveryInfo(c, t, navs, recovery, label, partial, distribution)
				return
			}
			info := label + "净值变化 " + pct(change)
			color := up(t, change)
			if partial && len(navs) > 1 {
				info += " · 净值自 " + navs[0].Date + " 起"
			}
			if managerMode && len(navs) > 1 {
				info += " · 自 " + managerSince.Format("2006-01-02") + " 起"
			}
			if distribution && a.navView == 0 && a.pick < 0 {
				info += " · 前复权"
			}
			if len(navs) == 1 {
				info = "只有 " + navs[0].Date + " 一个净值日"
				color = t.TextMuted
			}
			if a.navView == 1 {
				info = fmt.Sprintf("%s · %d 个交易日 · 区间 %s",
					label, len(navs), pct(change))
			}
			if a.pick >= 0 && a.pick < len(navs) {
				n := navs[a.pick]
				info = fmt.Sprintf("%s · 净值 %.4f · 当日 %s",
					n.Date, n.Unit, pct(n.Pct))
				color = up(t, n.Pct)
			}
			// The tooltip carries the whole story, including why a period
			// longer than the fund's history shows what it shows, and why
			// the line differs from the published unit values.
			tip := info
			if a.navView == 0 && a.pick < 0 {
				if managerMode {
					tip = managerTip(managerNames, managerSince)
					if distribution {
						tip += "；区间内有分红或拆分，走势图按前复权净值绘制，涨跌按当日公布涨跌幅计算"
					}
				} else {
					switch {
					case distribution && partial && len(navs) > 1:
						tip = fmt.Sprintf("区间内有分红或拆分，走势图按前复权净值绘制，涨跌按当日公布涨跌幅计算；该基金净值自 %s 起，只有 %d 个交易日，填不满%s，图上显示的是全部历史",
							navs[0].Date, len(navs), periods[a.period].label)
					case distribution:
						tip = "区间内有分红或拆分，走势图按前复权净值绘制，涨跌按当日公布涨跌幅计算"
					case partial && len(navs) > 1:
						tip = fmt.Sprintf("该基金净值自 %s 起，只有 %d 个交易日，填不满%s，图上显示的是全部历史",
							navs[0].Date, len(navs), periods[a.period].label)
					}
				}
			}
			if a.navView == 1 {
				switch {
				case managerMode && a.pick < 0:
					tip = managerTip(managerNames, managerSince)
					if distribution {
						tip += "；区间内有分红或拆分，区间涨跌按当日公布涨跌幅计算，与首尾单位净值之比不同"
					}
				case distribution:
					// The rows below are published unit values, whose first and
					// last do not give this figure.
					tip = "区间内有分红或拆分，区间涨跌按当日公布涨跌幅计算，与首尾单位净值之比不同"
				}
			}
			ui.Text(c, info).FontSize(11.5).
				TextColor(color).FontFeatures("tnum").SingleLine().Ellipsis("…").
				Grow(1).Basis(0).MinWidth(0).Tooltip(tip)
			// The comparison is chosen here even while the list view shows,
			// and takes the plot's shape when the line view comes back.
			a.compareButton(c, t)
		})
		if a.navView == 1 {
			a.navTable(c, t, navs)
			return
		}
		if len(navs) < 2 {
			ui.Column(c).Height(164).Center().Children(func() {
				ui.Text(c, "还没有足够的历史净值").FontSize(11.5).TextColor(t.TextMuted)
			})
			return
		}
		if a.navView == 2 {
			a.recoveryPlot(c, t, navs, recovery, axisFrom)
			a.recoveryRow(c, t, recovery)
			return
		}
		if comparing {
			a.comparePlot(c, t, lines, navs, axisFrom)
		} else {
			a.plot(c, t, f, navs, axisFrom, up(t, change))
		}
		a.riskRow(c, t, navs)
	})
}

// riskRow is the shown window's risk and return in one compact line: how far
// it fell from its peak, how much it moved day to day, and what that amounts
// to over a year. Each measure keeps its definition in a tooltip, because a
// small line has no room for the caveats.
func (a *App) riskRow(c *ui.Context, t *ui.Theme, navs []model.Nav) {
	if len(navs) < 2 {
		return
	}
	drawdown, peak, low := model.MaxDrawdown(navs)
	volatility, returns := model.AnnualizedVolatility(navs)
	annual, days := model.AnnualizedReturn(navs)

	type stat struct {
		label, value, tip string
		color             ui.Color
	}
	stats := []stat{{
		label: "最大回撤", value: fmt.Sprintf("%.2f%%", drawdown),
		color: t.Text,
		tip:   "按前复权净值计算",
	}}
	if peak != "" && drawdown < 0 {
		stats[0].tip = fmt.Sprintf("按前复权净值计算：从 %s 的高点回落到 %s 的低点，回撤 %.2f%%",
			peak, low, -drawdown)
	}
	if returns >= 20 {
		stats = append(stats, stat{
			label: "年化波动", value: fmt.Sprintf("%.2f%%", volatility),
			color: t.Text,
			tip:   "区间内每日涨跌的样本标准差（n−1）× √250，按前复权净值计算；少于 20 个交易日不显示",
		})
	}
	if days >= 365 {
		stats = append(stats, stat{
			label: "年化收益", value: pct(annual), color: up(t, annual),
			tip: "区间涨跌按 365 天折算成一年的收益率，区间须满 365 个自然日",
		})
	}
	ui.Row(c).Gap(18).Wrap().Children(func() {
		for _, s := range stats {
			ui.Column(c).Shrink(0).Gap(1).Children(func() {
				ui.Text(c, s.label).FontSize(10.5).TextColor(t.TextMuted).NoWrap()
				ui.Text(c, s.value).FontSize(12.5).FontWeight(600).
					TextColor(s.color).FontFeatures("tnum").NoWrap().
					Tooltip(s.tip)
			})
		}
	})
}

// recoveryInfo is the chart's readout in the recovery view: the day under
// the pointer, or how far the window still sits below its high.
func (a *App) recoveryInfo(c *ui.Context, t *ui.Theme, navs []model.Nav, rec model.Recovery, label string, partial, distribution bool) {
	info, color := "", t.Text
	tip := ""
	switch {
	case a.pick >= 0 && a.pick < len(navs):
		n := navs[a.pick]
		fall := drawdownAt(navs, a.pick)
		info = fmt.Sprintf("%s · 净值 %.4f · 回撤 %.2f%%", n.Date, n.Unit, -fall)
		color = up(t, fall)
		tip = fmt.Sprintf("这一天比此前的高点低 %.2f%%", -fall)
	case len(navs) == 1:
		info = "只有 " + navs[0].Date + " 一个净值日"
		color = t.TextMuted
		tip = info
	case rec.Drawdown == 0:
		info = label + " · 区间内没有回撤"
		color = t.TextMuted
		tip = "区间内的净值没有跌破过此前的高点"
	case rec.Fall == 0:
		info = label + " · 净值处于区间最高点"
		color = up(t, 1)
		tip = fmt.Sprintf("区间最高点 %s 的净值 %.4f，当前净值就在这一点", rec.HighDate, rec.High)
	default:
		info = label + " · 距区间最高点回撤约 " + fmt.Sprintf("%.2f%%", rec.Fall)
		color = up(t, -1)
		tip = fmt.Sprintf("区间最高点 %s 的净值 %.4f，当前净值低 %.2f%%；最深一次回撤 %.2f%%，已修复 %.2f%%",
			rec.HighDate, rec.High, rec.Fall, rec.Drawdown, rec.Progress)
	}
	if distribution {
		tip += "；区间内有分红或拆分，图中的净值按前复权绘制"
	}
	if len(navs) > 1 {
		tip += "；橙线是创新高的上涨，绿线和绿带是回落的过程，红线和浅红带是从低点回升的修复过程"
	}
	if partial && len(navs) > 1 {
		tip += fmt.Sprintf("；该基金净值自 %s 起，只有 %d 个交易日，图上显示的是全部历史",
			navs[0].Date, len(navs))
	}
	ui.Text(c, info).FontSize(11.5).TextColor(color).FontFeatures("tnum").
		SingleLine().Ellipsis("…").Grow(1).Basis(0).MinWidth(0).Tooltip(tip)
}

// drawdownAt is how far one day's value sits below the highest before it,
// as a negative percent: 0 on a day that set a new high.
func drawdownAt(navs []model.Nav, i int) float64 {
	if i <= 0 || i >= len(navs) {
		return 0
	}
	adj := model.Adjusted(navs[:i+1])
	top := adj[0]
	for _, v := range adj {
		top = math.Max(top, v)
	}
	if top <= 0 {
		return 0
	}
	return (adj[i]/top - 1) * 100
}

// recoveryRow is the recovery view's own measures: the deepest fall and how
// much of it the current fall gives back, how far the window still is from
// its high, and how long the deepest fall took to get back.
func (a *App) recoveryRow(c *ui.Context, t *ui.Theme, rec model.Recovery) {
	type stat struct {
		label, value, tip string
		color             ui.Color
	}
	high := "净值新高"
	highTip := "当前净值就在区间最高点"
	if rec.Fall > 0 {
		high = "-" + fmt.Sprintf("%.2f%%", rec.Fall)
		highTip = fmt.Sprintf("区间最高点 %s 的净值 %.4f，当前净值低 %.2f%%",
			rec.HighDate, rec.High, rec.Fall)
	}
	days, daysTip := "—", "净值还没有回到最大回撤前的高点"
	if rec.Days > 0 {
		days = fmt.Sprintf("%d 个交易日", rec.Days)
		daysTip = fmt.Sprintf("从低点 %s 回到 %s 的高点，用了 %d 个交易日",
			rec.Low, rec.Recovered, rec.Days)
	}
	stats := []stat{
		{label: "最大回撤", value: fmt.Sprintf("%.2f%%", rec.Drawdown), color: t.Text,
			tip: fmt.Sprintf("从 %s 的高点回落到 %s 的低点，跌去 %.2f%%（按前复权净值计算）",
				rec.Peak, rec.Low, rec.Drawdown)},
		// The progress compares the current fall with the deepest one, as
		// the fund apps do; the days below belong to the deepest fall alone.
		{label: "修复进度", value: fmt.Sprintf("%.2f%%", rec.Progress), color: t.Text,
			tip: fmt.Sprintf("（最大回撤 − 当前回撤）÷ 最大回撤 =（%.2f%% − %.2f%%）÷ %.2f%%",
				rec.Drawdown, rec.Fall, rec.Drawdown)},
		{label: "距区间高点", value: high, color: up(t, -rec.Fall), tip: highTip},
		{label: "回撤修复天数", value: days, color: t.Text, tip: daysTip},
	}
	if rec.Drawdown == 0 {
		stats[0].tip = "区间内的净值没有跌破过此前的高点"
		stats[1].value, stats[1].tip = "—", "区间内没有回撤，也就没有要修复的部分"
	}
	ui.Row(c).Gap(18).Wrap().Children(func() {
		for _, s := range stats {
			ui.Column(c).Shrink(0).Gap(1).Children(func() {
				ui.Text(c, s.label).FontSize(10.5).TextColor(t.TextMuted).NoWrap()
				ui.Text(c, s.value).FontSize(12.5).FontWeight(600).
					TextColor(s.color).FontFeatures("tnum").NoWrap().Tooltip(s.tip)
			})
		}
	})
}

// recoveryPlot draws the window as its change from the first day, colored by
// what the line does: a climb to a new high in orange, a fall in green, and
// a climb back in red. A pale band behind the current fall marks the fall
// from the last high to its low, and the climb back since, in two tints;
// tags name the high, the deepest fall and the current state. The tags keep
// to the plot's padding, clear of the line.
func (a *App) recoveryPlot(c *ui.Context, t *ui.Theme, navs []model.Nav, rec model.Recovery, axisFrom time.Time) {
	n := len(navs)
	t0, t1 := parseDay(navs[0].Date), parseDay(navs[n-1].Date)
	if !axisFrom.IsZero() && axisFrom.Before(t0) {
		t0 = axisFrom
	}
	span := t1.Sub(t0)
	if span <= 0 {
		return
	}

	adj := model.Adjusted(navs)
	xs := make([]float32, n)
	ys := make([]float32, n)
	pcts := make([]float64, n)
	dds := make([]float64, n)
	highs := make([]int, 0, n)
	lo, hi := 0.0, 0.0
	top := math.Inf(-1)
	for i := range navs {
		pcts[i] = (adj[i]/adj[0] - 1) * 100
		lo = math.Min(lo, pcts[i])
		hi = math.Max(hi, pcts[i])
		xs[i] = float32(float64(parseDay(navs[i].Date).Sub(t0)) / float64(span))
		if adj[i] > top {
			top = adj[i]
			highs = append(highs, i)
		}
		dds[i] = (adj[i]/top - 1) * 100
	}
	if hi-lo < 0.01 {
		hi += 0.1
		lo -= 0.1
	}
	// The padding leaves room for the tags above and below the line.
	pad := (hi - lo) * 0.22
	y := chart.NewLinear(lo-pad, hi+pad).Nice(4)
	ticks := y.Ticks(4)
	for i := range navs {
		ys[i] = y.At(pcts[i])
	}
	xt := chart.NewTime(t0, t1)
	xt.Format = timeAxisFormat(t0, t1)
	label := func(side chart.Side, tick chart.Tick) {
		text := tick.Label
		if side == chart.Left || side == chart.Right {
			text = axisPercent(tick.Value)
		}
		ui.Text(c, text).NoWrap().FontSize(10.5).TextColor(t.TextMuted).
			FontFeatures("tnum")
	}

	// The three parts of the line, as the reference chart draws them: the
	// rise in orange, the fall in green, and the climb back in red.
	rise, fall, back := ui.Hex("#f4834a"), ui.Hex("#5fbd6d"), ui.Hex("#eb545b")
	if t.Dark {
		rise, fall, back = ui.Hex("#ff9b5e"), ui.Hex("#6fd07e"), ui.Hex("#ff6b6b")
	}
	// tag writes a label in a solid color with white text, as the reference
	// does. The plot keeps its padding for them, so a tag never hides the
	// line it names.
	tag := func(text string, bg ui.Color) ui.Element {
		return ui.Text(c, text).FontSize(10.5).FontWeight(600).NoWrap().
			TextColor(ui.RGB(255, 255, 255)).Padding(2.5, 8).
			Radius(5).Background(bg)
	}

	// The current fall runs from the last high to its lowest value and back;
	// its two parts get bands of their own, the fall in one tint and the climb
	// back in another.
	episodeLow := rec.HighAt
	if rec.Fall > 0 && rec.HighAt >= 0 && rec.HighAt < n {
		for i := rec.HighAt + 1; i < n; i++ {
			if adj[i] <= adj[episodeLow] {
				episodeLow = i
			}
		}
	}

	chart.Plot(c, chart.Axes{Left: ticks, Bottom: xt.Ticks(5), Label: label},
		func(area ui.Element) {
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, tick := range ticks {
					yy := chart.Y(r, tick.At)
					p.Line(r.X, yy, r.X+r.W, yy, 1, t.Border)
				}
				zero := chart.Y(r, y.At(0))
				p.Line(r.X, zero, r.X+r.W, zero, 1, t.TextMuted.Alpha(0.40))
				// The current fall keeps a pale band from the last high to its
				// lowest value, and the climb back since a band of its own, so
				// the two parts read apart.
				if rec.Fall > 0 && rec.HighAt >= 0 && rec.HighAt < n {
					x0 := chart.X(r, xs[rec.HighAt])
					xLow := chart.X(r, xs[episodeLow])
					fallBand, backBand := fall.Alpha(0.05), back.Alpha(0.06)
					if t.Dark {
						fallBand, backBand = fall.Alpha(0.07), back.Alpha(0.10)
					}
					p.Fill(ui.Rect{X: x0, Y: r.Y, W: xLow - x0, H: r.H}, fallBand, 0)
					p.Fill(ui.Rect{X: xLow, Y: r.Y, W: r.X + r.W - xLow, H: r.H}, backBand, 0)
				}
				line := func(from, to int, c ui.Color) {
					if from < 0 || to >= n || to <= from {
						return
					}
					p.StrokePath(chart.Line(r, xs[from:to+1], ys[from:to+1],
						chart.CurveLinear), 2, c)
				}
				dot := func(i int, c ui.Color, size float32) {
					x, yy := chart.X(r, xs[i]), chart.Y(r, ys[i])
					p.Fill(ui.Rect{X: x - size/2, Y: yy - size/2, W: size, H: size},
						c, size/2)
				}
				// The climb to the high is orange, the fall from it green, and
				// the climb back since red.
				line(0, rec.HighAt, rise)
				if episodeLow > rec.HighAt {
					line(rec.HighAt, episodeLow, fall)
				}
				if rec.Fall > 0 {
					line(episodeLow, n-1, back)
				}
				// A hairline drops from the deepest low to the tag under the
				// plot, as the reference chart's leader does.
				if rec.Drawdown > 0 && rec.LowAt >= 0 && rec.LowAt < n {
					x := chart.X(r, xs[rec.LowAt])
					p.Line(x, chart.Y(r, ys[rec.LowAt]), x, r.Y+r.H, 1, fall.Alpha(0.55))
				}
				// A dot marks each value that set a new high, with the highest
				// one in the color the fall starts in; the deepest low and the
				// latest value keep a dot of their own.
				for _, i := range highs {
					if i >= rec.HighAt {
						break
					}
					dot(i, rise, 4.5)
				}
				if rec.Drawdown > 0 {
					dot(rec.HighAt, fall, 6.5)
					dot(rec.LowAt, fall, 6.5)
				}
				switch {
				case rec.Fall == 0:
					dot(n-1, rise, 6.5)
				case episodeLow < n-1:
					dot(n-1, back, 6.5)
				case rec.Drawdown > 0:
					dot(n-1, fall, 6.5)
				default:
					dot(n-1, rise, 6.5)
				}
			})
			chart.Select(c, area, &a.pick, xs, func(i int) string {
				nav := navs[i]
				return fmt.Sprintf("%s 净值 %.4f 回撤 %.2f%%", nav.Date, nav.Unit, -dds[i])
			})
			if a.pick >= 0 && a.pick < n {
				pick := rise
				if rec.Fall > 0 {
					switch {
					case a.pick > episodeLow:
						pick = back
					case a.pick > rec.HighAt:
						pick = fall
					}
				}
				chart.RuleX(c, xs[a.pick], 1).Background(t.TextMuted.Alpha(0.45))
				chart.Point(c, xs[a.pick], ys[a.pick], 9, 9).
					Radius(4.5).Background(pick)
			}

			// The deepest fall keeps its number under the plot at its low, the
			// highest high is named over the plot, and the current state sits
			// at the top right: all in the padding, clear of the line.
			if rec.Drawdown > 0 && rec.LowAt >= 0 && rec.LowAt < n {
				h := ui.Center
				switch {
				case xs[rec.LowAt] < 0.10:
					h = ui.Start
				case xs[rec.LowAt] > 0.90:
					h = ui.End
				}
				chart.Anchor(c, xs[rec.LowAt], 0, h, ui.End, func() {
					tag(fmt.Sprintf("最大回撤 %.2f%%", rec.Drawdown), fall).
						Margin(0, 0, 3, 0)
				})
			}
			if rec.Fall > 0 && rec.HighAt >= 0 && rec.HighAt < n && xs[rec.HighAt] < 0.62 {
				h := ui.Center
				switch {
				case xs[rec.HighAt] < 0.08:
					h = ui.Start
				case xs[rec.HighAt] > 0.54:
					h = ui.End
				}
				chart.Anchor(c, xs[rec.HighAt], 1, h, ui.Start, func() {
					tag("净值新高", rise).Margin(3, 0, 0, 0)
				})
			}
			status, statusColor := "净值新高", rise
			if rec.Fall > 0 {
				status, statusColor = "回撤修复中", back
			}
			chart.Anchor(c, 1, 1, ui.End, ui.Start, func() {
				tag(status, statusColor).Margin(3, 0, 0, 0)
			})
		}).Height(164).Gap(6).Label("净值修复")
}

// periodControl chooses the chart's window. Seven entries do not fit across
// the card at the window's minimum, so the four common periods stay segments
// and the three longest wait in a ⋯ menu: nothing is squeezed to fit, and
// the menu keeps the chosen period's name visible on its trigger. A fund
// whose manager changed recently offers that team's own window there too.
func (a *App) periodControl(c *ui.Context, t *ui.Theme, f *Fund, managerMode bool) {
	const direct = 4
	_, _, managerOK := f.managerChange(now())
	// The segments are drawn by hand rather than with ui.Segmented, which
	// has no segment to choose when a menu period is active and would mark
	// one of the four as if it were.
	ui.Row(c).Shrink(0).AlignItems(ui.Stretch).
		Padding(t.Space(0.5)).Gap(t.Space(0.5)).
		Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Children(func() {
		for i := range direct {
			period := i
			b := ui.ButtonBase(c).
				Padding(t.Space(1), t.Space(3)).
				Radius(max(t.Radius-t.Space(0.5), 0)).Shrink(0)
			switch {
			case !managerMode && a.period == period:
				face := t.SurfacePressed
				if !t.Dark {
					face = t.Background
				}
				b.Background(face).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			case b.Hovered():
				b.Background(t.SurfaceHover)
			}
			b.Children(func() { ui.Text(c, periods[period].short).SingleLine() })
			if b.Clicked() {
				a.period = period
				a.managerOnly = false
				a.pick = -1
				a.save()
			}
		}
	})
	label := "⋯"
	switch {
	case managerMode:
		label = "经理以来"
	case a.period >= direct:
		label = periods[a.period].short
	}
	ui.PopoverMenuButton(c, label, func(m *ui.Menu) {
		for i := direct; i < len(periods); i++ {
			p := i
			item := m.Item(periods[p].label).Radio(!managerMode && a.period == p)
			if item.Chosen() {
				a.period = p
				a.managerOnly = false
				a.pick = -1
				a.save()
			}
		}
		if managerOK {
			m.Separator()
			item := m.Item("现任经理管理以来").Radio(managerMode)
			if item.Chosen() {
				a.managerOnly = true
				a.pick = -1
				a.save()
			}
		}
	}).Shrink(0).Tooltip(periodTip(managerOK))
}

// periodTip says what the ⋯ menu holds, and names the manager window when
// the fund offers it.
func periodTip(managerOK bool) string {
	if managerOK {
		return "更多周期：今年来、近3年、成立以来；本基金近3年更换过基金经理，可看现任经理管理以来"
	}
	return "更多周期：今年来、近3年、成立以来"
}

// navTable lists each trading day's published value, newest first.
func (a *App) navTable(c *ui.Context, t *ui.Theme, navs []model.Nav) {
	if len(navs) == 0 {
		ui.Column(c).Height(120).Center().Children(func() {
			ui.Text(c, "还没有历史净值").FontSize(11.5).TextColor(t.TextMuted)
		})
		return
	}
	const dateW, numW = 104, 86
	head := func() {
		ui.Row(c).Padding(0, 7, 4, 7).Children(func() {
			ui.Text(c, "日期").FontSize(11).TextColor(t.TextMuted).Width(dateW)
			ui.Text(c, "单位净值").FontSize(11).TextColor(t.TextMuted).
				Grow(1).Basis(0).TextAlign(ui.End)
			ui.Text(c, "累计净值").FontSize(11).TextColor(t.TextMuted).
				Width(numW).TextAlign(ui.End)
			ui.Text(c, "日涨跌").FontSize(11).TextColor(t.TextMuted).
				Width(numW).TextAlign(ui.End)
		})
	}
	head()
	height := float32(len(navs))*27 + 8
	if height > 300 {
		height = 300
	}
	ui.Scroll(c).Height(height).Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			for i := len(navs) - 1; i >= 0; i-- {
				n := navs[i]
				row := ui.Row(c).Padding(3, 7).Radius(6).AlignItems(ui.Center)
				if (len(navs)-1-i)%2 == 1 {
					row.Background(t.Text.Alpha(0.035))
				}
				row.Children(func() {
					ui.Text(c, n.Date).FontSize(11.5).TextColor(t.TextMuted).
						FontFeatures("tnum").NoWrap().Width(dateW)
					ui.Textf(c, "%.4f", n.Unit).FontSize(12).FontWeight(500).
						FontFeatures("tnum").NoWrap().Grow(1).Basis(0).
						TextAlign(ui.End)
					acc := "—"
					if n.Acc > 0 {
						acc = fmt.Sprintf("%.4f", n.Acc)
					}
					ui.Text(c, acc).FontSize(11.5).TextColor(t.TextMuted).
						FontFeatures("tnum").NoWrap().Width(numW).TextAlign(ui.End)
					ui.Text(c, pct(n.Pct)).FontSize(11.5).TextColor(up(t, n.Pct)).
						FontFeatures("tnum").NoWrap().Width(numW).TextAlign(ui.End)
				})
			}
		})
	})
}

// plot draws the line of a fund's net asset value, and reads out the day
// under the pointer. Purchases from the fund's ledger are marked on the line
// where they were made. The window opens at axisFrom when it lies before the
// first value, as it does for a fund younger than the period: the line
// then takes only the tail of the plot, so a longer period looks
// different rather than identical.
func (a *App) plot(c *ui.Context, t *ui.Theme, f *Fund, navs []model.Nav, axisFrom time.Time, color ui.Color) {
	n := len(navs)
	t0, t1 := parseDay(navs[0].Date), parseDay(navs[n-1].Date)
	if !axisFrom.IsZero() && axisFrom.Before(t0) {
		t0 = axisFrom
	}
	span := t1.Sub(t0)
	if span <= 0 {
		return
	}

	// The line is back-adjusted, so that a dividend or split reads as the
	// return it was rather than as a step down; the readout below still
	// names each day's published value.
	adj := model.Adjusted(navs)
	xs := make([]float32, n)
	lo, hi := adj[0], adj[0]
	for i, nav := range navs {
		xs[i] = float32(float64(parseDay(nav.Date).Sub(t0)) / float64(span))
		lo = math.Min(lo, adj[i])
		hi = math.Max(hi, adj[i])
	}
	if hi-lo < 0.0001 {
		hi += 0.005
		lo -= 0.005
	}
	pad := (hi - lo) * 0.18
	y := chart.NewLinear(lo-pad, hi+pad).Nice(4)
	ticks := y.Ticks(4)
	// The plot takes fractions of its height, as the scale gives them.
	ys := make([]float32, n)
	for i := range navs {
		ys[i] = y.At(adj[i])
	}
	xt := chart.NewTime(t0, t1)
	xt.Format = timeAxisFormat(t0, t1)
	label := func(_ chart.Side, tick chart.Tick) {
		ui.Text(c, tick.Label).NoWrap().FontSize(10.5).TextColor(t.TextMuted).
			FontFeatures("tnum")
	}
	marks := buyMarks(navs, f.Txns)
	// A buy sits on the line as a dot, with its tag above it, in the color
	// a rise uses. Tags that would collide with the one before keep their dot
	// alone; the tooltip still tells the row's story.
	buyColor := up(t, 1)

	chart.Plot(c, chart.Axes{Left: ticks, Bottom: xt.Ticks(5), Label: label},
		func(area ui.Element) {
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, tick := range ticks {
					yy := chart.Y(r, tick.At)
					p.Line(r.X, yy, r.X+r.W, yy, 1, t.Border)
				}
				path := chart.Area(r, xs, nil, ys, chart.CurveLinear)
				p.FillPathGradient(path, ui.LinearGradient{Angle: 180,
					Stops: []ui.GradientStop{
						{Color: color.Alpha(0.20), At: 0},
						{Color: color.Alpha(0.01), At: 1},
					}})
				p.StrokePath(chart.Line(r, xs, ys, chart.CurveLinear), 2, color)
			})
			// The marks go above the line but under the pointer's own dot, so
			// hovering a purchase still reads the day out.
			lastTag := float32(-1)
			for _, m := range marks {
				tip := buyTip(m)
				x, yy := xs[m.at], ys[m.at]
				chart.Point(c, x, yy, 10, 10).Radius(5).
					Background(buyColor).Border(2, t.Surface).Tooltip(tip)
				if lastTag >= 0 && x-lastTag < 0.12 {
					continue
				}
				lastTag = x
				h := ui.Center
				switch {
				case x < 0.06:
					h = ui.Start
				case x > 0.94:
					h = ui.End
				}
				chart.Anchor(c, x, yy, h, ui.End, func() {
					ui.Text(c, "买入").FontSize(10.5).FontWeight(600).NoWrap().
						TextColor(ui.RGB(255, 255, 255)).Padding(2.5, 8).
						Radius(5).Background(buyColor).Margin(0, 0, 5, 0).
						Tooltip(tip)
				})
			}
			chart.Select(c, area, &a.pick, xs, func(i int) string {
				nav := navs[i]
				return fmt.Sprintf("%s 净值 %.4f %s", nav.Date, nav.Unit, pct(nav.Pct))
			})
			if a.pick >= 0 && a.pick < n {
				chart.RuleX(c, xs[a.pick], 1).Background(t.TextMuted.Alpha(0.45))
				chart.Point(c, xs[a.pick], ys[a.pick], 9, 9).
					Radius(4.5).Background(color)
			}
		}).Height(164).Gap(6).Label("净值走势")
}
