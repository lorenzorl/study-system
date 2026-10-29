# Presentation — decision record (parser deferred)

**Status: the deck surface is implemented (Reveal mount, slice 4); the `deck.md`
parser and component islands are still deferred. 2026-10-21, kept current for the
web app.**

The app gains a second way to consume the same study material: a slide deck.
This document records *why* the presentation layer is shaped the way it is, so
the UI is built against a contract instead of being retrofitted later, and so
none of these decisions get re-litigated when the implementation continues.

`ARCHITECTURE.md` governs the app's layers and slices. This document governs one
feature that lives inside that structure and does not change it.

**What is done:** the visual language, the slide composition, the type scale, the
bento grid, the empty and error states, and the Reveal mount inside a routed page
(`widgets/deck-viewer`, `pages/presentation`).

**What is not done:** parsing `deck.md`, mounting component islands, and resolving
links from a slide to its source. Those are gated on the parser existing.

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

**Decision.** The deck is rendered by Reveal.js, mounted inside the app.

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

### D2 — No fork of an existing Obsidian slides plugin (historical)

**Decision.** Reveal is a dependency mounted in our own view. We do not fork
Obsidian's core Slides, and we do not fork Advanced Slides. The decision was
taken while the UI lived in the Obsidian plugin; moving to the web app did not
change its answer, because the app still mounts Reveal itself instead of
adopting someone else's engine.

**Why.** The fork question only made sense while the deck looked like someone
else's product. It is not: the app already owns a Vue 3 + Pinia + FSD runtime
with `review-card`, `select-concept` and friends, a build pipeline, and a dev
server. A fork would mean inheriting an engine in a language and build we do not
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

### D4 — `markdown-it` for the deck parser

**Decision.** The deck will be parsed by `markdown-it`, with container support
and a wikilink rule.

**Why.** The parser has to run in the app and under vitest, with no host renderer
and no DOM. `markdown-it` produces a token stream, so a fenced block, a container and
a `[[link]]` can be tested as data before anything is mounted, and the same code
runs in the browser without a build step per deck. The original version of this
decision rested on the browser harness's `obsidian` mock exposing no
`MarkdownRenderer`; the mock and the plugin are gone, and the half that remains
is the one that matters — the app owns its parser.

**Rejected.** Obsidian's renderer plus extract-and-reinject of containers: there
is no vault and no Obsidian renderer in the app to call. A hand-written markdown
tokenizer: it would re-implement fenced blocks, containers and link rules, and
the dialect is not where our risk lives.

**Consequence.** We own the dialect. Callouts, embeds and Dataview are not
included; if the deck needs `![[image.png]]`, we implement that resolution
ourselves. Links are emitted as `data-href` plus `internal-link`; resolving them
is the page's job, and what a link may point at is open (§5 Q3).

### D5 — Interactive components are islands, mounted with `render()`

**Decision.** A fenced block in the markdown becomes a placeholder element that a
Vue component is rendered into with `render(h(Component, props), el)`, inside
`app.runWithContext(...)`.

**Why.** The app's Vue build resolves to the runtime-only bundle: SFCs are
compiled ahead of time by `@vitejs/plugin-vue`, and nothing ships the template
compiler. Adding it would introduce a compile-at-runtime path for content we did
not write. Islands need no compiler, no build step per deck, and they keep the
feature boundary: an island imports `review-card`'s public API and nothing else.
`runWithContext` carries the app's Pinia and router into the island, so an island
is not a second app.

**Rejected.** MDX or any build-time transform: a deck is content, not a module;
there is no build step per deck. Runtime template compilation: needs the compiler
we deliberately do not ship.

**Consequence.** `runWithContext` is mandatory, not optional — without it the
island mounts outside the Pinia and router context and `useReviewSession()` fails
at first use. And islands live outside Vue's patch cycle by construction, which
is exactly what D8 requires.

### D6 — Reveal is mounted as a guest, not as an owner of the window

