import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import { resetApiBase } from "@/shared/api/client"
import { useSyncNote } from "./useSyncNote"

/**
 * One sync touches three requests: the concept, its cards and the topic reload
 * the list needs. Only the network boundary is faked — the same seam
 * `entities/resource/model/useResourcesStore.test.ts` uses — so every assertion
 * reads the request the app actually sent.
 */

const CONCEPT_ID = "concept-agregados"
const TOPIC_ID = "topic-ddd"

const NOTES_PATH = "DDD/Agregados.md"

const MARKDOWN = [
  "¿Qué es un agregado?::Un conjunto consistente de cambios.",
  "¿Qué es un evento?::Un hecho del pasado que el dominio recuerda.",
].join("\n")

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

/** The fake server for a sync that works: concept, cards, reload. */
function happyServer(): Handler {
  return (call) => {
    if (call.url === "/api/sync/concept") {
      return jsonResponse({ concept_id: CONCEPT_ID }, 201)
    }
    if (call.url === "/api/sync/flashcards") return jsonResponse({ synced: 2 })
    if (call.url === "/api/concepts") {
      return jsonResponse([{ id: TOPIC_ID, name: "DDD", concepts: [] }])
    }
    throw new Error(`unexpected request ${call.url}`)
  }
}

function bodyOf(url: string): unknown {
  const call = calls.find((candidate) => candidate.url === url)
  expect(call).toBeDefined()

  return JSON.parse(String(call?.init.body))
}

function urls(): string[] {
  return calls.map((call) => call.url)
}

beforeEach(() => {
  setActivePinia(createPinia())
})

afterEach(() => {
  vi.unstubAllGlobals()
  resetApiBase()
})

describe("useSyncNote", () => {
  it("sends the topic, the concept and the note's path, then the parsed cards", async () => {
    stubServer(happyServer())
    const store = useSyncNote()

    const result = await store.syncCurrentNote(
      "DDD",
      "Agregados",
      NOTES_PATH,
      MARKDOWN,
    )

    // The order is the contract: the cards need the id the concept sync returns.
    expect(urls()).toEqual([
      "/api/sync/concept",
      "/api/sync/flashcards",
      "/api/concepts",
    ])
    expect(bodyOf("/api/sync/concept")).toEqual({
      topic_name: "DDD",
      concept_title: "Agregados",
      file_path: NOTES_PATH,
    })
    expect(bodyOf("/api/sync/flashcards")).toEqual({
      concept_id: CONCEPT_ID,
      cards: [
        {
          obsidian_id: "card-1",
          question: "¿Qué es un agregado?",
          answer: "Un conjunto consistente de cambios.",
        },
        {
          obsidian_id: "card-2",
          question: "¿Qué es un evento?",
          answer: "Un hecho del pasado que el dominio recuerda.",
        },
      ],
    })
    expect(result).toEqual({
      conceptId: CONCEPT_ID,
      topicId: TOPIC_ID,
      synced: 2,
    })
    expect(store.error).toBeNull()
  })

  it("refuses a note without a card pair, and sends no cards", async () => {
    stubServer((call) => {
      if (call.url === "/api/sync/concept") {
        return jsonResponse({ concept_id: CONCEPT_ID }, 201)
      }
      throw new Error(`unexpected request ${call.url}`)
    })
    const store = useSyncNote()

    const result = await store.syncCurrentNote(
      "DDD",
      "Agregados",
      NOTES_PATH,
      "# Agregados\nUn agregado agrupa invariantes.",
    )

    expect(result).toBeNull()
    expect(store.error).toBe(
      "No se encontraron tarjetas en la nota. Usá el formato pregunta::respuesta.",
    )
    // Only the concept was asked for: a note with no card is not a card sync.
    expect(urls()).toEqual(["/api/sync/concept"])
  })

  it("surfaces the server's own refusal message", async () => {
    const refusal = "topic_name, concept_title, and file_path are required"
    stubServer((call) => {
      if (call.url === "/api/sync/concept") {
        return jsonResponse({ error: refusal }, 400)
      }
      throw new Error(`unexpected request ${call.url}`)
    })
    const store = useSyncNote()

    const result = await store.syncCurrentNote(
      "DDD",
      "Agregados",
      NOTES_PATH,
      MARKDOWN,
    )

    expect(result).toBeNull()
    expect(store.error).toBe(refusal)
    expect(urls()).toEqual(["/api/sync/concept"])
  })

  it("opens and closes its surface, dropping a refusal from the last attempt", async () => {
    stubServer(happyServer())
    const store = useSyncNote()

    expect(store.panelOpen).toBe(false)

    store.openPanel()
    expect(store.panelOpen).toBe(true)

    await store.syncCurrentNote("DDD", "Agregados", NOTES_PATH, "# sin tarjetas")
    expect(store.error).not.toBeNull()

    store.closePanel()
    expect(store.panelOpen).toBe(false)
    expect(store.error).toBeNull()
  })
})
