# web-app — migrate the Obsidian plugin SPA to a standalone web app

Goal: `apps/web`, a Vite + Vue 3 + FSD app served by the Go binary, replacing the
Obsidian plugin. Slices 1-6 of `PRESENTATION-UI.md` survive; only the platform
edges change.

## Specs

S1 — Abandon the Obsidian plugin. Verbatim request: "Quiero que sea un proyecto
web vamos a abandonar el plugin de obsidian, cual es lamejor manera?"

S2 — Migration path A, verbatim: "A · Extraer apps/web y congelar el plugin
(recomendado)". Its committed shape:
`apps/web/` is a new app built on the current code (`src/` moves into it);
`apps/obsidian-plugin/` is frozen — no new features — and deleted once parity
holds. Rollback: the plugin keeps working at every point until deletion.

S3 — Deployment, verbatim: "Solo local (un binario Go, localhost)":
`go run ./cmd/server` → `http://localhost:8080`; the binary embeds the SPA; no
auth; not exposed to the network. Consequence for this repo: the Go server must
answer `/` with the built SPA (today it 404s), and dev uses the Vite proxy so
there is no cross-origin request.

S4 — Material capture, verbatim: "Subir archivo + pegar texto + URL": "Material =
documento del corpus", "POST /api/rag/documents (pdf/archivo)", "pegar texto →
mismo endpoint", "URL → descarga + ingest". Consequence: the backend needs a
text path and a URL path; today `POST /api/rag/documents` accepts PDF only
(`internal/rag/http/upload_handler.go`, `pdfMagic = "%PDF-"`).

## Tasks

- T1 — Extract `apps/web` from the plugin SPA: it builds, its 22 suites pass, and `/` serves the resource launcher against the real API (S2) — route: inline — commit: `1deecd4`
- T2 — Serve the built SPA from the Go binary at `/` with an SPA fallback (S3) — route: inline — commit: `b7ae10d`
- T3 — Text ingest: `POST /api/rag/documents/text` stores pasted markdown as a document with its chunks (S4) — route: gentle-ai-worker (stalled on its last turn; the parent completed the wiring and one test assertion) — commit: `38de868`
- T4 — URL ingest: `POST /api/rag/documents/url` fetches http(s) only, refuses loopback/private targets, then ingests (S4) — route: gentle-ai-worker + independent `gentle-ai-verify` (both findings closed by the parent) — commit: `5be5c1d`
- T5 — Capture UI in the launcher: upload a file, paste text, paste a URL, wired to the endpoints (S4) — route: gentle-ai-worker, re-run after it found a real spec defect (an empty `source_uri` cannot be registered; the parent chose `rag://documents/<id>`) — commit: `58a6fa9`
- T6 — Replace Obsidian's "sync active note" with a paste-markdown flow on the topics page (S1, S2) — route: gentle-ai-worker — commit: `2461c81`
- T7 — Delete `apps/obsidian-plugin` and update `README`/`ARCHITECTURE.md`/`PRESENTATION-UI.md` (S2) — route: parent (deletion) + gentle-ai-worker (docs, README, CI) — commit: `cd0fcad`

## Log

L1 — Verbatim request: "Quiero que sea un proyecto web vamos a abandonar el
plugin de obsidian, cual es lamejor manera?"

L2 — Decisions from the three-option questionnaire, verbatim option labels:
"1. ¿Qué camino de migración tomamos?" → "A · Extraer apps/web y congelar el
plugin (recomendado)"; "2. ¿Dónde va a correr la web?" → "Solo local (un binario
Go, localhost)"; "3. ¿Qué reemplaza al flujo de 'nota activa' del vault?" →
"Subir archivo + pegar texto + URL".

L3 — Evidence gathered before planning (read-only): 8,915 lines across
`src/**/*.{ts,vue}`, 24 `*.test.ts` suites; only 5 files import `obsidian`
(`app/main.ts`, `app/study-view.ts`, `app/settings-tab.ts`, `app/settings-store.ts`,
`shared/obsidian/*`, ~318 lines); `dev/` already mounts the same SPA in a browser
with `createWebHistory` and a `/api` proxy; the Go server has no static mount
(`internal/server/*.go:134` → `rootMux.Handle("/", studyRoutes)`, so `/` 404s);
`resources` table holds 0 rows; RAG exposes GET/POST `/api/rag/documents`,
`/search`, `/{id}/status`; `POST /api/rag/documents` accepts PDF only and
requires `STUDY_OPENROUTER_API_KEY`.

L4 — Findings this plan accepts as scope:
- The vault-only behaviors lose their meaning in web: `ensureFolder` on
  create-topic/create-concept (deleted), `getActiveFile*` on the topics page
  (replaced by paste in T6), `openVaultPath` for a material (T5 gives materials a
  real target). No `shared/platform` port is introduced: with a single platform
  it would be a fiction. Revisit only when a second platform exists.
- Settings persistence moves from Obsidian's `data.json` to `localStorage`;
  `shared/config` is already pure and port-based, so only the adapter changes.
  Delivered as a narrower change than planned: with one deployment (same origin),
  there is nothing to persist on load, so the app has no settings load at all and
  the port keeps no consumer yet.
- URL ingest is a security item: server-side fetch of a user-supplied URL needs a
  scheme allowlist, private/link-local/loopback refusal, a timeout, and the
  existing size ceiling. T4 carries it.
- The plugin's "sync active note" command and its vault folders disappear in T7;
  the plugin directory is deleted, not archived, because git holds it.

