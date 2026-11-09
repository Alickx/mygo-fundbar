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

// windowOf is the fund's net asset value over the chosen period.
func windowOf(f *Fund, period int) []model.Nav {
	if len(f.Navs) == 0 {
		return nil
	}
	if period < 0 || period >= len(periods) {
		period = 0
	}
	last := parseDay(f.Navs[len(f.Navs)-1].Date)
	from := last.AddDate(0, 0, -periods[period].days)
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

// chartCard shows the fund's net asset value over a period the user picks.
func (a *App) chartCard(c *ui.Context, t *ui.Theme, f *Fund) {
	if a.period < 0 || a.period >= len(periods) {
		a.period = 0
	}
	navs := windowOf(f, a.period)
	change := 0.0
	if len(navs) > 1 && navs[0].Unit > 0 {
		change = (navs[len(navs)-1].Unit/navs[0].Unit - 1) * 100
	}
	if a.pick >= len(navs) {
		a.pick = -1
	}

	ui.Column(c).Gap(10).Padding(13, 15).Radius(14).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "净值走势").FontSize(12.5).FontWeight(600)
			if len(navs) > 1 {
				info := periods[a.period].label + " " + pct(change)
				if a.pick >= 0 && a.pick < len(navs) {
					n := navs[a.pick]
					info = fmt.Sprintf("%s · 净值 %.4f · %s", n.Date, n.Unit, pct(n.Pct))
				}
				ui.Text(c, info).FontSize(10.5).TextColor(up(t, change)).
					FontFeatures("tnum").NoWrap()
			}
			ui.Spacer(c)
			ui.Segmented(c, &a.period, "近1月", "近3月", "近6月", "近1年").
				OnChange(func() { a.pick = -1 })
		})
		if len(navs) < 2 {
			ui.Column(c).Height(176).Center().Children(func() {
				ui.Text(c, "还没有足够的历史净值").FontSize(11.5).TextColor(t.TextMuted)
			})
			return
		}
		a.plot(c, t, navs, up(t, change))
	})
}

// plot draws the line of a fund's net asset value, and reads out the day
// under the pointer.
func (a *App) plot(c *ui.Context, t *ui.Theme, navs []model.Nav, color ui.Color) {
	n := len(navs)
	t0, t1 := parseDay(navs[0].Date), parseDay(navs[n-1].Date)
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
	label := func(_ chart.Side, tick chart.Tick) {
		ui.Text(c, tick.Label).NoWrap().FontSize(10).TextColor(t.TextMuted).
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
		}).Height(176).Gap(6).Label("净值走势")
}
