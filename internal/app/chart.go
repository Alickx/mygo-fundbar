package app

import (
	"fmt"
	"math"
	"time"

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
	from := periodStart(f, period)
	var out []model.Nav
	for _, n := range f.Navs {
		if !parseDay(n.Date).Before(from) {
			out = append(out, n)
		}
	}
	return out
}

func parseDay(date string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", date, cst)
	if err != nil {
		return time.Time{}
	}
	return t
}

// chartCard shows the fund's net asset value over a period the user picks,
// as a line or as the value of each day.
func (a *App) chartCard(c *ui.Context, t *ui.Theme, f *Fund) {
	if a.period < 0 || a.period >= len(periods) {
		a.period = 0
	}
	if a.navView < 0 || a.navView > 1 {
		a.navView = 0
	}
	navs := windowOf(f, a.period)
	// A young fund has less history than the period asks for, so the longer
	// periods show the same days. Cast the span as the fund's whole history,
	// and keep the wider window in the plot, so it reads as a new fund rather
	// than a chart that did not change.
	from := periodStart(f, a.period)
	partial := len(f.Navs) > 0 && parseDay(f.Navs[0].Date).After(from)
	// The plot opens at the window's start when the fund is younger than it,
	// so the line takes only the tail of the plot.
	axisFrom := time.Time{}
	if partial {
		axisFrom = from
	}
	label := periods[a.period].label
	if partial {
		label = "成立以来"
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

	card(c, t).Gap(8).Children(func() {
		// The readout has its own line: selecting a date never pushes the
		// period controls out of the card or changes the plot's position.
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			title := "净值走势"
			if a.navView == 1 {
				title = "历史净值"
			}
			ui.Text(c, title).FontSize(13).FontWeight(600).NoWrap().Shrink(0)
			ui.Spacer(c)
			ui.Segmented(c, &a.navView, "走势", "列表").Shrink(0).
				OnChange(func() {
					a.pick = -1
					a.save()
				})
			a.periodControl(c, t)
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
			info := label + "净值变化 " + pct(change)
			color := up(t, change)
			if partial && len(navs) > 1 {
				info += " · 净值自 " + navs[0].Date + " 起"
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
			if a.navView == 1 && distribution {
				// The rows below are published unit values, whose first and
				// last do not give this figure.
				tip = "区间内有分红或拆分，区间涨跌按当日公布涨跌幅计算，与首尾单位净值之比不同"
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
		if comparing {
			a.comparePlot(c, t, lines, navs, axisFrom)
		} else {
			a.plot(c, t, navs, axisFrom, up(t, change))
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

// periodControl chooses the chart's window. Seven entries do not fit across
// the card at the window's minimum, so the four common periods stay segments
// and the three longest wait in a ⋯ menu: nothing is squeezed to fit, and
// the menu keeps the chosen period's name visible on its trigger.
func (a *App) periodControl(c *ui.Context, t *ui.Theme) {
	const direct = 4
	// The segments are drawn by hand rather than with ui.Segmented, which
	// has no segment to choose when a menu period is active and would mark
	// one of the four as if it were.
	ui.Row(c).Shrink(0).AlignItems(ui.Stretch).
		Padding(t.Space(0.5)).Gap(t.Space(0.5)).
		Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Children(func() {
		for i := 0; i < direct; i++ {
			period := i
			b := ui.ButtonBase(c).
				Padding(t.Space(1), t.Space(3)).
				Radius(max(t.Radius-t.Space(0.5), 0)).Shrink(0)
			switch {
			case a.period == period:
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
				a.pick = -1
				a.save()
			}
		}
	})
	label := "⋯"
	if a.period >= direct {
		label = periods[a.period].short
	}
	ui.PopoverMenuButton(c, label, func(m *ui.Menu) {
		for i := direct; i < len(periods); i++ {
			p := i
			item := m.Item(periods[p].label).Radio(a.period == p)
			if item.Chosen() {
				a.period = p
				a.pick = -1
				a.save()
			}
		}
	}).Shrink(0).Tooltip("更多周期：今年来、近3年、成立以来")
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
// under the pointer. The window opens at axisFrom when it lies before the
// first value, as it does for a fund younger than the period: the line
// then takes only the tail of the plot, so a longer period looks
// different rather than identical.
func (a *App) plot(c *ui.Context, t *ui.Theme, navs []model.Nav, axisFrom time.Time, color ui.Color) {
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