L5 — Slice boundary for the extraction (T1): one commit, 129 files / ~9.6k inserted
lines, of which ~9.1k are the app's own files moved out of the plugin with no edit.
The authored delta is the scaffold (package/tsconfig/vite/vitest/index.html), the
entry, the router, the two defaults (api base same-origin), the App shell and its
render test, the four obsidian-coupled call sites, and the boundary rule. It is one
commit because none of those halves builds alone — the same reason
`PRESENTATION-UI.md` §9 records for slice 2. T2 is 620 lines, inside budget.

L6 — `.github/workflows/release.yml` still builds and releases the Obsidian plugin
on a version tag. It does not run on PRs and T1/T2 do not break it, but T7 owns
replacing or removing it: a release that only ships the plugin would be the last
lie in the repo.

L7 — T3 was delegated to `gentle-ai-worker` and the run stalled after 24 turns,
leaving a tree that did not compile: `NewRouter` had gained a fifth parameter while
`server.go` and two existing test routers still passed four, and one test file was
missing an import. The parent finished the wiring (`composition.TextIngest`, the
router mount, the two test routers) and corrected one assertion: the test expected
`json.Unmarshal`'s "unexpected end of JSON input", but the handler streams with a
decoder, which says "unexpected EOF". The assertion now pins what the test owns —
the 400 carries the decoder's reason — instead of the stdlib's wording.

L8 — T4 was delegated to `gentle-ai-worker` and then independently verified by
`gentle-ai-verify` before the commit. The verifier proved two findings by mutation:
the dial-control test passed with `Control:` removed from the fetch client, and the
whole suite passed with the production rule replaced by allow-all. Both are closed:
- The production constructor now takes no rule and installs `PublicAddressRule`
  itself, so no wiring can relax it; tests use `NewIngestURLHandlerWithRule`.
- Two tests fail without `Control`: a fake-DNS rebinding test and a dial-abort test
  (both checked by removing the line and watching the 201 come back).
The parent also closed two residual risks the verifier named: NAT64/6to4 embedded
IPv4 is now judged as the IPv4 it carries (refused for loopback, allowed for a
public address), and the response header block is capped. The 25-second deadline
test was replaced by a 50ms one through an injected timeout, so the suite runs in
2.4s instead of 25s.

L9 — Accepted limitations of T4, named so they are not silent:
- No port policy: the server will fetch an http URL on any port of a public
  address, so it can probe a public Redis or Memcached. Refusing the ports would
  need the rule to see the port, not just the address, and the spec named address
  classes only. A candidate follow-up, not a gap in the loopback guarantee.
- An HTML page is refused, not extracted: turning markup into study material needs
  an extractor, which is a separate behaviour with its own tests.
- A malformed `Location` surfaces as 502 (the Go client parses the header before
  the handler sees it), so the handler's 400 branch for that case is unreachable.
  Harmless, and left rather than restructured mid-slice.

L10 — Native review could not start, and the candidate is left unreviewed for that
reason. RDD is on; the user never left this target unreviewed explicitly, so START
was attempted for target `sha256:6b3d5c87…` (master..HEAD, nine commits) and then for
progressively smaller candidates:
1. the whole committed range, four lenses → refused;
2. T4 alone (`38de868..5be5c1d`, 2,033 lines), risk + resilience → refused;
3. T2 alone (`1deecd4..b7ae10d`, 620 lines), risk + resilience → refused;
4. a two-file, no-behaviour chore (`b7ae10d..cbec75e`), one lens → refused.
All four failed at preflight with the same provider-owned code,
`lens_context_budget_exceeded`: `mutation_outcome: none`, `lineage_created: false`,
no authority created, nothing to abandon or repair, and no consent prompt ever
reached the human. Because a two-file single-lens candidate fails identically, the
cause is the provider's reviewer budget in this environment and not this change's
scope — so the remedy the failure names ("split it into smaller reviewable
commits") cannot work here, and further attempts would be ceremony.
What stands in the review's place: the independent `gentle-ai-verify` run on the
security unit (L8, two findings found and closed), two mutation checks (dropping
the dial control; widening the production rule), the full suites (16 Go packages,
31 web suites / 192 tests), and the real-process proofs recorded above.
If a native review is wanted later, the budget has to be fixed first; nothing in
this repository can raise it.

L11 — A second target was offered after the L10 record was committed
(`sha256:5ee10e1a…`). `inspect` confirmed it is the same candidate scope: the same
`paths_digest` (`sha256:654548c1…`, 280 paths) and the same `base-ref`, differing
only in the snapshot identity because the docs commit changed. No START was issued
for it: the ceiling was already proven independent of candidate size by probe 4 in
L10 (two files, one lens, same refusal), and this candidate is strictly larger than
that one. The disposition stands — unreviewed, cause recorded, nothing to repair.

L12 — L10 and L11 were wrong, and this corrects them. The reviewer budget is a real
size ceiling, not a broken one: every probe in L10 was a large candidate because
`baseRef..HEAD` always ends at the tip, so passing an older base made the range
BIGGER, not smaller (the "two files" probe actually carried seven commits). The
probe that settled it used a candidate whose tip WAS the slice:
`git switch -c review/t4 5be5c1d`, START with `baseRef=38de868`. The review ran with
lineage `review-31d1edca294d85bb`, tier medium, lenses risk + resilience, 15 files /
2,071 lines, two reviewer runs through `pi_host_relay`, and closed **approved**; the
authority was acknowledged and burned (`gentle-ai.review-acknowledged/v1`).
Six advisory findings came back, all non-blocking and each recorded as later work:
three from risk (`userinfo credential persisted`, `resolved address disclosed in a
400`, `unauthenticated outbound egress`) and three from resilience (`R4-001`, `R4-002`,
`R4-003`). The fix below closes the only one that leaks a secret to disk and screen.
A lesson worth keeping: to review one commit in isolation, that commit must be the
tip; the candidate is never a subset of the branch.
