import { describe, expect, it } from "vitest"
import { resolveDeckOptions } from "./deckOptions"

// The decided defaults are the contract with PRESENTATION.md D6. These cases pin
// the values a caller gets for free, that an override replaces one default and
// keeps the rest, and the one number the UI is allowed to correct.

describe("resolveDeckOptions", () => {
  it("returns the decided defaults when called with nothing", () => {
    const options = resolveDeckOptions()

    expect(options.embedded).toBe(true)
    expect(options.hash).toBe(false)
    expect(options.scrollActivationWidth).toBe(768)
    expect(options.controls).toBe(true)
    expect(options.progress).toBe(true)
    expect(options.slideNumber).toBe("c/t")
    expect(typeof options.keyboardCondition).toBe("function")
  })

  it("applies overrides without dropping the remaining defaults", () => {
    const options = resolveDeckOptions({ controls: false, slideNumber: "h.v" })

    expect(options.controls).toBe(false)
    expect(options.slideNumber).toBe("h.v")
    expect(options.embedded).toBe(true)
    expect(options.hash).toBe(false)
    expect(options.scrollActivationWidth).toBe(768)
    expect(options.progress).toBe(true)
    expect(typeof options.keyboardCondition).toBe("function")
  })

  it("raises a scroll activation width below 320 to 320", () => {
    expect(
      resolveDeckOptions({ scrollActivationWidth: 200 }).scrollActivationWidth,
    ).toBe(320)
  })
})
