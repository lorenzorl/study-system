import { describe, expect, it } from "vitest"
import { FIXTURE_DECK_HTML } from "./fixtureDeck"

// The fixture stands in for what the deck parser will emit, so it has to obey the
// same rules the parser will: sibling sections, no nesting, no scripts, no inline
// handlers. A fixture that breaks them would validate the wrong input (D3/D8).

describe("fixture deck markup", () => {
  it("is a flat sequence of sections", () => {
    const sections = FIXTURE_DECK_HTML.match(/<section[\s>]/g) ?? []

    expect(sections).toHaveLength(4)
    expect((FIXTURE_DECK_HTML.match(/<\/section>/g) ?? []).length).toBe(4)
    // No section opened inside another one.
    expect(FIXTURE_DECK_HTML.indexOf("</section>")).toBeLessThan(
      FIXTURE_DECK_HTML.lastIndexOf("<section"),
    )
  })

  it("carries no executable or embedding markup", () => {
    expect(FIXTURE_DECK_HTML).not.toMatch(/<script/i)
    expect(FIXTURE_DECK_HTML).not.toMatch(/\son\w+=/i)
    expect(FIXTURE_DECK_HTML).not.toMatch(/javascript:/i)
    expect(FIXTURE_DECK_HTML).not.toMatch(/<(iframe|object|embed|style)/i)
  })
})
