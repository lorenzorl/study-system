# Resource launcher reads the API — `GET /api/resources` and the `material` kind

**Status:** in progress · **Created:** 2026-10-22 · **Depends on:** the
`resources` table and `POST /api/sync/resource` (already in the middleware), the
`pages/resources` launcher slice, and `PRESENTATION-UI.md` §§2, 3, 6.

Verdict recorded before any source write: **viable, two apps, one read path.** The
middleware already stores real resources (`book | note | article | video`) but
exposes only the write; the launcher renders eight fixtures. This slice adds the
read endpoint, promotes `entities/resource` with its superseding entry in
`ARCHITECTURE.md` §14, and gives the launcher a fourth kind, `material`.

## Specs

**S1 — The request.** "quiero que modifiques el plugin para que obtenga los
recursos de la api"

**S2 — The source (approved).** "Nuevo GET /api/resources (tabla resources)", in
the shape the approved option stated: "Go: domain.ListResources + sqlite List +
use case + handler + router", "Plugin: entities/resource con fetch, kind
\"material\"", "Riesgo: el launcher pasa de 3 kinds a 4 y la status line cambia".

**S3 — The other three kinds (approved).** "Se quedan, vacíos hasta tener
endpoint" — "El launcher lee la API y solo Material trae datos.
Tarjetas/Diapositivas/Exámenes quedan como kinds del modelo sin tarjetas (los
chips no muestran nada). No borro diseño ni tests, pero el modal de repaso y la
ruta de deck quedan sin origen de datos."

**S4 — The card action (approved).** "Abrir source_uri" — "La tarjeta muestra el
tipo (libro/nota/artículo/video) en la línea de estado y un botón que abre el
source_uri (URL externa o ruta del vault)."

## Tasks

| ID | Task | Specs | Route | Commit |
| --- | --- | --- | --- | --- |
| T1 | Read path in the study context: `domain.ResourceWithTopic`, `ResourceRepository.List`, the SQLite query joining `topics`, `ListResourcesUseCase`, composition wiring | S1, S2 | inline, TDD | `54b3111` |
| T2 | `GET /api/resources`: DTO, handler, router parameter, server wiring, README row | S1, S2 | inline, TDD | `54b3111` |
| T3 | `entities/resource`: DTO and domain types, `fetchResources`, `useResourcesStore`, public API; `ARCHITECTURE.md` entry superseding §14 | S2 | inline | `8de4899` |
| T4 | Launcher `material` kind: view model, mapping (`type` label + days since creation), filter chips, status line, loading/error/retry/empty states, fixtures no longer feed the list | S2, S3 | inline, TDD | `bec0bfd` |
| T5 | Open action: `sourceTargetOf` in the page model, `openVaultPath` in `shared/obsidian`, page wiring | S4 | inline, TDD | `bec0bfd` |
| T6 | Docs: `PRESENTATION-UI.md` kind, data source and states; `ARCHITECTURE.md` §15 | S2, S3, S4 | inline | `8de4899`, `bec0bfd` |
| T7 | Verification: `go test ./...`, `pnpm test`, `pnpm build`, HTTP smoke, differential check against `origin/master` | all | parent | — |

**Delivery slices (recorded before the last commit).** Three work-unit commits,
each reviewable on its own and in this order: `54b3111` the middleware read path
(code + tests + README), `8de4899` the `entities/resource` slice (+ §15), `bec0bfd`
the launcher's `material` kind (+ `PRESENTATION-UI.md`). If the owner wants them
apart for review, that is the chained-PR split, in that order.

## Log

**L1 (2026-10-22, the request).** "quiero que modifiques el plugin para que
obtenga los recursos de la api"

**L2 (scope resolved).** I asked where the launcher's data must come from,
because the middleware exposed `POST /api/sync/resource` only and the launcher's
model (`flashcards | deck | exam` with `due/total`, `slides`, `lastScore`) exists
nowhere in the backend. Answer: **S2**.

**L3 (scope resolved).** I asked what happens to the three fixture-only kinds and
what a material card does. Answers: **S3** and **S4**. Consequence recorded: the
launcher list stops using fixtures, so the "continue" zone renders only when a
progress source exists (`continueResource` stays `null`, the section is
conditional), and the review modal keeps its card fixtures as dev-harness card
content — not launcher data.

**L4 (design decisions taken inside the approved shape).**

- `GET /api/resources` answers `200` with a JSON array, never `null` for an empty
  corpus, ordered `created_at DESC, id ASC` so the launcher's stable order is the
  server's, not the client's.
- The item carries `id`, `title`, `type`, `source_uri`, `topic_name`,
  `created_at`. No `topic_id`: the launcher never renders it.
- All four stored types map to the single `material` kind; the type is a label in
  the status line (`Libro · agregado hace 3 días`), not a fifth spread of kinds.
- An unparsable `created_at` drops the age from the status line instead of
  inventing "hoy".
- Opening a material: `http(s)` opens a new tab; anything else is a vault path or
  an `obsidian://` URI and is opened through `workspace.openLinkText`.
- The three other kinds keep their model, filter and tests untouched; only their
  data source is gone until an endpoint exists (S3).

**L5 (TDD evidence, observed).** RED first: `go test ./internal/study/...` failed
with `undefined: app.NewListResourcesUseCase`, `resourceRepo.List undefined` and
`undefined: httppkg.NewListResourcesHandler`; the plugin suite failed to resolve
`./materials`, `./openSource` and `./useResourcesStore`. GREEN after the
implementations: `go test ./...` every package ok, `pnpm test` 24 files / 174
tests, `pnpm build` (tsc + esbuild) clean.

**L6 (the endpoint over real HTTP).** Built the server against a throwaway
`STUDY_HOME`: an empty corpus answers `200 []`, `POST /api/sync/resource` then
`GET /api/resources` returns the resource with its `topic_name` and a UTC
`created_at`, and `POST /api/resources` answers `405`.

**L7 (differential check).** The change touches shared code — the study router's
parameter list and the `ResourceRepository` port — so I built `origin/master`'s
server in a detached worktree and ran the same sequence against both binaries:
`sync/concept`, `sync/flashcards`, `sync/resource` (valid and invalid type),
`topics`, `concepts` POST and GET, `study/due`, `study/review` on an unknown card.
After normalizing ids and timestamps the two outputs were **identical**;
`GET /api/resources` is the only new surface.
