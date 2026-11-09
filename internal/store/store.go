// Package store keeps the user's funds in a JSON file, and what was read
// about them in a cache beside it.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
}

// Data is the app's own state, as the file keeps it.
type Data struct {
	Funds    []Entry `json:"funds"`
	Selected string  `json:"selected,omitempty"`
	Period   int     `json:"period,omitempty"`
	// SidebarWidth is how wide the user made the list of funds.
	SidebarWidth float32 `json:"sidebarWidth,omitempty"`
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Snapshot is what the app read about a fund, kept to open the window with
// it before the network answers.
type Snapshot struct {
	At       time.Time              `json:"at"`
	Fund     *source.Fund           `json:"fund"`
	Holdings []model.Holding        `json:"holdings"`
	Report   string                 `json:"report"`
	Quotes   map[string]model.Quote `json:"quotes,omitempty"`
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
