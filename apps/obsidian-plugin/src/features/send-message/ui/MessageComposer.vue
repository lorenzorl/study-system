<template>
  <form class="message-composer" @submit.prevent="onSubmit">
    <textarea
      v-model="draft"
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
      :disabled="!draft.trim()"
    >
      Enviar
    </button>
  </form>
</template>

<script setup lang="ts">
/**
 * The draft belongs to the caller.
 *
 * This component used to own the text and clear it on submit, which lost the
 * question whenever the send never happened: a blocked preflight asks the user to
 * start the server and press send again, and the text has to still be there. Only
 * the caller knows whether the send happened, so it owns the draft and this
 * component does nothing but report the intent to send it.
 */
const draft = defineModel<string>({ required: true })

defineProps<{ busy: boolean }>()

const emit = defineEmits<{
  (e: "submit", question: string): void
  (e: "cancel"): void
}>()

function onSubmit(): void {
  const text = draft.value.trim()
  if (!text) return

  emit("submit", text)
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
