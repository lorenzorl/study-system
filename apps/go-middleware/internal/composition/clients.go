package composition

import (
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
)

// Clients are the two model clients the RAG context uses.
//
// They share one credential and one API root, and they are a struct rather than
// two return values because "no API key" is a state every caller branches on. The
// zero value means exactly that: no key was configured, so no model work is
// possible, and the server and the commands both stay usable without one.
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

// NewClients builds both clients.
//
// It is what the server uses, because the server does all three things: it
// searches (embedding), it transcribes (model), and it runs the pipeline (both).
func NewClients(cfg *config.Config) (Clients, error) {
	return buildClients(cfg, true, true)
}

// NewEmbeddingClients builds only the embedding client.
//
// The commands that search the index or rebuild the vectors never transcribe a
// page, so they must not construct a chat client they will not use — and must not
// be able to fail on one either.
func NewEmbeddingClients(cfg *config.Config) (Clients, error) {
	return buildClients(cfg, true, false)
}

// NewTranscriptionClients builds only the chat client.
//
// The conversion run turns pages into text and leaves the index and the vectors
// to later steps, so it never embeds a query or a chunk.
func NewTranscriptionClients(cfg *config.Config) (Clients, error) {
	return buildClients(cfg, false, true)
}

// buildClients is the one place the "no key means no client" rule lives, and the
// one place that names the credential in its errors.
//
// A missing key is not an error: the server boots and serves lexical search and
// status without one, and each command decides for itself whether the mode it is
// running needs a model. Requiring the key here would make "no credential"
// indistinguishable from "misconfigured".
func buildClients(cfg *config.Config, wantEmbedding, wantModel bool) (Clients, error) {
	if cfg.OpenRouterAPIKey == "" {
		return Clients{}, nil
	}

	var clients Clients

	if wantEmbedding {
		embedding, err := llm.NewEmbeddingClient(
			cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.EmbeddingModel, llm.Options{})
		if err != nil {
			return Clients{}, fmt.Errorf("create embedding client: %w", err)
		}
		clients.Embedding = embedding
	}

	if wantModel {
		model, err := llm.NewClient(
			cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.LLMModel, llm.Options{})
		if err != nil {
			return Clients{}, fmt.Errorf("create model client: %w", err)
		}
		clients.Model = model
	}

	return clients, nil
}

// RequireEmbedding reports a configured API key that produced no embedding
// client.
//
// It guards the callers whose behaviour would otherwise degrade **silently**:
// without a client, a hybrid search becomes a lexical one, which still answers
// and looks correct. The constructors above cannot produce that state today — a
// key always builds the client — so this fails only if the wiring drifts, which
// is exactly when a quiet downgrade would be hardest to notice.
func RequireEmbedding(cfg *config.Config, clients Clients) error {
	if cfg.OpenRouterAPIKey == "" || clients.Embedding != nil {
		return nil
	}

	return fmt.Errorf(
		"%s is set but no embedding client was built: this is an internal wiring error, not a configuration one",
		config.EnvOpenRouterAPIKey)
}
