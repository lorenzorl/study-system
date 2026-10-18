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

/**
 * The line that marks an answer the stream cut short.
 *
 * Plain text and not markdown: the thread renders a turn's content verbatim, so
 * an italic marker would show its own asterisks. It travels inside the turn's
 * content, which means the model reads it too — and that is the point: the next
 * answer should know the previous one was interrupted.
 */
export const INTERRUPTED_MARKER = "— respuesta interrumpida"

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

    // One send at a time. A second question while a stream is in flight would
    // interleave two answers, append four turns for two questions and leave the
    // first request's controller unreachable, so `cancel()` would stop working.
    if (sending.value) return

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

      // Whatever arrived is text the user already read on screen, so letting it
      // vanish when the stream ends loses an answer instead of protecting them
      // from half of one. It is saved marked as interrupted, from a failure and
      // from their own cancel alike (findings R4-partial-content-dropped and
      // R4-002).
      await savePartialAnswer(chats)
    } finally {
      controller = null
      sending.value = false
    }
  }

  /**
   * Save what arrived before the stream was cut, marked as interrupted.
   *
   * Nothing arrived means nothing to save: an empty turn would leave a marker in
   * the transcript for a question the model never began to answer.
   */
  async function savePartialAnswer(
    chats: ReturnType<typeof useChatsStore>,
  ): Promise<void> {
    const partial = answer.value.trim()

    if (partial === "") return

    try {
      await chats.appendTurn({
        role: "assistant",
        content: `${partial}\n\n${INTERRUPTED_MARKER}`,
        sources: [],
      })
    } catch {
      // The chat store reports its own write failures in its own error, and a
      // failure here must not replace the reason the answer was cut.
    }
  }

  /** Abort the stream in flight; what arrived so far is saved as interrupted. */
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
