package app

import "fundbar/internal/source"

// PreviewSearch fills the search field and its results, for drawing the
// window without the network, as tools/shot does.
func (a *App) PreviewSearch(query string, hits []source.Hit) {
	a.query = query
	a.hits = hits
}
