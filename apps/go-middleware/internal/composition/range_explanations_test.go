package composition

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
	ragdomain "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studyapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
)

// openStudyDatabase opens the shared database with both contexts' migrations.
func openStudyDatabase(t *testing.T) *sql.DB {
	t.Helper()

	dsn, _ := fileDSN(t)
	sqlDB, err := OpenDatabase(context.Background(), testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	return sqlDB
}

// seedIngestedDocument registers a document, a finished job with totalPages, and
// one recorded page of text, which is the state the explanation route reads.
func seedIngestedDocument(t *testing.T, sqlDB *sql.DB, totalPages int) string {
	t.Helper()

	ctx := context.Background()
	now := time.Now().UTC()

	document := &ragdomain.Document{
		ID:            uuid.New().String(),
		Title:         "Domain-Driven Design",
		ArtifactID:    "artifact-1",
		SourceURI:     "ddd.pdf",
		ContentSHA256: strings.Repeat("a", 64),
		CreatedAt:     now,
	}
	require.NoError(t, ragsqlite.NewDocumentRepository(sqlDB).Create(ctx, document))

	job := &ragdomain.IngestionJob{
		ID:         uuid.New().String(),
		DocumentID: document.ID,
		Status:     ragdomain.JobCompleted,
		TotalPages: totalPages,
		StartedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, ragsqlite.NewJobRepository(sqlDB).Create(ctx, job))

	require.NoError(t, ragsqlite.NewPageOutcomeRepository(sqlDB).SaveOutcome(ctx, ragdomain.PageOutcome{
		JobID:      job.ID,
		PageNumber: 89,
		Status:     ragdomain.PageExtracted,
		Method:     ragdomain.MethodExtract,
		Text:       "Cluster the ENTITIES and VALUE OBJECTS into AGGREGATES",
		UpdatedAt:  now,
	}))

	return document.ID
}

// The adapter is the one place the study context crosses into the ingestion
// context, and it repeats no bound: every refusal the pages use case makes arrives
// as the sentinel the study transport maps to 400.
func TestPageRangeText_PropagatesThePagesUseCaseVerdicts(t *testing.T) {
	sqlDB := openStudyDatabase(t)
	documentID := seedIngestedDocument(t, sqlDB, 114)

	port := NewRAG(sqlDB).Pages()
	adapter := newPageRangeText(port)

	t.Run("an unknown document is not found", func(t *testing.T) {
		_, err := adapter.ReadRange(t.Context(), uuid.New().String(), 1, 5)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("a range above the page cap is invalid input", func(t *testing.T) {
		_, err := adapter.ReadRange(t.Context(), documentID, 1, 51)
		assert.ErrorIs(t, err, apperror.ErrInvalidInput,
			"the pages use case owns the cap, and its 400 must survive the adapter")
	})

	t.Run("a range past the last page is invalid input", func(t *testing.T) {
		_, err := adapter.ReadRange(t.Context(), documentID, 100, 115)
		assert.ErrorIs(t, err, apperror.ErrInvalidInput)
	})

	t.Run("a recorded range is returned with its page numbers", func(t *testing.T) {
		pages, err := adapter.ReadRange(t.Context(), documentID, 88, 114)
		require.NoError(t, err)
		require.Len(t, pages, 1)
		assert.Equal(t, 89, pages[0].PageNumber)
		assert.Contains(t, pages[0].Text, "Cluster the ENTITIES")
	})
}

// newGapServer builds a model client pointed at a test server, so no test reaches
// the network.
func newGapServer(t *testing.T, answer string) *llm.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoded, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": answer}}},
		})
		require.NoError(t, err)
		_, _ = w.Write(encoded)
	}))
	t.Cleanup(server.Close)

	client, err := llm.NewClient("test-key", server.URL, "test/model", llm.Options{HTTPClient: server.Client()})
	require.NoError(t, err)

	return client
}

func gapSource() studyapp.GapSource {
	return studyapp.GapSource{
		FromPage:    88,
		ToPage:      114,
		RangeText:   "--- page 89 ---\nCluster the ENTITIES and VALUE OBJECTS into AGGREGATES",
		Explanation: "An aggregate is a consistency boundary.",
	}
}

// The model's structured answer becomes a gap list; a Markdown fence around it is
// tolerated because the model emits one often despite the instruction.
func TestModelGapFinder_ParsesTheModelsAnswer(t *testing.T) {
	client := newGapServer(t, "```json\n"+
		`{"gaps":[{"point":"the boundary is defined by the invariants","page":89,"quote":"Cluster the ENTITIES"}]}`+
		"\n```")

	gaps, err := newModelGapFinder(client).FindGaps(t.Context(), gapSource())
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Equal(t, "the boundary is defined by the invariants", gaps[0].Point)
	assert.Equal(t, 89, gaps[0].Page)
	assert.Equal(t, "Cluster the ENTITIES", gaps[0].Quote)
}

