package spa

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// The rules below are the handler's contract, so they are tested against a tree
// this file controls: a test that read the real `dist/` would change meaning
// every time the app is rebuilt.
func testTree() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><div id=app></div>")},
		"assets/index-abc.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc.css": {Data: []byte("body{}")},
	}
}

func request(t *testing.T, tree fs.FS, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	New(tree).ServeHTTP(recorder, httptest.NewRequest(method, target, nil))

	return recorder
}

func TestServesTheAppAtTheRoot(t *testing.T) {
	response := request(t, testTree(), http.MethodGet, "/")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("content-type = %q, want text/html", got)
	}
	if got := response.Body.String(); !strings.Contains(got, `id=app`) {
		t.Errorf("body = %q, want the app document", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("cache-control = %q, want no-cache: the document names hashed assets", got)
	}
}

func TestServesHashedAssetsAsImmutable(t *testing.T) {
	response := request(t, testTree(), http.MethodGet, "/assets/index-abc.js")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Body.String(); got != "console.log(1)" {
		t.Errorf("body = %q, want the asset bytes", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Errorf("content-type = %q, want a javascript type", got)
	}
	if got := response.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("cache-control = %q, want immutable: the name is the content hash", got)
	}
}

func TestAnswersAClientRouteWithTheAppDocument(t *testing.T) {
	for _, target := range []string{
		"/topics",
		"/topic/abc/concept/def",
		"/present/deck-atencion",
		"/assets/unknown-file.js",
	} {
		t.Run(target, func(t *testing.T) {
			response := request(t, testTree(), http.MethodGet, target)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			if got := response.Body.String(); !strings.Contains(got, `id=app`) {
				t.Errorf("body = %q, want the app document", got)
			}
		})
	}
}

func TestNeverAnswersTheAPIPrefixWithTheAppDocument(t *testing.T) {
	// The API's own router handles its routes; this handler only sees the ones it
	// has no match for, and turning those into a 200 page would silently break
	// every client that checks a status code.
	for _, target := range []string{"/api/resources", "/api/rag/documents", "/api/"} {
		t.Run(target, func(t *testing.T) {
			response := request(t, testTree(), http.MethodGet, target)

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", response.Code)
			}
			if strings.Contains(response.Body.String(), "id=app") {
				t.Error("body contains the app document")
			}
		})
	}
}

func TestRejectsWriteMethods(t *testing.T) {
	response := request(t, testTree(), http.MethodPost, "/topic/abc")

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
	if got := response.Header().Get("Allow"); got != "GET, HEAD" {
		t.Errorf("allow = %q, want %q", got, "GET, HEAD")
	}
}

func TestAnswersHEADWithoutABody(t *testing.T) {
	response := request(t, testTree(), http.MethodHead, "/")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Errorf("body has %d bytes, want none", response.Body.Len())
	}
	if got := response.Header().Get("Content-Length"); got == "" {
		t.Error("content-length is missing, want the document's size")
	}
}

func TestCannotReadOutsideTheEmbeddedTree(t *testing.T) {
	tree := testTree()
	tree["../secret"] = &fstest.MapFile{Data: []byte("secret")}

	for _, target := range []string{"/../secret", "/../../secret", "/assets/../../secret"} {
		t.Run(target, func(t *testing.T) {
			response := request(t, tree, http.MethodGet, target)

			if got := response.Body.String(); strings.Contains(got, "secret") {
				t.Errorf("body = %q, want the app document, never a file above the tree", got)
			}
		})
	}
}

func TestReportsAMissingAppDocumentAsAnIncompleteBuild(t *testing.T) {
	// A tree without `index.html` is a build mistake, not a missing route: a 404
	// would send the reader looking for a URL that is actually fine.
	tree := fstest.MapFS{"assets/index-abc.js": {Data: []byte("console.log(1)")}}

	response := request(t, tree, http.MethodGet, "/topics")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if got := response.Body.String(); !strings.Contains(got, "sync-web.sh") {
		t.Errorf("body = %q, want the step that produces the missing file", got)
	}
}

func TestTheEmbeddedTreeCarriesTheAppDocument(t *testing.T) {
	// The tree committed in this package is what the binary serves. Without this
	// check, an empty or stale `dist/` would only be visible as a blank page at
	// runtime.
	info, err := fs.Stat(App, "index.html")
	if err != nil {
		t.Fatalf("stat index.html in the embedded tree: %v", err)
	}
	if info.Size() == 0 {
		t.Error("index.html is empty in the embedded tree")
	}
}
