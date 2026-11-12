package app

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"fundbar/internal/model"
)

// maxCompare is how many funds may be drawn beside the chosen one: four
// lines, with the chosen fund, are as many as the legend and the palette
// can tell apart at the smallest window.
const maxCompare = 3

// compareLine is one fund over the chart's window: the dates it has values
// for and its cumulative change in percent from the first of them.
type compareLine struct {
	Code  string
	Name  string
	Dates []time.Time
	Pcts  []float64
}

// compareSeries gives each fund's cumulative change over the window that
// start and end bound. Every fund is normalised at its own first value on
// or after start, so a younger fund's line starts later, at 0%, and the
// change is worked out on back-adjusted values so a dividend does not dent
// a line. A fund with no value in the window is left out.
func compareSeries(start, end time.Time, funds []*Fund) []compareLine {
	out := make([]compareLine, 0, len(funds))
	for _, f := range funds {
		if f == nil {
			continue
		}
		var navs []model.Nav
		for _, n := range f.Navs {
			at := parseDay(n.Date)
			if at.IsZero() || at.Before(start) {
				continue
			}
			if at.After(end) {
				break
			}
			navs = append(navs, n)
		}
		if len(navs) == 0 {
			continue
		}
		adj := model.Adjusted(navs)
		line := compareLine{Code: f.Code, Name: fundName(f),
			Dates: make([]time.Time, len(navs)), Pcts: make([]float64, len(navs))}
		for i, n := range navs {
			line.Dates[i] = parseDay(n.Date)
			line.Pcts[i] = (adj[i]/adj[0] - 1) * 100
		}
		out = append(out, line)
	}
	return out
}

// at is the line's change at a moment, from its last value at or before
// it, and that value's place. A line that starts after the moment has
// nothing to say there.
func (l compareLine) at(at time.Time) (pct float64, index int, ok bool) {
	i := sort.Search(len(l.Dates), func(i int) bool { return l.Dates[i].After(at) }) - 1
	if i < 0 {
		return 0, 0, false
	}
	return l.Pcts[i], i, true
}

// compareCandidates lists the other funds the compare menu offers: the
// holding group's first, then the rest, each by name.
func (a *App) compareCandidates(chosen string) []*Fund {
	var held, rest []*Fund
	for _, f := range a.all {
		if f.Code == chosen {
			continue
		}
		if f.In(holdingGroup) {
			held = append(held, f)
		} else {
			rest = append(rest, f)
		}
	}
	byName := func(funds []*Fund) {
		sort.Slice(funds, func(i, j int) bool {
			ni, nj := fundName(funds[i]), fundName(funds[j])
			if ni != nj {
				return ni < nj
			}
			return funds[i].Code < funds[j].Code
		})
	}
	byName(held)
	byName(rest)
	return append(held, rest...)
}

// inCompare reports that a fund is drawn beside the chosen one.
func (a *App) inCompare(code string) bool {
	for _, c := range a.compare {
		if c == code {
			return true
		}
	}
	return false
}

// toggleCompare adds a fund to the comparison, or takes it out. At most
// maxCompare funds are compared, and the chosen fund is never one of them.
func (a *App) toggleCompare(code string) {
	if code == "" {
		return
	}
	if f := a.Selected(); f != nil && f.Code == code {
		return
	}
	for i, c := range a.compare {
		if c == code {
			a.compare = append(a.compare[:i], a.compare[i+1:]...)
			return
		}
	}
	if len(a.compare) >= maxCompare {
		return
	}
	a.compare = append(a.compare, code)
}

// clearCompare draws the chosen fund on its own again.
func (a *App) clearCompare() { a.compare = nil }

// pruneCompare takes the chosen fund and the funds no longer followed out
// of the comparison. The chart calls it every frame, so a fund chosen in
// the list or removed from it leaves the comparison without a separate
// click.
func (a *App) pruneCompare(chosen string) {
	if len(a.compare) == 0 {
		return
	}
	kept := a.compare[:0]
	for _, code := range a.compare {
		if code == chosen || a.byCode(code) == nil {
			continue
		}
		kept = append(kept, code)
	}
	a.compare = kept
}

