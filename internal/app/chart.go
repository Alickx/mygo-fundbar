package app

import (
	"fmt"
	"math"
	"time"

	"fundbar/internal/model"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// periods are the stretches of history the chart shows.
var periods = []struct {
	label string
	days  int
}{
	{"近1月", 30},
	{"近3月", 92},
	{"近6月", 183},
	{"近1年", 365},
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
	change := 0.0
	if len(navs) > 1 && navs[0].Unit > 0 {
		change = (navs[len(navs)-1].Unit/navs[0].Unit - 1) * 100
	}
	if a.pick >= len(navs) {
		a.pick = -1
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
			ui.Segmented(c, &a.period, "近1月", "近3月", "近6月", "近1年").Shrink(0).
				OnChange(func() { a.pick = -1 })
		})
		ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() {
			if len(navs) == 0 {
				return
			}
			info := label + "净值变化 " + pct(change)
			color := up(t, change)
			if partial && len(navs) > 1 {
				info += " · 净值自 " + navs[0].Date + " 起"
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
			// longer than the fund's history shows what it shows.
			tip := info
			if partial && len(navs) > 1 && a.navView == 0 && a.pick < 0 {
				tip = fmt.Sprintf("该基金净值自 %s 起，只有 %d 个交易日，填不满%s，图上显示的是全部历史",
					navs[0].Date, len(navs), periods[a.period].label)
			}
			ui.Text(c, info).FontSize(11.5).
				TextColor(color).FontFeatures("tnum").SingleLine().Ellipsis("…").Tooltip(tip)
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
		a.plot(c, t, navs, axisFrom, up(t, change))
	})
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

	xs := make([]float32, n)
	lo, hi := navs[0].Unit, navs[0].Unit
	for i, nav := range navs {
		xs[i] = float32(float64(parseDay(nav.Date).Sub(t0)) / float64(span))
		lo = math.Min(lo, nav.Unit)
		hi = math.Max(hi, nav.Unit)
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
	for i, nav := range navs {
		ys[i] = y.At(nav.Unit)
	}
	xt := chart.NewTime(t0, t1)
	// The months read as the rest of the window does.
	xt.Format = func(at time.Time) string {
		if at.Day() == 1 {
			return fmt.Sprintf("%d月", int(at.Month()))
		}
		return fmt.Sprintf("%d月%d日", int(at.Month()), at.Day())
	}
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
