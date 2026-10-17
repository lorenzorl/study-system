package domain

import "context"

// PageTranscriber turns one rendered page into Markdown text with a vision
// model.
//
// The port is one method, takes no shared type, and is declared by this context
// rather than by the adapter: platform/llm satisfies it implicitly and knows
// nothing about ingestion, which is what lets the model client live in platform
// without either package importing the other.
//
// The rendered image is passed as bytes rather than as a path so the caller owns
// the render's lifetime; nothing is persisted on the transcribing side.
type PageTranscriber interface {
	// Transcribe sends one page's PNG render with prompt and returns the model's
	// Markdown. The returned text is stored verbatim, exactly like extracted
	// text.
	Transcribe(ctx context.Context, prompt string, imagePNG []byte) (string, error)
}
