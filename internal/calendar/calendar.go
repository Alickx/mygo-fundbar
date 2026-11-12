// Package calendar tells the days the A-share market trades: weekdays that
// are not public holidays. It holds no network and no clock of its own.
package calendar

import "time"

// CST is the time zone the A-share market keeps, UTC+8.
var CST = time.FixedZone("CST", 8*60*60)

// closures names the weekdays the market is closed, by year. A weekend is
// closed whatever the table says, so the weekend make-up workdays (调休) of
// a holiday are not listed: they trade no shares even though they are
// working days. A year the table does not name trades every weekday.
//
// 2026's closures are the weekdays on which three long-history funds
// published no net asset value, cross-checked against the State Council's
// holiday notice.
//
// 2027's closures come from 国务院办公厅《关于2027年部分节假日安排的通知》
// (国办发明电〔2026〕10号, 2026-11-05),
// https://www.gov.cn/zhengce/content/202611/content_7085101.htm. Only the
// holiday days that fall on a weekday are listed.
var closures = map[int]map[string]bool{
	2026: {
		"2026-01-01": true, // 元旦
		"2026-01-02": true,
		"2026-02-16": true, // 春节
		"2026-02-17": true,
		"2026-02-18": true,
		"2026-02-19": true,
		"2026-02-20": true,
		"2026-02-23": true,
		"2026-04-06": true, // 清明节
		"2026-05-01": true, // 劳动节
		"2026-05-04": true,
		"2026-05-05": true,
		"2026-06-19": true, // 端午节
		"2026-09-25": true, // 中秋节
		"2026-10-01": true, // 国庆节
		"2026-10-02": true,
		"2026-10-05": true,
		"2026-10-06": true,
		"2026-10-07": true,
	},
	2027: {
		"2027-01-01": true, // 元旦
		"2027-02-05": true, // 春节
		"2027-02-08": true,
		"2027-02-09": true,
		"2027-02-10": true,
		"2027-02-11": true,
		"2027-02-12": true,
		"2027-04-05": true, // 清明节
		"2027-05-03": true, // 劳动节
		"2027-05-04": true,
		"2027-05-05": true,
		"2027-06-09": true, // 端午节
		"2027-09-15": true, // 中秋节
		"2027-10-01": true, // 国庆节
		"2027-10-04": true,
		"2027-10-05": true,
		"2027-10-06": true,
		"2027-10-07": true,
	},
}

// Covers reports whether the table names the year's closures.
func Covers(year int) bool {
	_, ok := closures[year]
	return ok
}

// IsTradingDay reports whether the market trades on t's date.
func IsTradingDay(t time.Time) bool {
	t = t.In(CST)
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	return !closures[t.Year()][t.Format("2006-01-02")]
}

// PrevTradingDay is the trading day before t's date.
func PrevTradingDay(t time.Time) time.Time {
	day := t.In(CST).AddDate(0, 0, -1)
	for !IsTradingDay(day) {
		day = day.AddDate(0, 0, -1)
	}
	return day
}
