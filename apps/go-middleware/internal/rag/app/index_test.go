package app_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// indexInstant is a fixed instant, so a test that pins the clock cannot pass on
// a machine whose zone or precision handling is wrong.
var indexInstant = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// indexHarness wires the real SQLite repositories over an in-memory database, so
// the use cases are exercised against the same schema the command applies.
type indexHarness struct {
	sqlDB    *sql.DB
	docs     domain.DocumentRepository
	jobs     domain.JobRepository
	pages    domain.PageOutcomeRepository
	chunks   domain.ChunkRepository
	indexer  *app.IndexDocumentUseCase
	searcher *app.SearchUseCase
}

func newIndexHarness(t *testing.T) *indexHarness {
	t.Helper()
	ctx := t.Context()

	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	require.NoError(t, db.Migrate(ctx, sqlDB,
		slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())),
		"indexing runs over the schema the composition root applies")

	h := &indexHarness{
		sqlDB:  sqlDB,
		docs:   ragsqlite.NewDocumentRepository(sqlDB),
		jobs:   ragsqlite.NewJobRepository(sqlDB),
		pages:  ragsqlite.NewPageOutcomeRepository(sqlDB),
		chunks: ragsqlite.NewChunkRepository(sqlDB),
	}
	h.indexer = app.NewIndexDocumentUseCase(h.docs, h.jobs, h.pages, h.chunks)
	h.searcher = app.NewSearchUseCase(h.chunks)

	return h
}

// seedRecordedPages creates a document with a job and records the given page
// texts, which is what a settled ingestion run leaves behind.
func (h *indexHarness) seedRecordedPages(t *testing.T, title string, pages map[int]string) *domain.Document {
	t.Helper()
	ctx := t.Context()

	document := &domain.Document{
		ID:            uuid.NewString(),
		Title:         title,
		ArtifactID:    uuid.NewString(),
		SourceURI:     "/books/" + title + ".pdf",
		ContentSHA256: sha256Hex([]byte(title)),
		CreatedAt:     indexInstant,
	}
	require.NoError(t, h.docs.Create(ctx, document))

	total := 0
	for page := range pages {
		if page > total {
			total = page
		}
	}

	job, err := domain.NewJob(uuid.NewString(), document.ID, total, indexInstant)
	require.NoError(t, err)
	require.NoError(t, h.jobs.Create(ctx, job))

	for page := 1; page <= total; page++ {
		require.NoError(t, h.pages.SaveOutcome(ctx, domain.PageOutcome{
			JobID:      job.ID,
			PageNumber: page,
			Status:     domain.PageExtracted,
			Method:     domain.MethodExtract,
			Text:       pages[page],
			UpdatedAt:  indexInstant,
		}))
	}

	return document
}

// seedDocumentWithoutPages creates a document nobody has converted.
func (h *indexHarness) seedDocumentWithoutPages(t *testing.T, title string) *domain.Document {
	t.Helper()

	document := &domain.Document{
		ID:            uuid.NewString(),
		Title:         title,
		ArtifactID:    uuid.NewString(),
		SourceURI:     "/books/" + title + ".pdf",
		ContentSHA256: sha256Hex([]byte(title)),
		CreatedAt:     indexInstant,
	}
	require.NoError(t, h.docs.Create(t.Context(), document))

	return document
}

// seedDocumentWithEmptyJob creates a document whose run recorded no page.
func (h *indexHarness) seedDocumentWithEmptyJob(t *testing.T, title string) *domain.Document {
	t.Helper()
	ctx := t.Context()

	document := h.seedDocumentWithoutPages(t, title)

	job, err := domain.NewJob(uuid.NewString(), document.ID, 10, indexInstant)
	require.NoError(t, err)
	require.NoError(t, h.jobs.Create(ctx, job))

	return document
}

