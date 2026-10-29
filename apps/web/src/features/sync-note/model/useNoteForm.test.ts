import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import { resetApiBase } from "@/shared/api/client"
import { noteFilePath, useNoteForm } from "./useNoteForm"

/**
 * The paste form's own rules — what makes it submittable, what it sends, and
 * what a refusal leaves behind — driven without a DOM: the vitest environment
 * is `node`, and the fields are plain refs.
 */

const CONCEPT_ID = "concept-agregados"
const TOPIC_ID = "topic-ddd"
const MARKDOWN = "¿Qué es un agregado?::Un conjunto consistente de cambios."

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

function happyServer(): Handler {
  return (call) => {
    if (call.url === "/api/sync/concept") {
      return jsonResponse({ concept_id: CONCEPT_ID }, 201)
    }
    if (call.url === "/api/sync/flashcards") return jsonResponse({ synced: 1 })
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

beforeEach(() => {
  setActivePinia(createPinia())
})

afterEach(() => {
  vi.unstubAllGlobals()
  resetApiBase()
})

describe("noteFilePath", () => {
  it("names the concept's note after its topic and title", () => {
    expect(noteFilePath("DDD", "Agregados")).toBe("DDD/Agregados.md")
    expect(noteFilePath("  DDD ", " Agregados ")).toBe("DDD/Agregados.md")
  })
})

describe("useNoteForm", () => {
  it("stays unsubmittable until all three fields carry something", () => {
    const form = useNoteForm()

    expect(form.canSubmit.value).toBe(false)

    form.topicName.value = "DDD"
    form.conceptTitle.value = "Agregados"
    form.markdown.value = "   \n "
    expect(form.canSubmit.value).toBe(false)

    form.markdown.value = MARKDOWN
    expect(form.canSubmit.value).toBe(true)
  })

  it("sends the trimmed fields with the note's path and parses the cards", async () => {
    stubServer(happyServer())
    const form = useNoteForm()

    form.topicName.value = " DDD "
    form.conceptTitle.value = " Agregados "
    form.markdown.value = MARKDOWN

    const result = await form.submit()

    expect(bodyOf("/api/sync/concept")).toEqual({
      topic_name: "DDD",
      concept_title: "Agregados",
      // There is no file in this app, so the path is the note's own name.
      file_path: "DDD/Agregados.md",
    })
    expect(bodyOf("/api/sync/flashcards")).toEqual({
      concept_id: CONCEPT_ID,
      cards: [
        {
          obsidian_id: "card-1",
          question: "¿Qué es un agregado?",
          answer: "Un conjunto consistente de cambios.",
        },
      ],
    })
    expect(result).toEqual({
      conceptId: CONCEPT_ID,
      topicId: TOPIC_ID,
      synced: 1,
    })

    // The note is synced, so the next opening starts empty.
    form.reset()
    expect(form.topicName.value).toBe("")
    expect(form.conceptTitle.value).toBe("")
    expect(form.markdown.value).toBe("")
  })

  it("keeps every field when the server refuses", async () => {
    const refusal = "No se encontraron tarjetas en la nota. Usá el formato pregunta::respuesta."
    stubServer((call) => {
      if (call.url === "/api/sync/concept") {
        return jsonResponse({ concept_id: CONCEPT_ID }, 201)
      }
      throw new Error(`unexpected request ${call.url}`)
    })
    const form = useNoteForm()

    form.topicName.value = " DDD "
    form.conceptTitle.value = " Agregados "
    form.markdown.value = "# Agregados\nSin tarjetas."

    const result = await form.submit()

    expect(result).toBeNull()
    expect(form.error.value).toBe(refusal)
    // Nothing is cleared: the reader's text is what they have to fix.
    expect(form.topicName.value).toBe(" DDD ")
    expect(form.conceptTitle.value).toBe(" Agregados ")
    expect(form.markdown.value).toBe("# Agregados\nSin tarjetas.")
    expect(form.canSubmit.value).toBe(true)
  })
})
