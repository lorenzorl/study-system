/**
 * The send-message interaction: compose, stream, persist.
 *
 * This is orchestration only. Every decision about what the model receives
 * lives in `prompt.ts`, and every decision about what the model may read lives
 * in `tool.ts`; this module decides the order of the steps — persist the
 * question, compose, converse, append the answer — and turns every failure into
 * a Spanish message instead of throwing at the UI.
 *
 * The conversation is one round trip when the chat has no material behind the
 * tool: no tools are declared and the backend is not touched, which is what lets
 * such a chat answer with the server down. A vault attachment is material that
 * travels in the message instead, read whole at send time; a corpus attachment is
 * material the model has to ask for, and the exchange that follows lives only in
 * memory: `ChatTurn` cannot represent an assistant turn with `tool_calls`, and
 * that machinery belongs to one answer rather than to the conversation.
 */
import { defineStore } from "pinia"
import { ref } from "vue"
import { getSettings } from "@/shared/config"
import { streamChatCompletion } from "@/shared/llm"
import type { ChatRequest } from "@/shared/llm"
import { useChatsStore } from "@/entities/chat"
import type { ChatAttachment } from "@/entities/chat"
import { buildMessages, buildSources } from "./prompt"
import type { ContextBlock, MaterialMode, PromptTurn } from "./prompt"
import { SEARCH_MATERIAL_TOOL, decideToolCall, runSearchTool } from "./tool"
import { readVaultMaterial } from "./vault-text"

const MISSING_KEY_ERROR =
  "Falta configurar la API key del modelo en los ajustes del plugin."
const NO_CHAT_ERROR =
  "No hay un chat abierto. Abrí o creá un chat antes de preguntar."
const SEND_ERROR = "No se pudo obtener la respuesta del modelo."
const CANCELLED_ERROR = "La respuesta se canceló."

/**
 * How many times the model may ask for a search in one message.
 *
 * Without a cap, a vague question chains searches and burns the user's key. At
 * the cap the answer is requested once more with what is already there, so the
 * turn ends with an answer instead of an error.
 */
export const MAX_TOOL_ITERATIONS = 3

