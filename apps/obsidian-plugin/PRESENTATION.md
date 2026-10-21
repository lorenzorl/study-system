# Presentation — decision record (implementation deferred)

**Status: decisions recorded, nothing implemented. 2026-10-21.**

The plugin gains a second way to consume the same study material: a slide deck.
This document records *why* the presentation layer is shaped the way it is, so
the UI that is being built now is built against a contract instead of being
retrofitted later, and so none of these decisions get re-litigated when the
implementation starts.

`ARCHITECTURE.md` governs the plugin's layers and slices. This document governs
one feature that will live inside that structure and does not change it.

**What is happening now:** UI only. Visual language, slide composition, type
scale, bento grid, empty and error states. No Reveal, no deck parser, no data.

**What is not happening now:** mounting Reveal, parsing `deck.md`, mounting
component islands, linking to source notes. Those are gated on the UI existing.

---

## 1. The constraint that decides everything

Reveal.js does not render *into* a container. It takes ownership of its slide
DOM: on `initialize()` it wraps, measures and positions; in scroll view it
rearranges the tree into page elements. Vue cannot patch a tree that Reveal
rewrote, and Reveal cannot follow a tree that Vue re-renders.

Everything below follows from that single fact. The presentation layer is a
**one-way handoff**: Vue builds a static tree and never touches it again,
Reveal owns it until `destroy()`, and the only things that stay alive after the
handoff are islands mounted *outside* Vue's patch cycle.

The second constraint is that a slide deck has **two consumption modes**, not
one: a fixed 16:9 surface for a projector or a monitor, and a narrow vertical
surface for a phone. Those are different reading experiences. The rest of the
document is mostly about not pretending otherwise.

---

## 2. Decisions

### D1 — Reveal.js as the renderer

**Decision.** The deck is rendered by Reveal.js, mounted inside the plugin.

**Why.** The deck has to work on monitors and phones. Every slide framework that
scales a fixed canvas — Reveal and Slidev included — degrades to a 219-pixel-tall
strip on a phone. Reveal is the only one of the two that ships *supported* ways
out: `scrollActivationWidth` switches to a scrollable document below a width
threshold (`435` by default), and `disableLayout` hands layout entirely to CSS.
Slidev offers neither; escaping its canvas means overriding inline styles with
`!important` against internals. Separately, the feature list this deck was
specified against — autoplay, loop, postMessage plus postMessageEvents — is
Reveal configuration, not code to write and maintain.

**Rejected.** Slidev: better authoring DX (Vue components, layouts, Shiki,
`slidev export`), but its fixed canvas is the one thing we cannot have, and the
workaround is a hack that breaks on upgrades.

**Consequence.** We give up a built-in theme system, markdown layouts, Shiki, and
a one-command PDF export. Those become our work if we want them.

### D2 — No fork of an existing Obsidian slides plugin

**Decision.** Reveal is a dependency mounted in our own view. We do not fork
Obsidian's core Slides, and we do not fork Advanced Slides.

**Why.** The fork question only made sense while the deck looked like someone
else's product. It is not: the plugin already owns a Vue 3 + Pinia + FSD runtime
with `review-card`, `select-concept` and friends, a build pipeline, and a browser
harness. A fork would mean inheriting an engine in a language and build we do not
control, to gain features we already have. What we actually need is an
**extension point** for our own components, and an extension point is cheaper to
own than an engine to maintain.

**Rejected.** Advanced Slides: mature, bundles Reveal 4.5 and grid layouts, but
its component story is HTML and reveal plugins, not reactive components, and its
Reveal allowlist predates the responsive options we need. Forking it would put
every future UI change behind a merge.

**Consequence.** We maintain the mounting code, the CSS scoping and the parser.
Estimated at a few hundred lines, not a product.

### D3 — The deck is one markdown file

**Decision.** A deck is `decks/<id>/deck.md`: frontmatter for deck-level Reveal
options, `---` as slide separator, `<!-- .slide: ... -->` comments for per-slide
config, `::: card` containers for bento, fenced blocks for interactive
components, `[[...]]` for source links.

**Why.** This is Reveal's own storage model: its documentation recommends an HTML
shell plus an external `.md` loaded at runtime, with exactly those separator and
attribute conventions. The content author is an LLM, and the format's cost is
paid at authoring time: markdown needs no escaping, no schema tree and no
versioning, while a JSON document forces every prose block into `\n`-escaped
strings and turns a layout choice into a data-model change. The deck also has to
link to the `.md` note it summarizes; markdown links are the native form of that,
and the plugin's renderer can resolve them without inventing a second link
system.

