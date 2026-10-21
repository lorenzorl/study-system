import { describe, expect, it } from "vitest"
import { resolveDeckOptions, type DeckRoot } from "./deckOptions"

// The guest contract of PRESENTATION.md D6 is not a default a caller may change:
// these cases pin that the locked keys survive an override attempt and that the
// keyboard predicate is scoped to one deck root.

function rootContaining(inside: object): DeckRoot {
  return { contains: (node) => node === inside }
}

function keydownOn(target: unknown): KeyboardEvent {
  return { target } as unknown as KeyboardEvent
}

describe("resolveDeckOptions — guest contract", () => {
  it("ignores an attempt to override the locked keys", () => {
    const options = resolveDeckOptions({
      // A cast is exactly the caller this guards against: a future frontmatter
      // value or a stray prop carrying values the deck does not own.
      embedded: false,
      hash: true,
      keyboardCondition: () => true,
    } as never)

    expect(options.embedded).toBe(true)
    expect(options.hash).toBe(false)
    expect(options.keyboardCondition(keydownOn({}))).toBe(false)
  })

  it("accepts keys only from inside its own deck root", () => {
    const inside = {}
    const outside = {}
    const options = resolveDeckOptions({}, rootContaining(inside))

    expect(options.keyboardCondition(keydownOn(inside))).toBe(true)
    expect(options.keyboardCondition(keydownOn(outside))).toBe(false)
    expect(options.keyboardCondition(keydownOn(null))).toBe(false)
  })

  it("drops keys a deck does not own", () => {
    const options = resolveDeckOptions({
      transition: "zoom",
      plugins: [],
    } as never)

    expect("transition" in options).toBe(false)
    expect("plugins" in options).toBe(false)
  })

  it("keeps the scroll threshold inside the usable range", () => {
    const width = (value: number) =>
      resolveDeckOptions({ scrollActivationWidth: value }).scrollActivationWidth

    expect(width(320)).toBe(320)
    expect(width(321)).toBe(321)
    expect(width(0)).toBe(320)
    expect(width(-100)).toBe(320)
    expect(width(Number.NaN)).toBe(768)
    expect(width(Number.POSITIVE_INFINITY)).toBe(768)
  })
})
