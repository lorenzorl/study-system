# Backlog — open findings

**Single source of truth for everything deliberately not fixed yet.**
Updated: 2026-10-17 (step 6 — HTTP upload and processing status)

A follow-up is a finding that was real, but outside the scope of the step that
found it. Fixing it mid-step would make that step unverifiable: you could no
longer tell whether a regression came from the step's change or the side fix.

Tasks that are part of the current step's acceptance criteria are **not**
follow-ups. They are tasks.

Step files carry a pointer to this document instead of their own copy, because a
duplicated list rots: the first version of this backlog was copied across four
step files and three entries were stale within two steps.

---

## 🟡 Unverified assumptions

### A1 — `config.DefaultCORSOrigins` was never measured against a real device

The allowlist (`app://obsidian.md`, `capacitor://localhost`, `http://localhost:*`)
comes from general knowledge, not from capturing a real `Origin` header. If it is
wrong for a legitimate client, the plugin fails with a confusing CORS error.

**How to close:** open the plugin in Obsidian on desktop and on mobile, capture
the actual `Origin`, update the list in `internal/platform/config/config.go`.

**The browser dev harness cannot close this.** `vite.config.ts` proxies `/api` to
`:8080`, so the harness's requests reach the server server-side and never exercise
the CORS path at all. Verified: the harness serves fine (`GET /` and `GET /main.ts`
return 200) precisely because it proxies. A real Obsidian client is the only way.

**Escape hatch while open:** `STUDY_CORS_ORIGINS='*'` restores the previous
permissive behaviour with no code change.

**This is the only assumption in the codebase with no evidence behind it.**

---

### A2 — The figure signal only sees raster images, and it was never tested against the real book

`domain.NeedsModel` sends a page to the model when `pdfimages -list` reports an
embedded image on it. On the three downloaded PDFs measured here the signal
behaved correctly: a born-digital text-only paper sent 0 pages, the LaTeX paper
sent exactly its two figure pages, the astrophysics paper sent its five.

**What is not established:** a diagram drawn with vector operators (TikZ, Visio,
a pgfplots chart) is not an embedded image, so a page whose only figure is vector
and whose text is long enough never reaches the model. That matters, because
describing diagrams is the main thing the model adds over `pdftotext`.

**An earlier version of this entry claimed the defect was proven. It was not.**
The `<path>` count used to prove it also counts text glyphs, and rendering the
page in question showed plain text and no figure. The claim was withdrawn after
looking at the page.

**How to close:** measure the real 359-page book in step 5 — `pdfimages -list` per
page, then render the pages `DDD-full.md` described as diagrams and look at them.
If those figures turn out to be vector, extend the signal (count drawing
operators per page, or compare rendered ink coverage) or accept that only raster
figures are ever described. Do not extend it before measuring: a second signal
would add cost to every page of every book.

---

## 🟢 Cosmetic and tooling

### C1 — `dev/main.ts` fails type checking, but the dev harness works

`apps/obsidian-plugin/tsconfig.json` includes only `src/**/*.ts` and
`src/**/*.vue`, so `dev/main.ts` is outside the build's type check and outside the
scope of `src/shims-vue.d.ts`, which is what declares the `*.vue` module shape.
An editor analysing that file standalone therefore reports that it cannot find
`../src/vue-app/App.vue`.

**Pre-existing, not caused by the restructure:** `dev/` was last touched by
`464410a` (June), and the Dify removal never edited it.

**It is cosmetic, verified rather than assumed:** `npm run build` passes,
`npm test` passes, the plugin works in Obsidian, and vite serves the harness fine
(`GET /` and `GET /main.ts` both 200, with `App.vue` resolved by
`@vitejs/plugin-vue`). Vite needs no TypeScript declaration to handle a `.vue`
import; only the type checker does.

**Why it is not fixed here:** the obvious fix — adding `dev/**/*.ts` to `include`
— would make the build type check that file, and since the shim does not reach it,
the build would start failing. So the fix is not a one-liner, and doing it inside
an unrelated slice would risk breaking a build that currently works.

