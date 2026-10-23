<template>
  <form class="topic-view__new-form" @submit.prevent="submit">
    <input
      v-model="title"
      type="text"
      class="topic-view__new-input"
      placeholder="Título del concepto (ej. Aggregates, Value Objects)"
      :disabled="submitting"
    />
    <div class="topic-view__new-actions">
      <button
        type="submit"
        class="topic-view__new-submit"
        :disabled="submitting || !title.trim()"
      >
        <span v-if="submitting" class="spinner"></span>
        <span v-else>Guardar</span>
      </button>
      <button
        type="button"
        class="topic-view__new-cancel"
        :disabled="submitting"
        @click="emit('cancel')"
      >
        Cancelar
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  modelValue: string
  submitting?: boolean
}>()

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void
  (e: "submit"): void
  (e: "cancel"): void
}>()

const title = computed({
  get: () => props.modelValue,
  set: (value: string) => emit("update:modelValue", value),
})

function submit() {
  emit("submit")
}
</script>

<style scoped>
.topic-view__new-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  margin-bottom: 0.25rem;
}

.topic-view__new-input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.topic-view__new-input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.topic-view__new-input:disabled {
  opacity: 0.5;
}

.topic-view__new-actions {
  display: flex;
  gap: 0.5rem;
}

.topic-view__new-submit {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  transition: opacity 0.15s ease;
}

.topic-view__new-submit:hover:not(:disabled) {
  opacity: 0.9;
}

.topic-view__new-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.topic-view__new-cancel {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.topic-view__new-cancel:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.topic-view__new-cancel:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.spinner {
  display: inline-block;
  width: 20px;
  height: 20px;
  border: 2px solid var(--background-modifier-border);
  border-top: 2px solid var(--interactive-accent);
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
