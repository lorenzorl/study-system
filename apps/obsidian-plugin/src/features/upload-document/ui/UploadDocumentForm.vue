<template>
  <form class="upload-document" @submit.prevent="submit">
    <label class="upload-document__label" for="upload-document-file">
      Agregar un PDF al corpus
    </label>
    <input
      id="upload-document-file"
      type="file"
      accept="application/pdf,.pdf"
      class="upload-document__input"
      :disabled="uploading"
      @change="onFileChange"
    />
    <button
      type="submit"
      class="upload-document__submit"
      :disabled="uploading || !selected"
    >
      Subir
    </button>

    <p v-if="uploading" class="upload-document__state">Subiendo...</p>
    <p v-else-if="error" class="upload-document__error">{{ error }}</p>
    <p v-else-if="uploaded" class="upload-document__state">
      Documento {{ uploaded.document_id }} aceptado ({{ uploaded.state }}).
    </p>
  </form>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia"
import { ref } from "vue"
import { useUploadDocument } from "../model/useUploadDocument"

const store = useUploadDocument()
const { uploading, error, uploaded } = storeToRefs(store)

const selected = ref<File | null>(null)

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  selected.value = input.files?.[0] ?? null
}

function submit() {
  if (!selected.value || uploading.value) return
  store.upload(selected.value)
}
</script>

<style scoped>
.upload-document {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

.upload-document__label {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.upload-document__input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.upload-document__submit {
  align-self: flex-start;
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

.upload-document__submit:hover:not(:disabled) {
  opacity: 0.9;
}

.upload-document__submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.upload-document__state {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.upload-document__error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-error);
}
</style>
