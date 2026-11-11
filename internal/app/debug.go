package app

import (
	"github.com/egoist/mygo/ui"

	"fundbar/internal/source"
)

// The hooks below draw parts of the app without the network or a window,
// as tools/shot does.

// PreviewSearch fills the search field and its results.
func (a *App) PreviewSearch(query string, hits []source.Hit) {
	a.query = query
	a.hits = hits
}

// PreviewSettings draws the settings window.
func (a *App) PreviewSettings(c *ui.Context) { a.settingsView(c) }

// PreviewEdit unfolds the fields of the shares held.
func (a *App) PreviewEdit(on bool) { a.editing = on }

// PreviewGroup shows a group the user made, making it when it is new.
func (a *App) PreviewGroup(name string) {
	for _, g := range a.groups {
		if g.Name == name {
			a.SelectGroup(g.ID)
			return
		}
	}
	a.beginNewGroup()
	a.groupDraft = name
	a.CommitGroup()
}

// PreviewGroupEdit unfolds the field of a new group.
func (a *App) PreviewGroupEdit() {
	a.beginNewGroup()
	a.groupDraft = "关注"
}

// PreviewLogin sets the login item a preview shows.
func (a *App) PreviewLogin(on bool) { a.previewLogin = on }

// PreviewUpdate shows the strip of a newer version, as a check found one.
func (a *App) PreviewUpdate(version, notes, step string, notesShown bool) {
	s := updateFound
	switch step {
	case "fetching":
		s = updateFetching
	case "ready":
		s = updateReady
	}
	a.upd = updateState{step: s, version: version, notes: notes, notesShown: notesShown}
	if s == updateFetching {
		a.upd.progress = 0.42
	}
}
