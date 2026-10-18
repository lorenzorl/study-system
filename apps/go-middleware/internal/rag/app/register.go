package app

import (
	"context"
	"errors"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// DocumentRegistrar is the slice of the document port the registration rule
// needs. It is declared next to the rule, so both callers — the ingestion run
// and the HTTP upload — can pass the full repository or a fake.
type DocumentRegistrar interface {
	FindByContentHash(ctx context.Context, contentSHA256 string) (*domain.Document, error)
	Create(ctx context.Context, document *domain.Document) error
}

// FindOrCreateDocument returns the document for contentHash, creating it from
// artifact when those bytes have never been seen.
//
// Two callers register a document: the ingestion run, which stores the bytes it
// was given, and the HTTP upload, which has already streamed the bytes into the
// store and holds the artifact. Both must agree on the identity rule — the
// content hash — and on what a concurrent insert means, so the rule lives here
// once rather than being re-implemented per caller.
//
// A create that races another insert is a conflict; the winner's document is
// returned rather than splitting one book across two rows.
func FindOrCreateDocument(
	ctx context.Context,
	documents DocumentRegistrar,
	artifact storage.Artifact,
	contentHash, title, sourceURI string,
	now time.Time,
	newID func() string,
) (*domain.Document, error) {
	existing, err := documents.FindByContentHash(ctx, contentHash)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, apperror.ErrNotFound) {
		return nil, err
	}

	document := &domain.Document{
		ID:            newID(),
		Title:         title,
		ArtifactID:    artifact.ID,
		SourceURI:     sourceURI,
		ContentSHA256: contentHash,
		CreatedAt:     now,
	}
	if err := documents.Create(ctx, document); err != nil {
		if errors.Is(err, apperror.ErrConflict) {
			found, findErr := documents.FindByContentHash(ctx, contentHash)
			if findErr != nil {
				return nil, findErr
			}
			return found, nil
		}
		return nil, err
	}

	return document, nil
}
