package http_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// The tests in this file are the ones that fail when a guarantee is removed
// rather than merely restated. Each was checked against the mutation it guards:
// dropping the dialer's Control, and widening the production rule, both make
// them fail.

const urlFetchCeiling = 1024

type urlFetchUseCase struct {
	mu    sync.Mutex
	calls int
	got   app.IngestTextRequest
}

func (u *urlFetchUseCase) Execute(_ context.Context, req app.IngestTextRequest) (app.IngestTextReport, error) {
	u.mu.Lock()
	u.calls++
	u.got = req
	u.mu.Unlock()
	return app.IngestTextReport{DocumentID: "doc-fetched", Chunks: 1}, nil
}

func urlFetchPost(t *testing.T, target string) *http.Request {
	t.Helper()
	b, err := json.Marshal(httppkg.IngestURLRequest{URL: target})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/rag/documents/url", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func urlFetchServe(t *testing.T, h *httppkg.IngestURLHandler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, urlFetchPost(t, target))
	return rec
}

func urlFetchError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &e) != nil {
		return rec.Body.String()
	}
	return e.Error
}

// loopbackOnlyRule applies the production rule to everything except the test
// fixtures on 127.0.0.1, which production refuses and which every fixture needs.
func loopbackOnlyRule(addr netip.Addr) error {
	if addr.Unmap() == netip.MustParseAddr("127.0.0.1") {
		return nil
	}
	return httppkg.PublicAddressRule(addr)
}

// T4/S4 — the constructor production code calls carries the production rule, so
// no wiring can ship a fetch that reaches the machine it runs on. This is the
// test that fails if the two constructors were ever merged back into one that
// takes an optional rule.
func TestNewIngestURLHandler_InstallsTheProductionRule(t *testing.T) {
	useCase := &urlFetchUseCase{}

	rec := urlFetchServe(t, httppkg.NewIngestURLHandler(useCase, urlFetchCeiling), "http://127.0.0.1:1/nota.md")

	require.Equal(t, http.StatusBadRequest, rec.Code, urlFetchError(t, rec))
	require.Contains(t, urlFetchError(t, rec), "an address the server will not fetch")
	require.Zero(t, useCase.calls, "a refused target never reaches the corpus")
}

// miniDNS is a one-A-record DNS responder: the first A query answers public, every
// later A query answers private, AAAA queries answer NOERROR with no record. It is
// what makes a real rebinding test possible without touching the network: the
// pre-flight check and the dialer ask the same name twice and get two answers.
type miniDNS struct {
	conn    *net.UDPConn
	public  netip.Addr
	private netip.Addr
	queries atomic.Int32
	mu      sync.Mutex
	log     []string
}

func newMiniDNS(t *testing.T, public, private netip.Addr) *miniDNS {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	d := &miniDNS{conn: conn, public: public, private: private}
	go d.serve()
	t.Cleanup(func() { _ = conn.Close() })
	return d
}

func (d *miniDNS) serve() {
	buf := make([]byte, 4096)
	for {
		n, addr, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if resp := d.answer(append([]byte(nil), buf[:n]...)); resp != nil {
			_, _ = d.conn.WriteToUDP(resp, addr)
		}
	}
}

func (d *miniDNS) answer(q []byte) []byte {
	if len(q) < 12 {
		return nil
	}
	i := 12
	var labels []string
	for i < len(q) && q[i] != 0 {
		l := int(q[i])
		if i+1+l > len(q) {
			return nil
		}
		labels = append(labels, string(q[i+1:i+1+l]))
		i += 1 + l
	}
	if i+4 >= len(q) {
		return nil
	}
	qEnd := i + 5
	qtype := binary.BigEndian.Uint16(q[i+1 : i+3])

	out := []byte{q[0], q[1], 0x81, 0x80, 0x00, 0x01}
	var ip netip.Addr
	if qtype == 1 {
		out = append(out, 0x00, 0x01)
		if d.queries.Add(1) == 1 {
			ip = d.public
		} else {
			ip = d.private
		}
	} else {
		out = append(out, 0x00, 0x00)
	}
	out = append(out, 0x00, 0x00, 0x00, 0x00)
	out = append(out, q[12:qEnd]...)
	d.mu.Lock()
	d.log = append(d.log, fmt.Sprintf("%s type=%d -> %v", strings.Join(labels, "."), qtype, ip))
	d.mu.Unlock()
	if qtype == 1 {
		b := ip.As4()
		out = append(out, 0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04)
		out = append(out, b[:]...)
	}
	return out
}

func (d *miniDNS) resolver() *net.Resolver {
	addr := d.conn.LocalAddr().String()
	return &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return net.Dial("udp", addr)
		},
	}
}

