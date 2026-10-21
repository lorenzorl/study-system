<template>
  <div ref="revealRoot" class="reveal deck-viewer">
    <div ref="slidesEl" class="slides"></div>
    <p v-if="mountError" class="deck-viewer__error" role="alert">
      No se pudo abrir la presentación: {{ mountError }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue"
import Reveal from "reveal.js"
import "reveal.js/reveal.css"
import "reveal.js/theme/black.css"
import { resolveDeckOptions, type DeckOverrides } from "../model/deckOptions"

/**
 * The mount adapter: the only place in the plugin that knows Reveal exists
 * (PRESENTATION.md D6). It builds nothing, parses nothing and reacts to nothing
 * but its own lifecycle — the deck's markup arrives complete and is written into
 * the DOM once, never through a template binding (D8).
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

const revealRoot = ref<HTMLDivElement | null>(null)
const slidesEl = ref<HTMLDivElement | null>(null)
const mountError = ref<string | null>(null)

let deck: DeckInstance | null = null
let layoutFrame = 0
let observer: ResizeObserver | null = null
let unmounted = false

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
  const root = revealRoot.value
  const slides = slidesEl.value
  if (!root || !slides) return

  // The one-way handoff (D8): after this line the tree belongs to Reveal.
  slides.innerHTML = props.html

  const instance = new Reveal(root)

  try {
    await instance.initialize(resolveDeckOptions(props.options, root))
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
  // is inside the deck. Nothing in the slides is focusable, so without this the
  // target is `body`, the predicate refuses every key and the deck cannot be
  // navigated at all.
  root.tabIndex = -1
  root.focus({ preventScroll: true })

  // Reveal already re-lays out on window resize; a split pane fires no resize
  // event, so the container is observed instead of duplicating that listener.
  if (typeof ResizeObserver !== "undefined") {
    observer = new ResizeObserver(scheduleLayout)
    observer.observe(root)
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
  width: 100%;
  height: 100%;
  min-height: 0;
}
</style>
