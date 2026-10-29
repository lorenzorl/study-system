/**
 * Deck content layout, injected inside the deck's shadow root.
 *
 * This used to live in `PresentationPage.vue` as an unscoped `<style>` block,
 * because after the handoff Vue's scope attribute never reaches the elements
 * Reveal owns (`PRESENTATION.md` D8). The shadow root now sits between the two:
 * a document-level stylesheet cannot style anything inside it, so the rules
 * travel with the widget that builds the shadow tree.
 *
 * The `deck-` prefix still keeps these names out of the host application's
 * namespace, and the grid itself stays in the plugin rather than in the deck
 * (`PRESENTATION-UI.md` Q4). Type and colour stay with the Reveal theme — only
 * structure is defined here.
 */
export const DECK_CONTENT_STYLES = `
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
`
