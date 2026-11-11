// Package web embeds the built frontend.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets is the contents of web/dist. Before a frontend build it holds only
// .gitkeep, and the server serves a placeholder page.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
