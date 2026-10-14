package web

import (
	"embed"
	"io/fs"
)

// staticFS embeds the whole web UI (index.html, app.js, style.css).
//
//go:embed all:static
var staticFS embed.FS

// uiFS returns the embedded static files rooted at the UI root.
func uiFS() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
