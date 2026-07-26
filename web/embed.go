// Package web embeds the built admin UI into the nps binary and exposes it as
// an fs.FS rooted at the build output.
//
// web/dist is populated by `pnpm build` in web/ui. It is checked in only as an
// empty directory with a .gitkeep, so a clean checkout still compiles; the
// resulting binary then serves an explanatory 404 instead of a blank page (see
// web/spa.Handler.Available).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the built UI rooted at the dist directory, so that a request
// for /assets/x.js maps to the file assets/x.js.
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist is embedded above, so this cannot fail at runtime; failing loudly
		// beats silently serving nothing if that ever stops being true.
		panic("web: embedded dist directory is missing: " + err.Error())
	}
	return sub
}
