<template>
  <!-- Overlay: the same contract as the capture panel — one overlay at a time,
       Escape closes it, and the first field is ready to type. -->
  <div v-if="panelOpen" class="sync-note">
    <div
      ref="dialogEl"
      class="sync-note__dialog"
      role="dialog"
      aria-modal="true"
      aria-label="Sincronizar nota"
    >
      <header class="sync-note__header">
        <h2 class="sync-note__title">Sincronizar nota</h2>
        <span class="sync-note__hint">Esc para salir</span>
      </header>

      <form class="sync-note__form" @submit.prevent="submit">
        <label class="sync-note__field">
          <span class="sync-note__label">Tema</span>
          <input
            ref="topicEl"
            v-model="topicName"
            class="sync-note__input"
            type="text"
            placeholder="Tema (ej. DDD, Matemáticas)"
            :disabled="loading"
          />
        </label>

        <label class="sync-note__field">
          <span class="sync-note__label">Concepto</span>
          <input
            v-model="conceptTitle"
            class="sync-note__input"
            type="text"
            placeholder="Concepto (ej. Agregados)"
            :disabled="loading"
          />
        </label>

        <label class="sync-note__field">
          <span class="sync-note__label">Markdown</span>
          <!-- The format is taught here, not only in the refusal: a note without
               a `::` pair is the one input the flow rejects outright. -->
          <span class="sync-note__format">
            Una tarjeta por línea, con el formato pregunta::respuesta
          </span>
          <textarea
            v-model="markdown"
            class="sync-note__input sync-note__textarea"
            rows="10"
            placeholder="¿Qué es un agregado?::Un conjunto consistente de cambios."
            :disabled="loading"
          ></textarea>
        </label>

        <!-- The store's own message: it names the cause, so it is shown as-is. -->
        <p v-if="error" class="sync-note__error" role="alert">{{ error }}</p>

        <div class="sync-note__actions">
          <button
            type="submit"
            class="sync-note__submit"
            :disabled="loading || !canSubmit"
          >
            <span v-if="loading" class="sync-note__spinner" aria-hidden="true"></span>
            <span v-else>Sincronizar</span>
          </button>
          <button
            type="button"
            class="sync-note__cancel"
            :disabled="loading"
            @click="cancel"
          >
            Cancelar
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue"
import { useNoteForm } from "../model/useNoteForm"
import type { SyncNoteResult } from "../model/useSyncNote"

const emit = defineEmits<{
  synced: [result: SyncNoteResult]
}>()

// The request belongs to the store and the form's rules to `useNoteForm`; this
// component owns only the overlay and the hand-off.
const {
  topicName,
  conceptTitle,
  markdown,
  canSubmit,
  loading,
  error,
  panelOpen,
  closePanel,
  submit: runSync,
  reset,
} = useNoteForm()

const dialogEl = ref<HTMLElement | null>(null)
const topicEl = ref<HTMLInputElement | null>(null)

async function submit() {
  const result = await runSync()

  // A refusal keeps the panel open with its fields: the error is already
  // rendered, and the reader's own text is what they have to fix.
  if (result === null) return

  closePanel()
  reset()
  emit("synced", result)
}

function cancel() {
  // Nothing is in flight, so a half-pasted note is dropped with the panel.
  if (!panelOpen.value || loading.value) return

  closePanel()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== "Escape") return
  event.preventDefault()
  cancel()
}

// The first field is where a paste starts, so the panel opens ready to type.
watch(panelOpen, async (isOpen) => {
  if (!isOpen) return
  await nextTick()
  ;(topicEl.value ?? dialogEl.value)?.focus()
})

onMounted(() => {
  window.addEventListener("keydown", onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown)
})
</script>

<style scoped>
.sync-note {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  /* Neutral scrim: the app has no variable for modal dimming. */
  background: rgba(0, 0, 0, 0.5);
}

.sync-note__dialog {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  width: 100%;
  max-width: 34rem;
  max-height: 90vh;
  overflow-y: auto;
  padding: 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
  color: var(--text-normal);
}

.sync-note__header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem;
}

.sync-note__title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
}

.sync-note__hint {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.sync-note__form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.sync-note__field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.sync-note__label {
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--text-muted);
}

.sync-note__format {
  font-size: 0.75rem;
  color: var(--text-faint, var(--text-muted));
}

.sync-note__input {
  min-height: 44px;
  padding: 0.5rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  font-family: inherit;
  font-size: 0.85rem;
}

.sync-note__textarea {
  min-height: 10rem;
  resize: vertical;
}

.sync-note__error {
  margin: 0;
  padding: 0.5rem;
  border-radius: 6px;
  background: var(--background-modifier-error, rgba(255, 0, 0, 0.1));
  color: var(--text-error);
  font-size: 0.85rem;
}

.sync-note__actions {
  display: flex;
  gap: 0.5rem;
}

.sync-note__submit,
.sync-note__cancel {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
}

.sync-note__submit {
  border: 1px solid var(--interactive-accent);
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
}

.sync-note__submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.sync-note__cancel {
  border: 1px solid var(--background-modifier-border);
  background: var(--background-secondary);
  color: var(--text-normal);
}

.sync-note__spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: sync-note-spin 0.6s linear infinite;
}

@keyframes sync-note-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
