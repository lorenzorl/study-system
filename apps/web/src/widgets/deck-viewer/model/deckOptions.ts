/**
 * Deck options — the Reveal configuration this plugin owns.
 *
 * PRESENTATION.md D6 decides that Reveal is a guest, not the owner of the
 * window, and lists the option that answers each way it could break the host:
 * `embedded` makes Reveal measure its container instead of the window,
 * `keyboardCondition` keeps deck keys from swallowing editor shortcuts,
 * `hash: false` because the host owns the URL. D7 adds the second reading mode:
 * below `scrollActivationWidth` the deck becomes a scrollable document instead
 * of a scaled 16:9 canvas.
 *
 * This module is the only place those values are written down; the widget passes
 * the result to Reveal untouched.
 */

/** The slide-number formats a deck is allowed to ask for. */
export type DeckSlideNumber = "c/t" | "c" | "h.v" | "h/v"

export interface DeckOptions {
  embedded: boolean
  hash: boolean
  scrollActivationWidth: number
  controls: boolean
  progress: boolean
  slideNumber: DeckSlideNumber
  /** Reveal only reacts to keys whose event target sits inside the deck. */
  keyboardCondition: (event: KeyboardEvent) => boolean
}

/**
 * A deck that switches to scroll view earlier than a phone is wide is a deck
 * whose canvas is already unusable when it flips; 320 is the narrowest width
 * worth composing for, so nothing below it is accepted.
 */
export const MIN_SCROLL_ACTIVATION_WIDTH = 320

/**
 * The slice of an element this module needs to scope the keyboard predicate.
 * Duck-typed because the predicate is created in plain module scope and is
 * unit-tested outside a DOM.
 */
export interface DeckRoot {
  contains?: (node: unknown) => boolean
}

/**
 * Keys are accepted only while the focus is inside *this* deck. Reveal hands its
 * `keyboardCondition` predicate every keydown it sees while mounted, so without
 * this the deck becomes a keyboard trap for the whole application — and with a
 * `document`-wide check, two open decks would both navigate on one keypress.
 */
function keyboardScopedTo(root?: DeckRoot | null) {
  return (event: KeyboardEvent): boolean => {
    const target = event.target
    if (!target) return false

    if (root && typeof root.contains === "function") {
      return root.contains(target)
    }

    const candidate = target as { closest?: (selector: string) => unknown }
    if (typeof candidate.closest !== "function") return false

    return candidate.closest(".reveal") !== null
  }
}

/** The keys a deck may configure. The guest-contract keys are not among them. */
export type DeckOverrides = Partial<{
  scrollActivationWidth: number
  controls: boolean
  progress: boolean
  slideNumber: DeckSlideNumber
}>

const DEFAULT_DECK_OPTIONS: Readonly<
  Omit<DeckOptions, "keyboardCondition">
> = {
  embedded: true,
  hash: false,
  scrollActivationWidth: 768,
  controls: true,
  progress: true,
  slideNumber: "c/t",
}

function resolveScrollActivationWidth(width: number | undefined): number {
  const requested = width ?? DEFAULT_DECK_OPTIONS.scrollActivationWidth

  // A non-numeric threshold silently disables scroll view; fall back to the
  // default instead of passing a value Reveal cannot compare.
  if (!Number.isFinite(requested)) {
    return DEFAULT_DECK_OPTIONS.scrollActivationWidth
  }

  return Math.max(MIN_SCROLL_ACTIVATION_WIDTH, requested)
}

/**
 * Resolves the deck configuration, applying the decided defaults.
 *
 * Built key by key rather than spread, so an override object carrying keys the
 * deck does not own (a future frontmatter value, a stray prop) cannot reach
 * Reveal. Overrides always win over the default for the keys that are ours.
 */
export function resolveDeckOptions(
  overrides: DeckOverrides = {},
  deckRoot?: DeckRoot | null,
): DeckOptions {
  // `embedded`, `hash` and `keyboardCondition` are written here and never read
  // from `overrides`: they are the contract that keeps Reveal a guest (D6), so a
  // caller — a future frontmatter value, a stray prop, a cast — cannot unlock
  // the deck into owning the window.
  return {
    embedded: DEFAULT_DECK_OPTIONS.embedded,
    hash: DEFAULT_DECK_OPTIONS.hash,
    scrollActivationWidth: resolveScrollActivationWidth(
      overrides.scrollActivationWidth,
    ),
    controls: overrides.controls ?? DEFAULT_DECK_OPTIONS.controls,
    progress: overrides.progress ?? DEFAULT_DECK_OPTIONS.progress,
    slideNumber: overrides.slideNumber ?? DEFAULT_DECK_OPTIONS.slideNumber,
    keyboardCondition: keyboardScopedTo(deckRoot),
  }
}
