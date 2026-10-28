# Migrate the web app onto the shared/ui primitives

Replace the hand-rolled controls (53 `<button>`, 11 `<input>`, 3 `<textarea>`, 0
`<select>`, across 21 files and 2.459 lines of `<style>`) with the shadcn-vue
primitives already installed in `src/shared/ui`. Controls only: layout and grids
stay as CSS. `widgets/deck-viewer` is out of scope (reveal.js owns its shadow
root).

## Specs

- **S1** — The request: "ok entonces haz el merge con la rama principal, y dime con que
  podemos seguir para que toda la web app use los componentes".
- **S2** — Delivery: "Pushear recién cuando termine la migración" / "Primero migrar
  componentes, pushear una sola vez con todo junto". So `origin/master` stays at
  d70d9a2 until the migration closes, and every native review of an intermediate
  unit must pass an explicit `baseRef` + `committedOnly` (the origin/master default
  inflates the candidate — see `odd/tasks/shadcn-vue.md` L16 and the
  `rdd/native-review-base-ref` memory).
- **S3** — Visual safety net: "Playwright + screenshots (suma dependencia)" / "Playwright
  con smoke por página y screenshots. Detecta regresiones de estilos de verdad." /
  "Un smoke por ruta: 8 páginas, render + screenshot" / "Job en CI".
- **S4** — A migration, not a rewrite: every visible capability survives, the palette
  stays owned by `index.html` (§3.1 of `ARCHITECTURE.md`), the FSD guard stays green,
  and `shared/ui` primitives may not import from any higher layer.

## Tasks

Route: inline. The units are sequential (each needs the harness and the primitives
before it), so the Writer rule's "2+ independent units" condition does not hold.

| ID | Spec | What | Commit |
| --- | --- | --- | --- |
| T1 | S3 | Playwright harness: config, `/api/**` stubs, screenshot smoke over the routes, CI job | this commit |
| T2 | S1 | Add the missing primitives (`textarea`, `tabs`, `badge`) via the generator + the `cn` fix | this commit |
| T3 | S4 | Rewrite the 6 BEM assertions against accessibility (`role`/text), before any style moves | this commit |
| T4 | S1 | Forms → `input`/`label`/`textarea`/`button`: `create-topic`, `create-concept`, `sync-note`, `capture-material` | |
| T5 | S1 | `widgets/review-modal` → `dialog` (reka-ui focus trap, Escape, restore focus) | |
| T6 | S1 | `pages/flashcard-session` (15 buttons, 241 style lines) | |
| T7 | S1 | Entity cards (4 files) + `pages/resources` (badges) | |
| T8 | S1 | App shell, dashboard, lists | |
| T9 | S2 | Close: push once, after the migration is verified | |

## Log

- **L1** (user): "ok entonces haz el merge con la rama principal, y dime con que
  podemos seguir para que toda la web app use los componentes".
- **L2** (state): local `master` was already at 14ca817 (= HEAD), containing the six
  shadcn-vue commits; `origin/master` is d70d9a2, 19 commits behind. The merge itself
  needed nothing further, so S2 decides only when it gets published.
- **L3** (evidence, inventory): 53 `<button>`, 11 `<input>`, 3 `<textarea>`, 0
  `<select>` across 21 files. Hottest: `FlashcardSessionPage.vue` (15 buttons),
  `CaptureMaterialForm.vue` (3+5+1), `ResourcesPage.vue` (5). Palette usage is
  already covered by the bridge: `--background-modifier-border` 55×, `--text-muted`
  47×, `--text-normal` 42×, `--interactive-accent` 30×. `tabs` appears only in
  `CaptureMaterialForm.vue`, badges only in `ResourcesPage.vue`, and
  `DeckViewer.vue` attaches a shadow root (out of scope).
- **L4** (evidence, the trap): 6 SSR assertions target BEM classes that the migration
  deletes — `capture__submit`, `capture__tab--active`, `resource-card__action`,
  `review-modal__grade`, `study-dashboard__link--active`, `sync-note__submit`. T3
  moves them to `role`/text first, so the suite stops pinning the implementation.
- **L5** (decision): the smoke stubs `/api/**` with Playwright route interception
  instead of requiring the Go middleware, so a screenshot is deterministic and the
  CI job needs no backend. A page that only renders its error state is not a visual
  regression test.
- **L6** (user): chose S2 ("Pushear recién cuando termine la migración") and S3
  ("Playwright + screenshots"). The plan itself was presented in chat and these two
  choices adopt it.
- **L7** (defect found, T1): the first Playwright run died after the full 180s with
  `Timed out waiting 180000ms from config.webServer`. Cause, measured not guessed:
  `vite preview` binds **IPv6 loopback only** — `lsof` showed `TCP [::1]:4173
  (LISTEN)`, `curl 127.0.0.1:4173` returned `000`, `curl localhost:4173` and
  `[::1]:4173` returned 200. Playwright polled `127.0.0.1`, a socket that did not
  exist. Fixed by pinning both sides to IPv4 (`pnpm preview --host 127.0.0.1`).
  Second defect in the same run: `pnpm exec playwright test ... | tail -30` exited 0
  because `tail` succeeded, hiding the real failure — background commands now run
  bare. Both recorded in the `web/playwright-visual-net` memory.
- **L8** (evidence, T1 verified): baselines generated (8 files, `darwin`), then the
  suite ran a second time **without** `--update-snapshots`: 8/8 passed, exit 0, so
  the pixel comparison is deterministic across runs. Baseline sizes 11.9-38.9 kB, so
  every capture has content — a blank page would have "passed" at a few kB.
  `@playwright/test` needs no `allowBuilds` entry (no install scripts), same as
  `@tailwindcss/oxide`. Remaining gap, stated in `ARCHITECTURE` §3.2: the Linux
  baseline set must be generated once on Linux and committed; until then CI reports
  missing snapshots rather than a false green.
- **L9** (evidence, T2): `pnpm dlx shadcn-vue@2.8.2 add textarea tabs badge` created the
  nine files and **re-added `cn@0.5.0`**, writing `import { cn } from "cn"` in six of
  them. The defect recorded in `ARCHITECTURE` §3.1 is reproducible on every `add`, not
  a one-off — the documented `sed` plus `pnpm remove cn` is the required follow-up.
  After it: guard 15/15 (the nine new files pass), suite 200/200, build 175 modules.
  The primitive set is now `badge, button, card, dialog, input, label, tabs, textarea`.
- **L10** (evidence, T3): all six BEM assertions are gone (`grep` → 0). Three of them
  needed **no component change at all**, because the semantics already existed and the
  tests were pinning the class instead: `RouterLink` already emits
  `aria-current="page"` on the exact-active link, the capture tabs already bind
  `:aria-pressed`, and the grades already sit in a `role="group"
  aria-label="Calificar"` with visible names. The one real accessibility defect found
  is in `ResourcesPage`: every card's action button had the same accessible name
  ("Repasar", "Presentar"), so a screen reader announced four indistinguishable
  buttons. It now carries `aria-label` with the resource title.
  Two pitfalls worth remembering: a helper that returns only the opening tag cannot
  assert on a button's label (the text lives in the content), and building a helper
  that returns only the opening tag is exactly why the tab assertion failed once.
  Verified: suite 200/200, and the visual smoke still 8/8 — the baselines tolerate
  attribute-only changes, which is what makes them usable during the migration.