// compareColor is a line's color: the chosen fund takes the accent, and
// the compared funds three hues that read on both themes and stay clear of
// the red and green that mean a gain and a loss.
func compareColor(t *ui.Theme, i int) ui.Color {
	if i == 0 {
		return t.Accent
	}
	if t.Dark {
		switch i {
		case 1:
			return ui.Hex("#e5b454")
		case 2:
			return ui.Hex("#b39dff")
		default:
			return ui.Hex("#5fd3c4")
		}
	}
	switch i {
	case 1:
		return ui.Hex("#b7791f")
	case 2:
		return ui.Hex("#7c5cf0")
	default:
		return ui.Hex("#0e8f83")
	}
}

// compareButton is the small menu at the right end of the chart's info
// line. Its items are the other followed funds, checked while they are
// drawn, and it says how many are when any is.
func (a *App) compareButton(c *ui.Context, t *ui.Theme) {
	label := "对比"
	if n := len(a.compare); n > 0 {
		label = fmt.Sprintf("对比 %d", n)
	}
	ui.PopoverMenuButton(c, label, func(m *ui.Menu) {
		m.MaxWidth(300)
		chosen := ""
		if f := a.Selected(); f != nil {
			chosen = f.Code
		}
		for _, f := range a.compareCandidates(chosen) {
			code, name := f.Code, fundName(f)
			on := a.inCompare(code)
			it := m.Item(name).Checked(on).Tooltip(f.Code)
			if !on && len(a.compare) >= maxCompare {
				it.Disabled(true).Tooltip("最多同时对比 3 只")
			}
			if it.Chosen() {
				a.toggleCompare(code)
			}
		}
		if len(a.compare) > 0 {
			m.Separator()
			if m.Item("清除对比").Chosen() {
				a.clearCompare()
			}
		}
	}).Shrink(0).Tooltip("把关注的基金与本基金画在一起，比较区间涨跌")
}

// compareLegend names each line over the window: a dot in the line's own
// color, the fund, and its change in the color of gains and losses.
func (a *App) compareLegend(c *ui.Context, t *ui.Theme, lines []compareLine) {
	ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(10).AlignItems(ui.Center).
		Children(func() {
			for i, line := range lines {
				i, line := i, line
				p := 0.0
				if len(line.Pcts) > 0 {
					p = line.Pcts[len(line.Pcts)-1]
				}
				ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(5).
					AlignItems(ui.Center).Tooltip(line.Name + " · " + line.Code).
					Children(func() {
						ui.Box(c).Size(7, 7).Shrink(0).Radius(3.5).
							Background(compareColor(t, i))
						ui.Text(c, line.Name).FontSize(11.5).TextColor(t.Text).
							SingleLine().Ellipsis("…").Grow(1).Basis(0).MinWidth(0)
						ui.Text(c, pct(p)).FontSize(11.5).TextColor(up(t, p)).
							FontFeatures("tnum").NoWrap().Shrink(0)
					})
			}
		})
}

