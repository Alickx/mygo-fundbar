// Package store keeps the user's funds in a JSON file, and what was read
// about them in a cache beside it.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"fundbar/internal/model"
	"fundbar/internal/source"
)

// Entry is a fund the user follows.
type Entry struct {
	Code string `json:"code"`
	// Type is the fund's kind, as the search called it, such as
	// "指数型-股票".
	Type string `json:"type,omitempty"`
	// Shares is how much of the fund the user holds, and Cost what they paid
	// for each share.
	Shares float64 `json:"shares,omitempty"`
	Cost   float64 `json:"cost,omitempty"`
	// Groups are the groups the fund is listed in. "" is the holding group,
	// the funds the user holds or follows in the main list; any other id is
	// a group the user made to watch funds without holding them. A fund of
	// an older file names none, and is in the holding group alone.
	Groups []string `json:"groups,omitempty"`
	// TakeProfit and StopLoss are the cumulative return, in percent, at which
	// the app announces the fund: reaching +TakeProfit, or falling to
	// -StopLoss. Zero leaves the side off, so an older file loads silent.
	TakeProfit float64 `json:"takeProfit,omitempty"`
	StopLoss   float64 `json:"stopLoss,omitempty"`
}

// Alerts are the notification rules the user turned on. A file that names
// none, as an older one does, has every rule off.
type Alerts struct {
	Move    bool    `json:"move,omitempty"`
	MovePct float64 `json:"movePct,omitempty"`
	Settled bool    `json:"settled,omitempty"`
}

// Group is a list of funds the user made to watch, apart from the holding
// group.
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Data is the app's own state, as the file keeps it.
type Data struct {
	Funds  []Entry `json:"funds"`
	Groups []Group `json:"groups,omitempty"`
	// ActiveGroup is the group whose funds the windows list, "" being the
	// holding group.
	ActiveGroup string `json:"group,omitempty"`
	Selected    string `json:"selected,omitempty"`
	// GroupSelected is the chosen fund of each made group, by group id; the
	// holding group's stays in Selected.
	GroupSelected map[string]string `json:"groupSelected,omitempty"`
	Period        int               `json:"period,omitempty"`
	// NavView is what the chart card shows: 0 for the line, 1 for the list
	// of each day's value, 2 for the recovery from the deepest fall.
	NavView int `json:"navView,omitempty"`
	// ManagerOnly opens the chart at the day the fund's current management
	// team took over, rather than at the chosen period's start.
	ManagerOnly bool `json:"managerOnly,omitempty"`
	// SidebarWidth is how wide the user made the list of funds.
	SidebarWidth float32 `json:"sidebarWidth,omitempty"`
	// SkippedVersion is the newer version the user ignored: the app offers
	// the one after it.
	SkippedVersion string `json:"skippedVersion,omitempty"`
	// Alerts are the notification rules, nil when the file names none.
	Alerts *Alerts `json:"alerts,omitempty"`
	// TrayText is what the menu bar item shows: "money" for the day's gain,
	// "pct" for the day's change, or "icon" for the icon alone. An older
	// file names none, and shows the gain as it always did.
	TrayText string `json:"trayText,omitempty"`
	// GlobalShortcut turns on the shortcut that opens the panel. An older
	// file names none, and has it off: a new version does not take keys by
	// itself.
	GlobalShortcut bool `json:"globalShortcut,omitempty"`
	// HideIndices hides the market benchmarks in the panel. Its zero value
	// is false, which shows them, so an older file gets the row.
	HideIndices bool `json:"hideIndices,omitempty"`
	// LowCoverEstimate shows an intraday estimate even when the disclosed
	// holdings cover less than 20% of the fund. Its zero value is off: a fund
	// whose disclosed holdings are too small is not estimated, and an older
	// file keeps that.
	LowCoverEstimate bool `json:"lowCoverEstimate,omitempty"`
	// Theme is the appearance the user chose: "light", "dark", or "" to
	// follow the system. An older file names none, and follows the system as
	// it always did.
	Theme string `json:"theme,omitempty"`
}

