package dify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/infrastructure/dify"
)

func TestFetchAllDocuments_SinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/datasets/ds-test/documents", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		assert.Equal(t, "100", r.URL.Query().Get("limit"))

		resp := map[string]interface{}{
			"data": []map[string]string{
				{"id": "doc-1", "name": "Document One"},
				{"id": "doc-2", "name": "Document Two"},
			},
			"has_more": false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := dify.NewClient(server.URL, "ds-test")
	docs, err := client.FetchAllDocuments(context.Background())
	require.NoError(t, err)
	assert.Len(t, docs, 2)
	assert.Equal(t, "doc-1", docs[0].ID)
	assert.Equal(t, "Document One", docs[0].Name)
	assert.Equal(t, "doc-2", docs[1].ID)
	assert.Equal(t, "Document Two", docs[1].Name)
}

func TestFetchAllDocuments_Pagination(t *testing.T) {
	pageCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCalls++
		page := r.URL.Query().Get("page")

		var resp map[string]interface{}
		switch page {
		case "1":
			resp = map[string]interface{}{
				"data": []map[string]string{
					{"id": "a", "name": "A"},
					{"id": "b", "name": "B"},
				},
				"has_more": true,
			}
		case "2":
			resp = map[string]interface{}{
				"data": []map[string]string{
					{"id": "c", "name": "C"},
				},
				"has_more": false,
			}
		default:
			t.Fatalf("unexpected page: %s", page)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := dify.NewClient(server.URL, "ds-test")
	docs, err := client.FetchAllDocuments(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, pageCalls, "should have made 2 requests")
	assert.Len(t, docs, 3)
	assert.Equal(t, "a", docs[0].ID)
	assert.Equal(t, "b", docs[1].ID)
	assert.Equal(t, "c", docs[2].ID)
}

func TestFetchAllDocuments_EmptyDataset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data":     []map[string]string{},
			"has_more": false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := dify.NewClient(server.URL, "ds-test")
	docs, err := client.FetchAllDocuments(context.Background())
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestFetchAllDocuments_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"has_more":false}`))
	}))
	defer server.Close()

	client := dify.NewClient(server.URL, "ds-test")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.FetchAllDocuments(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch documents page 1")
}

func TestFetchAllDocuments_Non200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	client := dify.NewClient(server.URL, "ds-test")
	docs, err := client.FetchAllDocuments(context.Background())
	require.Error(t, err)
	assert.Nil(t, docs)
	assert.Contains(t, err.Error(), "status 500")
}

func TestFetchAllDocuments_ServerUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"has_more":false}`))
	}))
	url := server.URL
	server.Close() // kill it

	client := dify.NewClient(url, "ds-test")
	docs, err := client.FetchAllDocuments(context.Background())
	require.Error(t, err)
	assert.Nil(t, docs)
}
