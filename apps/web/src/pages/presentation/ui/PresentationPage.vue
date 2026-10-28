<template>
  <!--
    The deck owns the page, but not the way out: that is a plain button, so it is the
    `shared/ui` button with `variant="outline"` (secondary — leaving is not the deck's
    action) and the 44px floor this control has always had, kept as a class because
    the primitives are sized by utilities (`shadcn-migration.md` T8).
  -->
  <div class="presentation-page">
    <DeckViewer
      :key="deckKey"
      class="presentation-page__deck"
      :html="deckHtml"
    />
    <Button
      variant="outline"
      class="presentation-page__exit min-h-[44px] min-w-[44px]"
      aria-label="Salir de la presentación"
      @click="exit"
    >
      Salir
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { useRoute, useRouter } from "vue-router"
import { DeckViewer } from "@/widgets/deck-viewer"
import { Button } from "@/shared/ui/button"
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

/* What the page still owns: where the way out sits, and that it clears the deck's
 * own controls. Its box — padding, border, radius, fill, font — is the primitive's
 * now. */
.presentation-page__exit {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  /* Above Reveal's slide number (31) and navigation controls (11), so leaving is
     always reachable and never lands under a deck affordance. */
  z-index: 40;
}
</style>

<!-- The deck's own content layout (`.deck-*`) lives with the viewer, inside the
     deck's shadow root: the deck is isolated from the host stylesheet in both
     directions, so a document-level rule could not reach the slides. See
     `widgets/deck-viewer/ui/deckContentStyles.ts`. -->
