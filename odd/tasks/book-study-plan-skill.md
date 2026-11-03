# A skill for what to do after a book is uploaded

**Status:** in progress · **Created:** 2026-11-02 · **Depends on:** `skills/study-system`
(the package), the `rag` HTTP API (`documents`, `pages`, `search`).

The book gets ingested and nothing tells the reader what to do next. This skill
closes that gap: it inspects the book, classifies what kind of knowledge it carries,
and returns a study plan — which chapters are the core, in what order, with what
retrieval test, and what to skip.

## Specs

**S1 — The request.** "ok, antes de seguir con esto quiero que escribas una skill
donde se detalle el proceso que se debe seguir despues de subir un libro, donde se
clasifica, y se recomienda al usuario la mejor manera de leerlo, asi cuando cualquier
usuario suba un libro va a saber que hacer despues"

**S2 — It applies to any book, not to the one we happened to ingest.** "en un contexto
general, quiero estudiar un libro, no se absolutamente nada de ese libro como deberia
comenzar a estudiarlo?"

**S3 — The classification is the first output, and it is a real decision.** "donde se
clasifica". A book is classified by the kind of knowledge it carries, because that
decides the retrieval test: procedural → perform it, conceptual → apply the criterion,
factual → recall it, argumentative → reconstruct the thesis, reference → do not read.

**S4 — The recommendation is the second output.** "se recomienda al usuario la mejor
manera de leerlo": reading mode (linear, project-driven, survey, consult), the core
20-30% versus support, chapter order when it differs from the book's own, the session
protocol, and what to skip.

**S5 — The no-objective case is explicitly in scope, not an error.** "Que pasa si no
tengo ningun objetivo especifico, simplemente me dijeron que saber sobre ddd me ayudara
a disenar mejores sistemas, como deberia estudiarlo entonces?" The skill must offer the
pain-mining exercise and the survey mode, and must never invent the reader's objective.

**S6 — It runs against the HTTP API, so it works from a phone.** The existing skill's
premise: a client that can reach the server and holds the access credentials. No
database file, no shell on the server.

**S7 — No cards.** The skill returns a plan. Cards stay with `study-system`, one
chapter at a time, after the plan says which chapter.

**S8 — Nothing invented.** No page number that was not read, no chapter title the book
does not print, no objective the reader did not state. The section table exists in the
database but **is not exposed by the API** (backlog `P3`), so this skill works from the
book's printed table of contents and must say so when there is none.

## Tasks

| ID | What | Specs | Route | Commit |
| --- | --- | --- | --- | --- |
| T1 | `skills/study-plan/SKILL.md`: connect, find the book, inspect it, classify, ask, plan, hand off | S1-S8 | inline | `feat(study-plan-skill)` |
| T2 | `references/classification.md`: the four types, their signals, their retrieval test, the traps | S2, S3 | inline | same |
| T3 | `references/plan.md`: plan template, session protocol, what-to-study-next rule, card standard pointer | S4, S5, S7 | inline | same |
| T4 | `README.md`: the package now carries two skills | S1 | inline | same |
| T5 | Verification: run T1's steps against the live corpus for `DDD.pdf` and produce a real plan | S2-S4, S8 | inline | same |
| T6 | `odd/tasks/backlog.md`: `P3` — sections exist in the database and are not exposed by the API | S8 | inline | same |

## Log

**L1 (user, verbatim).** "ok, antes de seguir con esto quiero que escribas una skill
donde se detalle el proceso que se debe seguir despues de subir un libro, donde se
clasifica, y se recomienda al usuario la mejor manera de leerlo, asi cuando cualquier
usuario suba un libro va a saber que hacer despues"

**L2 — design decisions taken, with the alternative rejected.**

