// Package model holds the fund data the app shows and the arithmetic that
// turns a fund's holdings and the day's stock prices into an estimate.
package model

import (
	"math"
	"strings"
	"time"
)

// Holding is a stock a fund reports holding, as its quarterly report gives it.
type Holding struct {
	// Symbol is the exchange's symbol of the stock, such as "sh600519".
	Symbol string
	Code   string
	Name   string
	// Weight is the stock's share of the fund's net assets, in percent.
	Weight float64
}

// Market names the exchange a holding trades on, when it is not the
// mainland's: "hk" for Hong Kong, "us" for the United States. A holding of
// the mainland's exchanges, and one whose exchange the data does not name,
// is "" and needs no mark.
func (h Holding) Market() string {
	switch {
	case strings.HasPrefix(h.Symbol, "hk"):
		return "hk"
	case strings.HasPrefix(h.Symbol, "us"):
		return "us"
	}
	return ""
}

// Nav is one day's net asset value of a fund.
type Nav struct {
	Date string
	// Unit is 单位净值, Acc 累计净值.
	Unit float64
	Acc  float64
	// Pct is the day's change, in percent.
	Pct float64
}

// distributionTolerance is how far, in percentage points, a day's unit value
// may move from its published change before the two are taken to say
// different things. A rounded unit value moves by about a hundredth of a
// point; a dividend or split moves it much further.
const distributionTolerance = 0.1

// payoutEpsilon is the smallest payout per share, in yuan, that is taken for
// one. The values come to four decimals, so two of them differ by less than
// this through rounding alone.
const payoutEpsilon = 0.0005

// payout is what a fund paid out per share between two values: its
// accumulated value keeps what its unit value gives up. It reports false
// when the feed carries no accumulated value to tell by.
func payout(prev, next Nav) (float64, bool) {
	if prev.Acc <= 0 || next.Acc <= 0 {
		return 0, false
	}
	return (next.Acc - prev.Acc) - (next.Unit - prev.Unit), true
}

// DistributionDay reports that a day's two values include a dividend or a
// split: the unit value moved while the published change says something
// else.
//
// A day the feed gives no change for needs more care. The old history holds
// such days, on some of which the fund simply fell, and those keep the unit
// values' ratio; but a fund that paid out and otherwise stood still would
// read the same. The accumulated value tells the two apart: it falls with
// the unit value on a plain fall, and stays behind on a payout.
func DistributionDay(prev, next Nav) bool {
	if prev.Unit <= 0 {
		return false
	}
	if next.Pct == 0 {
		paid, ok := payout(prev, next)
		return ok && paid > payoutEpsilon
	}
	return math.Abs((next.Unit/prev.Unit-1)*100-next.Pct) > distributionTolerance
}

// DayReturn is the day's true change in percent, from the close of prev to
// the close of next: the unit values' ratio, except on a day the fund paid a
// dividend or split, when the published change is the real one. A payout on
// a day with no published change is worked out from the payout itself.
func DayReturn(prev, next Nav) float64 {
	if prev.Unit <= 0 {
		return next.Pct
	}
	if !DistributionDay(prev, next) {
		return (next.Unit/prev.Unit - 1) * 100
	}
	if next.Pct != 0 {
		return next.Pct
	}
	paid, _ := payout(prev, next)
	return ((next.Unit+paid)/prev.Unit - 1) * 100
}

// HasDistribution reports that any day in the slice paid a dividend or split.
func HasDistribution(navs []Nav) bool {
	for i := 1; i < len(navs); i++ {
		if DistributionDay(navs[i-1], navs[i]) {
			return true
		}
	}
	return false
}

// PeriodChange is the change in percent from the close of navs[0] to the
// close of the last, compounding each day's true return. Without a dividend
// or split in the slice it is the unit values' own ratio.
func PeriodChange(navs []Nav) float64 {
	if len(navs) < 2 {
		return 0
	}
	value := 1.0
	for i := 1; i < len(navs); i++ {
		value *= 1 + DayReturn(navs[i-1], navs[i])/100
	}
	return (value - 1) * 100
}

// Adjusted is the slice's net asset value series back-adjusted (前复权) to
// the last day: each earlier value is divided by the true return of the day
// after it, so that a dividend or split does not step the line down.
func Adjusted(navs []Nav) []float64 {
	if len(navs) == 0 {
		return nil
	}
	adj := make([]float64, len(navs))
	adj[len(adj)-1] = navs[len(navs)-1].Unit
	for i := len(navs) - 2; i >= 0; i-- {
		adj[i] = adj[i+1] / (1 + DayReturn(navs[i], navs[i+1])/100)
	}
	return adj
}