/** A corpus attachment, the only kind the search tool reads. */
type CorpusAttachment = Extract<ChatAttachment, { kind: "corpus" }>

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

  // Held across the whole loop so `cancel()` can abort the request in flight and
  // stop the next iteration from ever starting.
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

    const model = note.chat.settings.model ?? settings.llmModel
    const temperature = note.chat.settings.temperature ?? settings.llmTemperature
    const topK = note.chat.settings.topK ?? settings.retrievalTopK

    // `tools` is declared if and only if the chat has corpus material. A vault
    // file is sent whole and needs neither the tool nor the server.
    const corpusAttachments = note.attachments.filter(
      (attachment): attachment is CorpusAttachment => attachment.kind === "corpus",
    )
    const searchable = corpusAttachments.length > 0

    // Until a response arrives, a provider rejection may be a refusal of `tools`.
    let toolsAwaitingFirstResponse = searchable

    /**
     * The third state of the frame, decided here and never asked of the model.
     *
     * A search counts as executed only when `decideToolCall` accepted the call
     * and the tool actually ran: a call whose scope the client rejected read no
     * material, so it cannot turn a memory answer into a founded one. A chat
     * that declares no tool has nothing to consult, so its answer is founded by
     * definition and the badge never shows.
     */
    let searchedMaterial = false
    const materialConsulted = (): boolean => !searchable || searchedMaterial

    try {
      // The question is a fact whether or not the model answers: persist it first.
      await chats.appendTurn({
        role: "user",
        content: trimmed,
        sources: [],
        // Nothing was consulted by asking, so the question claims nothing.
        consulted: true,
      })

      // The vault files travel whole in this request (S7), so their text is read
      // now and not when they were attached: a note grows. A file that cannot be
      // read, or no longer fits its cap, refuses the send instead of going out
      // over the cap or being dropped quietly -- a chat that answers without
      // material it claimed to have is the lie this path exists to remove.
      const vault = await readVaultMaterial(note.attachments)

      if (!vault.ok) {
        // The question stays in the note, which is why it is persisted first.
        error.value = vault.error
        return
      }

      const blocks = vault.blocks

      // The vault text is already in the message, so the model grounds on what it
      // can see and cites each file by its path. A chat whose material is only
      // behind the tool keeps the searchable frame; a chat with both sends the
      // vault text and still declares the tool, because the frame speaks about the
      // material in front of the model and the tool is how it reaches the rest.
      const material: MaterialMode =
        blocks.length > 0 ? "in-message" : searchable ? "searchable" : "none"

      const messages = buildMessages({
        chatPrompt: note.chat.prompt,
        history,
        blocks,
        material,
        question: trimmed,
        maxTurns: settings.historyTurns,
      })

      controller = new AbortController()

      const signal = controller.signal
      const tools = searchable ? [SEARCH_MATERIAL_TOOL] : undefined
      /** The text of every round, which is the answer the user reads. */
      let text = ""
      /**
       * What the answer may cite: the vault files sent in the message plus the
       * passages the tool actually returned, in that order.
       */
      const citable: ContextBlock[] = [...blocks]

      for (let round = 0; round <= MAX_TOOL_ITERATIONS; round += 1) {
        ensureNotCancelled(signal)

        // The last pass is the final ask: `tools` is omitted so the model has to
        // answer with what is already in the conversation.
        const isFinalAsk = round === MAX_TOOL_ITERATIONS
        const request: ChatRequest = {
          apiKey: settings.llmApiKey,
          baseUrl: settings.llmBaseUrl,
          model,
          temperature,
          messages: [...messages],
          signal,
          onDelta: (delta) => {
            text += delta
            answer.value += delta
          },
        }

        // Declared only when there is something to declare: a chat without corpus
        // material sends exactly the request it sent before attachments existed.
        if (!isFinalAsk && tools !== undefined) request.tools = tools

        const result = await streamChatCompletion(request)

        toolsAwaitingFirstResponse = false

        // An answer without tool calls is the final answer.
        if (result.toolCalls.length === 0) break
        if (isFinalAsk) break

        // The assistant turn goes back exactly as it arrived: the provider wire
        // shape is what it expects to read again, so there is no translation.
        messages.push({
          role: "assistant",
          content: result.content,
          tool_calls: result.toolCalls,
        })

        for (const call of result.toolCalls) {
          ensureNotCancelled(signal)

          const decision = decideToolCall(call, corpusAttachments)

          if (!decision.ok) {
            messages.push({
              role: "tool",
              tool_call_id: call.id,
              content: decision.error,
            })
            continue
          }

          const outcome = await runSearchTool(decision, { limit: topK })

          searchedMaterial = true
          citable.push(...outcome.blocks)
          messages.push({
            role: "tool",
            tool_call_id: call.id,
            content: outcome.content,
          })
        }
      }

      await chats.appendTurn({
        role: "assistant",
        content: text,
        sources: buildSources(citable),
        consulted: materialConsulted(),
      })
    } catch (e) {
      // A cancellation is the user's own decision, not an error to report.
      if (!isAbort(e)) {
        const detail = messageOf(e, SEND_ERROR)

        // Capability is reported, never guessed: a rejection of the declared tool
        // before any answer reached us is the only case this layer can name.
        error.value =
          toolsAwaitingFirstResponse && isToolRejection(e)
            ? toolsRejectedMessage(model, detail)
            : detail
      }

      // Whatever arrived is text the user already read on screen, so letting it
      // vanish when the stream ends loses an answer instead of protecting them
      // from half of one. It is saved marked as interrupted, from a failure and
      // from their own cancel alike (findings R4-partial-content-dropped and
      // R4-002).
      await savePartialAnswer(chats, materialConsulted())
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
    consulted: boolean,
  ): Promise<void> {
    const partial = answer.value.trim()

    if (partial === "") return

    try {
      await chats.appendTurn({
        role: "assistant",
        content: `${partial}\n\n${INTERRUPTED_MARKER}`,
        sources: [],
        consulted,
      })
    } catch {
      // The chat store reports its own write failures in its own error, and a
      // failure here must not replace the reason the answer was cut.
    }
  }

  /** Abort the whole loop: the request in flight and every iteration after it. */
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

/**
 * The cut between iterations: an aborted signal ends the loop instead of
 * starting one more request or one more search.
 */
function ensureNotCancelled(signal: AbortSignal): void {
  if (!signal.aborted) return

  const cancelled = new Error(CANCELLED_ERROR)

  cancelled.name = "AbortError"

  throw cancelled
}

/**
 * Whether a provider error is a rejection of the declared `tools`.
 *
 * `400` and `422` mean the provider refused the request's content, and the only
 * content this layer adds for a chat with corpus material is the tool. Anything
 * else — auth, credit, rate limit, an unreachable host, a server fault — keeps
 * its own message, because reporting a capability problem for a network failure
 * would send the user to change a model that is fine.
 */
function isToolRejection(e: unknown): boolean {
  const status = (e as { status?: unknown })?.status

  return status === 400 || status === 422
}

/**
 * Said when the provider refuses the tool, naming the model the user can change.
 * It carries what the provider said, because capability is measured and not
 * assumed: there is no list of models that support tools in this plugin.
 */
function toolsRejectedMessage(model: string, detail: string): string {
  return (
    `El modelo «${model}» no puede buscar el material adjunto de este chat: ` +
    `el proveedor rechazó la herramienta de búsqueda. ` +
    `Elegí otro modelo para este chat en los ajustes. ${detail}`
  )
}
