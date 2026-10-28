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
| T4b | S1 | Forms → `input`/`label`/`textarea`/`button`: `create-concept`, `sync-note`, `capture-material` | 02e44b8 |
| T5 | S1 | `widgets/review-modal` → `dialog` (reka-ui focus trap, Escape, restore focus) | 1afd42d |
| T6 | S1 | `pages/flashcard-session` (15 buttons, 241 style lines): 11 migrated, grade row left hand-rolled | e8be1cf |
| T7 | S1 | Entity cards (4 files) + `pages/resources` (badges): 9 buttons, 2 badges, then the chip consistency + prop-leak fixes | 7e7edcb, 6349ec0 |
| T8 | S1 | App shell, dashboard, lists: 17 buttons in 9 files | 3dd2761, 94fae7f |
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
- **L15** (review, T1b): `review-74d04c2f3f68fe5f`, scoped to `5cc59d9..HEAD` with an
  explicit `baseRef` + `committedOnly`. `inspect` again offered `--base-ref=d70d9a2`
  (origin/master), which would have absorbed the whole unpublished mainline; passing
  the slice base overrode it. Tier **high** by `shell_source`, 91 lines, correction
  budget 46, lenses readability + reliability. Both reviewers ran and submitted on the
  first attempt — no admission refusal, no relay failure — and the review is
  **approved**, authority burned. Three advisory findings, all `informational`, none
  opened a correction:
  - `R2-001` (readability, `ci.yml:87-89`). The id is not descriptive and the envelope
    carries no prose, so nothing is claimed about its content here.
  - `R3-ci-render-smoke-removed` (reliability, `ci.yml:74-89`): the removal also took
    out what CI *did* still check — every route renders with no console error. That is
    a separate loss from the pixel comparison, and the follow-up, if we want it back,
    is a screenshot-free render step, not the visual one.
  - `R3-restore-command-naming` (reliability, `ci.yml:87-89`): SUGGESTION, naming.
- **L16** (T4b done, three defects surfaced): `create-concept`, `sync-note` and
  `capture-material` now use `Input`, `Textarea`, `Label` and `Button`; each keeps
  only what the primitives do not own (panel layout, spinner, refusal, the segmented
  source selector) and the 44px touch floor as a class. Three findings, all real:
  1. **The tests were lying after the migration.** `toContain("disabled")` matched the
     primitives' own `disabled:` utility classes, so an *enabled* button passed as
     disabled. The three forms now assert the attribute (`/(?:^|\s)disabled(?=[\s>=/])/`).
     Same class of bug as the BEM assertions: the test was pinned to text that the
     migration changed.
  2. **Slot content gains SSR fragment markers** (`<!--[-->Cancelar<!--]-->`), so a
     regex for `<button ...>Cancelar</button>` stops matching. The assertion strips
     comments first.
  3. **`/topic/:id` cannot be opened cold** — `TopicPage` never loads the topics store,
     so a direct visit renders "Tema no encontrado.". Worse: the committed
     `topic-darwin.png` route baseline *is* a capture of that error state, so the route
     loop gives false confidence about that page. Recorded as `backlog.md` D1; the new
     concept capture enters through `/topics` → card because that is the only route
     that mounts the page, and it collapses back to a direct visit once D1 closes.
  `create-concept` had no render test at all, so one was written against the
  pre-migration code first (5 cases). Two states were invisible to the net (both live
  behind a click), so each gained a capture; the sync-note dialog baseline was
  regenerated because the migration moves that panel ~16px, and the new image was
  read before it was accepted — a baseline updated without looking is a regression
  captured as the new truth. Verified: 12/12 e2e, 205 unit tests, typecheck exit 0.
  Not migrated on purpose: the source selector in `capture-material` stays a
  `role="group"` + `aria-pressed` segmented control, because T3 pinned that contract
  and moving it onto the `tabs` primitive is its own unit, not a form migration.