### C2 — Two contexts keep their own copy of two shared rules

`study/domain/time.go` and `rag/domain/time.go` hold the same `InstantPrecision`
and `Now()`, and `study/sqlite/errors.go` and `rag/sqlite/errors.go` hold the same
constraint-violation matching. Both copies are deliberate: a context must not
import another context, and promoting either into `platform` would have meant
editing already-applied study code inside an unrelated slice.

**Why it is tracked instead of shrugged at:** the timestamp rule is load-bearing,
not cosmetic. Stored instants are compared as strings in at least one query, and
that is only correct while every writer shares one fixed-width layout. Two copies
can drift apart silently, and the symptom would be an ordering bug in a distant
feature — a resume that picks the wrong job, or a due-cards query that misses.

**How to close:** the first time a third context needs either rule, move it to
`platform` and delete both copies in the same commit. Not before: a shared helper
with two users is still cheaper as two copies than as an edit to code that no
longer has a test failing.

### C3 (closed) — Step 7's review left six non-blocking findings

The composition extraction ([`composition-root.md`](composition-root.md)) was
approved with six advisory findings. None blocked anything and none was a defect
a failing check proved. They were opened and closed in the same session: the
follow-up work was going to touch these same lines, so carrying them would have
meant re-reading them twice.

| Id | Lens | Severity | Location |
| --- | --- | --- | --- |
| `R3-unused-client-construction` | reliability | WARNING | `internal/composition/clients.go` |
| `R2-004` | readability | WARNING | `internal/composition/composition.go` (`OpenPlanningDatabase`) |
| `R2-001` | readability | WARNING | `internal/composition/rag.go` (`Process`) |
| `R3-ingest-factory-untested` | reliability | SUGGESTION | `internal/composition/rag.go` (`Ingest`) |
| `R2-002` | readability | SUGGESTION | `internal/composition/composition_test.go` (the study wiring test) |
| `R2-003` | readability | SUGGESTION | `internal/composition/rag.go` (`Search`) |

**What each one changed.** `R3-unused-client-construction`: `NewClients` split
into three role constructors — `NewClients`, `NewEmbeddingClients`,
`NewTranscriptionClients` — over one `buildClients` helper, so `search` and
`embed` build only the embedding client and the conversion run only the chat
client. (The flags those names came from are subcommands since step 8.) `R2-004`: the discarded read-only error is named, and the comment now
says the fallback covers every failure, not only a missing file. `R2-001`:
`Process`'s comment stopped restating `Embed`'s and says what the error return is
for. `R3-ingest-factory-untested`:
`TestRAGIngest_ZeroClientsDoesNotBecomeATypedNilTranscriber` pins the narrowing
through the only seam there is, reflection, and says so. `R2-002`:
`TestNewStudy_WiresEveryUseCaseToTheGivenDatabase` now exercises all eight use
cases. `R2-003`: the parameter is `embeddingModel` in `Search`, `Embed`,
`Process`, and `Status`, and `Search`'s comment explains why it is required even
on the lexical path.

**The caveat worth keeping.** The review returned identifiers, lenses, severities,
and locations — never the findings' prose. Four of the six were read off their
line and interpreted. If the reviewer meant something else at `rag.go:56` or
`composition_test.go:206`, this entry is where that becomes visible, because what
was changed is written here and not only in the commit.

### C4 (closed) — the review of the six-finding fix left two more findings

Lineage `review-e0b962053adefb03`, tier medium, 300 lines, lenses readability and
reliability. Approved, both findings closed in the next commit.

| Id | Lens | Severity | Location |
| --- | --- | --- | --- |
| `R3-command-wiring-silent-fallback` | reliability | WARNING | `cmd/ingest/main.go` (`-search`) |
| `R3-planning-db-fallback-scope` | reliability | SUGGESTION | `internal/composition/composition.go` (`OpenPlanningDatabase`) |

