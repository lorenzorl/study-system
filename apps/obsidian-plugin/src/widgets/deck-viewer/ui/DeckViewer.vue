<template>
  <div class="deck-viewer">
    <!-- The host is the only element the rest of the app sees. Everything Reveal
         touches lives in its shadow root. -->
    <div ref="hostEl" class="deck-viewer__host"></div>
    <p v-if="mountError" class="deck-viewer__error" role="alert">
      No se pudo abrir la presentación: {{ mountError }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue"
import Reveal from "reveal.js"
import revealStylesheet from "reveal.js/reveal.css"
import themeStylesheet from "reveal.js/theme/black.css"
import { resolveDeckOptions, type DeckOverrides } from "../model/deckOptions"
import { DECK_CONTENT_STYLES } from "./deckContentStyles"

/**
 * The mount adapter: the only place in the plugin that knows Reveal exists
 * (PRESENTATION.md D6). It builds nothing, parses nothing and reacts to nothing
 * but its own lifecycle — the deck's markup arrives complete and is written into
 * the DOM once, never through a template binding (D8).
 *
 * The deck is mounted inside a **shadow root**, and that is the whole reason the
 * stylesheets above are imported as text. Reveal ships global rules
 * (`html:not(.print-pdf) body`, `html.reveal-print *`) and the theme declares its
 * palette on `:root`; Obsidian loads a single `styles.css` for the whole app, so
 * a light-DOM `<style>` would repaint the editor, the sidebar and every modal.
 * Inside a shadow root those selectors match nothing outside the deck, and
 * `destroy()` can leave no stylesheet behind at all.
 *
 * Deck markup carries no script and no handlers: nothing here re-attaches Vue to
 * a tree Reveal owns.
 */
const props = defineProps<{
  /**
   * Deck markup: a sequence of `<section>` elements.
   *
   * Assigned imperatively before Reveal is constructed, and never again. A
   * `v-html` binding would be live: Vue rewrites the subtree whenever the string
   * changes, and by then Reveal owns those nodes — in scroll view it has
   * re-parented them into page elements, so a patch silently destroys the deck.
   * Structure and sanitising belong to the deck parser, not here.
   */
  html: string
  /** Overrides for the values a deck may configure; the guest keys are locked. */
  options?: DeckOverrides
}>()

type DeckInstance = InstanceType<typeof Reveal>

const hostEl = ref<HTMLDivElement | null>(null)
const mountError = ref<string | null>(null)

let deck: DeckInstance | null = null
let layoutFrame = 0
let observer: ResizeObserver | null = null
let unmounted = false

/**
 * The single stylesheet of the shadow root: Reveal's base rules, the theme, and
 * the deck's own content layout.
 *
 * `:root` is retargeted to `:host` on the way in. No element is the document
 * root inside a shadow tree, so the theme's custom properties would otherwise
 * match nothing, resolve to nothing, and the deck would render without its
 * palette. The host is the right home for them: custom properties inherit from
 * it into the whole shadow tree.
 */
function revealShadowStyles(): string {
  return `${revealStylesheet}\n${themeStylesheet}`
    .replace(/:root\b/g, ":host")
    .concat(DECK_CONTENT_STYLES)
}

/**
 * Builds the shadow tree: one `<style>`, then the structure Reveal takes over.
 * Created here rather than in the template because shadow content is not part of
 * Vue's render tree, and it must not become one (D8).
 */
function createShadowTree(host: HTMLElement) {
  const shadow = host.shadowRoot ?? host.attachShadow({ mode: "open" })

  const style = document.createElement("style")
  style.textContent = revealShadowStyles()

  const revealElement = document.createElement("div")
  revealElement.className = "reveal"

  const slidesElement = document.createElement("div")
  slidesElement.className = "slides"

  revealElement.appendChild(slidesElement)
  shadow.append(style, revealElement)

  return { revealElement, slidesElement }
}

/**
 * Reveal measures its container once and keeps the scale; a resized container
 * leaves a stale scale behind, which is a visual bug with no error. One frame per
 * burst: `layout()` is synchronous and expensive.
 */
function scheduleLayout(): void {
  if (layoutFrame !== 0) return

  layoutFrame = requestAnimationFrame(() => {
    layoutFrame = 0
    deck?.layout()
  })
}

onMounted(async () => {
  const host = hostEl.value
  if (!host) return

  const { revealElement, slidesElement } = createShadowTree(host)

  // The one-way handoff (D8): after this line the tree belongs to Reveal.
  slidesElement.innerHTML = props.html

  // The element is passed explicitly, and the options with it. Given a single
  // argument, Reveal's constructor takes it as the config and looks the deck up
  // with `document.querySelector(".reveal")` — a query that cannot cross a
  // shadow boundary, so it would find nothing and throw.
  //
  // The keyboard predicate is scoped to the *host*: events inside a shadow root
  // are retargeted to it on the way out, so `target` is always the host. A
  // predicate scoped to the inner `.reveal` would reject every key and the deck
  // would be unnavigable.
  const instance = new Reveal(
    revealElement,
    resolveDeckOptions(props.options, host),
  )

  try {
    await instance.initialize()
  } catch (error) {
    instance.destroy()
    mountError.value =
      error instanceof Error ? error.message : "error desconocido"
    return
  }

  // The deck can be closed while `initialize()` is still settling (a fast route
  // change, a closed tab). Nothing has been registered yet at that point, so the
  // instance is destroyed here instead of being left with listeners on document.
  if (unmounted) {
    instance.destroy()
    return
  }

  deck = instance

  // Reveal listens on `document` and the predicate only accepts keys whose target
  // is the deck's host. Nothing in the slides is focusable, so without this the
  // target is `body`, the predicate refuses every key and the deck cannot be
  // navigated at all. Focus lands on the host because that is the element the
  // document can address.
  host.tabIndex = -1
  host.focus({ preventScroll: true })

  // Reveal already re-lays out on window resize; a split pane fires no resize
  // event, so the host is observed instead of duplicating that listener.
  if (typeof ResizeObserver !== "undefined") {
    observer = new ResizeObserver(scheduleLayout)
    observer.observe(host)
  }
})

onBeforeUnmount(() => {
  unmounted = true

  observer?.disconnect()
  observer = null

  if (layoutFrame !== 0) {
    cancelAnimationFrame(layoutFrame)
    layoutFrame = 0
  }

  // Without this the instance keeps its own listeners on `document` and the deck
  // outlives the view that opened it (D6).
  deck?.destroy()
  deck = null
})
</script>

<style scoped>
.deck-viewer {
  /* Reveal sizes itself to its container (`width/height: 100%` in reveal.css) and,
     because `embedded: true`, measures that container rather than the window. The
     container therefore has to have a real size: whatever the page gives it. */
  position: relative;
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
}

.deck-viewer__host {
  /* The host is the deck's container and its focus target; it has to fill the
     wrapper, and it has to be able to shrink inside a split pane. */
  flex: 1 1 auto;
  width: 100%;
  min-height: 0;
}

.deck-viewer__error {
  margin: 0.5rem 0 0;
  color: var(--text-error, #e5534b);
  font-size: 0.9rem;
}
</style>
