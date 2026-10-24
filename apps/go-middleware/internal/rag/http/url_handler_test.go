package http_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// urlBodyCeiling is the fetch ceiling the handler tests configure. It is small so
// an oversized upstream body stays cheap to serve; production passes
// STUDY_MAX_UPLOAD_BYTES.
const urlBodyCeiling = 1024

// urlFixtureBody is the text the fixture server serves. The word it carries is
// not in any other fixture, so a search that finds it found this document.
const (
	urlFixtureBody = "La orquestacion de servicios distribuidos es el tema de esta nota."
	urlFixtureWord = "orquestacion"
)

// allowEveryAddress is the injected policy the fetch tests need: the fixture
// servers live on 127.0.0.1, which the production rule refuses.
func allowEveryAddress(netip.Addr) error { return nil }

// newURLHandler builds the handler over a fake use case, so a test can assert
// what the fetch forwarded as well as what it answered.
func newURLHandler(useCase *fakeIngestTextUseCase) *httppkg.IngestURLHandler {
	return httppkg.NewIngestURLHandlerWithRule(useCase, urlBodyCeiling, allowEveryAddress)
}

// newStrictURLHandler builds the handler with the production address rule. Its
// refusals happen before any connection, so no test using it needs the network.
func newStrictURLHandler(useCase *fakeIngestTextUseCase) *httppkg.IngestURLHandler {
	return httppkg.NewIngestURLHandlerWithRule(useCase, urlBodyCeiling, httppkg.PublicAddressRule)
}

// urlPost builds a JSON POST of the URL-ingest endpoint.
func urlPost(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/rag/documents/url", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// urlPostFor builds the body from a title and a URL, which is what every test
// below actually needs.
func urlPostFor(t *testing.T, title, target string) *http.Request {
	t.Helper()
	body, err := json.Marshal(httppkg.IngestURLRequest{Title: title, URL: target})
	require.NoError(t, err)
	return urlPost(string(body))
}

// decodeError reads the error envelope a refusal carries.
func decodeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	return errResp.Error
}

// fixtureServer serves one payload with one content type, over loopback.
func fixtureServer(t *testing.T, contentType, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// redirectServer serves a chain of `hops` redirects from /0 and then the final
// handler, so the redirect bound is exercised at its exact edges.
func redirectServer(t *testing.T, hops int, final http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for i := range hops {
		next := fmt.Sprintf("/%d", i+1)
		mux.HandleFunc(fmt.Sprintf("/%d", i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, next, http.StatusFound)
		})
	}
	mux.HandleFunc(fmt.Sprintf("/%d", hops), final)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// T4/S4: only http and https are fetched. Every other scheme is the client's
// mistake and the message names the scheme that was refused, so a caller can fix
// the URL instead of guessing.
func TestIngestURLHandler_RefusesOtherSchemes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		url     string
		message string
	}{
		{"file", "file:///etc/passwd", `the URL scheme "file" is not allowed`},
		{"ftp", "ftp://example.com/notes.md", `the URL scheme "ftp" is not allowed`},
		{"javascript", "javascript:alert(1)", `the URL scheme "javascript" is not allowed`},
		{"data", "data:text/plain,hola", `the URL scheme "data" is not allowed`},
		{"no scheme", "example.com/notes.md", "the URL has no scheme"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useCase := &fakeIngestTextUseCase{}

			rec := httptest.NewRecorder()
			newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", tc.url))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, 0, useCase.calls, "a refused URL never reaches the corpus")
			assert.Contains(t, decodeError(t, rec), tc.message)
		})
	}
}

