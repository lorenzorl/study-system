# Backlog — open findings

**Single source of truth for everything deliberately not fixed yet.**
Updated: 2026-10-17 (B1 closed)

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

---

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
| `ingestion_jobs` table | step 4 (`internal/rag` migration) | The RAG context does not exist. It is what makes an ingestion resumable, which is what the retired `optimizebooks` script lacked: its run died at page 179 and was patched by hand. |
| Read `STUDY_OPENROUTER_API_KEY` / `STUDY_LLM_MODEL` | step 4 (ingestion) | Documented in the README, deliberately not read yet so there is no dead config field. |
| HTTP multipart upload endpoint + body-size middleware | step 4 (`rag/http`) | Uploads belong to the context that owns documents, together with `http.MaxBytesReader`. |
| Conversation/retrieval surface | step 5+ | Nothing is designed for retrieval yet. |

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
