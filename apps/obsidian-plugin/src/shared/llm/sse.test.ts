import { describe, expect, it } from "vitest"
import {
  extractDelta,
  extractErrorMessage,
  extractToolCallFragments,
  parseSseChunk,
} from "./sse"

/** Collect every complete `data:` payload a buffer produces. */
function collect(buffer: string): { payloads: string[]; tail: string } {
  const payloads: string[] = []
  const tail = parseSseChunk(buffer, (payload) => payloads.push(payload))
  return { payloads, tail }
}

describe("parseSseChunk", () => {
  it("emits one payload from one event and consumes the buffer", () => {
    const { payloads, tail } = collect('data: {"choices":[{"delta":{"content":"hola"}}]}\n\n')

    expect(payloads).toEqual(['{"choices":[{"delta":{"content":"hola"}}]}'])
    expect(tail).toBe("")
  })

  it("emits several payloads contained in one chunk, in order", () => {
    const { payloads, tail } = collect('data: {"a":1}\ndata: {"b":2}\n')

    expect(payloads).toEqual(['{"a":1}', '{"b":2}'])
    expect(tail).toBe("")
  })

  it("buffers a payload split across two chunks and emits nothing until the newline arrives", () => {
    const first = collect('data: {"choices":[{"delta":{"cont')

    expect(first.payloads).toEqual([])
    expect(first.tail).toBe('data: {"choices":[{"delta":{"cont')

    const second = collect(first.tail + 'ent":"mundo"}}]}\n\n')

    expect(second.payloads).toEqual(['{"choices":[{"delta":{"content":"mundo"}}]}'])
    expect(second.tail).toBe("")
  })

  it("keeps a payload with no trailing newline in the tail", () => {
    const { payloads, tail } = collect('data: {"a":1}')

    expect(payloads).toEqual([])
    expect(tail).toBe('data: {"a":1}')
  })

  it("ignores a comment / keep-alive line", () => {
    const { payloads, tail } = collect(": OPENROUTER PROCESSING\n")

    expect(payloads).toEqual([])
    expect(tail).toBe("")
  })

  it("ignores blank lines that separate events", () => {
    const { payloads } = collect('\n\ndata: {"a":1}\n\n\n')

    expect(payloads).toEqual(['{"a":1}'])
  })

  it("strips a trailing carriage return from a line", () => {
    const { payloads } = collect('data: {"a":1}\r\n\r\n')

    expect(payloads).toEqual(['{"a":1}'])
  })

  it("emits the [DONE] payload as a payload", () => {
    const { payloads } = collect("data: [DONE]\n\n")

    expect(payloads).toEqual(["[DONE]"])
  })

  it("accepts data: with no space after the colon", () => {
    const { payloads } = collect('data:{"a":1}\n\n')

    expect(payloads).toEqual(['{"a":1}'])
  })
})

describe("extractDelta", () => {
  it("returns the text delta of a chat chunk", () => {
    expect(extractDelta('{"choices":[{"delta":{"content":"hola"}}]}')).toBe("hola")
  })

  it("returns an empty string when the chunk carries no delta", () => {
    expect(extractDelta('{"choices":[{"delta":{}}],"usage":{"total_tokens":5}}')).toBe("")
    expect(extractDelta('{"choices":[]}')).toBe("")
  })

  it("throws an LlmError naming a malformed stream payload", () => {
    expect(() => extractDelta("{not json")).toThrow(/malformad/i)
  })
})

describe("extractToolCallFragments", () => {
  it("reports the index, the id and the name of the first fragment of a call", () => {
    const payload =
      '{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search_material"}}]}}]}'

    expect(extractToolCallFragments(payload)).toEqual([
      { index: 0, id: "call_1", name: "search_material" },
    ])
  })

  it("reports only the argument piece of a later fragment for the same index", () => {
    const payload =
      '{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\\\"query\\\":"}}]}}]}'

    expect(extractToolCallFragments(payload)).toEqual([{ index: 0, arguments: '{"query":' }])
  })

  it("reports no fragment when the payload carries no tool calls", () => {
    expect(extractToolCallFragments('{"choices":[{"delta":{"content":"hola"}}]}')).toEqual([])
    expect(extractToolCallFragments('{"choices":[{"delta":{}}]}')).toEqual([])
    expect(extractToolCallFragments('{"choices":[]}')).toEqual([])
  })

  it("reports the content and the tool calls of a payload that carries both", () => {
    const payload =
      '{"choices":[{"delta":{"content":"pensando","tool_calls":[{"index":0,"id":"call_1","function":{"name":"search_material","arguments":""}}]}}]}'

    expect(extractDelta(payload)).toBe("pensando")
    expect(extractToolCallFragments(payload)).toEqual([
      { index: 0, id: "call_1", name: "search_material", arguments: "" },
    ])
  })

  it("throws an LlmError naming a malformed stream payload", () => {
    expect(() => extractToolCallFragments("{not json")).toThrow(/malformad/i)
  })
})

describe("extractErrorMessage", () => {
  it("returns the provider message inside a JSON error body", () => {
    expect(extractErrorMessage('{"error":{"message":"No auth credentials found"}}')).toBe(
      "No auth credentials found",
    )
  })

  it("returns an empty string when the body has no provider message", () => {
    expect(extractErrorMessage("")).toBe("")
    expect(extractErrorMessage("not json")).toBe("")
    expect(extractErrorMessage('{"error":{}}')).toBe("")
  })
})
