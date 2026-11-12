package app

import (
	"fmt"
	"math"
	"time"

	"fundbar/internal/model"
	"fundbar/internal/store"
)

// estimateFinalAt is when a day's estimate is taken as final, in minutes
// after midnight: the exchanges have closed by then (Hong Kong's last, at
// 16:10), so the quotes of a refresh at this hour are the day's last. A
// refresh on a later date is as final, because its quotes are still the
// quote day's closing prices.
const estimateFinalAt = 16*60 + 15

// estimateRecord is the record a refresh makes for a fund at at. It reads no
// clock, so every boundary can be tried with its own moment. A provisional
// estimate is not recorded: what the app knew after the publication is the
// only thing that can be compared with it.
func estimateRecord(f *Fund, at time.Time) (store.EstimateRecord, bool) {
	if !f.HasEst || f.QuoteAt.IsZero() {
		return store.EstimateRecord{}, false
	}
	quote := f.QuoteAt.In(cst)
	day := quote.Format("2006-01-02")
	at = at.In(cst)
	after := at.Format("2006-01-02") > day
	sameEvening := at.Format("2006-01-02") == day &&
		at.Hour()*60+at.Minute() >= estimateFinalAt
	if !after && !sameEvening {
		return store.EstimateRecord{}, false
	}
	return store.EstimateRecord{Date: day, Est: f.Est.Pct, Cover: f.Est.Cover}, true
}

// recordEstimates keeps the day's final estimates and drops the funds the
// user no longer follows. It reports whether anything changed.
func (a *App) recordEstimates(at time.Time) bool {
	keep := make(map[string]bool, len(a.all))
	for _, f := range a.all {
		keep[f.Code] = true
	}
	changed := a.estimates.Prune(keep)
	for _, f := range a.all {
		if r, ok := estimateRecord(f, at); ok && a.estimates.AddRecord(f.Code, r) {
			changed = true
		}
	}
	return changed
}

// pruneEstimates forgets the funds the user no longer follows, so that
// removing the last fund does not leave their records behind for good.
func (a *App) pruneEstimates() {
	keep := make(map[string]bool, len(a.all))
	for _, f := range a.all {
		keep[f.Code] = true
	}
	if !a.estimates.Prune(keep) {
		return
	}
	if err := store.SaveEstimates(a.dir, a.estimates); err != nil && a.failure == "" {
		a.failure = "保存估值记录失败：" + err.Error()
	}
}

// fillActuals matches the records still waiting with the fund's published
// values: a record's day that now has a net asset value takes that day's
// true change, so a distribution compares with the published one rather
// than the unit values' ratio. It reports whether anything changed.
func (a *App) fillActuals(f *Fund) bool {
	recs := a.estimates[f.Code]
	if len(recs) == 0 || len(f.Navs) < 2 {
		return false
	}
	at := make(map[string]int, len(f.Navs))
	for i, n := range f.Navs {
		at[n.Date] = i
	}
	changed := false
	for i := range recs {
		if recs[i].Actual != nil {
			continue
		}
		j, ok := at[recs[i].Date]
		if !ok || j == 0 {
			// A value can lag its date, as a QDII's does: the record stays
			// open and is counted if the value ever arrives.
			continue
		}
		actual := model.DayReturn(f.Navs[j-1], f.Navs[j])
		recs[i].Actual = &actual
		changed = true
	}
	return changed
}

// estimateAccuracy says how far the app's estimates have been from the
// values the funds published. It is a value of its own so that the window
// and its tests can ask without the app around them.
type estimateAccuracy struct {
	// Mean is the average absolute difference, in percentage points, over
	// the recent finished records.
	Mean float64
	// Days is how many finished records the mean covers, up to 20.
	Days int
	// Aligned counts those whose direction agreed, among records whose
	// published move was larger than a rounding wobble.
	Aligned int
	// Finished is how many records have a published value at all.
	Finished int
}

// estimateAccuracyWindow is how many of the newest finished records the
// figure looks at: recent enough to describe the estimate as it is now.
const estimateAccuracyWindow = 20

// summarizeEstimateAccuracy reads the finished records. Direction agreement
// only counts a day whose published move was plainly a move, so that a fund
// that barely budged cannot flatter the estimate.
func summarizeEstimateAccuracy(recs []store.EstimateRecord) estimateAccuracy {
	finished := make([]store.EstimateRecord, 0, len(recs))
	for _, r := range recs {
		if r.Actual != nil {
			finished = append(finished, r)
		}
	}
	out := estimateAccuracy{Finished: len(finished)}
	if len(finished) > estimateAccuracyWindow {
		finished = finished[len(finished)-estimateAccuracyWindow:]
	}
	out.Days = len(finished)
	var sum float64
	for _, r := range finished {
		actual := *r.Actual
		sum += math.Abs(r.Est - actual)
		if actual > 0.01 || actual < -0.01 {
			if (r.Est > 0 && actual > 0) || (r.Est < 0 && actual < 0) {
				out.Aligned++
			}
		}
	}
	if out.Days > 0 {
		out.Mean = sum / float64(out.Days)
	}
	return out
}

// tip is the longer story the figure tells: what it measured, over how many
// days, and how often the two moved the same way.
func (acc estimateAccuracy) tip() string {
	return fmt.Sprintf("最近 %d 个交易日，盘中估值与当晚公布净值平均差 %.2f 个百分点，方向一致 %d 天。偏差越小，盘中估值越可信。",
		acc.Days, acc.Mean, acc.Aligned)
}