// AlertState is what the alert rules have already announced, kept beside the
// funds rather than in them, so that a restart does not announce the same
// thing twice and the user's backup file stays as it was.
type AlertState struct {
	// Day is the trading date the marks below belong to. A new date starts
	// every rule afresh.
	Day string `json:"day,omitempty"`
	// Move is the funds the move rule announced today.
	Move []string `json:"move,omitempty"`
	// Settled says the settled rule announced today.
	Settled bool `json:"settled,omitempty"`
	// Targets are the funds with a price target: whether each side was in its
	// zone at the last look, and whether it already announced today.
	Targets map[string]TargetMark `json:"targets,omitempty"`
}

// TargetMark is one fund's price targets, as the last check left them.
type TargetMark struct {
	Up       bool `json:"up,omitempty"`
	Down     bool `json:"down,omitempty"`
	UpSent   bool `json:"upSent,omitempty"`
	DownSent bool `json:"downSent,omitempty"`
}

// Equal reports that two states hold the same marks, so that a refresh which
// changed nothing writes no file.
func (s AlertState) Equal(o AlertState) bool {
	return s.Day == o.Day && s.Settled == o.Settled &&
		slices.Equal(s.Move, o.Move) && maps.Equal(s.Targets, o.Targets)
}

// Load reads the file, and an empty state when there is none.
func Load(path string) *Data {
	d := &Data{}
	body, err := os.ReadFile(path)
	if err != nil {
		return d
	}
	if json.Unmarshal(body, d) != nil {
		return &Data{}
	}
	return d
}

// Save writes the file, replacing it whole so a crash cannot leave half of
// it behind.
func (d *Data) Save(path string) error {
	body, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, body)
}

// writeFile writes body to path whole, in place of what was there.
func writeFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// alertPath is the file beside the funds that keeps what was announced.
func alertPath(dir string) string { return filepath.Join(dir, "alerts.json") }

// LoadAlerts reads what the alert rules announced, and an empty state when
// there is none.
func LoadAlerts(dir string) *AlertState {
	s := &AlertState{}
	body, err := os.ReadFile(alertPath(dir))
	if err != nil {
		return s
	}
	if json.Unmarshal(body, s) != nil {
		return &AlertState{}
	}
	return s
}

// SaveAlerts writes what the alert rules announced.
func SaveAlerts(dir string, s *AlertState) error {
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(alertPath(dir), body)
}

// Snapshot is what the app read about a fund, kept to open the window with
// it before the network answers.
type Snapshot struct {
	At       time.Time                     `json:"at"`
	Fund     *source.Fund                  `json:"fund"`
	Holdings []model.Holding               `json:"holdings"`
	Report   string                        `json:"report"`
	Quotes   map[string]model.Quote        `json:"quotes,omitempty"`
	Profiles map[string]model.StockProfile `json:"profiles,omitempty"`
	// ManagersAt is when the management history was last read. Its zero
	// value means a cache from before the history was kept.
	ManagersAt time.Time `json:"managersAt,omitempty"`
}

func snapshotPath(dir, code string) string {
	return filepath.Join(dir, "cache", code+".json")
}

// LoadSnapshot reads the cache of a fund.
func LoadSnapshot(dir, code string) (*Snapshot, bool) {
	body, err := os.ReadFile(snapshotPath(dir, code))
	if err != nil {
		return nil, false
	}
	var s Snapshot
	if json.Unmarshal(body, &s) != nil || s.Fund == nil || len(s.Fund.Navs) == 0 {
		return nil, false
	}
	return &s, true
}

// SaveSnapshot writes the cache of a fund, in the background's time.
func SaveSnapshot(dir, code string, s *Snapshot) {
	body, err := json.Marshal(s)
	if err != nil {
		return
	}
	path := snapshotPath(dir, code)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(path, body, 0o644); err != nil && !errors.Is(err, fs.ErrPermission) {
		return
	}
}

// DropSnapshot forgets a fund's cache.
func DropSnapshot(dir, code string) {
	_ = os.Remove(snapshotPath(dir, code))
}
