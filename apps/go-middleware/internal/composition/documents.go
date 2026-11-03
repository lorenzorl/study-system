package composition

// Cross-context adapter: study names a material, rag owns its title.

import (
	"context"

	ragdomain "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// documentTitles adapts the ingestion context's document repository to the port the
// study context declares for naming a material.
//
// It is an adapter and not an import in the other direction for the boundary the two
// contexts keep: study stores a document id and never reads the rag tables, and the
// one place that knows both is here, the composition root.
type documentTitles struct {
	documents *ragsqlite.DocumentRepository
}

func newDocumentTitles(documents *ragsqlite.DocumentRepository) *documentTitles {
	return &documentTitles{documents: documents}
}

// Title returns the document's title, or apperror.ErrNotFound when no document has
// that id — the repository already answers a miss that way, so the sentinel travels
// unchanged and the transport turns it into 404.
func (d *documentTitles) Title(ctx context.Context, documentID string) (string, error) {
	var document *ragdomain.Document
	document, err := d.documents.FindByID(ctx, documentID)
	if err != nil {
		return "", err
	}

	return document.Title, nil
}
