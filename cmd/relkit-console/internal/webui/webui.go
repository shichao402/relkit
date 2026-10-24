// Package webui embeds the built operator console front end.
//
// The console binary ships as one file, so the front end cannot be a directory
// next to it: `go build` has to produce everything a box needs. `npm run build`
// in cmd/relkit-console/web writes into dist/, which this package embeds, and
// the release pipeline rebuilds it before compiling so the shipped bytes always
// match the sources in the same commit.
//
// dist/ is committed on purpose. A Go-only checkout must stay buildable: the
// embed directive fails the whole build when the directory is missing, and
// requiring Node to build a Go binary would break `go install` and the Rust and
// Python SDK jobs.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Assets is the built front end rooted at the index.html it contains.
func Assets() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		// The embedded tree is a compile-time constant: a missing subdirectory
		// is a build bug, not a runtime condition.
		panic(err)
	}
	return sub
}
