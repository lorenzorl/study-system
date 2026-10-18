import { defineStore } from "pinia"
import { ref } from "vue"
import type { ChatMessage, ChatRole } from "@/entities/chat-message"

/** What the UI shows while `POST /api/chat` does not exist yet. */
const PENDING_ENDPOINT =
  "El chat todavía no está disponible: falta el endpoint POST /api/chat."

/**
 * The ask-chat interaction: append the user's question and hold the answer.
 *
 * The backend endpoint does not exist yet (see README.md), so `ask` records the
 * question locally and reports the gap instead of calling the network. The
 * promise-shaped signature is what the call site keeps when the request lands.
 */
export const useAskChat = defineStore("ask-chat", () => {
  const messages = ref<ChatMessage[]>([])
  const error = ref<string | null>(null)

  let sequence = 0

  function append(role: ChatRole, content: string): void {
    sequence += 1
    messages.value.push({
      id: `local-${Date.now()}-${sequence}`,
      role,
      content,
      created_at: new Date().toISOString(),
    })
  }

  async function ask(question: string): Promise<void> {
    const trimmed = question.trim()
    if (!trimmed) return

    error.value = null
    append("user", trimmed)
    error.value = PENDING_ENDPOINT
  }

  function clear(): void {
    messages.value = []
    error.value = null
  }

  return {
    messages,
    error,
    ask,
    clear,
  }
})
