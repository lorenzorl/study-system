package app

import "fmt"

// pagePrompt is the instruction sent with every rendered page.
//
// It lives in this package, not in the model client, because what to ask the
// model is domain knowledge while how to reach it is transport.
//
// The last paragraph is the whole point of the hybrid design: an extractor that
// fails produces visibly garbled text that matches no query, but a model that is
// unsure invents confidently, and a confident invention matches queries and
// answers them wrongly. Telling the model to mark what it cannot read instead of
// filling it in is what bounds how much of the corpus can be silently wrong.
const pagePrompt = `You are converting ONE page of a book into Markdown for a retrieval index.

Transcribe the page's text exactly as printed: same wording, same order. Do not
summarise, do not paraphrase, do not explain, do not add commentary.
Do not reproduce running headers, footers, or page numbers.
For every diagram, figure, chart, or image, add a short factual description as a
blockquote line starting with "> Figure:".
Reproduce tables as Markdown tables.
Never invent content you cannot read. If a region is illegible, write [illegible]
instead of guessing.
This is page %d of %d. Output only the Markdown for the page, with no preamble.`

// TranscriptionPrompt renders the page prompt for one page of a book.
//
// The page number is part of the prompt because the render itself carries no page
// number when the source page has none of its own, and the model is told that
// page numbers are not to be reproduced.
func TranscriptionPrompt(page, total int) string {
	return fmt.Sprintf(pagePrompt, page, total)
}
