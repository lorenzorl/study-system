<template>
  <section class="chat-prompt">
    <label class="chat-prompt__label" for="chat-prompt-textarea">
      Prompt de este chat
    </label>
    <p class="chat-prompt__hint">
      Se guarda en la nota de este chat y solo se aplica a este chat.
    </p>
    <textarea
      id="chat-prompt-textarea"
      v-model="draft"
      class="chat-prompt__textarea"
      rows="6"
    ></textarea>
    <p class="chat-prompt__note">
      Las reglas de citas son fijas: tu prompt se agrega dentro de esas reglas.
    </p>

    <div class="chat-prompt__actions">
      <button
        type="button"
        class="chat-prompt__save"
        :disabled="!dirty"
        @click="onSave"
      >
        Guardar
      </button>
      <button type="button" class="chat-prompt__discard" @click="onDiscard">
        Descartar
      </button>
      <span v-if="saved" class="chat-prompt__saved" role="status">Guardado</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue"
import { storeToRefs } from "pinia"
import { useChatsStore } from "@/entities/chat"
import { useChatPrompt } from "../model/useChatPrompt"

const chats = useChatsStore()
const { note } = storeToRefs(chats)

const { draft, dirty, loadFrom, save, reset } = useChatPrompt()

const saved = ref(false)
let savedTimer: ReturnType<typeof setTimeout> | null = null

// The draft follows whichever chat is open; saving the same prompt is a no-op.
watch(
  note,
  (value) => {
    if (value) loadFrom(value)
  },
  { immediate: true },
)

async function onSave(): Promise<void> {
  await save()
  saved.value = true

  if (savedTimer !== null) clearTimeout(savedTimer)
  savedTimer = setTimeout(() => {
    saved.value = false
  }, 2000)
}

function onDiscard(): void {
  reset()
}

onBeforeUnmount(() => {
  if (savedTimer !== null) clearTimeout(savedTimer)
})
</script>

<style scoped>
.chat-prompt {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.chat-prompt__label {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-normal);
}

.chat-prompt__hint,
.chat-prompt__note {
  margin: 0;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.chat-prompt__textarea {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
  resize: vertical;
}

.chat-prompt__textarea:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.chat-prompt__actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.chat-prompt__save,
.chat-prompt__discard {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  font-family: inherit;
  transition: opacity 0.15s ease;
}

.chat-prompt__save {
  border: none;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
}

.chat-prompt__discard {
  border: 1px solid var(--background-modifier-border);
  background: transparent;
  color: var(--text-normal);
}

.chat-prompt__save:hover:not(:disabled),
.chat-prompt__discard:hover:not(:disabled) {
  opacity: 0.9;
}

.chat-prompt__save:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.chat-prompt__saved {
  font-size: 0.75rem;
  color: var(--text-muted);
}
</style>
