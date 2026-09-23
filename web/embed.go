// Package web embeds the built frontend so the server ships as a single binary.
package web

import (
	"embed"
	"io/fs"
)

// dist holds the compiled Vite build (web/dist). Build it with `make build`
// before compiling the Go binary; a placeholder file keeps this compiling
// even when the frontend has not been built yet.
//
//go:embed all:dist
var dist embed.FS

// FS returns the embedded frontend assets rooted at dist/.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