// The prompt carries the range's text under its page markers and the explanation
// verbatim: without the markers the model has no honest way to name a page.
func TestModelGapFinder_SendsTheRangeAndTheExplanation(t *testing.T) {
	var bodies = make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"gaps\":[]}"}}]}`)
	}))
	defer server.Close()

	client, err := llm.NewClient("test-key", server.URL, "test/model", llm.Options{HTTPClient: server.Client()})
	require.NoError(t, err)

	_, err = newModelGapFinder(client).FindGaps(t.Context(), gapSource())
	require.NoError(t, err)

	prompt := <-bodies
	assert.Contains(t, prompt, "--- page 89 ---")
	assert.Contains(t, prompt, "Cluster the ENTITIES")
	assert.Contains(t, prompt, "An aggregate is a consistency boundary.")
	assert.Contains(t, prompt, "pages 88 to 114")
}

// A reasoning model repeats the answer template inside its thinking, so the first
// JSON in the answer is the template with placeholder values. The payload is the
// last one: taking the first stored point="..." and quote="..." on a live run.
func TestModelGapFinder_PrefersTheLastJSONObjectOverTheTemplate(t *testing.T) {
	client := newGapServer(t, "Thinking Process:\n\n1. Output format is "+
		`{"gaps":[{"point":"...","page":89,"quote":"..."}]}`+
		"\n2. Compare the explanation with the page.\n\nFinal answer:\n"+
		`{"gaps":[{"point":"the invariants define the boundary","page":89,"quote":"Cluster the ENTITIES"}]}`)

	gaps, err := newModelGapFinder(client).FindGaps(t.Context(), gapSource())

	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Equal(t, "the invariants define the boundary", gaps[0].Point,
		"the answer must be the last JSON object, not the template the model echoed")
	assert.Equal(t, "Cluster the ENTITIES", gaps[0].Quote)
}

// The model wraps its answer in prose often, despite the instruction, so the parse
// must locate the JSON payload instead of assuming the whole answer is it.
func TestModelGapFinder_ToleratesProseAroundTheJSON(t *testing.T) {
	client := newGapServer(t, "The explanation left out two things.\n"+
		`{"gaps":[{"point":"a","page":89,"quote":"q"},{"point":"b","page":90,"quote":"r"}]}`+
		"\nThose are the gaps I found.")

	gaps, err := newModelGapFinder(client).FindGaps(t.Context(), gapSource())

	require.NoError(t, err)
	require.Len(t, gaps, 2, "the two gaps around the prose must both be read")
	assert.Equal(t, 89, gaps[0].Page)
}

// A model that drops the envelope and answers with the bare array is still
// answering the question.
func TestModelGapFinder_AcceptsABareGapArray(t *testing.T) {
	client := newGapServer(t, "Here they are:\n"+`[{"point":"a","page":89,"quote":"q"}]`)

	gaps, err := newModelGapFinder(client).FindGaps(t.Context(), gapSource())

	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Equal(t, "a", gaps[0].Point)
}

// A model that answers prose despite the instruction gets one corrective attempt.
// Failing the reader thirty seconds in, when a second short call usually fixes it,
// is the worse trade — and a second failure is still reported, never read as an
// empty list.
func TestModelGapFinder_RetriesOnceWithACorrectivePrompt(t *testing.T) {
	var calls int
	var corrective string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)

		if calls == 1 {
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"None: the explanation covers the range."}}]}`)
			return
		}

		corrective = string(body)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"gaps\":[{\"point\":\"a\",\"page\":89,\"quote\":\"q\"}]}"}}]}`)
	}))
	defer server.Close()

	client, err := llm.NewClient("test-key", server.URL, "test/model", llm.Options{HTTPClient: server.Client()})
	require.NoError(t, err)

	gaps, err := newModelGapFinder(client).FindGaps(t.Context(), gapSource())

	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Equal(t, 2, calls, "the second call is the corrective attempt")
	assert.Contains(t, corrective, "JSON object only")
}

// An answer with no gap list must fail the request instead of reading as "you left
// nothing out", and it must not be classified as the caller's mistake.
func TestModelGapFinder_RefusesAnAnswerWithNoGapList(t *testing.T) {
	client := newGapServer(t, `{"answer":"I cannot help with that"}`)

	_, err := newModelGapFinder(client).FindGaps(t.Context(), gapSource())

	require.Error(t, err)
	assert.NotErrorIs(t, err, apperror.ErrInvalidInput)
	assert.Contains(t, err.Error(), "no gap list")
}

// Without a key the server still boots, so the endpoint must refuse at call time
// rather than panic on a nil client.
func TestNewModelGapFinder_WithoutAClientRefuses(t *testing.T) {
	_, err := newModelGapFinder(nil).FindGaps(t.Context(), gapSource())

	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvOpenRouterAPIKey)
	assert.NotErrorIs(t, err, apperror.ErrInvalidInput, "a missing key is not the reader's mistake")
}

// The whole study context, wired the way the CLI wires it: the explanation use case
// exists even with no model, refuses with the variable's name, and stores nothing.
func TestNewStudy_ExplainsRangesWithARefusalWhenNoModelIsConfigured(t *testing.T) {
	sqlDB := openStudyDatabase(t)
	documentID := seedIngestedDocument(t, sqlDB, 114)

	study := NewStudy(sqlDB)
	require.NotNil(t, study.ExplainRange, "the context is built whole, with no nil field")

	_, err := study.ExplainRange.Execute(t.Context(), studyapp.ExplainRangeInput{
		DocumentID: documentID,
		FromPage:   88,
		ToPage:     99,
		Text:       strings.Repeat("the aggregate is a consistency boundary. ", 80),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvOpenRouterAPIKey)

	var stored int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM range_explanations`).Scan(&stored))
	assert.Zero(t, stored, "a refused explanation stores nothing")
}
