/**
 * The send-message interaction: retrieve, compose, stream, persist.
 *
 * This is orchestration only. Every decision about what the model receives
 * lives in `prompt.ts`; this module decides the order of the steps — persist
 * the question, retrieve, compose, stream, append the answer — and turns every
 * failure into a Spanish message instead of throwing at the UI.
 */
import { defineStore } from "pinia"
import { ref } from "vue"
import { getSettings } from "@/shared/config"
import { streamChatCompletion } from "@/shared/llm"
import { useChatsStore } from "@/entities/chat"
import { searchCorpus } from "@/entities/search-result"
import type { SearchResult } from "@/entities/search-result"
import { buildMessages, buildSources } from "./prompt"
import type { PromptPassage, PromptTurn } from "./prompt"

const MISSING_KEY_ERROR =
  "Falta configurar la API key del modelo en los ajustes del plugin."
const NO_CHAT_ERROR =
  "No hay un chat abierto. Abrí o creá un chat antes de preguntar."
const RETRIEVAL_ERROR = "No se pudo buscar en el corpus."
const SEND_ERROR = "No se pudo obtener la respuesta del modelo."

export const useSendMessage = defineStore("send-message", () => {
  const sending = ref(false)
  /** The deltas received so far, so a UI can render the answer as it arrives. */
  const answer = ref("")
  const error = ref<string | null>(null)

  // Held across the stream so `cancel()` can abort the request in flight.
  let controller: AbortController | null = null

  async function send(question: string): Promise<void> {
    const trimmed = question.trim()

    // A blank question is not a question.
    if (!trimmed) return

    const settings = getSettings()

    if (settings.llmApiKey.trim() === "") {
      error.value = MISSING_KEY_ERROR
      return
    }

    const chats = useChatsStore()
    const note = chats.note

    if (!note) {
      error.value = NO_CHAT_ERROR
      return
    }

    error.value = null
    answer.value = ""
    sending.value = true

    // The history excludes the question, which is appended below.
    const history: PromptTurn[] = note.turns.map((turn) => ({
      role: turn.role,
      content: turn.content,
    }))

    try {
      // The question is a fact whether or not the model answers: persist it first.
      await chats.appendTurn({ role: "user", content: trimmed, sources: [] })

      let results: SearchResult[]

      try {
        const topK = note.chat.settings.topK ?? settings.retrievalTopK

        results = await searchCorpus(trimmed, topK)
      } catch (e) {
        error.value = messageOf(e, RETRIEVAL_ERROR)
        return
      }

      const passages: PromptPassage[] = results.map((result) => ({
        title: result.document_title,
        page: result.page_number,
        text: result.text,
      }))

      const messages = buildMessages({
        chatPrompt: note.chat.prompt,
        history,
        passages,
        question: trimmed,
        maxTurns: settings.historyTurns,
      })

      controller = new AbortController()

      const result = await streamChatCompletion({
        apiKey: settings.llmApiKey,
        baseUrl: settings.llmBaseUrl,
        model: note.chat.settings.model ?? settings.llmModel,
        temperature: note.chat.settings.temperature ?? settings.llmTemperature,
        messages,
        signal: controller.signal,
        onDelta: (delta) => {
          answer.value += delta
        },
      })

      await chats.appendTurn({
        role: "assistant",
        content: result.content,
        sources: buildSources(passages),
      })
    } catch (e) {
      // A cancellation is the user's own decision, not an error to report.
      if (!isAbort(e)) {
        error.value = messageOf(e, SEND_ERROR)
      }
    } finally {
      controller = null
      sending.value = false
    }
  }

  /** Abort the stream in flight; the partial answer stays in `answer`. */
  function cancel(): void {
    controller?.abort()
  }

  function clearError(): void {
    error.value = null
  }

  return {
    sending,
    answer,
    error,
    send,
    cancel,
    clearError,
  }
})

function messageOf(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback
}

function isAbort(e: unknown): boolean {
  return e instanceof Error && e.name === "AbortError"
}