**Rejected.** JSON with a validated schema. It buys typed layout and typed
component slots, but the real value claimed for it — catching LLM output errors —
is available more cheaply: validate the frontmatter, the presence of slides, the
parseability of each fenced block, and that every `[[link]]` resolves. Four
checks, not a schema. The escaping cost and the schema-migration cost stay with
the LLM and with us, forever.

**Consequence.** Structure is convention, not contract. The conventions must be
written down in one place (`decks/README.md`) and are the specification the
generator reads. A deck that violates them fails at render, not at parse.

### D4 — `markdown-it`, not Obsidian's `MarkdownRenderer`

**Decision.** The deck is parsed by `markdown-it`, with container support and a
wikilink rule, not by Obsidian's renderer.

**Why.** The browser harness aliases `obsidian` to
`src/shared/obsidian/__mocks__/obsidian.ts`, and that mock exposes
`normalizePath` and empty classes — no `MarkdownRenderer`. Anything that depends
on the Obsidian renderer cannot be developed or observed in the harness, which is
where all UI work happens. `markdown-it` behaves identically in the harness and
inside Obsidian, so the deck can be finished and judged before it ever runs in
the plugin.

**Rejected.** Obsidian's renderer plus extract-and-reinject of containers: it
would give callouts, embeds and Dataview for free, but it makes the harness
second-class and forces a pre-processing pass that has to stay in sync with the
renderer.

**Consequence.** We own the dialect. Callouts, embeds and Dataview are not
included; if the deck needs `![[image.png]]`, we implement that resolution
ourselves. Links stay cheap: emit `data-href` plus `internal-link`, which is what
Obsidian's own post-processor already understands when the deck runs there.

### D5 — Interactive components are islands, mounted with `render()`

**Decision.** A fenced block in the markdown becomes a placeholder element that a
Vue component is rendered into with `render(h(Component, props), el)`, inside
`app.runWithContext(...)`.

**Why.** Both builds — the harness and the plugin — alias `vue` to
`vue.runtime.esm-bundler`, which has no template compiler. Adding the compiler
would contradict that build choice and introduce a compile-at-runtime path for
content we did not write. Islands need no compiler, no build step per note, and
they keep the feature boundary: the deck imports `review-card`'s public API and
nothing else.

**Rejected.** MDX or any build-time transform: the notes are vault content, not
repo content; there is no build step per deck. Runtime template compilation:
needs the compiler we deliberately do not ship.

**Consequence.** `runWithContext` is mandatory, not optional — without it the
island mounts outside the Pinia and router context and `useReviewSession()` fails
at first use. And islands live outside Vue's patch cycle by construction, which
is exactly what D8 requires.

### D6 — Reveal is mounted as a guest, not as an owner of the window

**Decision.** `embedded: true`, `keyboardCondition` scoped to the deck,
`Reveal.layout()` on view resize, `Reveal.destroy()` on close, `hash: false`.

**Why.** Each option answers a specific way the plugin could break the host.
`embedded` makes Reveal measure its container instead of the window, because in
Obsidian the window is not the deck. `keyboardCondition` keeps deck navigation
from swallowing editor shortcuts — the deck must not be a modal keyboard trap.
`layout()` because the view can be resized or split, and a stale scale is a
visual bug with no error. `destroy()` because a leaked Reveal instance keeps
listeners on the document and shows up as a phantom deck after closing a tab.
`hash: false` because Obsidian owns the URL.

**Consequence.** The mount code is not `Reveal.initialize(config)`; it is a small
adapter with a lifecycle. That adapter is the only place that knows Reveal exists.

### D7 — Responsive means a second mode, not a fluid slide

**Decision.** Narrow viewports get Reveal's scroll view (a scrollable document).
Wide viewports get the scaled 16:9 canvas. We do not try to make one composition
serve both.

**Why.** A slide composed for a projector does not become readable on a phone by
reflowing; it becomes a different artifact. Two modes let each be good at one
thing instead of both being mediocre. It also means the *content* does not have
to carry two layouts: the slide is composed once, and the narrow mode is a
reading surface over the same sections.

**Rejected.** `disableLayout` everywhere plus media queries: it works, but it
makes every slide a hand-authored responsive page, and it throws away the
16:9 composition the deck exists for. `disableLayout` stays available as a
per-deck escape hatch, not as the default.

**Consequence.** The UI must be judged twice: as a slide and as a document. Type
scale, spacing and containers that only look right in 16:9 are a defect.