// T4/S4: the production rule refuses loopback, link-local and literal-private
// targets, and it does so before any connection is opened. This is the rule the
// whole endpoint exists to protect: without it, the endpoint is a way to read the
// machine the server runs on.
func TestIngestURLHandler_RefusesBlockedHosts(t *testing.T) {
	for _, target := range []string{
		"http://localhost:8099/api/resources",
		"http://127.0.0.1:8099/api/resources",
		"http://[::1]:8099/api/resources",
		"http://169.254.169.254/latest/meta-data/",
	} {
		t.Run(target, func(t *testing.T) {
			useCase := &fakeIngestTextUseCase{}

			rec := httptest.NewRecorder()
			newStrictURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", target))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, 0, useCase.calls)
			assert.Contains(t, decodeError(t, rec), "an address the server will not fetch")
		})
	}
}

// T4/S4: a redirect is not a way around the address rule. Every hop is validated
// before it is fetched, so a public URL that redirects to a private one is
// refused and the message names the target.
func TestIngestURLHandler_RefusesARedirectToABlockedTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	// The fixture lives on loopback, so loopback is allowed for this test and the
	// metadata address is not: exactly the shape of a public page redirecting
	// into the host's own network.
	rule := func(addr netip.Addr) error {
		if addr == netip.MustParseAddr("169.254.169.254") {
			return errors.New("metadata service")
		}
		return nil
	}
	useCase := &fakeIngestTextUseCase{}
	handler := httppkg.NewIngestURLHandlerWithRule(useCase, urlBodyCeiling, rule)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, urlPostFor(t, "", server.URL))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, useCase.calls)
	message := decodeError(t, rec)
	assert.Contains(t, message, "169.254.169.254", "the refusal names the target it refused")
	assert.Contains(t, message, "an address the server will not fetch")
}

// T4/S4: three redirects are followed, four are refused. The bound exists so a
// redirect loop cannot hold a request slot until the deadline.
func TestIngestURLHandler_FollowsAtMostThreeRedirects(t *testing.T) {
	final := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(urlFixtureBody))
	}

	t.Run("three redirects", func(t *testing.T) {
		server := redirectServer(t, 3, final)
		useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{DocumentID: "doc-1"}}

		rec := httptest.NewRecorder()
		newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "Notes", server.URL+"/0"))

		require.Equal(t, http.StatusCreated, rec.Code)
		require.Equal(t, 1, useCase.calls)
		assert.Equal(t, urlFixtureBody, useCase.got.Text)
	})

	t.Run("four redirects", func(t *testing.T) {
		server := redirectServer(t, 4, final)
		useCase := &fakeIngestTextUseCase{}

		rec := httptest.NewRecorder()
		newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "Notes", server.URL+"/0"))

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, 0, useCase.calls)
		assert.Contains(t, decodeError(t, rec), "redirects")
	})
}

// T4/S4: only textual payloads are ingested, and the refusal says what to do
// with the ones this server cannot read.
func TestIngestURLHandler_RefusesNonTextPayloads(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		message     string
	}{
		{"pdf", "application/pdf", "upload it through POST /api/rag/documents"},
		{"html", "text/html; charset=utf-8", "an HTML page is not supported yet"},
		{"image", "image/png", "upload it through POST /api/rag/documents"},
		{"no content type", "", "upload it through POST /api/rag/documents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := fixtureServer(t, tc.contentType, "%PDF-1.7")
			useCase := &fakeIngestTextUseCase{}

			rec := httptest.NewRecorder()
			newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", server.URL))

			require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
			assert.Equal(t, 0, useCase.calls)
			assert.Contains(t, decodeError(t, rec), tc.message)
		})
	}
}

// T4/S4: the charset parameter is not a different type. A markdown or JSON
// response that declares one is still readable.
func TestIngestURLHandler_AcceptsCharsetParameters(t *testing.T) {
	for _, contentType := range []string{
		"text/plain; charset=utf-8",
		"text/markdown; charset=UTF-8",
		"application/json",
	} {
		t.Run(contentType, func(t *testing.T) {
			server := fixtureServer(t, contentType, `{"note":"`+urlFixtureBody+`"}`)
			useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{DocumentID: "doc-1"}}

			rec := httptest.NewRecorder()
			newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", server.URL))

			require.Equal(t, http.StatusCreated, rec.Code)
			require.Equal(t, 1, useCase.calls)
		})
	}
}

