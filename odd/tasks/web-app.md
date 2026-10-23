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
- T2 — Serve the built SPA from the Go binary at `/` with an SPA fallback (S3) — route: inline — commit: —
- T3 — Text ingest: `POST /api/rag/documents/text` stores pasted markdown as a document with its chunks (S4) — route: inline — commit: —
- T4 — URL ingest: `POST /api/rag/documents/url` fetches http(s) only, refuses loopback/private targets, then ingests (S4) — route: inline — commit: —
- T5 — Capture UI in the launcher: upload a file, paste text, paste a URL, wired to the endpoints (S4) — route: inline — commit: —
- T6 — Replace Obsidian's "sync active note" with a paste-markdown flow on the topics page (S1, S2) — route: inline — commit: —
- T7 — Delete `apps/obsidian-plugin` and update `README`/`ARCHITECTURE.md`/`PRESENTATION-UI.md` (S2) — route: inline — commit: —

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
- URL ingest is a security item: server-side fetch of a user-supplied URL needs a
  scheme allowlist, private/link-local/loopback refusal, a timeout, and the
  existing size ceiling. T4 carries it.
- The plugin's "sync active note" command and its vault folders disappear in T7;
  the plugin directory is deleted, not archived, because git holds it.
