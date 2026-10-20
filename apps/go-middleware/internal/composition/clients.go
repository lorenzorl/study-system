package composition

import (
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
)

// Clients are the two model clients the RAG context uses.
//
// They are built together because they share one credential and one API root,
// and they are a struct rather than two return values because "no API key" is a
// state both callers branch on. The zero value means exactly that: no key was
// configured, so no model work is possible, and the server and the command both
// stay usable without one.
//
// The fields are concrete pointers on purpose. A nil *llm.EmbeddingClient stored
// in a domain.Embedder interface is not == nil, so a use case checking for nil
// would pass the check and panic on the first call. Keeping the nil here, in a
// field the caller inspects, removes the trap instead of documenting it.
type Clients struct {
	// Embedding serves semantic search and the embedding stage of the pipeline.
	Embedding *llm.EmbeddingClient

	// Model transcribes a page's image during ingestion.
	Model *llm.Client
}

// NewClients builds both clients, or returns the zero Clients when no API key is
// configured.
//
// A missing key is not an error here: the server boots and serves lexical search
// and status without one, and the commands decide for themselves whether the mode
// they are running needs a model. Requiring the key in this constructor would make
// "no credential" indistinguishable from "misconfigured".
func NewClients(cfg *config.Config) (Clients, error) {
	if cfg.OpenRouterAPIKey == "" {
		return Clients{}, nil
	}

	embedding, err := llm.NewEmbeddingClient(
		cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.EmbeddingModel, llm.Options{})
	if err != nil {
		return Clients{}, fmt.Errorf("create embedding client: %w", err)
	}

	model, err := llm.NewClient(
		cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.LLMModel, llm.Options{})
	if err != nil {
		return Clients{}, fmt.Errorf("create model client: %w", err)
	}

	return Clients{Embedding: embedding, Model: model}, nil
}
