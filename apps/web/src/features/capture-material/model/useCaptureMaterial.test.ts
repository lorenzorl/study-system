import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import { useResourcesStore } from "@/entities/resource"
import type { ResourceResponse } from "@/entities/resource"
import { resetApiBase } from "@/shared/api/client"
import { useCaptureMaterial } from "./useCaptureMaterial"
import type { CaptureSource } from "./useCaptureMaterial"

/**
 * One capture touches three requests: the corpus ingest, the resource
 * registration and the launcher's reload. The network boundary is the only thing
 * faked — the same seam `entities/resource/model/useResourcesStore.test.ts` uses —
 * so every assertion below reads the request the app actually sent.
 */

const DOCUMENT_ID = "doc-1"

const INGEST_PATHS = {
  file: "/api/rag/documents",
  text: "/api/rag/documents/text",
  url: "/api/rag/documents/url",
} as const

const REGISTERED: ResourceResponse = {
  id: "r-1",
  title: "Atención selectiva",
  type: "note",
  source_uri: `rag://documents/${DOCUMENT_ID}`,
  topic_name: "Atención",
  created_at: "2026-03-01T12:00:00Z",
}

const INGESTED = {
  document_id: DOCUMENT_ID,
  state: "indexing",
  chunks: 1,
  embedded: 0,
}

interface Intercepted {
  url: string
  init: RequestInit
}

type Handler = (call: Intercepted) => Response

let calls: Intercepted[] = []

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

/** Answer every request from one handler, recording what the app asked for. */
function stubServer(handler: Handler): void {
  calls = []
  const fetchMock = vi.fn(
    async (input: string | URL | Request, init?: RequestInit) => {
      const call: Intercepted = { url: String(input), init: init ?? {} }
      calls.push(call)
      return handler(call)
    },
  )
  vi.stubGlobal("fetch", fetchMock)
}

/** The fake server for a capture that works: ingest, registration, reload. */
function happyServer(): Handler {
  return (call) => {
    if (call.url === INGEST_PATHS.file) return jsonResponse(INGESTED, 202)
    if (call.url === INGEST_PATHS.text) return jsonResponse(INGESTED, 201)
    if (call.url === INGEST_PATHS.url) {
      return jsonResponse({ ...INGESTED, source_uri: "https://example.com/paper" }, 201)
    }
    if (call.url === "/api/sync/resource") {
      return jsonResponse({ resource_id: REGISTERED.id }, 201)
    }
    if (call.url === "/api/resources") return jsonResponse([REGISTERED])
    throw new Error(`unexpected request ${call.url}`)
  }
}

function bodyOf(call: Intercepted): unknown {
  return JSON.parse(String(call.init.body))
}

function headerNames(call: Intercepted): string[] {
  const headers = (call.init.headers ?? {}) as Record<string, string>
  return Object.keys(headers).map((name) => name.toLowerCase())
}

function urls(): string[] {
  return calls.map((call) => call.url)
}

const TEXT_SOURCE: CaptureSource = {
  mode: "text",
  topicName: "Atención",
  title: "Atención selectiva",
  text: "La atención selectiva filtra estímulos.",
}

const URL_SOURCE: CaptureSource = {
  mode: "url",
  topicName: "DDD",
  title: "Paper",
  url: "https://example.com/papers/attention",
}

function fileSource(): CaptureSource {
  return {
    mode: "file",
    topicName: "DDD",
    file: new File(["%PDF-1.7"], "clean-arch.pdf", {
      type: "application/pdf",
    }),
  }
}

