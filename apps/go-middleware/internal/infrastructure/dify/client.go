package dify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DifyDocument represents a document entry in Dify's knowledge base.
type DifyDocument struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DifyClient defines the interface for interacting with the Dify API.
type DifyClient interface {
	FetchAllDocuments(ctx context.Context) ([]DifyDocument, error)
}

// client is the concrete implementation of DifyClient using net/http.
type client struct {
	baseURL   string
	datasetID string
	httpClient *http.Client
}

// NewClient creates a new DifyClient with the given base URL and dataset ID.
func NewClient(baseURL, datasetID string) DifyClient {
	return &client{
		baseURL:   baseURL,
		datasetID: datasetID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// difyDocumentListResponse represents the paginated document list response from Dify API.
type difyDocumentListResponse struct {
	Data    []difyDocumentEntry `json:"data"`
	HasMore bool                `json:"has_more"`
}

// difyDocumentEntry is a single document in Dify's paginated response.
type difyDocumentEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FetchAllDocuments retrieves all documents from the configured Dify dataset,
// following pagination until has_more is false.
func (c *client) FetchAllDocuments(ctx context.Context) ([]DifyDocument, error) {
	var allDocs []DifyDocument
	page := 1
	limit := 100

	for {
		url := fmt.Sprintf("%s/v1/datasets/%s/documents?page=%d&limit=%d",
			c.baseURL, c.datasetID, page, limit)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch documents page %d: %w", page, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response body page %d: %w", page, err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("dify API returned status %d page %d: %s", resp.StatusCode, page, string(body))
		}

		var listResp difyDocumentListResponse
		if err := json.Unmarshal(body, &listResp); err != nil {
			return nil, fmt.Errorf("unmarshal response page %d: %w", page, err)
		}

		for _, entry := range listResp.Data {
			allDocs = append(allDocs, DifyDocument{
				ID:   entry.ID,
				Name: entry.Name,
			})
		}

		if !listResp.HasMore {
			break
		}
		page++
	}

	return allDocs, nil
}
