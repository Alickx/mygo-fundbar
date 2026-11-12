package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/store"
)

// batchCodes reads search text that is nothing but two or more six-digit fund
// codes, separated by spaces, commas, 、, semicolons or new lines. It returns
// the codes deduplicated, in the order they were typed; any other text, or a
// token that is not six digits, makes it an ordinary search.
func batchCodes(text string) ([]string, bool) {
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', '　',
			',', '，', '、', ';', '；':
			return true
		}
		return false
	})
	if len(tokens) < 2 {
		return nil, false
	}
	seen := make(map[string]bool, len(tokens))
	codes := make([]string, 0, len(tokens))
	for _, code := range tokens {
		if !isFundCode(code) {
			return nil, false
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		codes = append(codes, code)
	}
	return codes, len(codes) > 0
}

// isFundCode reports that a token is exactly six digits.
func isFundCode(s string) bool { return store.ValidCode(s) }

// addBatch follows the codes in the group the window shows, as Add does, but
// with one refresh for the lot: their names arrive together, and the ten
// searches a batch replaces do not ask the network nine extra times.
func (a *App) addBatch(codes []string) {
	a.query = ""
	a.hits = nil
	a.pick = -1
	added := false
	for _, code := range codes {
		if !store.ValidCode(code) {
			continue
		}
		if f := a.byCode(code); f != nil {
			if !f.In(a.group) {
				f.Groups = append(f.Groups, a.group)
				added = true
			}
			continue
		}
		f := &Fund{Code: code, Groups: []string{a.group}}
		// A ledger file found for the code is this fund's history, as in Add.
		a.adoptLedger(f)
		a.all = append(a.all, f)
		added = true
	}
	if !added {
		a.redraw()
		return
	}
	if len(codes) > 0 {
		a.selCode[a.group] = codes[0]
	}
	a.rebuildList()
	a.save()
	if a.win != nil {
		a.refresh()
	}
	a.redraw()
}

// batchResults draws the one row the search area shows for a batch: how many
// of the codes are not in the group yet, and each code, with the ones already
// there marked.
func (a *App) batchResults(c *ui.Context, t *ui.Theme, codes []string) {
	pending := 0
	for _, code := range codes {
		if f := a.byCode(code); f == nil || !f.In(a.group) {
			pending++
		}
	}
	row := ui.Column(c.Key("batch-add")).Padding(8, 10).Radius(9).Gap(6)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Transition(ui.ElementTransition{Duration: 120 * time.Millisecond})
	row.Children(func() {
		ui.Text(c, batchLabel(pending)).FontSize(12.5).
			FontWeight(500).TextColor(t.Accent)
		ui.Row(c).Wrap().Gap(10).Children(func() {
			for _, code := range codes {
				line := code
				if f := a.byCode(code); f != nil && f.In(a.group) {
					line += "  已添加"
				}
				ui.Text(c, line).FontSize(11).TextColor(t.TextMuted).
					FontFeatures("tnum").NoWrap()
			}
		})
	})
	if row.Clicked() {
		a.addBatch(codes)
	}
}

// batchLabel names the row's action, shared with the tests that look for it.
func batchLabel(n int) string { return fmt.Sprintf("添加这 %d 只基金", n) }
