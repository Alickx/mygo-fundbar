package app

import (
	"bytes"
	"encoding/csv"
	"os"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
)

// positionsHeader is the columns of the positions sheet, in the order a
// spreadsheet reader expects them.
var positionsHeader = []string{
	"代码", "名称", "分组", "持有份额", "成本价", "最新净值", "净值日期",
	"持仓市值", "今日收益", "累计收益", "累计收益率(%)", "交易笔数",
}

// positionsCSV writes every followed fund as one row of a spreadsheet. A fund
// that holds no shares leaves the holding cells empty; the rest of its row
// still says what it is and how it moved. Numbers are plain decimals without
// thousands separators, and the file starts with a byte-order mark so that
// Excel reads the Chinese names correctly.
func positionsCSV(funds []*Fund, groupName func(string) string) ([]byte, error) {
	var buf bytes.Buffer
	// The byte-order mark is what tells Excel the file is UTF-8.
	buf.WriteString("\uFEFF")
	w := csv.NewWriter(&buf)
	if err := w.Write(positionsHeader); err != nil {
		return nil, err
	}
	for _, f := range funds {
		row := make([]string, len(positionsHeader))
		row[0], row[1] = f.Code, f.Name
		row[2] = fundGroupsText(f, groupName)
		if last, ok := f.LastNav(); ok {
			row[5] = trim(last.Unit, 4)
			row[6] = last.Date
		}
		if f.Shares > 0 {
			row[3] = trim(f.Shares, 4)
			row[4] = trim(f.Cost, 6)
			row[7] = trim(f.Value(), 2)
			row[8] = trim(f.TodayProfit(), 2)
			row[9] = trim(f.TotalProfit(), 2)
			row[10] = trim(f.TotalPct(), 2)
		} else if f.ledgered() {
			// A position sold out through its ledger holds nothing now, but
			// what its sales and dividends made is still the fund's result.
			row[9] = trim(f.TotalProfit(), 2)
			row[10] = trim(f.TotalPct(), 2)
		}
		row[11] = strconv.Itoa(len(f.Txns))
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fundGroupsText names the groups a fund is listed in, joined by a slash, with
// the holding group written as 持有.
func fundGroupsText(f *Fund, groupName func(string) string) string {
	groups := f.Groups
	if len(groups) == 0 {
		groups = []string{holdingGroup}
	}
	names := make([]string, 0, len(groups))
	for _, id := range groups {
		names = append(names, groupName(id))
	}
	return strings.Join(names, "/")
}

// ExportCSV asks where to save the positions sheet and writes it there. The
// rows are read on the calling thread, before the dialog opens, so that the
// file is one the window's state at the click made.
func (a *App) ExportCSV() {
	a.dataErr = ""
	a.invalidateSettings()
	body, err := positionsCSV(a.all, a.groupName)
	if err != nil {
		a.dataErr = "导出持仓表失败：" + err.Error()
		a.invalidateSettings()
		return
	}
	if a.win == nil {
		a.saveCSV(body)
		return
	}
	go a.saveCSV(body)
}

func (a *App) saveCSV(body []byte) {
	path, err := a.saveFile(mygo.SaveDialogOptions{
		Title:       "导出持仓表",
		ButtonLabel: "导出",
		DefaultPath: backupPath("FundBar 持仓表 " + now().Format("2006-01-02") + ".csv"),
		Filters:     []mygo.FileFilter{{Name: "CSV", Extensions: []string{"csv"}}},
	})
	if err != nil {
		a.reportData("导出持仓表失败：" + err.Error())
		return
	}
	if path == "" {
		return
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		a.reportData("导出持仓表失败：" + err.Error())
		return
	}
	a.onMain(func() {
		a.dataErr = ""
		a.notice("已导出持仓表")
		a.invalidateSettings()
	})
}