- **L17** (T5 done, one silent defect and one harness cost): the review modal now
  renders through `Dialog` + `DialogContent` + `DialogTitle as-child`. The primitive
  owns the portal, the scrim, the focus trap, Escape and focus restoration; the
  widget keeps the card, the grades group, the grading shortcuts and the bottom bar,
  and its frame CSS is unchanged — `DialogContent` carries only the utilities that
  re-express the old full-screen centering box. Verified in a browser, not by
  assumption: one `[role="dialog"]`, `aria-labelledby` from the heading, 12 × Tab
  never leaving the surface, Escape emitting exactly one `close` and returning focus
  to the opener, key `2` grading and advancing, and the panel geometry
  (640×326 at x=320 y=237, padding 16/18, 1px #ddd border, 10px radius) identical to
  the pre-migration measurements.
  Two things this unit surfaced:
  1. **The modal has no reachable route.** `ReviewModal` is mounted only by
     `ResourcesPage`, `continueResource` is hardcoded `null`, and
     `toMaterialResources` maps every resource to kind `material`, so the
     `flashcards` branch never runs. And `/study/review` — the `review-darwin.png`
     baseline — is a capture of "Concepto no encontrado.", i.e. a third error-state
     baseline. Recorded as `backlog.md` D2. The worker was right to refuse to
     regenerate that baseline: my brief predicted it would change, and the premise
     was wrong.
  2. **The render test mocks the primitive, and that is the honest cost.** reka's
     portal is mount-gated (`useMounted()`), so `renderToString` emits only the
     placeholder; without the mock the harness cannot see the panel it exists to
     pin. The stubs pass their slot through and mark `data-slot`, so reverting to a
     hand-rolled `role="dialog"` fails the tests rather than silently passing them.
     The primitive's own aria wiring is therefore covered by the browser run, not
     by the suite — stated, not hidden.
  Deliberate delta left open: the primitive overlay is `bg-black/80` while the two
  still-hand-rolled overlays (capture panel, sync-note dialog) keep `rgba(0,0,0,0.5)`,
  so the app has two scrim opacities until those two migrate to the same primitive.
  Verified: 10/10 widget tests (9 before), 206/206 unit, typecheck 0, 12/12 e2e.
- **L18** (T6 done; the net's blind spot measured, which is the real finding): 11
  of 15 buttons on `flashcard-session` are the primitive now — 2 `default`, 9
  `outline`, every one keeping `min-h-[44px]` — and the style block went 241 → 189
  lines. 13 cases were written first: 12 characterization cases green against the
  untouched component (that is what pins the contract) and one invariant case, "the
  actions render through the shared/ui button", which is the only one that could go
  red. The 4 grade buttons stay hand-rolled because their colours are literals
  (`#e67e22`, `#27ae60`, `#2980b9`) instead of palette tokens, which is an S4
  violation, and no `Button` variant expresses four distinct fills; recorded as
  backlog D3.
  The page is not the story here, the gate is. The secondary fill moved from
  `--background-secondary` to the variant's `bg-background` plus a shadow — ~0.4% of
  the page — and **the suite went green against a stale baseline** because
  `maxDiffPixelRatio` was 0.01. It surfaced only because two PNGs were compared by
  hand. Setting the ratio to 0 failed **8 of 12** captures; two runs produced
  identical pixel counts (105, 136, 271, 352, 3722, 1435, 1672), which rules out
  rendering noise, and a bisect pinned the drift to `index.html` +
  `src/app/styles.css`: reverting just those two to the commit that captured the
  baselines makes `due-cards` pixel-identical. So the baselines were stale against
  the base-layer fix of L11/L12 — a change we wanted — not the code. All of them
  were regenerated after reading before/after images, and the ratio is now 0 with
  `threshold` still absorbing anti-aliasing.
  Two transferable lessons: (a) Playwright's `--update-snapshots` in its default
  `changed` mode **refuses to rewrite** an image whose diff is under the tolerance,
  so a stale baseline needs `--update-snapshots=all` (never without `-g`) — that
  refusal is itself proof the tolerance is too wide; (b) nothing in the net sees the
  grade row or the concept-mode session (`answerRevealed` lives inside
  `useReviewSession`, the flip is a click, and no fixture populates the flashcard
  cache), so a regression in the controls that *did* migrate there would pass every
  gate. `ReviewModal.render.test.ts` still asserts `toContain("disabled")` on its own
  hand-rolled grade row — the L16 trap, harmless only while that row stays
  hand-rolled.
  Verified: 219/219 unit (206 + 13), typecheck 0, 12/12 e2e at ratio 0.
- **L19** (T7 done; the net proved itself twice, and two gate-trust defects fell
  out): nine buttons across five files became `Button`, the launcher's kind chip and
  both entity counts became `Badge`, ~120 lines of launcher CSS are gone, and four
  new render tests pin each card as a single interactive element.
  `Card` was **not** adopted, and that is structural: in all four entity cards the
  card *is* the control (one `<button>` owns the click), while `Card` is a
  non-interactive column. Hosting it means moving the click target inward or nesting
  a button in a card — redesigns. The frame stays the card's own CSS, with the box
  restated in full so the primitive's `h-9`/`justify-center`/`font-medium` cannot
  reshape it. That does freeze those four cards outside their files; a future
  primitive change will not reach them.
  The tolerance of 0 paid for itself immediately: the first pass clamped `DueCardItem`
  to 56px with its content overflowing and let `font-medium` leak into secondary text.
  Both were caught at 13,879 and 678 px and fixed instead of blessed. One user-visible
  delta accepted after reading the images: the topic card is 70 → 48px tall because its
  count stopped being a 14px span with a 44px line-height faking a touch target on a
  label nobody can touch; the concept card followed (70 → 63px) in the follow-up so the
  two chips would agree.
  Three defects the audit produced, all now closed except the first:
  1. `Badge` forwarded its whole prop bag, so every badge rendered
     `<div … variant="outline">` — an attribute HTML does not have. Fixed the way
     `Button` avoids it (explicit `as`/`asChild`); verified in SSR and in the live
     bundle. `DialogContent` had the same defect with `showCloseButton` and is fixed
     with a one-key omission, but **its observation is still owed**: the only consumer
     is the review modal, whose route is unreachable (D2), so the attribute cannot be
     read in a browser yet.
  2. **The net could compare a bundle it did not build.**
     `reuseExistingServer: !CI` meant a leftover `vite preview` made Playwright skip
     `webServer.command`, build included — it produced a false 12/12 in this session,
     with CSS the change had deleted still present in `dist/assets`. Now `false`, so
     the `--strictPort` inside the command makes a busy port a loud failure. Verified
     by putting a listener on 127.0.0.1:4173: the suite stops with "already used … or
     set reuseExistingServer:true", exit 1.
  3. Recorded for later: `--primary-foreground: #fff` (and the destructive one) are
     literals in the token bridge, against `index.html`'s rule that the palette is the
     only place a colour is defined — now load-bearing, because the migrated primary
     buttons take their text colour from it. Backlog D4.
  Verified: 231/231 unit, typecheck 0, 12/12 e2e at ratio 0 (six baselines regenerated
  across the unit and its follow-up, each one read first).
- **L20** (T8 done — the migration's last unit; and D1 was half the net): 17
  hand-rolled buttons across nine files are `Button` now, `variant` chosen by role and
  the 44px floor kept everywhere it existed. Style blocks shrank: shell 79 → 59, topic
  114 → 75, due-cards 96 → 61, dashboard 92 → 56, feynman 110 → 86, sync trigger
  19 → 0. 32 cases were added (6 new files, 3 extended), the "renders through the
  shared/ui button" invariant red before the migration and green after; ten baselines
  were regenerated one `-g` run at a time, each diff read and attributed to a specific
  control — including the two that changed shape rather than paint (the dashboard's
  "+" and "Salir" are 44×44 outline controls now).
  Two card-shaped controls (the daily-review card, the two concept action rows) keep
  their own frame with the box restated, because the primitive's
  `h-9`/`justify-center`/`font-medium`/`nowrap` would reshape content that is not a
  button's content. Same trade as the entity cards: those three are now frozen outside
  their files.
  T8's own risk, closed by observation instead of argument: `SyncNoteTrigger` hands
  focus back through `$el`, which cannot fail loudly. One behavioural e2e case now
  opens the panel from `/topics`, presses Escape and asserts `document.activeElement`
  is the trigger again (`94fae7f`). It passes, so `$el` is the rendered `<button>`.
  Scope honest: that pins the Escape path; the same call also runs after a successful
  sync, which is the same code but not the same case.
  **D1 was half the net.** `ConceptPage` and `FeynmanPage` also never call
  `loadTopics()` (only their `retry` does), so a cold visit renders "Concepto no
  encontrado." — which means four of the eight route baselines (`topic`, `concept`,
  `feynman`, `review`) are captures of a failure. Consequences: the net covers four
  real pages, not eight; the concept action rows and the Feynman submit have no pixel
  coverage at all; and this unit's deltas on those pages were only the shell's back
  button, since the pages themselves were showing an error. `backlog.md` D1 now says
  so.
  Closing inventory: of the original 53 `<button>` lines, **10 remain** in product
  code, none of them ordinary debt — 8 are the two grade rows blocked by D3 (their
  colours are literals), 1 is the capture panel's segmented source selector (T3
  pinned its `aria-pressed` contract), and 1 is `FlashcardCard`, where the card *is*
  the button. 21 files consume `@shared/ui/*`.
  Verified: 263/263 unit (231 + 32, FSD guard included), 13/13 e2e at ratio 0
  (12 captures + 1 behavioural case), typecheck 0.
