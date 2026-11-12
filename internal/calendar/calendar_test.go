package calendar

import (
	"testing"
	"time"
)

func day(date string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", date, CST)
	if err != nil {
		panic(err)
	}
	return t
}

func TestClosuresAreNotTradingDays(t *testing.T) {
	closures := []string{
		"2026-01-01", "2026-01-02",
		"2026-02-16", "2026-02-17", "2026-02-18", "2026-02-19", "2026-02-20",
		"2026-02-23", "2026-04-06", "2026-05-01", "2026-05-04", "2026-05-05",
		"2026-06-19", "2026-09-25", "2026-10-01", "2026-10-02", "2026-10-05",
		"2026-10-06", "2026-10-07",
		"2027-01-01",
		"2027-02-05", "2027-02-08", "2027-02-09", "2027-02-10", "2027-02-11",
		"2027-02-12", "2027-04-05", "2027-05-03", "2027-05-04", "2027-05-05",
		"2027-06-09", "2027-09-15", "2027-10-01", "2027-10-04", "2027-10-05",
		"2027-10-06", "2027-10-07",
	}
	for _, date := range closures {
		if IsTradingDay(day(date)) {
			t.Errorf("%s is a closure, want no trading", date)
		}
	}
}

func TestNeighbouringDaysTrade(t *testing.T) {
	// The first trading day before and after each stretch of closures, and a
	// few ordinary days.
	opens := []string{
		"2026-01-05", "2026-02-13", "2026-02-24", "2026-04-07", "2026-04-30",
		"2026-05-06", "2026-06-22", "2026-09-24", "2026-09-28", "2026-09-30",
		"2026-10-08", "2026-10-09", "2026-11-11",
	}
	for _, date := range opens {
		if !IsTradingDay(day(date)) {
			t.Errorf("%s is a trading day, want trading", date)
		}
	}
}

func TestWeekendsAreClosed(t *testing.T) {
	for _, date := range []string{"2026-11-14", "2026-11-15", "2027-05-08"} {
		if IsTradingDay(day(date)) {
			t.Errorf("%s is a weekend, want no trading", date)
		}
	}
}

func TestUncoveredYearTradesWeekdays(t *testing.T) {
	if Covers(2028) {
		t.Fatal("the table does not name 2028")
	}
	if !IsTradingDay(day("2028-01-03")) {
		t.Fatal("an uncovered year should trade on a weekday")
	}
	if IsTradingDay(day("2028-01-08")) {
		t.Fatal("an uncovered year should not trade on a weekend")
	}
	if !Covers(2026) || !Covers(2027) {
		t.Fatal("expected 2026 and 2027 to be covered")
	}
}

func TestPrevTradingDay(t *testing.T) {
	cases := map[string]string{
		// The first day after the National Day holiday.
		"2026-10-08": "2026-09-30",
		// A holiday itself, after the evening.
		"2026-10-05": "2026-09-30",
		// An ordinary Monday, back past the weekend.
		"2026-11-09": "2026-11-06",
		// Back past the Spring Festival and its weekend.
		"2026-02-24": "2026-02-13",
		// Back over the New Year closures and the weekend into the year
		// before, which the table does not cover.
		"2026-01-05": "2025-12-31",
	}
	for from, want := range cases {
		if got := PrevTradingDay(day(from)).Format("2006-01-02"); got != want {
			t.Errorf("PrevTradingDay(%s) = %s, want %s", from, got, want)
		}
	}
}
