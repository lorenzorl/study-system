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
- **S3** — Visual safety net (rewritten by L14): "Playwright + screenshots (suma
  dependencia)" / "Playwright con smoke por página y screenshots. Detecta regresiones
  de estilos de verdad." / "Un smoke por ruta: 8 páginas, render + screenshot". The
  original selection also said "Job en CI"; L14 withdrew that half, because a
  baseline is per platform and the committed set is macOS only. The net runs
  locally, on the machine that owns the baselines, and CI has no Playwright step
  (`ARCHITECTURE` §3.2).
- **S4** — A migration, not a rewrite: every visible capability survives, the palette
  stays owned by `index.html` (§3.1 of `ARCHITECTURE.md`), the FSD guard stays green,
  and `shared/ui` primitives may not import from any higher layer.

## Tasks

Route: inline. The units are sequential (each needs the harness and the primitives
before it), so the Writer rule's "2+ independent units" condition does not hold.

| ID | Spec | What | Commit |
| --- | --- | --- | --- |
| T1 | S3 | Playwright harness: config, `/api/**` stubs, screenshot smoke over the routes | 063f910 |
| T1b | S3 | Retire the CI step: the net runs locally until a Linux baseline set exists | 91d0488 |
| T2 | S1 | Add the missing primitives (`textarea`, `tabs`, `badge`) via the generator + the `cn` fix | f688a34 |
| T3 | S4 | Rewrite the 6 BEM assertions against accessibility (`role`/text), before any style moves | 3401a83 |
| T4a | S1 | Pilot: `create-topic` migrated, plus the CSS layer fix and the interactive-state captures it exposed | f48fb68 |
| T4b | S1 | Forms → `input`/`label`/`textarea`/`button`: `create-concept`, `sync-note`, `capture-material` | |
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
- **L11** (defect, T4a — caught by the net, invisible to 200 green tests): the first
  migrated form rendered with a **native black border** on `variant="outline"`, its
  label jammed against the edge, and the browser's own font. Three causes stacked:
  1. Skipping preflight leaves `<button>`/`<input>` with the UA frame, font and padding.
  2. `index.html` carried `* { margin: 0; padding: 0 }` **unlayered**, and unlayered
     CSS outranks every layer — so `px-4` was present in the bundle and had no
     effect. That single line silently defeats all of Tailwind.
  3. Nothing defined a default `border-color` (preflight does), so Tailwind's
     `border` — width and style only — fell back to `currentColor`: black.
- **L12** (fix, architectural): the universal reset moved from `index.html` into
  `@layer base` in `src/app/styles.css`, which still beats the browser's defaults
  while letting utilities and scoped component styles override it; the base layer
  also gained a default `border-color` from the palette and the form-control
  normalisation. Full preflight is still not imported, so `h1`-`h6` and lists stay
  untouched. Verified two ways: the DOM reports `padding: 8px 16px`, `border: 1px
  solid rgb(221,221,221)` and a `85.39 x 36` box, and all eight pre-existing route
  baselines still match — the reset did not move anything that was already there.
- **L13** (defect, T1 blind spot): the eight route captures could not see the
  migrated component at all, because the new-topic form only exists while its panel
  is open — the smoke was green and proved nothing. Added `interactive states`, and
  the rule that **each migration unit adds the capture of the state it changes**.
  Also learned: re-reading the same screenshot path three times showed a **cached
  image**, and the ground truth came from `getComputedStyle` and `boundingBox()`, not
  from the picture. 10/10 pass without `--update-snapshots`; 200 unit tests;
  `CreateTopicForm` went from 93 to 38 style lines.
- **L14** (user decision, S3 rewritten, T1b): "Cortalo: no quiero Docker en esto".
  The candidate fix for the red CI gate was to generate the Linux baseline set in a
  container that mirrors the runner — `ubuntu:24.04`, amd64, repo mounted read-only,
  `git archive HEAD` so no darwin `node_modules` leak in — and then let CI compare
  pixels with no flags. It was stopped on request, and it also died on its own
  (`qemu: uncaught target signal 6`, exit 134, while `pnpm install` ran under amd64
  emulation on this arm64 host). So the net leaves CI: `.github/workflows/ci.yml` has
  no Playwright step, and the workflow comment plus `ARCHITECTURE` §3.2 both record
  that re-adding one is only correct together with a committed Linux baseline set
  generated on Linux. Nothing was lost by removing it — `--ignore-snapshots` never
  compared a pixel — and what CI stops claiming is the thing the validator rejected.
  Verified: `pnpm test:e2e` on the machine that owns the baselines, 10/10 pass, exit 0.
  Also corrected in the same commit: T4a's real hash is `f48fb68`, not `7fececf`. The
  correction cycle's `git reset --soft HEAD~1` rewrote it, so the table had been
  pointing at a dangling commit that only `git cat-file` could still resolve.