function sourceFor(mode: "file" | "text" | "url"): CaptureSource {
  if (mode === "file") return fileSource()
  if (mode === "url") return URL_SOURCE
  return TEXT_SOURCE
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetApiBase()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useCaptureMaterial", () => {
  it("posts a pasted text and registers it as a note in the corpus", async () => {
    stubServer(happyServer())
    const store = useCaptureMaterial()

    const saved = await store.capture(TEXT_SOURCE)

    expect(saved).toBe(true)
    expect(store.error).toBeNull()
    expect(urls()).toEqual([
      INGEST_PATHS.text,
      "/api/sync/resource",
      "/api/resources",
    ])

    const ingest = calls[0]
    expect(ingest.init.method).toBe("POST")
    expect(headerNames(ingest)).toContain("content-type")
    expect(bodyOf(ingest)).toEqual({
      title: "Atención selectiva",
      text: "La atención selectiva filtra estímulos.",
    })

    const registration = calls[1]
    expect(registration.init.method).toBe("POST")
    expect(bodyOf(registration)).toEqual({
      topic_name: "Atención",
      resource_title: "Atención selectiva",
      type: "note",
      source_uri: `rag://documents/${DOCUMENT_ID}`,
    })
  })

  it("posts a URL and registers it as an article pointing at the typed URL", async () => {
    stubServer(happyServer())
    const store = useCaptureMaterial()

    expect(await store.capture(URL_SOURCE)).toBe(true)

    expect(calls[0].url).toBe(INGEST_PATHS.url)
    expect(bodyOf(calls[0])).toEqual({
      title: "Paper",
      url: "https://example.com/papers/attention",
    })
    expect(bodyOf(calls[1])).toEqual({
      topic_name: "DDD",
      resource_title: "Paper",
      type: "article",
      // The typed URL, not the answer's post-redirect source_uri.
      source_uri: "https://example.com/papers/attention",
    })
  })

  it("names a title-less article after its URL, as the ingest route does", async () => {
    stubServer(happyServer())
    const store = useCaptureMaterial()

    await store.capture({ mode: "url", topicName: "DDD", title: "  ", url: "https://example.com/papers/attention" })
    expect(bodyOf(calls[0])).toEqual({
      title: "",
      url: "https://example.com/papers/attention",
    })
    expect(bodyOf(calls[1])).toMatchObject({ resource_title: "attention" })

    await store.capture({ mode: "url", topicName: "DDD", title: "", url: "https://example.com" })
    // Per capture: the ingest, then the registration, then the reload.
    expect(bodyOf(calls[4])).toMatchObject({ resource_title: "example.com" })
  })

  it("uploads the picked file as multipart and registers it as a book", async () => {
    stubServer(happyServer())
    const store = useCaptureMaterial()

    expect(await store.capture(fileSource())).toBe(true)

    const upload = calls[0]
    expect(upload.url).toBe(INGEST_PATHS.file)
    expect(upload.init.method).toBe("POST")

    const form = upload.init.body
    expect(form).toBeInstanceOf(FormData)
    const sent = form as FormData
    expect([...sent.keys()]).toEqual(["file"])
    const part = sent.get("file")
    expect(part).toBeInstanceOf(File)
    expect((part as File).name).toBe("clean-arch.pdf")

    // The browser derives the multipart boundary from the body, so the app must
    // not declare a Content-Type: a hand-set one would name a boundary the body
    // does not carry.
    expect(headerNames(upload)).not.toContain("content-type")

    expect(bodyOf(calls[1])).toEqual({
      topic_name: "DDD",
      resource_title: "clean-arch.pdf",
      type: "book",
      source_uri: `rag://documents/${DOCUMENT_ID}`,
    })
  })

  const REFUSALS = [
    { status: 409, mode: "text" as const, message: "a document with the same text already exists" },
    { status: 413, mode: "text" as const, message: "the request body must be at most 10485760 bytes" },
    {
      status: 415,
      mode: "url" as const,
      message: 'the URL answered "text/html", which is not a text payload',
    },
    {
      status: 503,
      mode: "file" as const,
      message: "uploads require the STUDY_OPENROUTER_API_KEY environment variable",
    },
  ]

  it.each(REFUSALS)(
    "shows the server's own message for a $status and registers nothing",
    async ({ status, mode, message }) => {
      stubServer((call) =>
        call.url === INGEST_PATHS[mode]
          ? jsonResponse({ error: message }, status)
          : jsonResponse([], 200),
      )
      const store = useCaptureMaterial()

      const saved = await store.capture(sourceFor(mode))

      expect(saved).toBe(false)
      expect(store.error).toBe(message)
      // Only the ingest was attempted: a refused ingest registers nothing.
      expect(urls()).toEqual([INGEST_PATHS[mode]])
    },
  )

  it("reloads the launcher's list through the entity store after a capture", async () => {
    stubServer(happyServer())
    const store = useCaptureMaterial()
    const resources = useResourcesStore()
    expect(resources.resources).toEqual([])

    await store.capture(TEXT_SOURCE)

    expect(urls()).toContain("/api/resources")
    expect(resources.resources).toEqual([
      {
        id: "r-1",
        title: "Atención selectiva",
        type: "note",
        sourceUri: `rag://documents/${DOCUMENT_ID}`,
        topicTitle: "Atención",
        createdAt: "2026-03-01T12:00:00Z",
      },
    ])
  })

  it("keeps the message of a refused registration and does not reload", async () => {
    const refusal = "topic_name, resource_title, type, and source_uri are required"
    stubServer((call) => {
      if (call.url === INGEST_PATHS.text) return jsonResponse(INGESTED, 201)
      if (call.url === "/api/sync/resource") {
        return jsonResponse({ error: refusal }, 400)
      }
      return jsonResponse([])
    })
    const store = useCaptureMaterial()

    expect(await store.capture(TEXT_SOURCE)).toBe(false)
    expect(store.error).toBe(refusal)
    expect(urls()).toEqual([INGEST_PATHS.text, "/api/sync/resource"])
  })

  it("reports a dropped connection and registers nothing", async () => {
    stubServer(() => {
      throw new TypeError("fetch failed")
    })
    const store = useCaptureMaterial()

    expect(await store.capture(TEXT_SOURCE)).toBe(false)
    expect(store.error).toBe("No se pudo conectar al servidor. ¿Sigue corriendo?")
    expect(urls()).toEqual([INGEST_PATHS.text])
  })

  it("drops the previous error when a retry starts", async () => {
    stubServer(happyServer())
    const store = useCaptureMaterial()
    store.error = "caído"

    await store.capture(TEXT_SOURCE)

    expect(store.error).toBeNull()
  })
})
