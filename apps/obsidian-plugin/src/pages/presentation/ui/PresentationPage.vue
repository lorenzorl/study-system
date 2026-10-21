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

<style>
/* Deck content layout. Deliberately not scoped: after handoff Reveal owns the
   slide DOM (PRESENTATION.md D8), so Vue's scope attribute never reaches the
   elements this markup produces, and a scoped rule would simply never match. The
   `deck-` prefix keeps these names out of the host application's namespace, and
   the grid itself stays in the plugin rather than the deck (PRESENTATION-UI.md
   Q4). Type and colour stay with the Reveal theme — only structure is defined
   here. */
.deck-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1.5rem;
  align-items: start;
  text-align: left;
}

.deck-bento {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
  text-align: left;
}

.deck-bento__cell {
  border: 1px solid currentColor;
  border-radius: 8px;
  padding: 0.75rem 1rem;
  opacity: 0.85;
}

.deck-bento__cell h3 {
  margin: 0 0 0.25rem;
}

.deck-bento__cell p {
  margin: 0;
}

/* Narrow viewports are the deck's second reading mode (D7): the same sections,
   read as a document, so the two-column composition collapses instead of being
   squeezed into half a phone. */
@media (max-width: 768px) {
  .deck-columns {
    grid-template-columns: 1fr;
  }
}
</style>
