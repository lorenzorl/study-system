package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	studyapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	studydomain "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// This file holds the two adapters the study context's range-explanation ports
// need, and nothing else does.
//
// The ports are declared in study/app; the study context must not import the page
// store or the model client. The adapters live here for the same reason
// domain.DummyFSRS{} and clients.Model are injected here today: composition is the
// only package allowed to know every context.

// pageRangeText adapts the ingestion context's page read to the study context's
// RangeText port.
//
// The adapter is deliberately thin, and the page-range bounds are deliberately
// absent: the rag use case validates the range itself, and its *app.PageRangeError
// unwraps to apperror.ErrInvalidInput, which the study transport already maps to
// 400. Copying a bound here would give the two contexts two places to disagree
// about how long a document is.
type pageRangeText struct {
	pages *ragapp.DocumentPagesUseCase
}

// newPageRangeText narrows the pages use case into the study context's port.
func newPageRangeText(pages *ragapp.DocumentPagesUseCase) studyapp.RangeText {
	return pageRangeText{pages: pages}
}

// ReadRange returns the recorded text of the range, page ascending.
func (a pageRangeText) ReadRange(
	ctx context.Context,
	documentID string,
	from, to int,
) ([]studyapp.RangePage, error) {
	read, err := a.pages.Execute(ctx, documentID, from, to)
	if err != nil {
		// The error is wrapped, not replaced: errors.Is and errors.As must still
		// see the sentinel the pages use case classified, because that is what the
		// transport maps to 400 or 404.
		return nil, fmt.Errorf("read pages %d to %d of document %s: %w", from, to, documentID, err)
	}

	pages := make([]studyapp.RangePage, 0, len(read.Pages))
	for _, page := range read.Pages {
		pages = append(pages, studyapp.RangePage{PageNumber: page.PageNumber, Text: page.Text})
	}

	return pages, nil
}

// gapPrompt is the instruction the model reads for one range.
//
// It lives beside the parser on purpose, and not in study/app: the prompt and the
// answer's shape are one contract, and a prompt edited away from the parser is how
// the two drift until every answer fails to parse. What the gap *is* stays in the
// application layer, which is what validates and caps whatever comes back.
//
// The prompt is bounded by construction — the range's text and the explanation,
// nothing else — and it asks for the shape the parser reads, with the page markers
// it must cite.
//
// It names the language of the answer for the reason a live run showed: the book is
// usually in English and its text is the bulk of the prompt, so a Spanish
// explanation got its gaps back in English. The point is the reader's feedback and
// must arrive in their language; the quote is evidence and must not be translated,
// which is why the two are stated separately instead of as one "answer in X".
const gapPrompt = `You are checking one reader's explanation of pages %d to %d of a book against the book's own text.

Report only what the material states that the explanation leaves out. Answer with JSON
and nothing else, in exactly this shape:

{"gaps":[{"point":"...","page":89,"quote":"..."}]}

Each gap has three fields:
- "point": one sentence naming an idea the material states and the explanation does not, written
  in the language of the reader's explanation;
- "page": the page number that states it, taken from the "--- page N ---" markers below;
- "quote": a short passage copied verbatim from that page, as evidence; "quote" stays verbatim in
  the book's language even when "point" is in another one, because a translated quote no longer
  proves that the page says it.

Rules:
- at most %d gaps, the most important first;
- never name a page that is not one of the markers;
- never quote text that is not on the page you name;
- answer with the JSON object only, with no prose before or after it;
- if the explanation already covers the range, answer {"gaps":[]}.

The reader's explanation, verbatim:
"""
%s
"""

The recorded text of pages %d to %d:
%s`

// gapCorrectiveTurn is appended to the prompt when the first answer was not JSON.
// It repeats the shape and forbids the prose the model adds by habit.
const gapCorrectiveTurn = "\n\nYour previous answer was not JSON. Answer again with the JSON object only, and no prose before or after it."

// modelGapFinder is the GapFinder the server uses: one model call over the range.
type modelGapFinder struct {
	client *llm.Client
}

// newModelGapFinder narrows the client field into the study context's port.
//
// A nil client becomes a named refusal rather than a nil interface holding a nil
// pointer: the trap composition/clients.go documents is exactly this one, and the
// server boots without an API key.
func newModelGapFinder(client *llm.Client) studyapp.GapFinder {
	if client == nil {
		return unavailableGapFinder{}
	}
	return modelGapFinder{client: client}
}

