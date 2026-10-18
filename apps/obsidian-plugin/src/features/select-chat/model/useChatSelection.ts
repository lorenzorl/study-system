import { defineStore } from "pinia"
import { useChatsStore } from "@/entities/chat"

/**
 * Which chat the user is working on. Selecting the chat that is already open is
 * a no-op, so a click on the current row does not re-read the note.
 */
export const useChatSelection = defineStore("chat-selection", () => {
  const chats = useChatsStore()

  function select(path: string): void {
    if (chats.currentPath === path) return

    chats.selectChat(path)
  }

  return { select }
})