// T4/S4 — DNS rebinding is refused at the dial, not at the pre-flight check: the
// name answers a public address when it is validated and loopback when the
// connection is opened. The permissive control run proves the fake resolver
// really lands on the fixture, so the strict run's refusal is the rule and not a
// broken name.
func TestURLFetch_DNSRebindingIsRefusedAtTheDial(t *testing.T) {
	var hits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("rebinded payload"))
	}))
	defer fixture.Close()
	port := fixture.Listener.Addr().(*net.TCPAddr).Port
	target := fmt.Sprintf("http://rebind.internal:%d/nota.md", port)

	run := func(rule httppkg.AddressRule) (int, string, int32) {
		dns := newMiniDNS(t, netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1"))
		prev := net.DefaultResolver
		net.DefaultResolver = dns.resolver()
		defer func() { net.DefaultResolver = prev }()

		h := httppkg.NewIngestURLHandlerWithRule(&urlFetchUseCase{}, urlFetchCeiling, rule)
		before := hits.Load()
		rec := urlFetchServe(t, h, target)
		time.Sleep(50 * time.Millisecond)
		return rec.Code, urlFetchError(t, rec), hits.Load() - before
	}

	t.Run("permissive control reaches the fixture", func(t *testing.T) {
		code, msg, newHits := run(func(netip.Addr) error { return nil })
		require.Equal(t, http.StatusCreated, code, msg)
		require.Equal(t, int32(1), newHits)
	})

	t.Run("production rule refuses the second answer", func(t *testing.T) {
		code, msg, newHits := run(httppkg.PublicAddressRule)
		require.Equal(t, http.StatusBadRequest, code, msg)
		require.Zero(t, newHits, "the connection to the rebound address was never opened")
		require.Contains(t, msg, "refused when the connection was opened")
	})
}

// T4/S4 — the address control is installed on the fetch client and its refusal
// aborts the connection. A rule that allows the pre-flight answers and refuses at
// connect time must fail the fetch, must not reach the corpus, and must not touch
// the upstream. This is the test that fails if Control were dropped from
// newFetchClient or its error were logged instead of returned.
func TestURLFetch_DialTimeRefusalAbortsTheConnection(t *testing.T) {
	var hits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("payload"))
	}))
	defer fixture.Close()
	port := fixture.Listener.Addr().(*net.TCPAddr).Port
	target := fmt.Sprintf("http://localhost:%d/notas.md", port)

	run := func(allowFirst int) (*httptest.ResponseRecorder, *urlFetchUseCase) {
		var mu sync.Mutex
		seen := 0
		rule := func(addr netip.Addr) error {
			mu.Lock()
			n := seen
			seen++
			mu.Unlock()
			if n < allowFirst {
				return nil
			}
			return fmt.Errorf("refused by the probe rule: %s", addr)
		}
		uc := &urlFetchUseCase{}
		rec := urlFetchServe(t, httppkg.NewIngestURLHandlerWithRule(uc, urlFetchCeiling, rule), target)
		return rec, uc
	}

	rec, uc := run(1 << 10)
	require.Equal(t, http.StatusCreated, rec.Code, urlFetchError(t, rec))
	require.Equal(t, int32(1), hits.Load())
	require.Equal(t, 1, uc.calls)

	before := hits.Load()
	rec, uc = run(2)
	// The probe rule's error is not marked as an address refusal the way
	// PublicAddressRule's is, so it surfaces as an upstream failure; the rebinding
	// test covers the 400 mapping with the production rule. What matters here is
	// that the dial was refused by the rule at all: without Control installed the
	// connection would have succeeded and this would be a 201.
	require.Equal(t, http.StatusBadGateway, rec.Code, urlFetchError(t, rec))
	require.Contains(t, urlFetchError(t, rec), "refused by the probe rule")
	require.Zero(t, uc.calls, "a dial-time refusal never reaches the corpus")
	require.Equal(t, before, hits.Load(), "the connection was refused before it was opened")
}

// T4/S4 — the response header block is capped at one mebibyte by the transport,
// so an origin cannot turn a text fetch into a memory problem. 900 KiB still
// works; 2 MiB is refused as an upstream failure.
func TestURLFetch_CapsTheResponseHeaderBlock(t *testing.T) {
	h := httppkg.NewIngestURLHandlerWithRule(&urlFetchUseCase{}, urlFetchCeiling, loopbackOnlyRule)

	serve := func(size int) *httptest.ResponseRecorder {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("X-Big", strings.Repeat("h", size))
			_, _ = w.Write([]byte("fine"))
		}))
		defer srv.Close()
		return urlFetchServe(t, h, srv.URL+"/x")
	}

	under := serve(900 * 1024)
	require.Equal(t, http.StatusCreated, under.Code, urlFetchError(t, under))

	over := serve(2 * 1024 * 1024)
	require.Equal(t, http.StatusBadGateway, over.Code)
	require.Contains(t, urlFetchError(t, over), "response headers exceeded")
}

// T4/S4 — the ceiling applies to the bytes the corpus receives, so a compressed
// body that expands past it is 413 rather than an accepted short read: the
// transport decompresses before the handler's LimitReader sees the payload.
func TestURLFetch_GzipBombIsBounded(t *testing.T) {
	h := httppkg.NewIngestURLHandlerWithRule(&urlFetchUseCase{}, urlFetchCeiling, loopbackOnlyRule)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(strings.Repeat("z", 200000)))
	require.NoError(t, zw.Close())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	rec := urlFetchServe(t, h, srv.URL+"/x")
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, urlFetchError(t, rec))
}