// T4/S4: an upstream that answers badly is the upstream's fault, not the
// client's, so it is a bad gateway and the status travels with it.
func TestIngestURLHandler_NonSuccessStatusIsBadGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such note", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	useCase := &fakeIngestTextUseCase{}
	rec := httptest.NewRecorder()
	newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", server.URL))

	require.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Equal(t, 0, useCase.calls)
	message := decodeError(t, rec)
	assert.Contains(t, message, "404")
	assert.Contains(t, message, "Not Found")
}

// T4/S4: the corpus stores text, so a body that is not UTF-8 is refused instead
// of being stored as replacement characters nobody can search.
func TestIngestURLHandler_NonUTF8BodyIsBadGateway(t *testing.T) {
	server := fixtureServer(t, "text/plain", "hola \xff\xfe mundo")
	useCase := &fakeIngestTextUseCase{}

	rec := httptest.NewRecorder()
	newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", server.URL))

	require.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Equal(t, 0, useCase.calls)
	assert.Contains(t, decodeError(t, rec), "UTF-8")
}

// T4/S4: the download is bounded by the same ceiling the artifact store applies,
// and the overflow is answered with 413 so the caller knows the material is too
// large for this corpus rather than that the URL was wrong.
func TestIngestURLHandler_OversizedBodyIsRequestEntityTooLarge(t *testing.T) {
	server := fixtureServer(t, "text/plain", strings.Repeat("a", urlBodyCeiling+1))
	useCase := &fakeIngestTextUseCase{}

	rec := httptest.NewRecorder()
	newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", server.URL))

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, 0, useCase.calls)
	assert.Contains(t, decodeError(t, rec), "1024")
}

// T4/S4: the title is the caller's when they gave one, and otherwise the last
// path segment, and otherwise the host. A corpus list must never show a nameless
// row, so the fallback is part of the contract.
func TestIngestURLHandler_ChoosesTheTitle(t *testing.T) {
	for _, tc := range []struct {
		name          string
		path          string
		requestTitle  string
		expectedTitle string
	}{
		{"the request title wins", "/notas/atencion.md", "Apuntes", "Apuntes"},
		{"falls back to the last path segment", "/notas/atencion.md", "", "atencion.md"},
		{"falls back to the host", "/", "", "127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := fixtureServer(t, "text/plain", urlFixtureBody)
			useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{DocumentID: "doc-1"}}

			rec := httptest.NewRecorder()
			newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, tc.requestTitle, server.URL+tc.path))

			require.Equal(t, http.StatusCreated, rec.Code)
			require.Equal(t, 1, useCase.calls)
			assert.Equal(t, tc.expectedTitle, useCase.got.Title)
			assert.Equal(t, server.URL+tc.path, useCase.got.SourceURI)
		})
	}
}

// T4/S4: the recorded origin is the URL that was finally fetched, not the one the
// caller typed, so the corpus records where the bytes actually came from.
func TestIngestURLHandler_RecordsTheURLAfterRedirects(t *testing.T) {
	server := redirectServer(t, 1, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(urlFixtureBody))
	})
	useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{DocumentID: "doc-1"}}

	rec := httptest.NewRecorder()
	newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "Notes", server.URL+"/0"))

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, 1, useCase.calls)
	assert.Equal(t, server.URL+"/1", useCase.got.SourceURI)
}

// T4/S4: a URL and a body are the only required inputs.
func TestIngestURLHandler_RequiresAURL(t *testing.T) {
	for _, body := range []string{
		`{"title":"Notes"}`,
		`{"title":"Notes","url":"   "}`,
	} {
		useCase := &fakeIngestTextUseCase{}

		rec := httptest.NewRecorder()
		newURLHandler(useCase).ServeHTTP(rec, urlPost(body))

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, 0, useCase.calls)
		assert.Equal(t, "url is required", decodeError(t, rec))
	}
}

