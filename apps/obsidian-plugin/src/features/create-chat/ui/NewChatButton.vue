<template>
  <div class="new-chat">
    <button
      v-if="!open"
      type="button"
      class="new-chat__button"
      @click="openForm"
    >
      Nuevo chat
    </button>

    <form v-else class="new-chat__form" @submit.prevent="submit">
      <input
        ref="inputEl"
        v-model="title"
        type="text"
        class="new-chat__input"
        placeholder="Chat"
        aria-label="Título del chat"
        :disabled="creating"
        @keydown.esc.prevent="cancel"
      />
      <div class="new-chat__actions">
        <button type="submit" class="new-chat__create" :disabled="creating">
          Crear
        </button>
        <button
          type="button"
          class="new-chat__cancel"
          :disabled="creating"
          @click="cancel"
        >
          Cancelar
        </button>
      </div>
    </form>

    <p v-if="error" class="new-chat__error">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref } from "vue"
import { storeToRefs } from "pinia"
import { useCreateChat } from "../model/useCreateChat"

const DEFAULT_TITLE = "Chat"

const store = useCreateChat()
const { creating, error } = storeToRefs(store)

const open = ref(false)
const title = ref(DEFAULT_TITLE)
const inputEl = ref<HTMLInputElement | null>(null)

function openForm(): void {
  store.clearError()
  title.value = DEFAULT_TITLE
  open.value = true
  void nextTick(() => inputEl.value?.focus())
}

async function submit(): Promise<void> {
  const text = title.value.trim()

  await store.create(text === "" ? DEFAULT_TITLE : text)

  if (error.value === null) {
    open.value = false
  }
}

function cancel(): void {
  store.clearError()
  open.value = false
}
</script>

<style scoped>
.new-chat {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.new-chat__button,
.new-chat__create,
.new-chat__cancel {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  font-family: inherit;
  transition: opacity 0.15s ease;
}

.new-chat__button,
.new-chat__create {
  border: none;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
}

.new-chat__cancel {
  border: 1px solid var(--background-modifier-border);
  background: transparent;
  color: var(--text-normal);
}

.new-chat__button:hover,
.new-chat__create:hover:not(:disabled),
.new-chat__cancel:hover:not(:disabled) {
  opacity: 0.9;
}

.new-chat__button:disabled,
.new-chat__create:disabled,
.new-chat__cancel:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.new-chat__form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.new-chat__input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.new-chat__input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.new-chat__input:disabled {
  opacity: 0.5;
}

.new-chat__actions {
  display: flex;
  gap: 0.5rem;
}

.new-chat__error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-error);
}
</style>
