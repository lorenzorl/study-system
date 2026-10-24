# Study system

A local study system: a web app for a library of study resources, and one Go
binary that serves it and its API on a single port.

The corpus is yours. You add material as a PDF, as pasted Markdown, or as a URL;
the server converts and indexes it, and search answers from it. The app is where
you browse that material, turn notes into flashcards, review what is due, and
open the deck surface. Everything runs on your machine: SQLite for storage, no
accounts and no authentication. The server listens on every interface, so keep
it on a trusted network rather than exposing the port.

## Layout

```text
apps/
├── web/                  Vue 3 + Vite single-page app, Feature-Sliced Design
│   ├── src/              app, pages, widgets, features, entities, shared
│   └── *.md              the design record (see Docs below)
└── go-middleware/        the server: API, SQLite, ingestion, and the built app
    ├── cmd/              server and cli entrypoints
    ├── internal/         bounded contexts (study, rag), platform, composition
    ├── scripts/          sync-web.sh, build-android.sh
    └── internal/server/spa/dist/   the committed web build, embedded by go:embed
odd/tasks/                feature documents: specs, tasks and the decision log
.github/workflows/ci.yml  vet, tests, build and the embedded-tree drift check
```

The web app is ordinary FSD code; the server is organized by bounded context
first and layer second. `apps/web/ARCHITECTURE.md` is the contract the app's own
boundary test enforces, and `apps/go-middleware/README.md` explains the server's
architecture, API and configuration.

## Requirements

- Go 1.25 or newer (`apps/go-middleware/go.mod` pins it).
- Node 22.13 or newer and pnpm 11. `apps/web/package.json` pins the pnpm version
  in `packageManager`, so use that version.
- Poppler for PDF ingestion; the app and the API run without it, and only
  conversion needs it (the middleware README has the details).
- `STUDY_OPENROUTER_API_KEY` for model work: PDF conversion is refused without
  it, and search falls back to lexical only. Everything else boots and runs
  without a key.

## Development

Run the API and the Vite dev server in two terminals:

```bash
# terminal 1: the API on :8080 (the web app is served from this one too)
cd apps/go-middleware
go run ./cmd/server
```

```bash
# terminal 2: the app on :5173, with /api proxied to :8080
cd apps/web
pnpm install --frozen-lockfile
pnpm dev
```

Open <http://localhost:5173>. Vite proxies `/api` to the Go server, so the
browser only ever talks to one origin and development exercises the same
same-origin request path as production.

The database and the stored artifacts live under `apps/go-middleware/data/` by
default and are created on first run. Set `STUDY_HOME` to keep them somewhere
else.

## Production

The server embeds the built app with `go:embed`, so the app has to be built and
copied into the server's tree before `go build` (or `go run`) sees it:

```bash
# build the app and copy it into the embedded tree
apps/go-middleware/scripts/sync-web.sh

# serve the app and the API from one process
cd apps/go-middleware
go run ./cmd/server
```

Open <http://localhost:8080>. `/` serves the launcher, `/api/...` serves the API,
and any other path falls back to `index.html` so a client route survives a
reload.

`apps/go-middleware/internal/server/spa/dist/` is committed because the embed
happens at compile time. Any change under `apps/web/src` needs the sync again;
`go generate ./...` from `apps/go-middleware` runs the same script. CI fails when
the committed tree no longer matches a fresh build, so a stale build cannot be
committed quietly.

## Tests

```bash
cd apps/web
pnpm test            # vitest: models, components, and the FSD boundary guard

cd apps/go-middleware
go vet ./...
go test ./...
```

The web suite includes `apps/web/src/architecture.test.ts`, which walks the
source tree and fails on an upward import, a same-layer sibling import, a deep
import into a slice, or any import of a platform SDK. The Go suite needs no
network, no API key and no PDF tooling for the packages that use test doubles;
the PDF adapter tests use the fixture in `internal/rag/infra/pdf/testdata`.

## Docs

- `apps/web/ARCHITECTURE.md` — the FSD layers, the dependency rule, the platform
  boundary, and the guard that enforces them.
- `apps/web/PRESENTATION.md` — the deck: Reveal, the shadow-root mount, and the
  decisions behind them.
- `apps/web/PRESENTATION-UI.md` — the interface: the resource launcher, its
  surfaces, and the delivery slices.
- `apps/go-middleware/README.md` — the server: layout, API, configuration,
  ingestion, search, migrations and CLI.
- `odd/tasks/` — feature documents with their specs, tasks and logs.
  `odd/tasks/web-app.md` records this migration.

## The Obsidian plugin is gone

This app was an Obsidian plugin until the migration recorded in
`odd/tasks/web-app.md`. The plugin's SPA moved into `apps/web`, the plugin was
frozen, and it was deleted once the web app reached parity. Git history holds it.

Keeping it would have meant maintaining two hosts for one interface: the Obsidian
vault was the plugin's only platform, and the deployment is now a local web app
served by the Go binary. There is no second consumer to justify a platform port,
so the app imports no platform SDK at all, and the release pipeline that built
plugin artifacts is replaced by CI that only tests and builds.
