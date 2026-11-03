package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
)

// fakeSectionsUseCase is a hand-written DocumentSectionsUseCase. It records the
// document the handler forwarded, so a test can assert the path value reached
// the read untouched.
type fakeSectionsUseCase struct {
	sections []app.Section
	err      error
	calls    int
	gotID    string
}

func (f *fakeSectionsUseCase) Execute(
	_ context.Context, documentID string,
) ([]app.Section, error) {
	f.calls++
	f.gotID = documentID
	if f.err != nil {
		return nil, f.err
	}
	return f.sections, nil
}

// sectionsRequest builds a request already carrying the path value the mux would
// have set, so the handler is exercised without the router.
func sectionsRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents/doc-1/sections", nil)
	req.SetPathValue("documentID", "doc-1")
	return req
}

// TestSectionsHandler_ReturnsTheChapterTable pins the wire shape a client reads:
// six fields per section, the parent present and empty for a top-level one, and
// the provenance under its own key.
func TestSectionsHandler_ReturnsTheChapterTable(t *testing.T) {
	mock := &fakeSectionsUseCase{sections: []app.Section{
		{
			ID: "part-1", Level: 1, Title: "Part I: Putting the Domain Model to Work",
			StartPage: 3, EndPage: 120, Source: domain.SourceTOC,
		},
		{
			ID: "ch-1", ParentID: "part-1", Level: 2, Title: "Crunching Knowledge",
			StartPage: 3, EndPage: 40, Source: domain.SourceMixed,
		},
	}}
	handler := httppkg.NewSectionsHandler(mock)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, sectionsRequest())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "doc-1", mock.gotID)

	// The body is the bare array the route promises, not an object wrapping one.
	var raw []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Len(t, raw, 2)
	assert.ElementsMatch(t,
		[]string{"id", "parent_id", "level", "title", "start_page", "end_page", "source"},
		keysOf(raw[0]))

	var resp httppkg.DocumentSectionsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp, 2)

	assert.Equal(t, "part-1", resp[0].ID)
	assert.Empty(t, resp[0].ParentID)
	assert.Equal(t, 1, resp[0].Level)
	assert.Equal(t, "Part I: Putting the Domain Model to Work", resp[0].Title)
	assert.Equal(t, 3, resp[0].StartPage)
	assert.Equal(t, 120, resp[0].EndPage)
	assert.Equal(t, "toc", resp[0].Source)

	assert.Equal(t, "ch-1", resp[1].ID)
	assert.Equal(t, "part-1", resp[1].ParentID)
	assert.Equal(t, 2, resp[1].Level)
	assert.Equal(t, 3, resp[1].StartPage)
	assert.Equal(t, 40, resp[1].EndPage)
	assert.Equal(t, "mixed", resp[1].Source)
}

// TestSectionsHandler_NoStructureIsAnEmptyArray pins the encoding, not only the
// length: a client ranges over this body, and null is not something it should
// have to special-case.
func TestSectionsHandler_NoStructureIsAnEmptyArray(t *testing.T) {
	handler := httppkg.NewSectionsHandler(&fakeSectionsUseCase{sections: nil})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, sectionsRequest())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[]`, rec.Body.String())
}

// TestSectionsHandler_UnknownDocumentIsNotFound pins that the two silences stay
// distinguishable on the wire: an unregistered document is a 404, an unread one
// never is.
func TestSectionsHandler_UnknownDocumentIsNotFound(t *testing.T) {
	mock := &fakeSectionsUseCase{err: apperror.ErrNotFound}
	handler := httppkg.NewSectionsHandler(mock)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, sectionsRequest())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, mock.calls)
}

// TestRouter_RegistersTheSectionsRoute pins the path, because a handler wired to
// the wrong route answers nothing and every unit test above still passes.
func TestRouter_RegistersTheSectionsRoute(t *testing.T) {
	sections := &fakeSectionsUseCase{sections: []app.Section{
		{ID: "ch-1", Level: 2, Title: "Crunching Knowledge", StartPage: 3, EndPage: 40},
	}}
	router := httppkg.NewRouter(
		&mockSearchUseCase{},
		httppkg.NewUploadDocumentHandler(nil, nil, nil, false),
		httppkg.NewDocumentStatusHandler(nil),
		httppkg.NewPagesHandler(nil),
		httppkg.NewSectionsHandler(sections),
		httppkg.NewDocumentsHandler(&fakeDocumentListUseCase{}),
		newTextHandler(&fakeIngestTextUseCase{}),
		newURLHandler(&fakeIngestTextUseCase{}),
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/api/rag/documents/doc-1/sections", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "doc-1", sections.gotID)

	var resp httppkg.DocumentSectionsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "ch-1", resp[0].ID)
}