func (h *indexHarness) chunkRows(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM chunks`).Scan(&count))
	return count
}

func (h *indexHarness) search(t *testing.T, query string) []domain.SearchResult {
	t.Helper()

	results, err := h.searcher.Search(t.Context(), query, app.DefaultSearchLimit)
	require.NoError(t, err)
	require.NotNil(t, results)

	return results
}

func TestIndexDocumentUseCase_IndexesRecordedPagesAndFindsThemByQuery(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "Domain-Driven Design", map[int]string{
		1: "el agregado es la raíz de la consistencia",
		2: "un repositorio guarda y recupera objetos",
	})

	report, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, document.ID, report.DocumentID)
	assert.NotEmpty(t, report.JobID)
	assert.Equal(t, 2, report.Chunks, "one tiny page yields one chunk")

	results := h.search(t, "agregado")
	require.Len(t, results, 1)
	assert.Equal(t, "Domain-Driven Design", results[0].DocumentTitle)
	assert.Equal(t, 1, results[0].PageNumber)
	assert.Contains(t, results[0].Snippet, "[agregado]")

	other := h.search(t, "repositorio")
	require.Len(t, other, 1)
	assert.Equal(t, 2, other[0].PageNumber)
}

func TestIndexDocumentUseCase_ReindexingDoesNotDuplicateResults(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "un libro", map[int]string{
		1: "el agregado es la raíz",
		2: "el repositorio guarda objetos",
	})

	first, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	second, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	assert.Equal(t, first.Chunks, second.Chunks)
	assert.Equal(t, 2, h.chunkRows(t), "a rebuild replaces rows instead of appending them")
	assert.Len(t, h.search(t, "repositorio"), 1, "a rebuilt index returns the same result once")
}

func TestIndexDocumentUseCase_FindsSpanishTextHoweverTheAccentsAreTyped(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	accented := h.seedRecordedPages(t, "accented", map[int]string{
		3: "el obstáculo del diseño no detiene el árbol",
	})
	unaccented := h.seedRecordedPages(t, "unaccented", map[int]string{
		4: "el obstaculo y el diseno",
	})

	_, err := h.indexer.Execute(ctx, accented.ID)
	require.NoError(t, err)
	_, err = h.indexer.Execute(ctx, unaccented.ID)
	require.NoError(t, err)

	found := h.search(t, "obstaculo diseno")
	assertPagesByTitle(t, found, map[string]int{"accented": 3, "unaccented": 4},
		"an unaccented query must find both the accented and the unaccented text")

	reverse := h.search(t, "obstáculo diseño")
	assertPagesByTitle(t, reverse, map[string]int{"accented": 3, "unaccented": 4},
		"an accented query must find the unaccented text too")
}

// assertPagesByTitle checks that the results are exactly the expected
// title-to-page pairs, so a missing document fails the way a wrong page does.
func assertPagesByTitle(t *testing.T, results []domain.SearchResult, expected map[string]int, message string) {
	t.Helper()

	pages := make(map[string]int, len(results))
	for _, result := range results {
		pages[result.DocumentTitle] = result.PageNumber
	}
	assert.Equal(t, expected, pages, message)
}

func TestIndexDocumentUseCase_DocumentWithoutRecordedPagesIndexesNothing(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	neverRun := h.seedDocumentWithoutPages(t, "never run")
	jobWithoutPages := h.seedDocumentWithEmptyJob(t, "job without pages")

	for name, document := range map[string]*domain.Document{
		"no job at all":    neverRun,
		"job with no page": jobWithoutPages,
	} {
		t.Run(name, func(t *testing.T) {
			report, err := h.indexer.Execute(ctx, document.ID)
			require.NoError(t, err, "a document with nothing to index is not an error")
			assert.Equal(t, 0, report.Chunks)
			assert.Equal(t, 0, h.chunkRows(t))
		})
	}

	assert.Empty(t, h.search(t, "agregado"))
}

func TestIndexDocumentUseCase_UnknownDocumentIsNotFound(t *testing.T) {
	h := newIndexHarness(t)

	_, err := h.indexer.Execute(t.Context(), uuid.NewString())
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestIndexDocumentUseCase_ReindexAllRebuildsEveryDocumentWithRecordedPages(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	h.seedRecordedPages(t, "alpha", map[int]string{1: "alpha aggregate"})
	h.seedRecordedPages(t, "beta", map[int]string{1: "beta repository", 2: "beta invariants"})
	h.seedDocumentWithoutPages(t, "gamma")

	report, err := h.indexer.ReindexAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, report.Documents, "a document with no recorded page has nothing to rebuild")
	assert.Equal(t, 3, report.Chunks)

	assert.Len(t, h.search(t, "aggregate"), 1)
	assert.Len(t, h.search(t, "invariants"), 1, "the rebuilt index answers for both documents")

	again, err := h.indexer.ReindexAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, report, again, "a reindex is idempotent")
	assert.Equal(t, 3, h.chunkRows(t))
}

func TestSearchUseCase_NoMatchesIsAnEmptySlice(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "a", map[int]string{1: "aggregate root"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	results, err := h.searcher.Search(ctx, "...", 0)
	require.NoError(t, err)
	assert.NotNil(t, results, "a caller ranges over the result, so it must never be nil")
	assert.Empty(t, results)

	results, err = h.searcher.Search(ctx, "nothing matches this", 0)
	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Empty(t, results)
}
