package spa

import (
	"embed"
	"io/fs"
)

// embedded is the build as it lands in the repository: the embed pattern keeps
// the `dist` directory in the paths, so the tree has to be re-rooted before use.
//
// The tree is generated, not authored: `scripts/sync-web.sh` builds `apps/web`
// and copies its output into `dist/`, and `//go:generate` runs the same script.
// It is committed because the binary has to build from a fresh clone —
// `go build` fails outright on an embed pattern with no matching files.
//
//go:generate ../../../scripts/sync-web.sh
//
//go:embed all:dist
var embedded embed.FS

// App is the built web app, rooted at the directory that holds `index.html`.
//
// `fs.Sub` runs once, on a path the compiler has already proved: the embed
// directive fails the build when `dist/` is missing, and `dist` is a valid
// subdirectory name, so the error branch is unreachable.
var App = mustSub(embedded, "dist")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("spa: embedded app tree: " + err.Error())
	}

	return sub
}
