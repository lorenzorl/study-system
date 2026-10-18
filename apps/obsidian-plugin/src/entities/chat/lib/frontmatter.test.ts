import { describe, expect, it } from "vitest"
import { parseFrontmatter, serializeFrontmatter } from "./frontmatter"

describe("parseFrontmatter", () => {
  it("parses a leading block and keeps the body intact", () => {
    const raw =
      "---\nmodel: qwen/qwen3.5-flash-02-23\ntemperature: 0.2\ntop_k: 8\n---\nBody line\n"
    const { data, body } = parseFrontmatter(raw)

    expect(data).toEqual({
      model: "qwen/qwen3.5-flash-02-23",
      temperature: "0.2",
      top_k: "8",
    })
    expect(body).toBe("Body line\n")
  })

  it("keeps numeric-looking values as strings", () => {
    const { data } = parseFrontmatter("---\ntemperature: 0.2\ntop_k: 8\n---\n")

    expect(data.temperature).toBe("0.2")
    expect(typeof data.temperature).toBe("string")
    expect(data.top_k).toBe("8")
  })

  it("ignores blank lines and # comments", () => {
    const raw = "---\n# a comment\n\nmodel: m\n# another\n---\n"

    expect(parseFrontmatter(raw).data).toEqual({ model: "m" })
  })

  it("trims keys and values", () => {
    const raw = "---\n  model :   m  \n---\n"

    expect(parseFrontmatter(raw).data).toEqual({ model: "m" })
  })

  it("treats a block without a closing delimiter as body", () => {
    const raw = "---\nmodel: m\nstill body\n"
    const { data, body } = parseFrontmatter(raw)

    expect(data).toEqual({})
    expect(body).toBe(raw)
  })

  it("does not treat a later --- as frontmatter when the text does not start with ---", () => {
    const raw = "intro\n---\nmodel: m\n---\n"
    const { data, body } = parseFrontmatter(raw)

    expect(data).toEqual({})
    expect(body).toBe(raw)
  })

  it("returns the whole text as body when there is no frontmatter", () => {
    const raw = "Just text\n"

    expect(parseFrontmatter(raw)).toEqual({ data: {}, body: raw })
  })

  it("handles an empty frontmatter block", () => {
    const { data, body } = parseFrontmatter("---\n---\nbody")

    expect(data).toEqual({})
    expect(body).toBe("body")
  })
})

describe("serializeFrontmatter", () => {
  it("writes keys in the deterministic model, temperature, top_k order", () => {
    const block = serializeFrontmatter({
      top_k: 8,
      temperature: 0.2,
      model: "m",
    })

    expect(block).toBe("---\nmodel: m\ntemperature: 0.2\ntop_k: 8\n---\n")
  })

  it("produces the same bytes regardless of the input key order", () => {
    const a = serializeFrontmatter({ model: "m", temperature: 0.2, top_k: 8 })
    const b = serializeFrontmatter({ top_k: 8, model: "m", temperature: 0.2 })

    expect(a).toBe(b)
  })

  it("skips null, undefined and empty values", () => {
    const block = serializeFrontmatter({
      model: "m",
      temperature: null,
      top_k: undefined,
    })

    expect(block).toBe("---\nmodel: m\n---\n")
  })

  it("emits an empty block when nothing is set", () => {
    expect(
      serializeFrontmatter({ model: null, temperature: null, top_k: null }),
    ).toBe("---\n---\n")
  })

  it("round-trips through parseFrontmatter", () => {
    const block = serializeFrontmatter({
      model: "m",
      temperature: 0.2,
      top_k: 8,
    })
    const parsed = parseFrontmatter(`${block}body`)

    expect(parsed.data).toEqual({ model: "m", temperature: "0.2", top_k: "8" })
    expect(parsed.body).toBe("body")
  })
})
