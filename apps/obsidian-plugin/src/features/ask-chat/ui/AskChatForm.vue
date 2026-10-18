<template>
  <form class="ask-chat__form" @submit.prevent="submit">
    <input
      v-model="question"
      type="text"
      class="ask-chat__input"
      placeholder="Preguntá algo sobre tu corpus"
      aria-label="Pregunta"
    />
    <button
      type="submit"
      class="ask-chat__submit"
      :disabled="!question.trim()"
    >
      Preguntar
    </button>
  </form>
</template>

<script setup lang="ts">
import { ref } from "vue"

const emit = defineEmits<{
  (e: "submit", question: string): void
}>()

const question = ref("")

function submit() {
  const text = question.value.trim()
  if (!text) return
  emit("submit", text)
  question.value = ""
}
</script>

<style scoped>
.ask-chat__form {
  display: flex;
  gap: 0.5rem;
}

.ask-chat__input {
  flex: 1;
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.ask-chat__input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.ask-chat__submit {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.ask-chat__submit:hover:not(:disabled) {
  opacity: 0.9;
}

.ask-chat__submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