// urlFetchCorpus is the real database, store and use case, so the side-effect
// assertions read what the server would actually have written.
type urlFetchCorpus struct {
	db     *sql.DB
	dir    string
	ingest *app.IngestTextUseCase
}

func newURLFetchCorpus(t *testing.T) *urlFetchCorpus {
	t.Helper()
	ctx := t.Context()
	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, sqlDB, slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())))
	t.Cleanup(func() { _ = sqlDB.Close() })

	dir := t.TempDir()
	store, err := storage.New(dir, 1<<20)
	require.NoError(t, err)
	documents := ragsqlite.NewDocumentRepository(sqlDB)
	jobs := ragsqlite.NewJobRepository(sqlDB)
	pages := ragsqlite.NewPageOutcomeRepository(sqlDB)
	chunks := ragsqlite.NewChunkRepository(sqlDB)
	return &urlFetchCorpus{
		db:     sqlDB,
		dir:    dir,
		ingest: app.NewIngestTextUseCase(documents, jobs, pages, store, app.NewIndexDocumentUseCase(documents, jobs, pages, chunks)),
	}
}

func (c *urlFetchCorpus) rowsAndFiles(t *testing.T) (int, int, int, int, int) {
	t.Helper()
	var docs, jobs, pages, chunks int
	require.NoError(t, c.db.QueryRow(`SELECT count(*) FROM documents`).Scan(&docs))
	require.NoError(t, c.db.QueryRow(`SELECT count(*) FROM ingestion_jobs`).Scan(&jobs))
	require.NoError(t, c.db.QueryRow(`SELECT count(*) FROM job_pages`).Scan(&pages))
	require.NoError(t, c.db.QueryRow(`SELECT count(*) FROM chunks`).Scan(&chunks))
	entries, err := os.ReadDir(c.dir)
	require.NoError(t, err)
	files := 0
	for _, e := range entries {
		if e.IsDir() {
			sub, _ := os.ReadDir(c.dir + "/" + e.Name())
			files += len(sub)
			continue
		}
		files++
	}
	return docs, jobs, pages, chunks, files
}

// T4/S4 — a refused fetch leaves no document, job, page, chunk or artifact
// behind, and an accepted fetch records the URL it finally read as the origin of
// both the response and the stored document.
func TestURLFetch_RefusalsLeaveNothingAndOriginIsTheFinalURL(t *testing.T) {
	corpus := newURLFetchCorpus(t)
	h := httppkg.NewIngestURLHandlerWithRule(corpus.ingest, urlFetchCeiling, loopbackOnlyRule)

	require.Equal(t, [5]int{0, 0, 0, 0, 0}, [5]int(mustCounts(t, corpus)))

	for name, handler := range map[string]http.HandlerFunc{
		"unsupported type": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.7"))
		},
		"too large": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("a", urlFetchCeiling+1)))
		},
		"not utf-8": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("\xff\xfe"))
		},
		"upstream 500": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusInternalServerError)
		},
	} {
		srv := httptest.NewServer(handler)
		rec := urlFetchServe(t, h, srv.URL+"/doc")
		srv.Close()
		require.NotEqual(t, http.StatusCreated, rec.Code, name)
		require.Equal(t, [5]int{0, 0, 0, 0, 0}, [5]int(mustCounts(t, corpus)), name+" left state behind")
	}

	blocked := urlFetchServe(t, h, "http://169.254.169.254/latest/meta-data/")
	require.Equal(t, http.StatusBadRequest, blocked.Code)
	require.Equal(t, [5]int{0, 0, 0, 0, 0}, [5]int(mustCounts(t, corpus)), "a blocked target left state behind")

	mux := http.NewServeMux()
	mux.HandleFunc("/doc", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final/nota.md", http.StatusFound)
	})
	mux.HandleFunc("/final/nota.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("contenido aceptado de la nota"))
	})
	ok := httptest.NewServer(mux)
	defer ok.Close()

	created := urlFetchServe(t, h, ok.URL+"/doc")
	require.Equal(t, http.StatusCreated, created.Code, urlFetchError(t, created))

	var body struct {
		SourceURI string `json:"source_uri"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &body))
	require.Equal(t, ok.URL+"/final/nota.md", body.SourceURI)

	var storedSource, title string
	require.NoError(t, corpus.db.QueryRow(`SELECT source_uri, title FROM documents`).Scan(&storedSource, &title))
	require.Equal(t, ok.URL+"/final/nota.md", storedSource, "the stored origin is the final URL, not the one the caller typed")
	require.Equal(t, "nota.md", title)

	duplicate := urlFetchServe(t, h, ok.URL+"/doc")
	require.Equal(t, http.StatusConflict, duplicate.Code, urlFetchError(t, duplicate))
	require.Equal(t, [5]int{1, 1, 1, 1, 2}, [5]int(mustCounts(t, corpus)))
}

func mustCounts(t *testing.T, c *urlFetchCorpus) []int {
	t.Helper()
	docs, jobs, pages, chunks, files := c.rowsAndFiles(t)
	return []int{docs, jobs, pages, chunks, files}
}
