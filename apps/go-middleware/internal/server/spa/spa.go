// Package spa serves the single-page application from an `fs.FS`.
//
// The app is client-routed: `/topic/abc/concept/def` is a route, not a file on
// disk, so a request for a path the filesystem does not hold has to answer with
// `index.html` and let the app resolve it. The handler owns that rule, the two
// cache policies it implies, and the boundary that keeps it away from `/api/`.
//
// It takes an `fs.FS` rather than reading the embedded directory itself, so the
// rules are tested against a fake tree instead of against whatever the last
// build produced.
package spa

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

// indexFile is the document every unmatched path answers with.
const indexFile = "index.html"

// assetsPrefix is where the bundler puts content-hashed files. A file under it
// can never change under the same name, so it is cached forever; everything
// else has to be revalidated.
const assetsPrefix = "/assets/"

// Handler serves one embedded SPA.
type Handler struct {
	dist fs.FS
}

// New returns the handler for the built app in dist.
//
// dist is the output of the web build (`apps/web/dist`), embedded by the caller.
// A tree without `index.html` is not rejected here: the failure belongs to the
// request, and the caller cannot act on it at startup any better than a reader
// can on a blank page.
func New(dist fs.FS) *Handler {
	return &Handler{dist: dist}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A deep link is a GET; a form post to a client route is not this handler's
	// business, and answering it with HTML would hide that from the caller.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clean first: a traversal attempt resolves to a path inside the tree instead
	// of escaping it, and `fs.FS` would refuse it anyway.
	cleaned := path.Clean("/" + r.URL.Path)

	// The API owns its prefix. This handler is mounted at "/", so it sees
	// `/api/...` whenever the API itself has no route; answering with the app
	// document would turn a missing endpoint into a 200 page. `path.Clean`
	// strips the trailing slash, so the bare prefix is compared too.
	if cleaned == "/api" || strings.HasPrefix(cleaned, "/api/") {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimPrefix(cleaned, "/")

	// A directory path, or the root, is the app itself.
	if name == "" || strings.HasSuffix(cleaned, "/") {
		h.serveIndex(w, r)
		return
	}

	// A file that exists is served as itself; anything else is a client route.
	if data, ok := h.read(name); ok {
		writeFile(w, r, name, data, strings.HasPrefix(cleaned, assetsPrefix))
		return
	}

	h.serveIndex(w, r)
}

// read returns the file's bytes, and false for a missing file or a directory.
func (h *Handler) read(name string) ([]byte, bool) {
	info, err := fs.Stat(h.dist, name)
	if err != nil || info.IsDir() {
		return nil, false
	}

	data, err := fs.ReadFile(h.dist, name)
	if err != nil {
		return nil, false
	}

	return data, true
}

// serveIndex answers with the app document, which must never be cached: the
// assets it names carry new hashes on every build.
func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, ok := h.read(indexFile)
	if !ok {
		// Not a 404: the route exists, the binary is incomplete. Say which step
		// produces the missing file instead of leaving a blank page.
		log.Printf("spa: %s is missing from the embedded tree", indexFile)
		http.Error(w, "the web app is not embedded in this binary: run scripts/sync-web.sh and rebuild", http.StatusInternalServerError)
		return
	}

	writeFile(w, r, indexFile, data, false)
}

// writeFile writes one file with the headers its role implies.
func writeFile(w http.ResponseWriter, r *http.Request, name string, data []byte, immutable bool) {
	if immutable {
		// A hashed name is the identity of its bytes.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	if name == indexFile {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}

	// ServeContent derives the content type from the extension, answers HEAD
	// without a body, and sets Content-Length — none of which this handler needs
	// to reimplement. A zero modtime means "no Last-Modified", which is right for
	// a file identified by its hash or revalidated every time.
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

// String makes a misconfigured tree legible in a log line.
func (h *Handler) String() string {
	var names []string

	_ = fs.WalkDir(h.dist, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() {
			names = append(names, name)
		}
		return nil
	})

	if len(names) > 8 {
		names = append(names[:8], fmt.Sprintf("… and %d more", len(names)-8))
	}

	return fmt.Sprintf("spa(%s)", strings.Join(names, ", "))
}