// T4/S4: the title bound is the one the text route applies, so the two creation
// paths cannot disagree about how long a name may be.
func TestIngestURLHandler_RejectsATitleAboveTheBound(t *testing.T) {
	useCase := &fakeIngestTextUseCase{}
	long := strings.Repeat("a", httppkg.MaxTextDocumentTitleRunes+1)

	rec := httptest.NewRecorder()
	newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, long, "http://example.com/notes.md"))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, useCase.calls)
	assert.Equal(t, "title must be at most 200 characters", decodeError(t, rec))
}

// T4/S4: the corpus is the authority on identity, so a text it already holds is a
// duplicate here too — with the same message the text route answers, because it
// is the same refusal.
func TestIngestURLHandler_DuplicateContentIsAConflict(t *testing.T) {
	useCase := &fakeIngestTextUseCase{err: apperror.ErrConflict}
	server := fixtureServer(t, "text/plain", urlFixtureBody)

	rec := httptest.NewRecorder()
	newURLHandler(useCase).ServeHTTP(rec, urlPostFor(t, "", server.URL))

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, 1, useCase.calls)
	assert.Equal(t, duplicateTextMessage(), decodeError(t, rec))
}

// T4/S4: the whole path, once, against the real corpus: the URL is fetched, the
// document is listed with its origin recorded, and a word from the body finds it
// by search. This is the test that says the endpoint creates a document rather
// than only returning 201.
func TestIngestURLHandler_WritesASearchableDocument(t *testing.T) {
	corpus := newURLCorpus(t)
	server := fixtureServer(t, "text/plain; charset=utf-8", urlFixtureBody)

	handler := httppkg.NewIngestURLHandlerWithRule(corpus.ingest, urlBodyCeiling, allowEveryAddress)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, urlPostFor(t, "Notas de la web", server.URL+"/notas.md"))

	require.Equal(t, http.StatusCreated, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.ElementsMatch(t,
		[]string{"document_id", "state", "chunks", "embedded", "source_uri"}, keysOf(raw))

	var resp httppkg.IngestURLResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.DocumentID)
	assert.Positive(t, resp.Chunks, "the fetched text is chunked")
	assert.Zero(t, resp.Embedded, "no embedding client is wired in this test")
	assert.Equal(t, server.URL+"/notas.md", resp.SourceURI)

	document, err := corpus.documents.FindByID(t.Context(), resp.DocumentID)
	require.NoError(t, err)
	assert.Equal(t, "Notas de la web", document.Title)
	assert.Equal(t, server.URL+"/notas.md", document.SourceURI,
		"the document records where its bytes were read from")

	summaries, err := corpus.list.Execute(t.Context())
	require.NoError(t, err)
	require.Len(t, summaries, 1, "the fetched document is listed in the corpus")
	assert.Equal(t, resp.DocumentID, summaries[0].ID)
	assert.Equal(t, "Notas de la web", summaries[0].Title)

	results, err := corpus.search.Search(t.Context(), urlFixtureWord, 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "a word the fetched body carries finds its chunk")
	assert.Equal(t, resp.DocumentID, results[0].DocumentID)
}

// T4/S4 — a URL's credentials are used for the fetch and never persisted.
//
// The fixture only answers a request that carries them, so this proves both
// halves at once: the fetch still authenticates, and the corpus row and the
// response that name the material do not carry the password.
func TestIngestURLHandler_NeverPersistsURLCredentials(t *testing.T) {
	corpus := newURLCorpus(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "ana" || pass != "secreto" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(urlFixtureBody))
	}))
	defer server.Close()

	handler := httppkg.NewIngestURLHandlerWithRule(corpus.ingest, urlBodyCeiling, allowEveryAddress)

	withCredentials := strings.Replace(server.URL, "http://", "http://ana:secreto@", 1)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, urlPostFor(t, "", withCredentials+"/notas.md"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var resp httppkg.IngestURLResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, server.URL+"/notas.md", resp.SourceURI,
		"the response names the request without its credentials")

	document, err := corpus.documents.FindByID(t.Context(), resp.DocumentID)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/notas.md", document.SourceURI,
		"the stored origin never carries a password")
}

