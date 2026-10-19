/**
 * The corpus list request: one URL and one forwarded signal.
 *
 * `listDocuments` is the corpus preflight's probe, so the two things that matter
 * here are that the URL did not move and that the caller's `AbortSignal` reaches
 * `fetch`: the probe is a network await in front of the first model call, and a
 * server that accepts the connection and never answers has to be abandonable.
 */
import { afterEach, describe, expect, it, vi } from "vitest"
import { DEFAULT_API_BASE, resetApiBase } from "@/shared/api/client"
import { listDocuments } from "./document-api"
import type { DocumentSummary } from "../model/types"

const DOCUMENT: DocumentSummary = {
  id: "doc-ddd",
  title: "Domain-Driven Design",
  createdAt: "2024-01-01T00:00:00Z",
  pages: 42,
  chunks: 120,
}

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response
}

afterEach(() => {
  vi.unstubAllGlobals()
  resetApiBase()
})

describe("listDocuments", () => {
  it("lists the corpus with the existing URL and no signal when none is given", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse([DOCUMENT])))

    const documents = await listDocuments()

    expect(documents).toEqual([DOCUMENT])
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)
    const [url, init] = vi.mocked(fetch).mock.calls[0]

    expect(url).toBe(`${DEFAULT_API_BASE}/api/rag/documents`)
    // Existing callers pass nothing, so nothing is aborted for them.
    expect((init as RequestInit | undefined)?.signal).toBeUndefined()
  })

  it("forwards the caller's signal to the request", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse([])))
    const { signal } = new AbortController()

    await listDocuments(signal)

    const [, init] = vi.mocked(fetch).mock.calls[0]

    expect((init as RequestInit).signal).toBe(signal)
  })

  it("rejects when the request is aborted instead of resolving", async () => {
    // The client maps every `fetch` failure to its own error type, so what this
    // layer guarantees is only that it gives up: telling a cancel from a dead
    // server is the preflight's job, done from the signal.
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise((_resolve, reject) => {
            init?.signal?.addEventListener("abort", () => {
              const error = new Error("The operation was aborted.")

              error.name = "AbortError"
              reject(error)
            })
          }),
      ),
    )
    const controller = new AbortController()

    const pending = listDocuments(controller.signal)

    controller.abort()

    await expect(pending).rejects.toThrow()
  })
})
