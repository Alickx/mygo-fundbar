package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fundbar/internal/ledger"
)

// The bundle's name and version: a backup says which app wrote it, and a
// build refuses one written in a shape it does not know.
const (
	BundleApp     = "FundBar"
	BundleVersion = 1
)

// ErrNotFundBar is why a bundle of another app, or of no app at all, is
// refused.
var ErrNotFundBar = errors.New("这不是 FundBar 的备份文件")

// ErrNewerBundle is why a bundle of a newer build is refused: this build
// cannot be sure it reads the shape the newer one writes.
var ErrNewerBundle = errors.New("备份文件来自更新的 FundBar 版本，请先更新应用")

// Bundle is one file holding everything the app cannot fetch again: the
// followed funds, their groups and positions, and the transaction history.
// The cache, alerts.json and estimates.json are left out: the first is what
// the network gives back, and the other two are this Mac's own notes.
type Bundle struct {
	App      string                  `json:"app"`
	Version  int                     `json:"version"`
	Exported string                  `json:"exported"`
	Funds    json.RawMessage         `json:"funds"`
	Ledgers  map[string][]ledger.Txn `json:"ledgers"`
}

// BuildBundle reads a data directory into a bundle. Exported is the time the
// bundle was made, in the time's own zone.
func BuildBundle(dir string, at time.Time) (*Bundle, error) {
	body, err := os.ReadFile(filepath.Join(dir, "funds.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// A directory with no funds file yet still holds a usable, empty
		// backup.
		body = []byte("{}")
	case err != nil:
		return nil, err
	}
	funds, err := canonicalJSON(body)
	if err != nil {
		return nil, fmt.Errorf("自选数据无法读取：%w", err)
	}
	var data Data
	if err := json.Unmarshal(funds, &data); err != nil {
		return nil, fmt.Errorf("自选数据无法读取：%w", err)
	}

	b := &Bundle{
		App:      BundleApp,
		Version:  BundleVersion,
		Exported: at.Format(time.RFC3339),
		Funds:    funds,
		Ledgers:  map[string][]ledger.Txn{},
	}
	// A fund without rows keeps no entry, so the file says only what the user
	// actually recorded.
	for _, e := range data.Funds {
		rows, err := LoadLedger(dir, e.Code)
		if err != nil {
			return nil, fmt.Errorf("基金 %s 的交易记录无法读取：%w", e.Code, err)
		}
		if len(rows) > 0 {
			b.Ledgers[e.Code] = rows
		}
	}
	return b, nil
}

// SaveBundle writes a bundle whole to path.
func SaveBundle(path string, b *Bundle) error {
	if b.Ledgers == nil {
		b.Ledgers = map[string][]ledger.Txn{}
	}
	body, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, body)
}

// ReadBundle reads a bundle file, checking it as ParseBundle does.
func ReadBundle(path string) (*Bundle, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBundle(body)
}

// ParseBundle reads and checks a bundle. A bundle of another app, of a newer
// build, or with funds or rows this build cannot use is refused, and the
// reason is written in Chinese for the settings window. Nothing is written
// to any directory here.
func ParseBundle(body []byte) (*Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("备份文件无法解析：%w", err)
	}
	if b.App != BundleApp {
		return nil, ErrNotFundBar
	}
	if b.Version > BundleVersion {
		return nil, fmt.Errorf("%w（备份版本 %d）", ErrNewerBundle, b.Version)
	}
	if b.Version < 1 {
		return nil, fmt.Errorf("备份文件版本无效：%d", b.Version)
	}
	// The funds are kept as they came, so that a file this build has no field
	// for still survives a round trip; here they only have to parse.
	funds, err := canonicalJSON(b.Funds)
	if err != nil {
		return nil, fmt.Errorf("备份中的自选数据无法读取：%w", err)
	}
	var data Data
	if err := json.Unmarshal(funds, &data); err != nil {
		return nil, fmt.Errorf("备份中的自选数据无法读取：%w", err)
	}
	for _, e := range data.Funds {
		// Codes name files under the data directory, so one that is not a
		// fund's six digits is refused rather than followed.
		if !ValidCode(e.Code) {
			return nil, fmt.Errorf("备份中的基金代码无效：%s", e.Code)
		}
	}
	for code, rows := range b.Ledgers {
		if !ValidCode(code) {
			return nil, fmt.Errorf("备份中的基金代码无效：%s", code)
		}
		for _, t := range rows {
			if err := ledger.Check(t); err != nil {
				return nil, fmt.Errorf("基金 %s 的交易记录有误：%v", code, err)
			}
		}
	}
	b.Funds = funds
	if b.Ledgers == nil {
		b.Ledgers = map[string][]ledger.Txn{}
	}
	return &b, nil
}

// FundCount is how many funds the bundle's funds file holds.
func (b *Bundle) FundCount() int {
	var data Data
	if json.Unmarshal(b.Funds, &data) != nil {
		return 0
	}
	return len(data.Funds)
}

// ApplyBundle writes a bundle into a data directory. The ledgers go down
// first, a ledger the bundle does not hold is removed (its rows are in the
// pre-import backup), and funds.json goes last: a write torn part way leaves
// the ledgers a reader can still pair with the funds file.
func ApplyBundle(dir string, b *Bundle) error {
	if b == nil {
		return errors.New("没有可导入的备份")
	}
	// The codes name files under the directory. They are checked here as
	// well as where a bundle is read, because this is the function that
	// writes: a bundle put together in code must not name a file elsewhere.
	for code := range b.Ledgers {
		if !ValidCode(code) {
			return fmt.Errorf("备份中的基金代码无效：%s", code)
		}
	}
	for code, rows := range b.Ledgers {
		if err := SaveLedger(dir, code, rows); err != nil {
			return err
		}
	}
	if err := dropMissingLedgers(dir, b.Ledgers); err != nil {
		return err
	}
	funds := b.Funds
	if len(funds) == 0 {
		funds = json.RawMessage("{}")
	}
	funds, err := canonicalJSON(funds)
	if err != nil {
		return fmt.Errorf("备份中的自选数据无法读取：%w", err)
	}
	return writeFile(filepath.Join(dir, "funds.json"), funds)
}

// dropMissingLedgers sets aside the funds whose ledger files the bundle does
// not name: their rows are in the backup just taken, and setting them aside
// rather than deleting them keeps a hand-restored file recoverable. Only the
// files a build reads are touched; a ".bad" or ".removed" file is the user's
// own last copy of rows, and stays where it is.
func dropMissingLedgers(dir string, keep map[string][]ledger.Txn) error {
	entries, err := os.ReadDir(filepath.Join(dir, "ledger"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		code := strings.TrimSuffix(e.Name(), ".json")
		if _, ok := keep[code]; ok {
			continue
		}
		ShelveLedger(dir, code)
	}
	return nil
}

// BackupBeforeImport writes the data directory as a bundle of the same shape
// under backups/, and returns its path. It is what makes an import that
// replaced the wrong file recoverable.
func BackupBeforeImport(dir string, at time.Time) (string, error) {
	b, err := BuildBundle(dir, at)
	if err != nil {
		return "", err
	}
	name := "导入前 " + at.Format("20060102-150405") + ".json"
	path := filepath.Join(dir, "backups", name)
	if err := SaveBundle(path, b); err != nil {
		return "", err
	}
	return path, nil
}

// canonicalJSON rewrites JSON at the two-space indentation the app's own
// files use, so that a bundle round trip writes funds.json as it was. It
// keeps the fields and their order.
func canonicalJSON(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("empty JSON")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