- **A second skill, not a reference inside `skills/study-system/`.** Skills route by
  `description`, and the existing one is already busy ("estudiar, repasar, tarjetas,
  flashcards, mazos"). Two jobs in one description degrade routing for both. A sibling
  directory is discovered automatically by the package, so distribution costs nothing.
- **English, matching the existing skill and the repository's convention.** The plan is
  delivered to the reader *in the reader's language*; the source stays English.
- **The skeleton comes from the printed table of contents, read through `/pages`.** The
  `document_sections` table holds a better answer (110 rows for `DDD.pdf`) and no
  endpoint exposes it. Reading the TOC pages keeps the skill API-only (S6) and honest
  (S8). The missing endpoint is recorded as `P3` instead of worked around with
  `sqlite3`, which a phone cannot do.
- **The skill never writes.** No cards, no topics, no concepts. It proposes a plan; the
  reader decides. Same posture as `study-system`'s "no acceptance, no write".

**L3 — evidence for the classification framework.** The four knowledge types and their
retrieval tests come from the session's prior discussion, which is itself the
distillation of standard learning science: retrieval practice over rereading, spacing,
desirable difficulty, elaboration. The inspectional pass follows Adler's *How to Read a
Book*, whose three levels (inspectional, analytical, syntopical) are the source of the
"inspect before studying" rule.

**L4 — verification: the skill's own steps, run against the live corpus (`DDD.pdf`,
`c6d1e9a3…`, 359 pages).** Every step in `SKILL.md` was executed through the HTTP API,
not described. What each returned:

| Step | Request | What came back |
| --- | --- | --- |
| skeleton | `pages?from=1&to=6` | contents on pp. 3-5, preface on pp. 7-11, Part I divider on p. 12 |
| ending | `pages?from=357&to=359` | p. 357 bibliography, pp. 358-359 an appendix — **not** the conclusion, which starts at p. 349 |
| density | `pages?from=100&to=100` | FACTORIES, citing [GHJV95], a patterns catalogue |
| vocabulary | `search?q=bounded context&document_id=…` | pp. 239, 339, 340 — the probe works with `document_id` |

**Two defects in the skill that only running it exposed, both fixed:**

1. **"Read the last five pages" is wrong for a book with back matter.** `DDD.pdf` ends
   with a bibliography and an appendix, so the step would have sent a reader to
   references as if they were the conclusion. The step now says to check the contents
   first and read the end of the last chapter of content.
2. **The classification needs the catalogue signal.** The contents alone read as neutral;
   p. 100 (patterns by name, citing the GoF book) is what makes "conceptual with a
   procedural tail" defensible instead of guessed.

The plan the skill produces for this book, as evidence that the procedure terminates in
a usable artifact:

```text
Libro: DDD.pdf — 359 páginas
Tipo: conceptual, con cola procedimental (Parte II)
Test: aplicar el criterio a un caso nuevo, no definir el término

Núcleo (≈28% del libro)
  cap. 2   Communication and the Use of Language   pp.  24-36
  cap. 5   A Model Expressed in Software           pp.  60-87
  cap. 6   The Lifecycle of a Domain Object        pp.  88-114
  cap. 14  Maintaining Model Integrity             pp. 235-278

Orden: 2 → 5 → 6 → 14, y cap. 7 (pp. 115-132) como examen del núcleo
Descartar: Parte V (349-359, conclusión y glosario), cap. 15-17 (279-348)
           hasta tener un problema organizacional; cap. 8-13 después de tener
           un modelo propio que refactorizar
```

What the verification did **not** prove, said plainly: the skill has not been run by a
real reader from a phone against a remote server, and no book without a printed contents
was tried. Those are the two cases most likely to need another refinement.

**L5 — commit slice boundary.** Delivered as three reviewable units on
`feat/study-plan-skill`, all before any push:

| Commit | Unit | Lines |
| --- | --- | --- |
| `chore(gitignore)` | the ignore rule for a data directory moved aside | 4 |
| `docs(backlog)` | D8, D9, P2, P3 — the findings the re-ingest produced | 170 |
| `feat(skill)` | the skill, its two references, the README section and this document | 567 |

The slice closes here: 741 authored lines across three commits, past the ~400-line
review unit, so anything else — implementing `P3`, wiring `P2`, the ingest progress in
the capture panel — is a **new slice**, not an addition to this one.

