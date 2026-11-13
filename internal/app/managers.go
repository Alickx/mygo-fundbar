package app

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// managerChangeYears is how recent a manager change has to be for the chart
// to offer the current team's own window.
const managerChangeYears = 3

// managerRefresh is how long a fund's management history is kept before the
// page is asked again: manager changes are rare, and the page is one more
// request in every refresh.
const managerRefresh = 24 * time.Hour

// currentManagement reads the current team out of the management terms: the
// day the team took over, its members, and whether an earlier team managed
// the fund before it. A fund whose only term is its first has not changed
// managers, whatever the term's date.
func (f *Fund) currentManagement() (since time.Time, names []string, changed bool) {
	if len(f.ManagerTerms) == 0 {
		return time.Time{}, nil, false
	}
	current := 0
	for _, term := range f.ManagerTerms {
		if term.End != "至今" {
			continue
		}
		current++
		if at := parseDay(term.Start); !at.IsZero() && at.After(since) {
			since = at
		}
		for _, name := range term.Names {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	if current == 0 || since.IsZero() {
		return time.Time{}, nil, false
	}
	return since, names, current < len(f.ManagerTerms)
}

// managerChange is the current team's first day when it took over within the
// last few years, with the team's members. It reports false for a fund whose
// managers have not changed, or whose last change is older than that.
func (f *Fund) managerChange(at time.Time) (since time.Time, names []string, ok bool) {
	since, names, changed := f.currentManagement()
	// The change is a date, so the three years are counted in days: a team
	// that took over this morning three years ago is still within them.
	at = at.In(cst)
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, cst)
	if !changed || since.After(day) || since.Before(day.AddDate(-managerChangeYears, 0, 0)) {
		return time.Time{}, nil, false
	}
	return since, names, true
}

// managerTip says who manages the fund now and since when, for the chart's
// readout.
func managerTip(names []string, since time.Time) string {
	day := since.Format("2006-01-02")
	if len(names) == 0 {
		return "现任基金经理自 " + day + " 起管理本基金"
	}
	return fmt.Sprintf("现任基金经理 %s 自 %s 起管理本基金",
		strings.Join(names, "、"), day)
}