// T4/S4: the same URL twice is the same material twice, which the content hash
// already decides. The second answer is a 409 and the corpus keeps one document.
func TestIngestURLHandler_PostingTheSameURLTwiceIsAConflict(t *testing.T) {
	corpus := newURLCorpus(t)
	server := fixtureServer(t, "text/plain", urlFixtureBody)
	handler := httppkg.NewIngestURLHandlerWithRule(corpus.ingest, urlBodyCeiling, allowEveryAddress)

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, urlPostFor(t, "Notas", server.URL+"/notas.md"))
	require.Equal(t, http.StatusCreated, first.Code)

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, urlPostFor(t, "Notas again", server.URL+"/notas.md"))

	require.Equal(t, http.StatusConflict, second.Code)
	assert.Equal(t, duplicateTextMessage(), decodeError(t, second))

	summaries, err := corpus.list.Execute(t.Context())
	require.NoError(t, err)
	assert.Len(t, summaries, 1, "the refused second fetch creates no document")
}

// The route is mounted next to the text route, so the source of the material is
// one path segment apart on the same collection.
func TestRouter_RegistersTheURLIngestRoute(t *testing.T) {
	useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{DocumentID: "doc-1", Chunks: 1}}
	server := fixtureServer(t, "text/plain", urlFixtureBody)

	router := httppkg.NewRouter(
		&mockSearchUseCase{results: []domain.SearchResult{}},
		httppkg.NewUploadDocumentHandler(nil, nil, nil, false),
		httppkg.NewDocumentStatusHandler(nil),
		httppkg.NewDocumentsHandler(&fakeDocumentListUseCase{}),
		newTextHandler(&fakeIngestTextUseCase{}),
		newURLHandler(useCase),
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, urlPostFor(t, "Notes", server.URL+"/notas.md"))

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, 1, useCase.calls)
}

// duplicateTextMessage is the refusal both creation routes answer for material
// the corpus already holds. It is asserted here as the exact text a client sees.
func duplicateTextMessage() string {
	return "a document with the same text already exists"
}

// urlCorpus wires the real repositories, the real store, and the real read paths,
// so the end-to-end test goes through the same code the server runs.
type urlCorpus struct {
	documents *ragsqlite.DocumentRepository
	ingest    *app.IngestTextUseCase
	list      *app.ListDocumentsUseCase
	search    *app.SearchUseCase
}

func newURLCorpus(t *testing.T) *urlCorpus {
	t.Helper()
	ctx := t.Context()

	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, sqlDB) })

	require.NoError(t, db.Migrate(ctx, sqlDB,
		slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())),
		"the ingest runs over the schema the composition root applies")

	store, err := storage.New(t.TempDir(), 1<<20)
	require.NoError(t, err)

	documents := ragsqlite.NewDocumentRepository(sqlDB)
	jobs := ragsqlite.NewJobRepository(sqlDB)
	pages := ragsqlite.NewPageOutcomeRepository(sqlDB)
	chunks := ragsqlite.NewChunkRepository(sqlDB)
	indexer := app.NewIndexDocumentUseCase(documents, jobs, pages, chunks)

	return &urlCorpus{
		documents: documents,
		ingest:    app.NewIngestTextUseCase(documents, jobs, pages, store, indexer),
		list:      app.NewListDocumentsUseCase(documents, pages, chunks),
		search:    app.NewSearchUseCase(chunks),
	}
}

func closeDB(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	require.NoError(t, sqlDB.Close())
}
