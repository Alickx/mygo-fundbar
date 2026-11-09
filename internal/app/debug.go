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

// PreviewLogin sets the login item a preview shows.
func (a *App) PreviewLogin(on bool) { a.previewLogin = on }
