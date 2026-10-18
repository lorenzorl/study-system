<template>
  <form class="message-composer" @submit.prevent="onSubmit">
    <textarea
      v-model="message"
      class="message-composer__input"
      placeholder="Preguntá algo sobre tu corpus"
      aria-label="Mensaje"
      rows="2"
      :disabled="busy"
      @keydown.enter="onEnter"
    ></textarea>

    <button
      v-if="busy"
      type="button"
      class="message-composer__stop"
      @click="emit('cancel')"
    >
      Detener
    </button>
    <button
      v-else
      type="submit"
      class="message-composer__submit"
      :disabled="!message.trim()"
    >
      Enviar
    </button>
  </form>
</template>

<script setup lang="ts">
import { ref } from "vue"

defineProps<{ busy: boolean }>()

const emit = defineEmits<{
  (e: "submit", question: string): void
  (e: "cancel"): void
}>()

const message = ref("")

function onSubmit(): void {
  const text = message.value.trim()
  if (!text) return

  emit("submit", text)
  message.value = ""
}

function onEnter(event: KeyboardEvent): void {
  // Shift+Enter keeps the textarea's default newline; a plain Enter submits.
  if (event.shiftKey) return

  event.preventDefault()
  onSubmit()
}
</script>

<style scoped>
.message-composer {
  display: flex;
  align-items: flex-end;
  gap: 0.5rem;
}

.message-composer__input {
  flex: 1;
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

.message-composer__input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.message-composer__input:disabled {
  opacity: 0.5;
}

.message-composer__submit,
.message-composer__stop {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  font-family: inherit;
  transition: opacity 0.15s ease;
}

.message-composer__submit:hover:not(:disabled),
.message-composer__stop:hover {
  opacity: 0.9;
}

.message-composer__submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