**Decision.** `embedded: true`, `keyboardCondition` scoped to the deck,
`Reveal.layout()` on view resize, `Reveal.destroy()` on close, `hash: false`.

**Why.** Each option answers a specific way the deck could break the app.
`embedded` makes Reveal measure its container instead of the window, because the
deck is a routed page inside the app shell, not the window. `keyboardCondition`
keeps deck navigation from swallowing the app's own shortcuts — the deck must not
be a modal keyboard trap. `layout()` because the page can be resized, and a stale
scale is a visual bug with no error. `destroy()` because a leaked Reveal instance
keeps listeners on the document and shows up as a phantom deck after leaving the
route. `hash: false` because the router owns the URL: a Reveal hash would fight
`createWebHistory` and turn the deck's slide position into a second history the
app cannot address.

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

### D9 — UI first, wiring second (historical)

**Decision.** The slide UI was designed and built in the browser harness before
any Reveal integration existed.

**Why.** The visual decisions — grid, type scale, spacing, how a card looks at
1440px and at 390px — do not depend on the data pipeline, and the harness
iterated in seconds while a plugin reload in a vault did not. Building the
renderer first would have meant designing the UI through a parser, and any UI
change would then re-open parser questions. The contract above is what kept
those two phases from drifting apart. The harness went with the plugin; the app's
Vite dev server is where the same page is inspected now.

**Consequence.** During that phase the deck was a Vue component with fixture
content and no Reveal; the handoff boundary (D8) was rehearsed there before the
mount adapter was written. It is still a fixture deck: the parser is the next
slice.

---

## 3. Consequences and risks

| Risk | Why it matters | Mitigation |
| --- | --- | --- |
| Reveal's theme CSS touches `:root` and sometimes `body` | A theme can repaint the whole app document | Closed by the shadow root (§3.1) |
| Bundle growth (~200 KB) | The app ships Reveal's styles and theme in its JS bundle | Accepted; measured at ~600 KB in §3.1, and a theme without the embedded font is the follow-up |
| Dialect ownership (D4) | Callouts, embeds, Dataview are not free | Keep the deck's needs explicit; implement only what the UI shows |
| Island mounting is manual | Easy to leak Pinia context or listeners | One mount helper, one unmount path |
| Mobile browsers | The Fullscreen API is limited and inconsistent there | Scroll view is the mobile path; do not depend on fullscreen |

### 3.1 The deck is mounted in a shadow root (closes the first risk)

**Decision.** `DeckViewer` renders a host element and attaches a shadow root on
mount. Inside that root there is exactly one `<style>` — Reveal's base
stylesheet, the theme, and the deck's own `.deck-*` layout — followed by
`<div class="reveal"><div class="slides"></div></div>`. The slides markup is still
assigned once, imperatively, before Reveal is constructed (D8). Both of Reveal's
stylesheets are imported as **text**: `vite.config.ts` resolves those two
specifiers to `export default "<css>"` with a `pre` plugin that runs in serve and
in build. No `?raw`, no `?inline`, one specifier per file. The app's own palette
lives in `index.html`; Reveal's stylesheets stay out of it.

**Why.** `reveal.css` is written for a whole document: it styles
`html:not(.print-pdf) body`, `html.reveal-print *` and, through the theme,
declares the whole palette on `:root`. In the app the document is the whole
window, so a light-DOM `<style>` would centre text, reset margins and repaint
backgrounds in the header, the launcher and every route. (The rule was written
for the Obsidian plugin, where a single `styles.css` styled the entire app; the
app document is just as global.) The shadow root makes the leak structurally
impossible rather than something to be reviewed: none of those selectors can
match outside the deck, and `destroy()` leaves no stylesheet behind at all.

**Consequences.**

- `:root` is retargeted to `:host` on the way into the style element. No element
  is the document root inside a shadow tree, so the theme's custom properties
  would otherwise resolve to nothing and the deck would lose its palette. The
  host is where they belong: custom properties inherit from it into the tree.