// comparePlot draws the cumulative change of each line from the window's
// start, on one percent axis with a zero baseline. Lines only: an area
// under four of them would hide the ones above. axisFrom opens the plot
// earlier than the chosen fund's history when the fund is younger than the
// period, as the single-fund plot does.
func (a *App) comparePlot(c *ui.Context, t *ui.Theme, lines []compareLine, navs []model.Nav, axisFrom time.Time) {
	if len(lines) == 0 || len(lines[0].Dates) < 2 {
		return
	}
	t0 := lines[0].Dates[0]
	t1 := lines[0].Dates[len(lines[0].Dates)-1]
	if !axisFrom.IsZero() && axisFrom.Before(t0) {
		t0 = axisFrom
	}
	span := t1.Sub(t0)
	if span <= 0 {
		return
	}

	xs := make([][]float32, len(lines))
	ys := make([][]float32, len(lines))
	lo, hi := 0.0, 0.0
	for i, line := range lines {
		xs[i] = make([]float32, len(line.Dates))
		ys[i] = make([]float32, len(line.Pcts))
		for j, at := range line.Dates {
			xs[i][j] = float32(float64(at.Sub(t0)) / float64(span))
			lo = math.Min(lo, line.Pcts[j])
			hi = math.Max(hi, line.Pcts[j])
		}
	}
	if hi-lo < 0.01 {
		hi += 0.1
		lo -= 0.1
	}
	pad := (hi - lo) * 0.12
	y := chart.NewLinear(lo-pad, hi+pad).Nice(4)
	ticks := y.Ticks(4)
	for i := range lines {
		for j := range lines[i].Pcts {
			ys[i][j] = y.At(lines[i].Pcts[j])
		}
	}
	xt := chart.NewTime(t0, t1)
	xt.Format = timeAxisFormat(t0, t1)
	label := func(side chart.Side, tick chart.Tick) {
		text := tick.Label
		// The left axis is the percent scale; the bottom is time, which
		// chart.Time labels itself.
		if side == chart.Left || side == chart.Right {
			text = axisPercent(tick.Value)
		}
		ui.Text(c, text).NoWrap().FontSize(10.5).
			TextColor(t.TextMuted).FontFeatures("tnum")
	}

	chart.Plot(c, chart.Axes{Left: ticks, Bottom: xt.Ticks(5), Label: label},
		func(area ui.Element) {
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, tick := range ticks {
					yy := chart.Y(r, tick.At)
					p.Line(r.X, yy, r.X+r.W, yy, 1, t.Border)
				}
				// Zero is the line every fund is measured from, and it keeps
				// a weight of its own among the gridlines.
				zero := chart.Y(r, y.At(0))
				p.Line(r.X, zero, r.X+r.W, zero, 1, t.TextMuted.Alpha(0.45))
				for i, line := range lines {
					if len(line.Pcts) < 2 {
						continue
					}
					p.StrokePath(chart.Line(r, xs[i], ys[i], chart.CurveLinear),
						2, compareColor(t, i))
				}
			})
			chosen := lines[0]
			chart.Select(c, area, &a.pick, xs[0], func(i int) string {
				at := chosen.Dates[i]
				parts := []string{at.Format("2006-01-02")}
				for _, line := range lines {
					if v, _, ok := line.at(at); ok {
						parts = append(parts, truncate(line.Name, 12)+" "+pct(v))
					}
				}
				return strings.Join(parts, " · ")
			})
			if a.pick < 0 || a.pick >= len(chosen.Dates) {
				return
			}
			at := chosen.Dates[a.pick]
			chart.RuleX(c, xs[0][a.pick], 1).Background(t.TextMuted.Alpha(0.45))
			for i, line := range lines {
				v, j, ok := line.at(at)
				if !ok {
					continue
				}
				x := float32(float64(line.Dates[j].Sub(t0)) / float64(span))
				chart.Point(c, x, y.At(v), 9, 9).Radius(4.5).
					Background(compareColor(t, i))
			}
		}).Height(164).Gap(6).Label("净值走势")
}

// axisPercent writes a y-axis tick of the compare plot.
func axisPercent(v float64) string {
	if math.Abs(v) < 0.005 {
		return "0%"
	}
	return fmt.Sprintf("%+.1f%%", v)
}

// timeAxisFormat labels the plot's days: a window longer than a year names
// the year on its months, or two springs would read alike.
func timeAxisFormat(from, to time.Time) func(time.Time) string {
	long := to.Sub(from) > 400*24*time.Hour
	return func(at time.Time) string {
		switch {
		case long && at.Month() == 1 && at.Day() == 1:
			return fmt.Sprintf("%d年", at.Year())
		case long && at.Day() == 1:
			return fmt.Sprintf("%d年%d月", at.Year(), int(at.Month()))
		case long:
			return fmt.Sprintf("%d年%d月%d日", at.Year(), int(at.Month()), at.Day())
		case at.Day() == 1:
			return fmt.Sprintf("%d月", int(at.Month()))
		default:
			return fmt.Sprintf("%d月%d日", int(at.Month()), at.Day())
		}
	}
}