// FindGaps asks the model what the explanation left out.
//
// The answer is left raw for the use case to validate: the model cannot be trusted
// with the page bound, the cap, or the quote, and that is the application layer's
// one validation, not this adapter's.
func (f modelGapFinder) FindGaps(
	ctx context.Context,
	source studyapp.GapSource,
) ([]studydomain.RangeGap, error) {
	prompt := fmt.Sprintf(
		gapPrompt,
		source.FromPage,
		source.ToPage,
		studyapp.MaxRangeGaps,
		source.Explanation,
		source.FromPage,
		source.ToPage,
		source.RangeText,
	)

	answer, err := f.client.Complete(ctx, prompt)
	if err != nil {
		// Not wrapped in an apperror sentinel: a model failure is this server's
		// failure, and a 4xx would blame the reader for an answer nobody produced.
		return nil, fmt.Errorf("ask the model for the gaps: %w", err)
	}

	gaps, parseErr := parseGaps(answer)
	if parseErr == nil {
		return gaps, nil
	}

	// One corrective attempt. The prompt asks for JSON and models answer prose
	// anyway — often just to say "nothing was left out" — so a second short call is
	// cheaper than failing a reader who already waited for the first one. A second
	// failure is reported, never interpreted as an empty list.
	corrected, err := f.client.Complete(ctx, prompt+gapCorrectiveTurn)
	if err != nil {
		return nil, fmt.Errorf("ask the model for the gaps again: %w", err)
	}

	return parseGaps(corrected)
}

// unavailableGapFinder is what the endpoint gets when no API key is configured.
//
// It refuses instead of panicking, and its message names the variable that
// supplies the key, because that is the whole fix.
type unavailableGapFinder struct{}

// FindGaps reports that this process has no model to ask.
func (unavailableGapFinder) FindGaps(
	context.Context,
	studyapp.GapSource,
) ([]studydomain.RangeGap, error) {
	return nil, fmt.Errorf(
		"no model client was built: %s is not set, and explaining a range reads the range with the model",
		config.EnvOpenRouterAPIKey)
}

// gapPayload is one gap as the model writes it, before the use case applies its
// invariants.
type gapPayload struct {
	Point string `json:"point"`
	Page  int    `json:"page"`
	Quote string `json:"quote"`
}

// parseGaps decodes the model's answer.
//
// It locates the JSON payload instead of assuming the answer is JSON: a model
// emits prose around it often, despite the instruction, and it also drops the
// envelope and answers with the bare array. A malformed answer must not be
// mistakable for an empty list, because the reader reads empty as "you left
// nothing out".
func parseGaps(answer string) ([]studydomain.RangeGap, error) {
	payload := jsonSpan(answer)

	var envelope struct {
		Gaps *[]gapPayload `json:"gaps"`
	}
	envelopeErr := json.Unmarshal([]byte(payload), &envelope)
	if envelopeErr == nil && envelope.Gaps != nil {
		return toGaps(*envelope.Gaps), nil
	}

	var bare []gapPayload
	if err := json.Unmarshal([]byte(payload), &bare); err == nil {
		return toGaps(bare), nil
	}

	if envelopeErr != nil {
		return nil, fmt.Errorf("parse the model's gap answer: %w (answer: %s)", envelopeErr, excerpt(answer))
	}
	return nil, fmt.Errorf("the model's gap answer carries no gap list: %s", excerpt(answer))
}

// jsonSpan returns the last balanced JSON object or array in the answer.
//
// Last, not first: a reasoning model restates the answer template inside its
// thinking ({"gaps":[{"point":"..."}]}), so the first JSON in the text is the
// template and the payload is the one it writes last. Quotes are tracked so a
// brace inside a quoted string does not end a span early. An answer with no JSON
// at all is returned trimmed, so the caller reports a parse error against the real
// text instead of against an empty string.
func jsonSpan(answer string) string {
	text := stripCodeFence(answer)

	last := ""
	for start := 0; start < len(text); start++ {
		if text[start] != '{' && text[start] != '[' {
			continue
		}

		span, end, ok := balancedSpan(text, start)
		if !ok {
			continue
		}

		last = span
		start = end
	}

	if last != "" {
		return last
	}

	return strings.TrimSpace(text)
}

// balancedSpan returns the JSON value that starts at start, and the index of its
// closing byte. ok is false when the value never closes.
func balancedSpan(text string, start int) (string, int, bool) {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(text); i++ {
		character := text[i]

		if inString {
			switch {
			case escaped:
				escaped = false
			case character == '\\':
				escaped = true
			case character == '"':
				inString = false
			}
			continue
		}

		switch character {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return text[start : i+1], i, true
			}
		}
	}

	return "", 0, false
}

// toGaps maps the model's shape onto the context's, so the adapter is the only
// place that knows how the model words its answer.
func toGaps(payloads []gapPayload) []studydomain.RangeGap {
	gaps := make([]studydomain.RangeGap, 0, len(payloads))
	for _, payload := range payloads {
		gaps = append(gaps, studydomain.RangeGap{
			Point: payload.Point,
			Page:  payload.Page,
			Quote: payload.Quote,
		})
	}
	return gaps
}

// stripCodeFence removes a surrounding Markdown code fence, opening line included.
func stripCodeFence(answer string) string {
	trimmed := strings.TrimSpace(answer)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}

	if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
		trimmed = trimmed[newline+1:]
	}
	trimmed = strings.TrimSuffix(strings.TrimSpace(trimmed), "```")

	return strings.TrimSpace(trimmed)
}

// excerpt bounds an answer for an error message: a model can answer with a whole
// page of prose, and an error that long is unreadable in a log.
func excerpt(answer string) string {
	trimmed := strings.TrimSpace(answer)
	if len(trimmed) <= 200 {
		return trimmed
	}
	return trimmed[:200] + "..."
}