- The keyboard predicate and the `ResizeObserver` target the **host**, not the
  inner `.reveal`. Events inside a shadow root are retargeted to the host on the
  way out, so a predicate scoped to the inner element would reject every key and
  the deck would be unnavigable. Focus goes to the host for the same reason:
  it is the only node the rest of the app can address.
- The deck's `.deck-*` content layout moved from `PresentationPage.vue` into the
  deck-viewer widget (`ui/deckContentStyles.ts`). Isolation cuts both ways: a
  document-level rule can no longer reach the slides either.
- The host element must be the one Vue named, so the widget's root is a wrapper
  and the host is its child; attribute and class fallthrough still work.
- Bundle cost is real and larger than this table first estimated: the bundled
  `theme/black.css` of Reveal 6 carries its font as a base64 `@font-face`
  payload, so the text of both stylesheets adds roughly 600 KB to the app's JS
  chunk. Measured at the time of this slice; a self-hosted theme without the
  embedded font is the obvious follow-up.
- Reveal's constructor is called with the reveal element **and** the options.
  Given a single argument it treats it as the config and resolves the deck with
  `document.querySelector(".reveal")`, a query that cannot cross a shadow
  boundary.

---

## 4. Deferred, deliberately

Standalone HTML export, PDF export, presenter mode, a theme system, autoplay
policy, per-deck `disableLayout`, embedding of images, and any data flow
from the review session into the deck.

---

## 5. Open questions

**Q1 — Where do images in a deck come from?** The original form of this question
was vault image embeds (`![[image.png]]`), which needed a resolver from a vault
path to a resource URL. There is no vault now: an image would have to be a corpus
artifact or an external URL, and no resolver for either exists. Close by listing
what the first real decks contain.

**Q2 — How is a stale deck surfaced?** A deck points at the material it was
generated from; if that material changes, the deck is outdated. The manifest can
compare mtimes, but whether the UI should show that is a product decision, not a
parser one.

**Q3 — Where do decks live, and how are they discovered?** `decks/<id>/deck.md`
with a `deck: true` marker was the working assumption while decks would live in a
vault. The app has no vault index, so a deck file needs a storage and a discovery
rule, and neither exists. This is the first question the parser has to answer.

**Q4 — Who owns the bento grid: the deck or the app?** The container syntax is
the deck's, but the grid definition is CSS. Keeping it in the app means decks
stay declarative and cannot fork the visual language.

---

## 6. Verified versus assumed

| Claim | Status |
| --- | --- |
| Reveal ships `scrollActivationWidth` (default 435) and `disableLayout` | Verified in source (`js/reveal.js`, `js/config.ts`) |
| Reveal has native `autoSlide`, `loop`, `postMessage`, `postMessageEvents` | Verified in docs and source |
| Reveal's markdown defaults: `---` separator, `Note:` notes, `.slide:`/`.element:` comments | Verified in `plugin/markdown/plugin.js`, `examples/markdown.html` |
| Reveal has no `eval` / `new Function` in its source | Verified by grep; relevant because the deck renders content we did not write |
| The app has no markdown parser or renderer yet | Verified in `apps/web/package.json`; the parser is the next slice, and D4 chooses `markdown-it` for it |
| The app resolves `vue` to the runtime-only bundle | Verified in `vue`'s package exports (`dist/vue.runtime.esm-bundler.js`) |
| Reveal can be mounted and destroyed from a routed page | The adapter and its options contract are in `widgets/deck-viewer`; there is no automated DOM mount test |
| Scroll view coexists with manually mounted islands | **Assumed** — no run yet |
| Bundled Reveal CSS does not leak into the app document | **Verified by construction** — the stylesheets live in the deck's shadow root (§3.1); there is no global rule left to leak |

The remaining assumptions are the reason D9 exists: the harness rehearsed the
handoff before the app depended on it, and the deck surface has been wired into
the router since.
