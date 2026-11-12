package app

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"fundbar/internal/calendar"
	"fundbar/internal/store"
)

// AlertConfig is the alert rules as the settings set them. It is a value of
// its own so that the rules can be decided without the app around them.
type AlertConfig struct {
	Move    bool
	MovePct float64
	Settled bool
}

// defaultMovePct is the threshold an unset move rule uses, as it did when the
// setting was first added.
const defaultMovePct = 2

// Note is a notification the rules decided on, before anything shows it.
// Fund is the code the note is about, and "" when it is about more than one
// fund: a click then opens the window rather than a fund.
type Note struct {
	ID    string
	Title string
	Body  string
	Fund  string
}

// The id of a note says what a click on it opens: one that names a fund
// starts with fundNote and the fund's code, and any other with allNote. The
// system hands the id back with the click, also to a run after the one that
// showed the note, so the id itself carries what the app needs to open it.
const (
	fundNote = "fund:"
	allNote  = "all:"
)

// noteFund is the code a note's id names, if it names one.
func noteFund(id string) (string, bool) {
	rest, ok := strings.CutPrefix(id, fundNote)
	if !ok {
		return "", false
	}
	code, _, _ := strings.Cut(rest, ":")
	if code == "" {
		return "", false
	}
	return code, true
}

// evaluate applies the alert rules to the funds at at, and returns what to
// announce and the marks to keep for the next check. It reads no clock and
// shows nothing, so every rule can be tried with its own day and funds.
func evaluate(cfg AlertConfig, state store.AlertState, funds []*Fund, at time.Time) ([]Note, store.AlertState) {
	if cfg.MovePct <= 0 {
		cfg.MovePct = defaultMovePct
	}
	day := at.In(cst).Format("2006-01-02")
	prev := state
	if state.Day != day {
		// A new trading date arms every rule again. A fund that is still
		// inside a price target's zone stays known to be, so that one that
		// remains past its line for weeks is not announced every morning.
		prev = store.AlertState{Day: day, Targets: restTargets(state.Targets)}
	}
	next := store.AlertState{Day: day, Settled: prev.Settled}

	var notes []Note
	notes = append(notes, evaluateMove(cfg, &prev, &next, funds, day)...)
	notes = append(notes, evaluateSettled(cfg, &prev, &next, funds, at, day)...)
	notes = append(notes, evaluateTargets(&prev, &next, funds, day)...)
	return notes, next
}

// restTargets is the funds' price targets as a new day finds them: each side
// keeps whether the fund was inside its zone, and may announce again.
func restTargets(marks map[string]store.TargetMark) map[string]store.TargetMark {
	if len(marks) == 0 {
		return nil
	}
	out := make(map[string]store.TargetMark, len(marks))
	for code, mark := range marks {
		out[code] = store.TargetMark{Up: mark.Up, Down: mark.Down}
	}
	return out
}

// evaluateMove announces the funds whose estimate moved by the threshold
// today, once each. The funds that newly qualify go out as one note.
func evaluateMove(cfg AlertConfig, prev, next *store.AlertState, funds []*Fund, day string) []Note {
	// The marks stay as they were while the rule is off, so that turning it
	// back on does not announce what the day already said.
	next.Move = append([]string(nil), prev.Move...)
	if !cfg.Move {
		return nil
	}
	sent := make(map[string]bool, len(prev.Move))
	for _, code := range prev.Move {
		sent[code] = true
	}
	var hit []*Fund
	for _, f := range funds {
		if sent[f.Code] || !estimateToday(f, day) {
			continue
		}
		if math.Abs(f.Est.Pct) < cfg.MovePct-1e-9 {
			continue
		}
		hit = append(hit, f)
		sent[f.Code] = true
		next.Move = append(next.Move, f.Code)
	}
	if len(hit) == 0 {
		return nil
	}
	// The largest moves are the ones worth naming when there are several.
	sort.SliceStable(hit, func(i, j int) bool {
		return math.Abs(hit[i].Est.Pct) > math.Abs(hit[j].Est.Pct)
	})
	n := Note{Title: "盘中涨跌提醒", Body: moveBody(hit)}
	if len(hit) == 1 {
		n.Fund = hit[0].Code
		n.ID = fundNote + hit[0].Code + ":move:" + day
	} else {
		// The day's mark count gives each batch a note of its own: ids that
		// shared one would replace one another in the notification center,
		// and the funds of an earlier batch would go unread.
		n.ID = allNote + "move:" + day + ":" + strconv.Itoa(len(next.Move))
	}
	return []Note{n}
}

// estimateToday reports that the estimate is today's: the fund shows its
// estimate, and the prices behind it are of the day in the market's zone.
func estimateToday(f *Fund, day string) bool {
	return f.UsingEstimate() && !f.QuoteAt.IsZero() &&
		f.QuoteAt.In(cst).Format("2006-01-02") == day
}

// moveNames is how many funds a note about several names beside their
// changes: the system cuts a long note, and the names beyond this would
// hardly be read.
const moveNames = 3

