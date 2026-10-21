# Section metadata for chunks — chapter and section attribution

**Status:** in progress · **Created:** 2026-10-21 · **Depends on:** step 4
(`documents`, `ingestion_jobs`, `job_pages`), step 5 (`chunks`, `documents`),
`rag/infra/pdf` (poppler runner seam), `platform/db` (migrations).

Verdict recorded before any source write: **viable, and the section list is the
primary source, not the chunker's typography.** Attribution is by page; a
`document_sections` table holds the hierarchy; chunks join it at query time; the
structure is optional and rebuildable, so a PDF nobody can structure indexes and
answers exactly as it does today.

## Specs

**S1 — The request.** "ok quiero que evalues la posibilidad de agregar el
capitulo y la seccion a los metadatos de cada chunk, es viable? es una buena
idea?"

**S2 — Where the structure comes from.** "ok entonces el punto critico es sacar
las secciones del pdf, vamos a concentrarnos en eso, dime si esa es la mejor
manera de hacerlo y en donde se guardaria la lista de secciones"

**S3 — The approved design.** The proposal the user approved with "si": the
structure is extracted in a cascade — *(a)* the printed table of contents,
*(b)* confirmation by the page's typography, *(c)* numbered headings for
documents without a TOC, *(d)* nothing at all — and it lands in a normalized
`document_sections` table rather than in columns repeated per chunk:

```sql
document_sections(id, document_id, parent_id, level, title,
                  start_page, end_page, source, created_at)
```

`source` records provenance (`toc`, `typography`, `pattern`, `mixed`) and is not
CHECK-constrained, matching how `ingestion_jobs.status` was left open on purpose.
A chunk resolves its section from its `page_number`, at query time, with a join;
no chapter title is ever copied into `chunks`.

**S4 — Acceptance criteria (approved verbatim).** "≥95% de los capítulos
validados en la página exacta", "secciones solo donde el índice las nombra",
"cero secciones sin procedencia". Measured per document. A document that does
not clear the gate stores zero sections.

**S5 — The hard safety rule.** "la estructura nunca hace fallar la ingesta ni la
búsqueda". Extraction failure, an unreadable TOC, or a failed validation is a
document with no sections, not a failed document.

**S6 — Out of scope (approved verbatim).** "no prometas *filtrado por sección*
como feature de producto hasta medirlo en 2 o 3 PDFs más". This change exposes
the attribution in the result and in the CLI; it adds no section filter and no
UI.

**S7 — What is promised now.** "La **cita** ("Cap. 5, p. 91") sí la podés
prometer ya, porque en el 100% de los capítulos de este libro la página es
exacta." The delivery is a citation, not a retrieval filter.

**S8 — The embedding constraint.** "Nunca meter el heading dentro del `Text` del
chunk: invalida los embeddings y contamina el lexical." Section data lives in
columns; the chunk text and the stored vectors are untouched by this change.

**S9 — The lifecycle.** Sections are derived data, "reconstruibles con
`reindex`". `reindex` keeps its current property of needing the database and
nothing else: sections are computed during ingestion (the only stage that has
the PDF) and never re-read from the file at index time.

**S10 — Nothing is invented.** "Sin índice y sin texto (escaneado puro): `null`,
no se inventa nada." A page with no section yields a null attribution, and the
column stays nullable end to end.

**S11 — Constraints inherited from the system.** No new dependencies: poppler
tools only, through the existing injectable `Runner`. Tests stay hermetic — a
fake runner and fixture XML, never a real PDF, a real binary, or the network. No
new configuration. The candidate is an additive DTO field and a new route-free
change, so no consumer breaks. The migration takes the RAG context's next free
version, **103** (block 100–199).

## Tasks

Each task carries its tests (test-first) and its own work-unit commit. Route is
inline: the units are small, depend on each other, and none reaches the size
where a writer subagent pays for its own startup.

