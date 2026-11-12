package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"fundbar/internal/store"
)

// ExportBackup asks where to save a backup, writes the funds and the
// transaction history there as one JSON file, and says how it went.
//
// The bundle is read here, on the main thread, before the dialog opens: the
// dialog waits for the user, and a save made while it waits would otherwise
// let the bundle pair a newer funds file with an older history, or the other
// way round. The dialog itself blocks until it is dismissed, so only the
// asking and the writing run off the main thread; a window-less app, as a
// test draws it, runs the whole thing at once.
func (a *App) ExportBackup() {
	a.dataErr = ""
	a.invalidateSettings()
	b, err := store.BuildBundle(a.dir, now())
	if err != nil {
		a.reportData("导出备份失败：" + err.Error())
		return
	}
	if a.win == nil {
		a.exportBackup(b)
		return
	}
	go a.exportBackup(b)
}

func (a *App) exportBackup(b *store.Bundle) {
	path, err := a.saveFile(mygo.SaveDialogOptions{
		Title:       "导出备份",
		ButtonLabel: "导出",
		DefaultPath: backupPath("FundBar 备份 " + now().Format("2006-01-02") + ".json"),
		Filters:     []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}},
	})
	if err != nil {
		a.reportData("导出备份失败：" + err.Error())
		return
	}
	if path == "" {
		// The user cancelled: nothing to do, and nothing to say.
		return
	}
	if err := store.SaveBundle(path, b); err != nil {
		a.reportData("导出备份失败：" + err.Error())
		return
	}
	a.onMain(func() {
		a.dataErr = ""
		a.notice("已导出备份")
		a.invalidateSettings()
	})
}

// ImportBackup asks for a backup file, checks it, asks whether to replace
// what is here, and on a yes puts it in place after backing up the current
// state first. The dialogs block, so they run off the main thread; the files
// and the reload happen back on it.
func (a *App) ImportBackup() {
	a.dataErr = ""
	a.invalidateSettings()
	if a.win == nil {
		a.importBackup()
		return
	}
	go a.importBackup()
}

func (a *App) importBackup() {
	paths, err := a.openFile(mygo.OpenDialogOptions{
		Title:       "导入备份",
		ButtonLabel: "导入",
		Filters:     []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}},
	})
	if err != nil {
		a.reportData("导入备份失败：" + err.Error())
		return
	}
	if len(paths) == 0 || paths[0] == "" {
		return
	}
	b, err := store.ReadBundle(paths[0])
	if err != nil {
		// The refusal says why, and has written nothing anywhere.
		a.reportData("导入备份失败：" + err.Error())
		return
	}
	answer, err := a.confirm(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Title:              "导入备份",
		Message:            "导入会替换当前的自选、分组、持仓与交易记录。",
		Detail:             "当前数据会先自动备份到数据目录的 backups 文件夹。",
		Buttons:            []string{"取消", "导入并替换"},
		DefaultButton:      0,
		CancelButton:       0,
		DestructiveButtons: []int{1},
	})
	if err != nil {
		a.reportData("导入备份失败：" + err.Error())
		return
	}
	if answer.Button != 1 {
		// A declined confirmation leaves everything as it was.
		return
	}
	a.onMain(func() { a.applyImport(b) })
}

// applyImport writes the current state aside, puts the bundle in its place,
// and reloads what the windows show. It runs on the main thread.
func (a *App) applyImport(b *store.Bundle) {
	backup, err := store.BackupBeforeImport(a.dir, now())
	if err != nil {
		a.dataErr = "导入前备份失败：" + err.Error()
		a.invalidateSettings()
		return
	}
	if err := store.ApplyBundle(a.dir, b); err != nil {
		a.dataErr = "导入失败：" + err.Error() + a.undoImport(backup)
		a.invalidateSettings()
		return
	}
	// What the windows held belongs to the files that were just replaced: a
	// draft, an undo or a look-through of them has nothing left to act on.
	a.cancelTxn()
	a.txUndo = ledgerUndo{}
	a.exitPortfolio()
	hadShortcut := a.globalShortcut
	a.Load()
	if a.globalShortcut != hadShortcut {
		// The keys follow the imported setting, as its switch would set them.
		a.applyGlobalShortcut(a.globalShortcut)
	}
	a.dataErr = ""
	// What the old files said no longer matters now that they are replaced.
	a.failure = ""
	a.status = ""
	if a.win != nil {
		a.refresh()
	}
	a.updateTray()
	a.redraw()
	a.notice(fmt.Sprintf("已导入 %d 只基金", b.FundCount()))
	a.invalidateSettings()
}

// undoImport puts back what an import that failed part way had begun to
// replace, and says how that went, for the line that reports the failure. A
// directory left half new and half old is what the next launch would read.
func (a *App) undoImport(backup string) string {
	old, err := store.ReadBundle(backup)
	if err == nil {
		err = store.ApplyBundle(a.dir, old)
	}
	if err != nil {
		return "；原数据未能自动恢复，可重新导入 " + backup
	}
	return "；已恢复为导入前的数据"
}

// backupPath puts a default file name in the user's home, which is where the
// save dialog opens; without a home it leaves the name on its own.
func backupPath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}
	return filepath.Join(home, name)
}

// reportData says why the settings window's last backup or export failed,
// from whichever thread found out. It is the danger line under the 数据
// section; a success says so in the status row instead.
func (a *App) reportData(msg string) {
	a.onMain(func() {
		a.dataErr = msg
		a.invalidateSettings()
	})
}

// invalidateSettings redraws the settings window after a line under it
// changed.
func (a *App) invalidateSettings() {
	if a.settings != nil {
		a.settings.Invalidate()
	}
}