**What each one changed.** The first: `-search` could fall back to a lexical
answer if its embedding client went missing, and a lexical answer looks correct,
so the downgrade was invisible. `composition.RequireEmbedding` now fails when a
key is configured and no client was built, and both entrypoints call it. The
second: `OpenPlanningDatabase` fell back to an empty schema for **any** open
failure, so an existing database that could not be read planned as if it were
empty and reported "0 pages already recorded" for a book it knows. The fallback
is now scoped to a missing file, through `missingFile`.

**Deliberate behaviour change, and the only one in step 7:** a dry run against a
database that exists but cannot be opened now fails instead of planning empty.
Safe direction — silence there meant sending a whole book to the model — but it
is not a refactor, so it is written down here rather than only in the commit.

### C5 (closed) — the review of the D2 change left nine findings, all closed

Lineage `review-7e48e635cd3a6ff8`, tier medium, 20 files, 1330 lines, lenses
resilience, readability and reliability. Approved; authority burned. The nine
findings were advisory (none opened a correction), and all nine were fixed in the
commit that follows.

| Id | Lens | Severity | Location |
| --- | --- | --- | --- |
| `R3-due-fetch-failure-masked` | reliability | WARNING | `pages/flashcard-session/ui/FlashcardSessionPage.vue` (due-mode head) |
| `R3-async-mount-race` | reliability | WARNING | same file (`onMounted`) |
| `R4-1` | resilience | WARNING | same file (`onMounted`) |
| `R4-2` | resilience | WARNING | `pages/resources/ui/ResourcesPage.vue` (`openContinue`) |
| `R2-003` | readability | WARNING | `pages/resources/model/continueEntry.test.ts` (the id test) |
| `R2-001`, `R3-duplicated-decision-heading` | readability, reliability | SUGGESTION | `apps/web/PRESENTATION-UI.md` §8 |
| `R2-002`, `R3-stale-props-comment` | readability, reliability | SUGGESTION | `FlashcardSessionPage.vue` (the props comment) |

**The two that mattered were one class of defect: a failed load and an empty
result were the same screen.** On the session page, `dueModePending` did not know
about `reviewError`, so a failed fetch kept the spinner and never showed the
error; and the page claimed a session it had not loaded. Now `reviewError` wins
over pending, `startSession()` runs only when the load succeeded (including on a
retry, which is also how the page enters its session), and the launcher keeps its
continue row on failure with `No se pudieron cargar los pendientes de hoy.` plus
`Reintentar` instead of hiding it — "nothing to resume" and "we could not ask"
stop looking identical.

**One owner for the session.** `openContinue` used to call `startSession()` and
then navigate, so a navigation that did not commit left a started session behind,
and the session had two owners. The route owns it now, and the page reuses the
cards the launcher already counted (`hasCards` gates the fetch), so the entry
still costs one request.

**The async mount is guarded.** `resolveDueSession` awaits the fetch, so it sets
an `unmounted` flag in `onBeforeUnmount` and writes nothing after it — no store
mutation and no keydown listener left behind. **Declared limit:** no test covers
this one. The unit suite is node/SSR and never runs the mount lifecycle, and an
interrupted fetch is not reliably reproducible in Playwright; the guard is
asserted by construction.

**The dead field is gone.** `ContinueResource.id` was pinned by a test whose
comment claimed the action navigated by it — it did not, and nothing read it. The
field and that test are deleted; the zone holds at most one entry, and the commit
that adds a second kind of resume is the commit that adds an identifier.

The two doc suggestions were my own duplication: §8 carried the
`Decided 2026-10-30:` heading twice, and the props comment still said the
parameters were required after they became optional.

### C6 (closed) — the review of the C5 fix left six findings, and two of them corrected C5

Lineage `review-71bb6586ffe63576`, tier medium, 9 files, 230 lines, lenses
resilience and reliability. Approved; authority burned. All six closed in the
commit that follows. This candidate was scoped to `6cff49e..58166d5` — the source
commit alone — because the default projection (the whole unpublished range) would
have re-reviewed the already-approved D2 change and hit
`lens_context_budget_exceeded` on the minified bundles again.