| ID | Phase | Linked | What | Where | Commit |
| --- | --- | --- | --- | --- | --- |
| T1 | 1 | S3, S9 | Migration 103, `Section` value, `SectionRepository` port, `section_repo.go` (replace-per-document, list-by-document, section-at-page), with repo tests | `internal/rag/{sqlite,domain}` | `e1b8664`, `eb0b6f2`, `e5dbbe8` |
| T2 | 2 | S2, S3 | TOC extractor over `pdftohtml -xml` through the runner seam: index-page detection by leader-dot density, line clustering with a drop-cap-safe anchor, page after the dots, level by indent/number, de-spaced and small-caps titles | `internal/rag/infra/pdf`, `internal/rag/domain` | `605ca55` |
| T3 | 2 | S2, S10 | Numbered-heading fallback for documents without a TOC (`1. INTRODUCTION`, `2.1. TRES Spectra`), rejected when the line is a sentence or carries no following body | `internal/rag/infra/pdf` | pending |
| T4 | 2 | S3, S4 | Confirmation: infer the printed-page↔PDF-page offset by maximizing matches, confirm every entry against a heading larger than the body size, and apply the ≥95% gate at chapter level; below the gate the result is empty | `internal/rag/app` | pending |
| T5 | 3 | S5, S9 | `StructureDocumentUseCase` wired into ingestion: persists sections, is non-fatal on every path, and leaves a document with no structure indexing exactly as before | `internal/rag/app`, `internal/composition` | pending |
| T6 | 3 | S7, S8 | Attribution at query time: deepest section containing the chunk's page, exposed on `SearchResult`, the HTTP DTO and the CLI line; the chunk text and its vectors are untouched | `internal/rag/{sqlite,app,http}`, `internal/cli` | pending |
| T7 | 4 | S4, S7 | Acceptance measurement over the real corpus (DDD plus both papers): chapters validated on the exact page, source of every section, pages covered, chunks per section; failure of the gate recorded, not hidden | `data/`, task log | pending |
| T8 | 4 | S6, S9 | Docs and backfill: the README/ARCHITECTURE describe the table, its provenance and the null case; `reindex` verified to rebuild chunks against already-stored sections | `apps/go-middleware/README.md`, `ARCHITECTURE.md` | pending |

**Risk per task.** No task reaches a Task Size high-risk item on its own: the
schema is additive, the DTO field is additive, nothing stored is rewritten, and
the chunk text is untouched (S8). T4 carries the whole safety of the feature
instead of the risk list: without its gate, a wrong section is worse than none.
T6 changes the shared `searchSQL` used by both search and search-in-document, so
it owes the differential check on the existing result shape — same rows, same
order, one extra column.

**Phases as commits.** Phase 1 (`C1`) is the schema and the port, ~250 lines.
Phase 2 (`C2`) is the extractor and its gate, the largest unit. Phase 3 (`C3`)
wires persistence and exposure. Phase 4 (`C4`) is measurement and docs, and
closes the feature. Each commit lands with its tests and docs alongside the
behaviour.

## Delivery slices

The 400-line review slice is measured cumulatively on this branch, so the
foundation is delivered in slices rather than one commit per phase:

| Slice | Commit | Authored lines | What it adds |
| --- | --- | --- | --- |
| 1 | `e1b8664` | 415 (branch) | The `Section` value, its validation, the `SectionRepository` port, this document |
| 2 | `eb0b6f2` | 510 (branch) | Migration 103 and its migration tests |
| 3 | `e5dbbe8` | 1011 (branch) | The SQLite adapter and its repository tests (T1 closes here) |
| 4 | `605ca55` | 1950 (branch) | The TOC extractor, `domain.Heading` and their tests (T2 closes here) |
| 5 | next | — | The numbered-heading fallback (T3) |

A slice boundary is recorded here before the commit that follows it, which is
what keeps the review workload visible instead of inferring it from the diff.

## Log

**L1 (2026-10-21, original request)** — "ok quiero que evalues la posibilidad de
agregar el capitulo y la seccion a los metadatos de cada chunk, es viable? es
una buena idea?"

**L2 (2026-10-21, the Markdown question)** — "Pero si estamos extrayendo el
libro como un .md hay no se estan separando las secciones por los titulos? cada
titulo tiene un numero de # diferente o no?"

