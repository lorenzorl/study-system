package domain

import "time"

// Document is one source file acquired into the ingestion pipeline.
//
// Its identity is the content hash of the acquired bytes, not the path it was
// read from. That choice is load-bearing for resumability: a job's page outcomes
// describe one exact set of bytes, so a file that changed on disk must not resume
// against them. With a path key it would, silently, and the output would mix two
// versions of the book — the same class of defect that left two headers in the
// middle of DDD-full.md.
//
// SourceURI is therefore informational: it records where the bytes were read from
// when the document was first created, and is never used to decide identity.
type Document struct {
	ID string

	// Title is the human-readable name, usually the source file's base name.
	Title string

	// ArtifactID is the identifier of the stored payload in platform/storage. It
	// is a filesystem identifier, not a foreign key: artifacts are files, so the
	// database cannot enforce the reference. The store validates the identifier
	// on every read.
	ArtifactID string

	// SourceURI is where the file was read from. Informational only.
	SourceURI string

	// ContentSHA256 is the lowercase hex digest of the acquired bytes, and the
	// document's identity. The schema enforces the shape (64 lowercase hex
	// characters) so two spellings of one digest can never become two documents.
	ContentSHA256 string

	CreatedAt time.Time
}