// moveBody names each fund that moved, its change, and the money a held
// fund's shares gained. It names the largest moves, and counts the rest.
func moveBody(funds []*Fund) string {
	shown := funds
	if len(shown) > moveNames {
		shown = shown[:moveNames]
	}
	parts := make([]string, 0, len(shown))
	for _, f := range shown {
		line := fundName(f) + " " + pct(f.Est.Pct)
		if f.Shares > 0 {
			line += "（今日收益 " + signedMoney(f.TodayProfit()) + " 元）"
		}
		parts = append(parts, line)
	}
	body := strings.Join(parts, " · ")
	if len(funds) > moveNames {
		body += fmt.Sprintf(" 等 %d 只", len(funds))
	}
	return body
}

// evaluateSettled announces that the holding group's values are published:
// when all of them are, or from 22:00 when at least one is, with the rest
// counted. It stays quiet on a day the market is closed, and once it has
// spoken that day.
func evaluateSettled(cfg AlertConfig, prev, next *store.AlertState, funds []*Fund, at time.Time, day string) []Note {
	if !cfg.Settled || prev.Settled || !calendar.IsTradingDay(at) {
		return nil
	}
	var held []*Fund
	published := 0
	for _, f := range funds {
		if !f.In(holdingGroup) {
			continue
		}
		held = append(held, f)
		if last, ok := f.LastNav(); ok && last.Date == day {
			published++
		}
	}
	if len(held) == 0 {
		return nil
	}
	pending := len(held) - published
	// Before the evening, only the last value is worth saying; at 22:00, the
	// ones still to come are worth counting.
	if pending > 0 && !(at.In(cst).Hour() >= 22 && published > 0) {
		return nil
	}
	next.Settled = true
	return []Note{{
		ID:    allNote + "settled:" + day,
		Title: "净值公布提醒",
		Body:  settledBody(held, published, pending),
	}}
}

// settledBody says how many values are out and what the holding group's day
// was, as the menu bar item reports it.
func settledBody(funds []*Fund, published, pending int) string {
	count := fmt.Sprintf("%d 只已全部公布", len(funds))
	if pending > 0 {
		count = fmt.Sprintf("已公布 %d 只 · 待公布 %d 只", published, pending)
	}
	s := summarizeFunds(funds, true)
	if s.Holding {
		return count + " · 今日收益 " + signedMoney(s.Profit) + " 元（" + pct(s.Pct) + "）"
	}
	return count + " · 平均涨跌 " + pct(s.Pct)
}

// evaluateTargets announces a fund entering the zone of its price target. A
// fund that stays in the zone is left alone, and one that leaves may enter
// again, but each side speaks at most once a day because an estimate can
// flap around the line.
func evaluateTargets(prev, next *store.AlertState, funds []*Fund, day string) []Note {
	// Only a fund held now keeps a mark, and only a mark that says something,
	// so that the file does not grow with funds that have no target or that
	// the user no longer follows.
	marks := make(map[string]store.TargetMark)
	var notes []Note
	for _, f := range funds {
		if f.Shares <= 0 || f.Cost <= 0 {
			continue
		}
		mark := prev.Targets[f.Code]
		// The lines watch the shares held now, not what sales already took: a
		// realised gain must not announce a position that is now losing.
		change := f.HoldingPct()
		if f.TakeProfit > 0 {
			// A rounded value lands a hair either side of the line.
			in := change >= f.TakeProfit-1e-9
			if in && !mark.Up && !mark.UpSent {
				notes = append(notes, targetNote(f, true, day))
				mark.UpSent = true
			}
			mark.Up = in
		} else {
			mark.Up = false
		}
		if f.StopLoss > 0 {
			in := change <= -f.StopLoss+1e-9
			if in && !mark.Down && !mark.DownSent {
				notes = append(notes, targetNote(f, false, day))
				mark.DownSent = true
			}
			mark.Down = in
		} else {
			mark.Down = false
		}
		if mark != (store.TargetMark{}) {
			marks[f.Code] = mark
		}
	}
	if len(marks) > 0 {
		next.Targets = marks
	}
	return notes
}

// targetNote says which line a fund reached, its holding return, and the
// money its shares gained today.
func targetNote(f *Fund, up bool, day string) Note {
	side, title, line := "down", "止损提醒", "止损线 -"
	target := f.StopLoss
	if up {
		side, title, line = "up", "止盈提醒", "止盈线 +"
		target = f.TakeProfit
	}
	body := fundName(f) + " " + pct(f.HoldingPct()) + "，" + line + trim(target, 2) + "%"
	body += "（今日收益 " + signedMoney(f.TodayProfit()) + " 元）"
	return Note{
		ID: fundNote + f.Code + ":" + side + ":" + day, Title: title,
		Body: body, Fund: f.Code,
	}
}

// fundName is the name a note calls a fund by, and its code when it has
// none yet.
func fundName(f *Fund) string {
	if f.Name == "" {
		return f.Code
	}
	return f.Name
}