**E1 (evidence, answers L2).** The generated `data/DDD.md` is one header, three
metadata lines, and one `<!-- page N -->` block per page with the text verbatim
(`internal/rag/domain/markdown.go`). It has 18 lines starting with `#` for 359
pages: 1 is the file header, 1 is a false positive on page 183 (`##), design
employs…`), and the other 16 sit on pages that were transcribed by vision, where
the model improvised the marker. Every `#` page is in the transcribed set; no
heading comes from the extractor's typography. A real size-21 heading on page 45
("Letting the Bones Show; Why Models Matter to Users") is plain text. And the
index never reads the file: chunks come from `job_pages.text`.

**L3 (2026-10-21, the focus)** — "ok entonces el punto critico es sacar las
secciones del pdf, vamos a concentrarnos en eso, dime si esa es la mejor manera
de hacerlo y en donde se guardaria la lista de secciones"

**L4 (2026-10-21, authorization)** — "si", authorizing the read-only probe.

**E2 (probe evidence, `apps/go-middleware/data/.probe/`).** Over DDD.pdf (359
pages, 1045 of the corpus's 1180 chunks): the printed TOC parses into **110
entries** (5 parts, 17 chapters, 88 sections) including small-caps and
letter-spaced titles; typographic candidates at size ≥ body+4 (body = 15) are
100 in 63 pages; parts **5/5** and chapters **17/17** are confirmed on the exact
page; sections are 34/88 by typography; typography alone is 55/100 precise;
bold-at-body-size is 628 lines with 99 matching the index (15%), which is why it
cannot be a discovery rule. Coverage: pages 6–359, 3.5 pages and ~10.7 chunks per
entry. The two papers have no TOC but carry numbered headings in the text
(`1. INTRODUCTION`, `2.1. TRES Spectra`).

**L5 (2026-10-21, review disposition and the straight question)** — "dejalo sin
revisar. Por otro lado me dices que es viable pero que el parser que probaste es
solo un experimento. Dime si va a funcionar o no". The worktree candidate
`sha256:e3198a724b1aaea147c6510e065704b01190c967c6d82f7c11e1c3956b93c642`
(`README.md`, `scripts/build-android.sh`, lineage `review-17909a9c41be27b7`) is
**left unreviewed at the user's explicit request**; no review was started.

**E3 (evidence, corrects an earlier claim).** I first said "33 de 359 páginas son
`transcribed`" as if they were all DDD. Measured per document: DDD has 26
transcribed pages (2–357), `paper-figuras.pdf` 2, `img-2610.22254.pdf` 5 — 33
across the corpus. The specs above inherit the corrected numbers, not the 33.

**L6 (2026-10-21, authorization)** — "si", authorizing this document with the
scope and acceptance criteria above. Section filtering stays out (S6); the
citation is what ships (S7).

**L7 (2026-10-21, implementation start)** — "arranca". T1 implemented test-first
(RED observed as the compile failure of the new tests, then GREEN) and delivered
in slices 1 and 2 above; the adapter is slice 3.

**L8 (2026-10-21, T2 delivered and corrected)** — T2 was delegated to a worker
with the probe as its reference and returned 940 authored lines across
`domain/heading.go` and `infra/pdf/toc.go` plus 18 tests; I verified it and found
two defects that the worker's fixtures could not see:

1. **Dedupe by title alone lost real sections.** `Section` rows repeat a title
   across chapters ("Summary" closes most of them), and collapsing by
   `(level, title)` would keep the first and silently drop the rest. The key is
   now `(level, page, title)`: the same row reprinted across a page break still
   collapses, and the same title on another page survives. Pinned by
   `TestTOCExtractor_KeepsTheSameTitleOnDifferentPages`.
2. **De-spacing welded words.** The probe's rule (join when *any* token is short)
   produced "Modelto" and "ofa"; a real letter-spaced word can contain long glyphs
   ("ild", "l-Dr"). The rule is now a majority of short glyphs, plus a label
   boundary so "Part I. Putting…" does not become "I.Putting". Pinned by
   `JoinsAWordWhoseGlyphsAreLongerThanTwoCharacters` and
   `KeepsTheSpacesOfANormallySetTitle`.

The reality check ran the Go extractor over `DDD.pdf` and got **110 entries**,
the same count as the probe, with correct levels and pages. Two titles remain
imperfect ("Modelto", "ofa"): when the typesetter collapsed a word boundary to a
single space, no run-level geometry can recover it. That is recorded in
`despaced` and is what T4 fixes by taking the title from the page's own heading,
which is not letter-spaced.
