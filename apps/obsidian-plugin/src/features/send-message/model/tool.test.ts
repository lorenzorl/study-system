/**
 * The search tool: its specification, its scope decision and its execution.
 *
 * The scope invariant is the point of most of this file: the chat fixes what the
 * model may read, so a call that names a document which is not attached to the
 * chat is refused instead of executed. The execution tests check the two calls
 * the tool can make (scoped and unscoped) and that a passage is rendered by the
 * same composer the prompt uses, so it reads the same wherever it came from.
 */
import { afterEach, describe, expect, it, vi } from "vitest"
import type { ChatAttachment } from "@/entities/chat"
import { searchCorpus } from "@/entities/search-result"
import type { SearchResult } from "@/entities/search-result"
import type { ToolCall } from "@/shared/llm"
import { buildContext } from "./prompt"
import type { ContextBlock } from "./prompt"
import { SEARCH_MATERIAL_TOOL, SEARCH_MATERIAL_TOOL_NAME, decideToolCall, runSearchTool } from "./tool"

const DDD: ChatAttachment = { kind: "corpus", documentId: "doc-ddd", title: "Domain-Driven Design" }
const REFACTORING: ChatAttachment = { kind: "corpus", documentId: "doc-ref", title: "Refactoring" }
const VAULT: ChatAttachment = { kind: "vault", path: "Notas/Tesis.md" }

const ATTACHMENTS: ChatAttachment[] = [DDD, REFACTORING, VAULT]

const SEARCH_RESULT: SearchResult = {
  chunk_id: "c1",
  document_id: "doc-ddd",
  document_title: "Domain-Driven Design",
  page_number: 42,
  snippet: "[El] modelo del [dominio]",
  text: "El modelo del dominio es la raíz del lenguaje ubicuo.",
  stage: "hybrid",
  bm25: 1,
  similarity: 0.5,
}

const OTHER_RESULT: SearchResult = {
  ...SEARCH_RESULT,
  chunk_id: "c2",
  document_title: "Refactoring",
  document_id: "doc-ref",
  page_number: 7,
  text: "Reemplazar el valor mágico por una constante con nombre.",
}

const DDD_BLOCK: ContextBlock = {
  kind: "corpus",
  title: "Domain-Driven Design",
  page: 42,
  text: "El modelo del dominio es la raíz del lenguaje ubicuo.",
}

function toolCall(args: Record<string, unknown>, name = SEARCH_MATERIAL_TOOL_NAME): ToolCall {
  return {
    id: "call-1",
    type: "function",
    function: { name, arguments: JSON.stringify(args) },
  }
}

function rawToolCall(args: string): ToolCall {
  return { id: "call-1", type: "function", function: { name: SEARCH_MATERIAL_TOOL_NAME, arguments: args } }
}

