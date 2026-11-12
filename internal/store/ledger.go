package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"fundbar/internal/ledger"
)

// ErrLedgerUnreadable says a fund's ledger file does not parse. Reading
// leaves the file where it is: a preview reads too, and must not move the
// user's files. A running app sets it aside with SetAsideLedger.
var ErrLedgerUnreadable = errors.New("store: the ledger file could not be read")

// errBadCode is why a code that is not a fund's six digits is refused: the
// code names a file, and anything else could name one outside the directory.
var errBadCode = errors.New("store: not a fund code")

// ValidCode reports that a code is exactly six digits, as a fund's is. Codes
// name files under the data directory, so one that is anything else is not
// followed there, wherever it came from: a search answer, a file edited by
// hand, or a backup.
func ValidCode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ledgerPath is the file beside the funds that keeps one fund's
// transactions. They stay out of funds.json: an older build rewrites that
// file whole from what it knows, and would drop rows it has no field for.
func ledgerPath(dir, code string) string {
	return filepath.Join(dir, "ledger", code+".json")
}

// LoadLedger reads a fund's transactions, and none for a fund that keeps no
// ledger. A file that does not parse is reported as ErrLedgerUnreadable and
// left as it is.
func LoadLedger(dir, code string) ([]ledger.Txn, error) {
	if !ValidCode(code) {
		return nil, nil
	}
	body, err := os.ReadFile(ledgerPath(dir, code))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, err
	}
	var txns []ledger.Txn
	if json.Unmarshal(body, &txns) != nil {
		return nil, ErrLedgerUnreadable
	}
	return txns, nil
}

// SetAsideLedger moves a ledger file that does not parse out of the way, to
// "<code>.json.bad", and returns the name it took: the next save then writes
// a new ledger rather than over rows a person may still recover. A file set
// aside earlier is kept, and this one takes the next free name.
func SetAsideLedger(dir, code string) (string, error) {
	if !ValidCode(code) {
		return "", errBadCode
	}
	path := ledgerPath(dir, code)
	bad := path + ".bad"
	for i := 2; ; i++ {
		if _, err := os.Stat(bad); errors.Is(err, fs.ErrNotExist) {
			break
		}
		if i > 99 {
			return "", errors.New("store: too many ledger files set aside")
		}
		bad = fmt.Sprintf("%s.bad%d", path, i)
	}
	if err := os.Rename(path, bad); err != nil {
		return "", err
	}
	return bad, nil
}

// SaveLedger writes a fund's transactions whole, replacing what was there.
func SaveLedger(dir, code string, txns []ledger.Txn) error {
	if !ValidCode(code) {
		return errBadCode
	}
	if txns == nil {
		txns = []ledger.Txn{}
	}
	body, err := json.MarshalIndent(txns, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(ledgerPath(dir, code), body)
}

// DropLedger forgets a fund's transactions.
func DropLedger(dir, code string) {
	if !ValidCode(code) {
		return
	}
	_ = os.Remove(ledgerPath(dir, code))
}

// ShelveLedger sets a fund's transactions aside as "<code>.json.removed",
// for the removals that take a whole ledger with them: the rows are what the
// user typed in, which no network can give back, so a removal made by
// mistake leaves them where they can be put back by hand. Setting the same
// fund aside again replaces what was kept; saving a new ledger for it does
// not touch it.
func ShelveLedger(dir, code string) {
	if !ValidCode(code) {
		return
	}
	path := ledgerPath(dir, code)
	if _, err := os.Stat(path); err != nil {
		return
	}
	_ = os.Rename(path, path+".removed")
}