| Id | Lens | Severity | Location |
| --- | --- | --- | --- |
| `R3-003` | reliability | WARNING | `FlashcardSessionPage.vue` (`resolveDueSession`) |
| `R3-001` + `R4-generic-error-copy` | reliability, resilience | WARNING, SUGGESTION | `ResourcesPage.vue` (the failure row) |
| `R4-retry-click-fanout` | resilience | WARNING | `ResourcesPage.vue` (`retryDueCards`) |
| `R3-002` | reliability | WARNING | `FlashcardSessionPage.vue` (`attachSessionKeys`) |
| `R4-no-abort-on-unmount` | resilience | SUGGESTION | `FlashcardSessionPage.vue` (the `unmounted` comment) |

**C5 was wrong in two places, and this is the correction.** (1) C5 said the page
"reuses the cards the launcher already counted (`hasCards` gates the fetch), so the
entry still costs one request". That shortcut is a bug: *has cards* is not *the
cards are current*, so a queue the user had already graded could come back as
pending. The page always fetches now; one request is the cheaper price. (2) C5 said
the `unmounted` guard means "no store mutation". It does not: `fetchDueCards`
writes `dueCards`, `loading` and `error` as it settles and the store offers no
abort signal, so the guard stops the session and the listener, nothing more. The
comment says exactly that now.

**The rest.** The launcher's failure row printed a fixed sentence and threw the
store's own message away, while every other error surface on that page prints it;
it prints `reviewError` now. The retry is re-entrant-safe: disabled while a retry
is in flight, plus a guard in the handler. And the keydown listener is registered
in one place — `resolveDueSession` and `retryReview` both end in
`attachSessionKeys()` instead of repeating the two-line choice.

**Declared limits:** the always-fetch and the retry guard have no test that could
catch their removal. The unit suite is node/SSR (no mount lifecycle, no clicks) and
Playwright starts each page with an empty store, so "the store already had cards
and the page fetched anyway" is not a state it can reach. The listener dedupe is behaviour-preserving and
covered by the entry's click test.

---

## 🔴 Open defects

### D1 (closed in `35d51f0`) — `/topic/:id` could not be opened cold: the pages never loaded the topics store

`TopicPage.vue`'s `onMounted` only calls `topicSelection.selectTopic(props.topicId)`;
it never calls `topicsStore.loadTopics()`. `currentTopic` is a lookup in that
store, so a direct visit — deep link, reload, bookmark — renders
"Tema no encontrado." The page is reachable only by warming the store first, for
example `/topics` → topic card.

Found by T4b, which could not click "Agregar concepto" on a cold `/topic/t1`: the
Playwright failure snapshot showed only the paragraph "Tema no encontrado." and a
`← Volver` button.

Two consequences, the second worse than the first:

1. Deep links and reloads to a topic are broken.
2. The committed `topic-darwin.png` route baseline is a capture of that error
   state, so the visual net's route loop gives false confidence about the topic
   page: it screenshots a failure and passes. L5 stated the rule ("a page that
   only renders its error state is not a visual regression test"); it was never
   verified per route.

**Closed by** `35d51f0`: `shared/lib/loadIfEmpty.ts` (one rule, four unit cases,
in-flight dedupe, re-arm on settle), wired into `TopicPage`, `ConceptPage` and
`FeynmanPage`; `topic`, `concept` and `feynman` baselines regenerated against real
content, and the concept-form capture went back to a direct `/topic/t1`. What it did
**not** fix is recorded as D6.

**Broadened by T8:** it is not only `TopicPage`. `ConceptPage` and `FeynmanPage`
also never call `loadTopics()` (only their `retry` handler does), so a cold
`/topic/:t/concept/:c` or `/topic/:t/concept/:c/feynman` renders "Concepto no
encontrado." too. With D2's `/study/review`, **four of the eight route baselines
photograph a failure** — `topic`, `concept`, `feynman`, `review` — so the net covers
four real pages out of eight, and the concept action rows and the Feynman submit have
no pixel coverage at all. Fixing this is the same one change per page (load the store
when it is empty), then four baselines regenerated against real content.

### D2 (closed 2026-10-30) — The review modal had no reachable route, so the flashcards flow could not be used

`ReviewModal` was mounted only by `ResourcesPage`, and neither of its two entry
paths could fire:

- `continueResource` was hardcoded `null`, so the resume ("continuar") action was
  unreachable.
- `toMaterialResources` maps every API resource to kind `material`, so
  `openResource`'s `flashcards` branch never ran.

Consequence for the visual net: `/study/review` (`review-darwin.png`) was a capture
of "Concepto no encontrado." — another error-state baseline, the same class as D1's
`topic-darwin.png`. Regenerating it could never show the modal, because the route
never mounted one.

Found by T5, which migrated the modal to the `dialog` primitive and could verify it
only through a manual browser mount (dynamic import against the dev server), not
through the app's own navigation.

**The decision, taken by the owner before any source write:** the app keeps **one**
review surface. There were two implementations of the same interaction — the modal
(synchronous, fixture-driven, `grade` emitted to nobody) and the FSRS session at
`/study/review` (grades against the API, shows the next review date) — and a second
surface with no data source is exactly what D2 was made of.

**What closed it, three parts:**

1. **The launcher's continue entry has a real source.** `pages/resources/model/continueEntry.ts`
   turns `GET /api/study/due` into the one "Seguir donde quedaste" row (`Repaso ·
   Pendientes de hoy`, `0 / N`), and its action starts the session and routes to it.
   `continueResource` is a computed over the due cards, not a hardcoded `null`.
2. **`/study/review` owns its session.** The route carries no concept, so the page
   loads today's cards itself and enters due mode; a deep link or a reload reviews
   them instead of reporting a concept nobody asked for. The concept route
   (`/topic/:t/concept/:c/flashcards`) is unchanged, and the two modes are now one
   computed (`isDueMode`) instead of a flag another page had to set first.
3. **The modal is gone.** `widgets/review-modal` (component, `useCardSession`, both
test files) and `pages/resources/model/cardFixtures.ts` were deleted with its
wiring, so the widget inventory has no unreachable surface. The grade row that
lived in both places now lives in exactly one file.

**The visual net.** `review-darwin.png` is a real card now, `resources-darwin.png`
shows the continue row, and `interactive states` gained `review-revealed.png` — the
answer revealed with the four grades, which is also the pixel coverage D3 said that
row had nowhere (D3's colour half is still open).

**What is still true:** the `flashcards` kind still has no endpoint, so no flashcard
resource card can appear in the launcher; its chip renders an empty list, as S3 of
`resource-launcher-api.md` accepted. The due session, not a card, is how cards are
reached.

### D3 — The grade row's colours bypass the palette, and the net cannot see it

`FlashcardSessionPage` hardcodes three colours for its grade buttons — `#e67e22`
(Difícil), `#27ae60` (Bien), `#2980b9` (Fácil) — instead of taking them from the
palette `index.html` owns, which is what S4 requires. It is also the reason that row
could not migrate to `Button`: no variant expresses four distinct fills without
putting colour values into utilities, so T6 left those four buttons hand-rolled
while migrating the other eleven.

Two consequences worth naming:

- The `prefers-color-scheme: dark` block in `index.html` cannot retune those three
  colours: they are literals, not palette entries.
- Nothing in the visual net sees that row, nor the concept-mode session:
  `answerRevealed`/`showFeedback` live inside `useReviewSession`, the flip is a
  click, and no smoke fixture populates the flashcard cache, so
  `/topic/:t/concept/:c/flashcards` renders an empty state. A regression in the
  controls T6 *did* migrate there would pass every gate.

**How to close:** add the three grade colours to the palette in `index.html` and its
dark block, then migrate the row against those tokens. The pixel half is done:
`review-revealed.png` (added when D2 closed) captures the row in a real browser, so
a regression there now fails the net.

**Also here:** the `toContain("disabled")` trap the modal's render test carried is
gone with the modal (D2). The lesson stays: the assertion has to be on the
attribute, which is what `ResourcesPage.render.test.ts` and
`FlashcardSessionPage.render.test.ts` now do.

### D4 — Two token aliases are literal colours, and the migration made them load-bearing

`src/app/styles.css` declares `--primary-foreground: #fff` and
`--destructive-foreground: #fff` as literals, while ARCHITECTURE §3.1 says those
tokens are "references onto those variables, never new values". Until T7 that was
cosmetic; now every migrated primary button takes its text colour from
`--primary-foreground`, so a `prefers-color-scheme: dark` flip renders white text on
the accent instead of `--background-primary`.

**How to close:** point both at palette entries from `index.html` (the light and dark
blocks), then re-read the `due-cards` and `resources` baselines.

### D5 — `shared/ui/card` still has no consumer

The four entity cards cannot use it (the card *is* the button — see L19), and
`pages/resources` renders its own card frame. The primitive set therefore ships a
`card` nobody calls. The honest options are to give it a consumer in T8 (where the
launcher's cards keep the click target inward) or to delete it; shipping a primitive
no component uses is the one outcome that teaches nothing.

**Also here:** `pages/resources` still hand-rolls its search field with its own
padding, border and 44px floor although `shared/ui/input` exists; and `FlashcardCard`
has no pixel coverage anywhere (backlog D2), so its migration is pinned only by a
render test.

### D6 — Three loose ends D1 left behind

1. **`pages/flashcard-session` has the same mount gap.** Only its `retry` calls
   `loadTopics()`, so a cold `/topic/:t/concept/:c/flashcards` cannot resolve. It was
   outside D1's surfaces.
2. **Both fixed pages paint "no encontrado" for one frame** before `onMounted` runs and
   the store reports `loading`. Moving the load into setup would remove the flash, but
   it changes what the SSR render tests can observe — a separate decision.
3. **`FeynmanPage` has no error UI and no retry.** A failed load leaves it on the
   not-found paragraph with nothing to press. That was unchanged behaviour, but D1
   made the page reachable, so it is now observable.

**Also worth knowing:** the re-arm semantics mean a permanently failing `/api/topics`
refetches once per mount rather than once per session. Intentional (a dead end is worse
than a retry), unit-pinned, and worth revisiting if the endpoint gets flaky.

## 🟡 Missing policy

### P1 — Nothing prunes stored artifacts

Every `cmd/ingest` run accumulates a PDF in `data/artifacts` forever. With a
100 MiB per-artifact ceiling and a single user this is not urgent, but there is
no retention policy at all: not by age, not by count, and no delete command.

Note that `storage.Store.Delete(id)` already exists. What is missing is the
*policy*, not the tool.

---

## ⚪️ Not built by design, not debt

Recorded so they are not mistaken for oversights. They belong to a step that has
not run yet.

| Item | Owner | Why open |
| --- | --- | --- |
| Conversation/retrieval surface | step 5 slice 5.3 | Designed in `odd/tasks/step5-search.md`; the HTTP route is not built yet. |
| FTS5 query syntax (`NEAR`, prefixes, field filters) | step 5 slice 5.1 | The user's text is treated as literal terms joined with `OR`; recorded in `odd/tasks/step5-search.md`. |

---

## Resolved since being recorded

Kept so the same work is not proposed twice.

| Item | Resolved by |
| --- | --- |
| Migrations were not versioned | step 2 — `platform/db` runner with `schema_version` |
| SQLite ran without WAL, `busy_timeout`, or `foreign_keys=ON` | step 2 — PRAGMAs through the DSN |
| `study/sqlite` owned the migration mechanism | step 2 — mechanism moved to `platform/db`, contexts own their schema |
| Secrets and `data/study.db` were stageable | step 1 — committed root `.gitignore` |
| `httpx` read `STUDY_CORS_ORIGINS` from the environment itself | step 1 — `httpx.CORS` is a pure factory over an explicit list |
| Stored artifacts would have been committed | step 3 — `.gitignore` covers `data/artifacts/` |
| `B1` — HTTP responses carried timestamps off by the machine's UTC offset | B1 close — see the corrected description below |
| Instants did not survive a persist/read round trip | B1 close — `domain.Now()` truncates to `InstantPrecision` |
| `documents`, `ingestion_jobs`, and `job_pages` did not exist | step 4 slice 2 — `rag/sqlite` migration 100, with `(job_id, page_number)` as the primary key |
| `STUDY_OPENROUTER_API_KEY` and `STUDY_LLM_MODEL` were documented but unread | step 4 slice 4 — `platform/config` reads both, `platform/llm` consumes them, and `cmd/ingest` fails before page 1 when the key is missing |
| An ingestion run had no cost ceiling and no way to see the cost before paying it | ingest hardening — the run plans first and prints the plan, `-dry-run` stops there having written nothing, and `-max-model-pages N` refuses before anything is stored or created. The measured price of a real run (US$ 0,0013 per model page, 31s to 1m41s each) made time the real ceiling, which is why the plan reports counts and an estimate rather than only a cap |
| `-out` overwrote whatever was at that path | ingest hardening — a target without the generated header is refused, `-force` replaces it deliberately, and the file is written 0644 because a vault is shared |
| HTTP multipart upload endpoint + body-size middleware | step 6 — `POST /api/rag/documents` streams the body into `storage.Store.Put` under `http.MaxBytesReader`, and `GET /api/rag/documents/{id}/status` reports the derived state. An in-process background processor runs ingestion, indexing and embeddings; a restart loses in-flight state and re-uploading resumes |

### B1 (closed) — the original description was wrong, and that matters

The entry as first written said *"a card graded `Again` does not surface as due
for three hours in Argentina"*. **That was false**, and it was written from
reading the code rather than from running it.

What was actually true, established by test:

- `DummyFSRS` returned `time.Now()` — local, not UTC.
- But a `time.Time` is an **instant**: a local representation and a UTC
  representation of the same moment compare equal. `TestDummyFSRS_SchedulesThe
  SameInstantInAnyZone` passes both before and after the fix.
- The repository writes with `.UTC().Format(time.RFC3339)`, so the **database
  held the correct instant**.
- The due-cards query compares against `now.UTC()`, so **scheduling worked**.

The real defect was narrower and in the transport layer: four handlers formatted
with `Format("2006-01-02T15:04:05Z")`, where a bare `Z` is an ordinary character
rather than a zone token (that is `Z07:00`). Any value that was not already UTC
was therefore sent to the client **off by the machine's offset**. Three handler
tests demonstrated it directly: a scheduled `18:00:00Z` came back as
`15:00:00Z`.

Only `POST /api/study/review` was live: it returned the algorithm's value
directly. The other three were latent — their values happened to be UTC already,
so the hardcoded `Z` was right by accident. A latent landmine that fires the
moment a source changes is still worth removing.

**Fixed:** `fsrs.go` produces UTC; all four handlers call one `instant()` helper
that converts to UTC and uses `time.RFC3339`, so the format can no longer
misrepresent any value.

**Lesson worth keeping:** an unverified diagnosis in a backlog entry is its own
risk. This one overstated severity by claiming a broken feature where the defect
was a wrong field. The entry and the report to the user were both wrong until a
test settled it. Write the diagnosis with the evidence, or do not write it as
fact.

### The second defect, found while closing B1

RFC3339 carries whole seconds, but instants were created with sub-second
precision, so the value returned to a caller did not equal the value stored:

```text
reported  16:14:44.163922 +0000 UTC
stored    16:14:44        +0000 UTC
```

Eleven call sites each built their own `time.Now().UTC()`. They now all call
`domain.Now()`, which truncates to `InstantPrecision`, making the reported and
stored values the same one.

A note recorded there because it is not obvious and it is load-bearing: the
due-cards query filters with `next_review <= ?` **as a string**, so it relies on
lexicographic comparison. That is only correct because every stored value shares
one fixed-width layout. Switching to `RFC3339Nano` to preserve precision would
have broken it — `"16:14:44.1Z"` sorts *before* `"16:14:44Z"`, inverting two
instants a tenth of a second apart.