### D8 — Reveal owns the DOM after handoff

**Decision.** Slides are built once. After `initialize()`, Vue does not re-render
that subtree. Interactive state lives inside islands. Nothing inside the deck is
driven by a reactive value from outside.

**Why.** Scroll view rewrites the slide tree into page elements. A Vue patch that
runs afterwards operates on nodes Vue no longer recognizes, and the result is
intermittent breakage that only appears on resize or on the phone — the worst
class of bug to debug and the easiest to prevent by construction. The rule is a
design constraint, not a style preference: it is what makes mounting a
DOM-owning library inside a reactive framework safe at all.

**Consequence.** Anything that needs to react to app state (a theme change, a
review session ending) is handled by remounting the deck, not by patching it.

### D9 — UI first, wiring second

**Decision.** The slide UI is designed and built in the browser harness before
any Reveal integration exists.

**Why.** The visual decisions — grid, type scale, spacing, how a card looks at
1440px and at 390px — do not depend on the data pipeline, and the harness
iterates in seconds while a plugin reload in a vault does not. Building the
renderer first would mean designing the UI through a parser, and any UI change
would then re-open parser questions. The contract above is what keeps those two
phases from drifting apart.

**Consequence.** During this phase the deck is a Vue component with fixture
content and no Reveal. The handoff boundary (D8) is rehearsed in the harness
before it is trusted in Obsidian.

---

## 3. Consequences and risks

| Risk | Why it matters | Mitigation |
| --- | --- | --- |
| Reveal's theme CSS touches `:root` and sometimes `body` | A theme can repaint the whole Obsidian app | Own theme, or scope the bundled CSS to the view |
| Bundle growth (~200 KB) | Plugin already ships ~545 KB | Accepted; measure before shipping |
| Dialect ownership (D4) | Callouts, embeds, Dataview are not free | Keep the deck's needs explicit; implement only what the UI shows |
| Island mounting is manual | Easy to leak Pinia context or listeners | One mount helper, one unmount path, exercised in the harness |
| Obsidian mobile is a webview | Fullscreen API is limited there | Scroll view is the mobile path; do not depend on fullscreen |

---

## 4. Deferred, deliberately

Standalone HTML export, PDF export, presenter mode, a theme system, autoplay
policy, per-deck `disableLayout`, embedding of vault images, and any data flow
from the review session into the deck.

---

## 5. Open questions

**Q1 — Does the deck need vault image embeds (`![[image.png]]`)?** If yes, D4's
consequence becomes work: a resolver that maps vault paths to resource URLs. Close
by listing what the first real decks contain.

**Q2 — How is a stale deck surfaced?** A deck points at source notes; if those
change, the deck is outdated. The manifest can compare mtimes, but whether the UI
should show that is a product decision, not a parser one.

**Q3 — Where do decks live, and how are they discovered?** `decks/<id>/deck.md`
with a `deck: true` marker is the working assumption. Obsidian indexes markdown,
so decks appear as notes; whether that is desirable is undecided.

**Q4 — Who owns the bento grid: the deck or the plugin?** The container syntax is
the deck's, but the grid definition is CSS. Keeping it in the plugin means decks
stay declarative and cannot fork the visual language.

---

## 6. Verified versus assumed

| Claim | Status |
| --- | --- |
| Reveal ships `scrollActivationWidth` (default 435) and `disableLayout` | Verified in source (`js/reveal.js`, `js/config.ts`) |
| Reveal has native `autoSlide`, `loop`, `postMessage`, `postMessageEvents` | Verified in docs and source |
| Reveal's markdown defaults: `---` separator, `Note:` notes, `.slide:`/`.element:` comments | Verified in `plugin/markdown/plugin.js`, `examples/markdown.html` |
| Reveal has no `eval` / `new Function` in its source | Verified by grep; relevant because the plugin runs under a CSP |
| The harness mock has no `MarkdownRenderer` | Verified in `src/shared/obsidian/__mocks__/obsidian.ts` |
| Both builds alias `vue` to the runtime-only bundle | Verified in `esbuild.config.mjs` |
| Reveal mounts cleanly inside an Obsidian `ItemView` | **Assumed** — no run yet |
| Scroll view coexists with manually mounted islands | **Assumed** — no run yet |
| Bundled Reveal CSS does not leak into Obsidian's UI | **Assumed** — no run yet |

The three assumptions are the reason D9 exists: the harness rehearses the handoff
before the plugin depends on it.
