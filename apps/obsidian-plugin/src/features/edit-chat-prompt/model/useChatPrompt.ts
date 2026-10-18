import { computed, ref } from "vue"
import { useChatsStore } from "@/entities/chat"
import type { ChatNote } from "@/entities/chat"

/**
 * The draft of the current chat's own prompt.
 *
 * `loadFrom` seeds the draft from a note the caller passed, `dirty` compares it
 * with the note that is open now, and `save` persists through the chat store's
 * prompt action. The citation rules are not part of this draft: they live in
 * the code and the prompt is inserted inside them.
 */
export function useChatPrompt() {
  const chats = useChatsStore()
  const draft = ref("")

  const dirty = computed(() => draft.value !== currentPrompt())

  function loadFrom(note: ChatNote): void {
    draft.value = note.chat.prompt
  }

  async function save(): Promise<void> {
    await chats.savePrompt(draft.value)
  }

  function reset(): void {
    draft.value = currentPrompt()
  }

  function currentPrompt(): string {
    return chats.note?.chat.prompt ?? ""
  }

  return {
    draft,
    dirty,
    loadFrom,
    save,
    reset,
  }
}
