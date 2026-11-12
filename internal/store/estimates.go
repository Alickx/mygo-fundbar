package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// maxEstimateRecords is how many days of a fund's estimates the file keeps:
// enough to say how far off the estimates have been lately, without growing
// with the years the user follows the fund.
const maxEstimateRecords = 60

// EstimateRecord is one day's estimate and, once the fund has published its
// value for that day, the change it actually made. Actual is a pointer so
// that a published 0.00% is a finished record, not one still waiting.
type EstimateRecord struct {
	Date   string   `json:"date"`
	Est    float64  `json:"est"`
	Cover  float64  `json:"cover"`
	Actual *float64 `json:"actual,omitempty"`
}

// Estimates is the record of each followed fund's estimates, by fund code,
// oldest first.
type Estimates map[string][]EstimateRecord

// estimatePath is the file beside the funds that keeps the estimates.
func estimatePath(dir string) string { return filepath.Join(dir, "estimates.json") }

// LoadEstimates reads the estimate record, and an empty one when there is
// none.
func LoadEstimates(dir string) Estimates {
	e := Estimates{}
	body, err := os.ReadFile(estimatePath(dir))
	if err != nil {
		return e
	}
	if json.Unmarshal(body, &e) != nil || e == nil {
		return Estimates{}
	}
	return e
}

// SaveEstimates writes the estimate record.
func SaveEstimates(dir string, e Estimates) error {
	body, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(estimatePath(dir), body)
}

// AddRecord puts a day's record in place, keeping the days in order. A
// record whose actual change is already known is never replaced, because
// what the app knew before the publication is the thing being measured. The
// fund keeps only its newest maxEstimateRecords days. It reports whether the
// record changed anything, so that a refresh that learned nothing writes no
// file.
func (e Estimates) AddRecord(code string, r EstimateRecord) bool {
	recs := e[code]
	at := sort.Search(len(recs), func(i int) bool { return recs[i].Date >= r.Date })
	switch {
	case at < len(recs) && recs[at].Date == r.Date:
		if recs[at].Actual != nil {
			return false
		}
		if recs[at].Est == r.Est && recs[at].Cover == r.Cover {
			return false
		}
		recs[at] = r
	default:
		recs = append(recs, EstimateRecord{})
		copy(recs[at+1:], recs[at:])
		recs[at] = r
	}
	if len(recs) > maxEstimateRecords {
		recs = recs[len(recs)-maxEstimateRecords:]
	}
	e[code] = recs
	return true
}

// Prune forgets the funds the user no longer follows, and reports whether it
// dropped any.
func (e Estimates) Prune(keep map[string]bool) bool {
	changed := false
	for code := range e {
		if !keep[code] {
			delete(e, code)
			changed = true
		}
	}
	return changed
}