// MaxDrawdown is the largest fall from a peak in the back-adjusted series,
// as a negative percent, with the dates of the peak and the low it fell to.
// A series that only rises has a drawdown of zero, at its own first day.
func MaxDrawdown(navs []Nav) (pct float64, peak, low string) {
	adj := Adjusted(navs)
	if len(adj) == 0 {
		return 0, "", ""
	}
	peakAt, lowAt := 0, 0
	top := adj[0]
	for i, v := range adj {
		if v > top {
			top, peakAt = v, i
		}
		if fall := (top - v) / top; fall > pct {
			pct, lowAt = fall, i
			peak = navs[peakAt].Date
			low = navs[lowAt].Date
		}
	}
	if peak == "" {
		peak, low = navs[0].Date, navs[0].Date
	}
	if pct == 0 {
		return 0, peak, low
	}
	return -pct * 100, peak, low
}

// AnnualizedVolatility is the sample standard deviation of the window's
// daily true returns, times √250, in percent, with the number of returns it
// was computed from. A window of fewer than two values has neither.
func AnnualizedVolatility(navs []Nav) (pct float64, returns int) {
	if len(navs) < 2 {
		return 0, 0
	}
	returns = len(navs) - 1
	daily := make([]float64, returns)
	var mean float64
	for i := 1; i < len(navs); i++ {
		daily[i-1] = DayReturn(navs[i-1], navs[i])
		mean += daily[i-1]
	}
	mean /= float64(returns)
	if returns < 2 {
		return 0, returns
	}
	var sum float64
	for _, r := range daily {
		sum += (r - mean) * (r - mean)
	}
	sd := math.Sqrt(sum / float64(returns-1))
	return sd * math.Sqrt(250), returns
}

// AnnualizedReturn is the window's compounded change as a yearly rate, in
// percent, with the calendar days the window spans. It is only meaningful
// for a window of a year or more; a shorter one still returns its figure,
// so that the caller can decide whether to show it.
func AnnualizedReturn(navs []Nav) (pct float64, days int) {
	if len(navs) < 2 {
		return 0, 0
	}
	start, end := parseDate(navs[0].Date), parseDate(navs[len(navs)-1].Date)
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return 0, 0
	}
	days = int(end.Sub(start).Hours() / 24)
	total := 1 + PeriodChange(navs)/100
	if total <= 0 {
		return 0, days
	}
	return (math.Pow(total, 365/float64(days)) - 1) * 100, days
}

// parseDate reads a net asset value's date. It keeps model free of the
// app's trading calendar; a date that cannot be read has no span to measure.
func parseDate(date string) time.Time {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Industry is a share of a fund's net assets in one industry, as its
// quarterly report gives it.
type Industry struct {
	Name string
	// Weight is the industry's share of the fund's net assets, in percent.
	Weight float64
}

// StockProfile says what a stock is: the industry it belongs to, and the
// themes it is traded with.
type StockProfile struct {
	Industry string
	Concepts []string
}

// Quote is a stock's price now.
type Quote struct {
	Symbol string
	Price  float64
	// Pct is the change since the previous close, in percent.
	Pct float64
	// At is when the exchange last priced it.
	At time.Time
}

// Estimate is a fund's estimated change today, made of the fund's holdings
// and their prices.
type Estimate struct {
	// Pct is the estimated change of the fund's net asset value, in percent.
	Pct float64
	// Cover is the share of the fund's net assets the holdings account for,
	// in percent: how much of the fund the estimate is made of.
	Cover float64
	// Stocks is how many holdings had a price.
	Stocks int
}

// EstimateFund estimates the change of a fund's disclosed holdings today from
// their quotes, keyed by symbol. Only holdings quoted today count: the sum of
// each holding's share times its change, over the share that is covered. The
// result is the holdings' own change; ScaleEstimate brings it down to the
// fund.
func EstimateFund(holdings []Holding, quotes map[string]Quote) (Estimate, bool) {
	var weighted, cover float64
	var n int
	for _, h := range holdings {
		q, ok := quotes[h.Symbol]
		if !ok || h.Weight <= 0 {
			continue
		}
		weighted += h.Weight * q.Pct
		cover += h.Weight
		n++
	}
	if cover <= 0 || n == 0 {
		return Estimate{}, false
	}
	return Estimate{Pct: weighted / cover, Cover: cover, Stocks: n}, true
}

// ScaleEstimate brings a holdings' estimate down to the fund: the disclosed
// holdings are only the stock part of it, and the bonds and cash beside them
// do not move with the stocks. The estimate takes the stock position's share
// of the holdings' change, which is the same assumption EstimateFund makes
// of the holdings it could not quote, applied to the whole fund.
//
// A stock position outside (0, 100] says nothing usable, and leaves the
// estimate as the holdings gave it: the covered holdings are then all the app
// knows of the fund.
func ScaleEstimate(e Estimate, stockPosition float64) Estimate {
	if stockPosition <= 0 || stockPosition > 100 {
		return e
	}
	e.Pct *= stockPosition / 100
	return e
}
