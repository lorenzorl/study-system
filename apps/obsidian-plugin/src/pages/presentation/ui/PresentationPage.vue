<template>
  <div class="presentation-page">
    <DeckViewer
      :key="deckKey"
      class="presentation-page__deck"
      :html="deckHtml"
    />
    <button
      type="button"
      class="presentation-page__exit"
      aria-label="Salir de la presentación"
      @click="exit"
    >
      Salir
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { useRoute, useRouter } from "vue-router"
import { DeckViewer } from "@/widgets/deck-viewer"
import { FIXTURE_DECK_HTML } from "../model/fixtureDeck"

// No data flow yet (D9): the page renders the fixture deck, and the route's
// `deckId` is URL shape only until the parser and the launcher exist.
const deckHtml = FIXTURE_DECK_HTML
const route = useRoute()
const router = useRouter()

// The deck is a DOM-owning component (D8): opening another deck must remount it,
// not patch it. The route param is the remount key.
const deckKey = computed(() => String(route.params.deckId ?? "fixture"))

function exit() {
  router.push("/")
}
</script>

<style scoped>
.presentation-page {
  position: relative;
  /* Full bleed: the harness shell pads and scrolls its main area, and a deck owns
     every pixel it is given. The negative margin cancels that 0.75rem frame, and
     the extra height compensates for it. */
  height: calc(100% + 1.5rem);
  margin: -0.75rem;
  overflow: hidden;
}

.presentation-page__deck {
  height: 100%;
}

.presentation-page__exit {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  /* Above Reveal's slide number (31) and navigation controls (11), so leaving is
     always reachable and never lands under a deck affordance. */
  z-index: 40;
  min-width: 44px;
  min-height: 44px;
  padding: 0.35rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
  font-size: 0.85rem;
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.presentation-page__exit:hover {
  background: var(--background-modifier-hover);
}
</style>

<!-- The deck's own content layout (`.deck-*`) lives with the viewer, inside the
     deck's shadow root: the deck is isolated from the host stylesheet in both
     directions, so a document-level rule could not reach the slides. See
     `widgets/deck-viewer/ui/deckContentStyles.ts`. -->
