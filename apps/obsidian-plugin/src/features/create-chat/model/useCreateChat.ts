import { defineStore } from "pinia"
import { ref } from "vue"
import { useChatsStore } from "@/entities/chat"
import type { ChatNoteSettings } from "@/entities/chat"
import { getSettings } from "@/shared/config"

const CREATE_CHAT_ERROR = "No se pudo crear el chat."

/**
 * A new chat starts from the global prompt and no overrides, so it inherits the
 * plugin's configuration until the user edits the chat's own panel. The store
 * error is surfaced as this feature's message; nothing is thrown at the UI.
 */
export const useCreateChat = defineStore("create-chat", () => {
  const creating = ref(false)
  const error = ref<string | null>(null)
  const chats = useChatsStore()

  async function create(title: string): Promise<void> {
    creating.value = true
    error.value = null

    const startingSettings: ChatNoteSettings = {
      model: null,
      temperature: null,
      topK: null,
    }

    try {
      await chats.createChat(title, getSettings().defaultChatPrompt, startingSettings)

      if (chats.error) {
        error.value = chats.error
      }
    } catch {
      error.value = CREATE_CHAT_ERROR
    } finally {
      creating.value = false
    }
  }

  function clearError(): void {
    error.value = null
  }

  return {
    creating,
    error,
    create,
    clearError,
  }
})
