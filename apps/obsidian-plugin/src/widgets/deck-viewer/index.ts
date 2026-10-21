// Public API of the `deck-viewer` widget slice.
export { default as DeckViewer } from "./ui/DeckViewer.vue"
export {
  resolveDeckOptions,
  MIN_SCROLL_ACTIVATION_WIDTH,
} from "./model/deckOptions"
export type { DeckOptions, DeckSlideNumber } from "./model/deckOptions"