function fakeSearch(results: SearchResult[] = [SEARCH_RESULT]) {
  return vi.fn(async () => results)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("SEARCH_MATERIAL_TOOL", () => {
  it("offers one function that searches the material attached to the chat", () => {
    expect(SEARCH_MATERIAL_TOOL.type).toBe("function")
    expect(SEARCH_MATERIAL_TOOL.function.name).toBe(SEARCH_MATERIAL_TOOL_NAME)
    expect(SEARCH_MATERIAL_TOOL_NAME).toBe("search_material")
    expect(SEARCH_MATERIAL_TOOL.function.description.toLowerCase()).toContain("material")
    expect(SEARCH_MATERIAL_TOOL.function.description.length).toBeGreaterThan(0)
  })

  it("takes a required query and an optional resource id, and nothing else", () => {
    const parameters = SEARCH_MATERIAL_TOOL.function.parameters as {
      type: string
      properties: Record<string, { type: string; description: string }>
      required: string[]
    }

    expect(parameters.type).toBe("object")
    expect(Object.keys(parameters.properties)).toEqual(["query", "resource_id"])
    expect(parameters.properties.query.type).toBe("string")
    expect(parameters.properties.resource_id.type).toBe("string")
    expect(parameters.required).toEqual(["query"])
  })
})

describe("decideToolCall", () => {
  it("accepts a call with no resource id and runs it unscoped", () => {
    expect(decideToolCall(toolCall({ query: "lenguaje ubicuo" }), ATTACHMENTS)).toEqual({
      ok: true,
      query: "lenguaje ubicuo",
      documentId: null,
    })
  })

  it("accepts a resource id that is one of the chat's corpus attachments", () => {
    expect(
      decideToolCall(toolCall({ query: "ubicuo", resource_id: "doc-ddd" }), ATTACHMENTS),
    ).toEqual({ ok: true, query: "ubicuo", documentId: "doc-ddd" })
  })

  it("refuses a resource id that is not attached, with a message the model can read", () => {
    const decision = decideToolCall(
      toolCall({ query: "ubicuo", resource_id: "doc-ajeno" }),
      ATTACHMENTS,
    )

    expect(decision.ok).toBe(false)
    if (decision.ok) throw new Error("unreachable")
    expect(decision.error).toContain("doc-ajeno")
    expect(decision.error).toContain("no está adjunto")
    // The refusal has to say what *is* reachable, or the model retries blindly.
    expect(decision.error).toContain("Domain-Driven Design")
    expect(decision.error).toContain("Refactoring")
  })

  it("refuses a vault attachment used as a resource id: the tool only reads the corpus", () => {
    const decision = decideToolCall(
      toolCall({ query: "estado", resource_id: "Notas/Tesis.md" }),
      ATTACHMENTS,
    )

    expect(decision.ok).toBe(false)
  })

  it("treats a blank resource id as not narrowing, which cannot widen the scope", () => {
    expect(decideToolCall(toolCall({ query: "ubicuo", resource_id: "" }), ATTACHMENTS)).toEqual({
      ok: true,
      query: "ubicuo",
      documentId: null,
    })
  })

  it("refuses a call whose arguments are not a JSON object", () => {
    const decision = decideToolCall(rawToolCall("no soy JSON"), ATTACHMENTS)

    expect(decision.ok).toBe(false)
    if (decision.ok) throw new Error("unreachable")
    expect(decision.error).toContain("query")
  })

  it("refuses a call without a query instead of searching for nothing", () => {
    const missing = decideToolCall(toolCall({ resource_id: "doc-ddd" }), ATTACHMENTS)
    const blank = decideToolCall(toolCall({ query: "   " }), ATTACHMENTS)

    expect(missing.ok).toBe(false)
    expect(blank.ok).toBe(false)
  })
})

describe("runSearchTool", () => {
  it("searches only the named document when the model narrowed", async () => {
    const search = fakeSearch()

    await runSearchTool(
      { ok: true, query: "lenguaje ubicuo", documentId: "doc-ddd" },
      { search: search as never, limit: 6 },
    )

    expect(search).toHaveBeenCalledWith("lenguaje ubicuo", 6, "doc-ddd")
  })

  it("searches the whole attached material when the model did not narrow", async () => {
    const search = fakeSearch()

    await runSearchTool(
      { ok: true, query: "lenguaje ubicuo", documentId: null },
      { search: search as never, limit: 6 },
    )

    expect(search).toHaveBeenCalledWith("lenguaje ubicuo", 6, undefined)
  })

  it("turns the passages into blocks and into the text of a tool result", async () => {
    const outcome = await runSearchTool(
      { ok: true, query: "lenguaje ubicuo", documentId: null },
      { search: fakeSearch() as never },
    )

    expect(outcome.blocks).toEqual([DDD_BLOCK])
    // The same composer as the prompt: a passage reads the same wherever it came from.
    expect(outcome.content).toBe(buildContext([DDD_BLOCK]))
    expect(outcome.content).toBe("[1] Domain-Driven Design — p. 42\n" + SEARCH_RESULT.text)
  })

  it("keeps the order and the page of every passage it got", async () => {
    const outcome = await runSearchTool(
      { ok: true, query: "modelo", documentId: null },
      { search: fakeSearch([SEARCH_RESULT, OTHER_RESULT]) as never },
    )

    expect(outcome.blocks).toEqual([
      DDD_BLOCK,
      {
        kind: "corpus",
        title: "Refactoring",
        page: 7,
        text: OTHER_RESULT.text,
      },
    ])
    expect(outcome.content).toBe(buildContext(outcome.blocks))
  })

  it("says plainly when the search returned nothing, and carries no block", async () => {
    const outcome = await runSearchTool(
      { ok: true, query: "algo que no está", documentId: null },
      { search: fakeSearch([]) as never },
    )

    expect(outcome.blocks).toEqual([])
    expect(outcome.content).toContain("no devolvió ningún pasaje")
  })
})

describe("searchCorpus", () => {
  function stubFetch() {
    // Typed with fetch's own signature so the tests can read the requested URL: a
    // parameterless `vi.fn` types its call list as an empty tuple.
    const fetchMock = vi.fn(
      async (_url: string, _init?: RequestInit) =>
        new Response("[]", { status: 200, headers: { "Content-Type": "application/json" } }),
    )

    vi.stubGlobal("fetch", fetchMock)

    return fetchMock
  }

  it("sends exactly today's query string when no document is given", async () => {
    const fetchMock = stubFetch()

    await searchCorpus("lenguaje ubicuo", 6)

    // The corpus search page calls this same function; its URL must not change.
    expect(fetchMock.mock.calls[0][0]).toBe(
      "http://localhost:8080/api/rag/search?q=lenguaje+ubicuo&limit=6",
    )
  })

  it("sends neither limit nor document_id when neither is given", async () => {
    const fetchMock = stubFetch()

    await searchCorpus("solo")

    expect(fetchMock.mock.calls[0][0]).toBe("http://localhost:8080/api/rag/search?q=solo")
  })

  it("adds document_id and nothing else when a document is given", async () => {
    const fetchMock = stubFetch()

    await searchCorpus("lenguaje ubicuo", 6, "doc-ddd")

    expect(fetchMock.mock.calls[0][0]).toBe(
      "http://localhost:8080/api/rag/search?q=lenguaje+ubicuo&limit=6&document_id=doc-ddd",
    )
  })
})
