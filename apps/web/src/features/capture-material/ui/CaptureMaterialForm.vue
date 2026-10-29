<template>
  <!-- Overlay: the same shape as the review modal, so closing behaves the same.
       The overlay itself is still hand-rolled: the `dialog` primitive's turn is T5. -->
  <div class="capture">
    <div
      ref="dialogEl"
      class="capture__dialog"
      role="dialog"
      aria-modal="true"
      aria-label="Agregar material"
    >
      <header class="capture__header">
        <h2 class="capture__title">Agregar material</h2>
        <span class="capture__hint">Esc para salir</span>
      </header>

      <form class="capture__form" @submit.prevent="submit">
        <!-- The source selector keeps its own markup: it is a segmented control
             bound to `aria-pressed`, not a form control, and T3 pinned that
             contract deliberately. Moving it onto the `tabs` primitive is its own
             unit; a migration does not quietly change what a reader hears. -->
        <div class="capture__tabs" role="group" aria-label="Origen">
          <button
            v-for="source in SOURCES"
            :key="source.value"
            type="button"
            class="capture__tab"
            :class="{ 'capture__tab--active': mode === source.value }"
            :aria-pressed="mode === source.value"
            :disabled="loading"
            @click="select(source.value)"
          >
            {{ source.label }}
          </button>
        </div>

        <div class="capture__field">
          <Label class="capture__label" for="capture-topic">Tema</Label>
          <Input
            id="capture-topic"
            v-model="topicName"
            type="text"
            placeholder="Tema (ej. DDD, Matemáticas)"
            :disabled="loading"
            class="min-h-[44px]"
          />
        </div>

        <!-- One source at a time: the mode decides which two or three fields show. -->
        <template v-if="mode === 'file'">
          <div class="capture__field">
            <Label class="capture__label" for="capture-file">Archivo PDF</Label>
            <Input
              id="capture-file"
              type="file"
              accept="application/pdf"
              :disabled="loading"
              class="min-h-[44px]"
              @change="onFileChange"
            />
          </div>
        </template>

        <template v-else-if="mode === 'text'">
          <div class="capture__field">
            <Label class="capture__label" for="capture-text-title">Título</Label>
            <Input
              id="capture-text-title"
              v-model="title"
              type="text"
              placeholder="Título del material"
              :disabled="loading"
              class="min-h-[44px]"
            />
          </div>
          <div class="capture__field">
            <Label class="capture__label" for="capture-text-body">Texto</Label>
            <Textarea
              id="capture-text-body"
              v-model="text"
              class="capture__textarea"
              rows="8"
              placeholder="Pegá el texto o el markdown acá"
              :disabled="loading"
            />
          </div>
        </template>

        <template v-else>
          <div class="capture__field">
            <Label class="capture__label" for="capture-url-title">
              Título (opcional)
            </Label>
            <Input
              id="capture-url-title"
              v-model="title"
              type="text"
              placeholder="Se usa el nombre del enlace si lo dejás vacío"
              :disabled="loading"
              class="min-h-[44px]"
            />
          </div>
          <div class="capture__field">
            <Label class="capture__label" for="capture-url-address">URL</Label>
            <Input
              id="capture-url-address"
              v-model="url"
              type="url"
              placeholder="https://…"
              :disabled="loading"
              class="min-h-[44px]"
            />
          </div>
        </template>

        <!-- The server's own message: it names the cause, so it is shown as-is. -->
        <p v-if="error" class="capture__error" role="alert">{{ error }}</p>

        <div class="capture__actions">
          <Button
            type="submit"
            class="min-h-[44px]"
            :disabled="loading || !canSubmit"
          >
            <span v-if="loading" class="capture__spinner" aria-hidden="true"></span>
            <span v-else>Agregar</span>
          </Button>
          <Button
            type="button"
            variant="outline"
            class="min-h-[44px]"
            :disabled="loading"
            @click="cancel"
          >
            Cancelar
          </Button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue"
import { storeToRefs } from "pinia"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { Textarea } from "@/shared/ui/textarea"
import { useCaptureMaterial } from "../model/useCaptureMaterial"
import type { CaptureMode, CaptureSource } from "../model/useCaptureMaterial"

const emit = defineEmits<{
  saved: []
  cancel: []
}>()

// The request belongs to the store; this component holds only the fields.
const store = useCaptureMaterial()
const { loading, error } = storeToRefs(store)

const SOURCES: { value: CaptureMode; label: string }[] = [
  { value: "file", label: "Archivo" },
  { value: "text", label: "Texto" },
  { value: "url", label: "URL" },
]

const mode = ref<CaptureMode>("file")
const topicName = ref("")
const title = ref("")
const text = ref("")
const url = ref("")
const file = ref<File | null>(null)
const dialogEl = ref<HTMLElement | null>(null)

const canSubmit = computed(() => {
  if (topicName.value.trim() === "") return false

  switch (mode.value) {
    case "file":
      return file.value !== null
    case "text":
      return title.value.trim() !== "" && text.value.trim() !== ""
    case "url":
      return url.value.trim() !== ""
  }
})

function select(value: CaptureMode) {
  mode.value = value
  // A refusal belongs to the mode that produced it; switching sources must not
  // leave "the URL answered text/html" sitting over the Archivo tab.
  store.clearError()
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  file.value = input.files?.[0] ?? null
}

function sourceOf(): CaptureSource | null {
  const topic = topicName.value.trim()
  if (topic === "") return null

  switch (mode.value) {
    case "file":
      return file.value === null
        ? null
        : { mode: "file", topicName: topic, file: file.value }
    case "text":
      return {
        mode: "text",
        topicName: topic,
        title: title.value.trim(),
        text: text.value,
      }
    case "url":
      return {
        mode: "url",
        topicName: topic,
        title: title.value.trim(),
        url: url.value.trim(),
      }
  }
}

async function submit() {
  const source = sourceOf()
  if (source === null || loading.value) return

  // The launcher owns the panel, so closing it is its decision: this reports the
  // capture and stays open if the server refused.
  if (await store.capture(source)) emit("saved")
}

function cancel() {
  // Nothing is in flight, so a half-typed capture is dropped with the panel.
  if (loading.value) return
  emit("cancel")
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== "Escape") return
  event.preventDefault()
  cancel()
}

onMounted(() => {
  window.addEventListener("keydown", onKeydown)

  // The first field is where a capture starts, so the panel opens ready to type.
  // It is found through the DOM, not through a component ref: `Input` is a
  // component now, and reaching into its internals for `$el` would couple this
  // file to the primitive's shape.
  const firstField = dialogEl.value?.querySelector<HTMLInputElement>("input")
  ;(firstField ?? dialogEl.value)?.focus()
})

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown)
})
</script>

<style scoped>
/*
 * The five fields and the two actions are `shared/ui` primitives now. What stays
 * here is the overlay, the panel's frame, the segmented source selector (its own
 * unit, see the comment in the template), the label rhythm, the refusal, the
 * spinner and the 8rem floor the textarea has always had.
 */
.capture {
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

.capture__dialog {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  width: 100%;
  max-width: 560px;
  max-height: 100%;
  overflow-y: auto;
  padding: 1rem 1.125rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 10px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
  box-shadow: 0 8px 24px var(--background-modifier-box-shadow, rgba(0, 0, 0, 0.3));
}

.capture__header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem;
}

.capture__title {
  margin: 0;
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--text-normal);
}

.capture__hint {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.capture__form {
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
}

.capture__tabs {
  display: flex;
  gap: 0.375rem;
}

.capture__tab {
  flex: 1 1 auto;
  min-height: 44px;
  padding: 0.375rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 999px;
  background: var(--background-secondary);
  color: var(--text-muted);
  cursor: pointer;
  font-family: inherit;
  font-size: 0.85rem;
  transition: color 0.15s ease, border-color 0.15s ease;
}

.capture__tab--active {
  border-color: var(--text-accent);
  background: var(--background-primary);
  color: var(--text-accent);
}

.capture__field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

/* Unlayered on purpose: the panel's own type scale wins over the primitive's
   utility classes, which live in a layer (`src/app/styles.css`). */
.capture__label {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.capture__textarea {
  min-height: 8rem;
  resize: vertical;
  line-height: 1.4;
}

.capture__error {
  margin: 0;
  padding: 0.5rem 0.625rem;
  border: 1px solid var(--text-error, #d64545);
  border-radius: 6px;
  color: var(--text-error, #d64545);
  font-size: 0.85rem;
}

.capture__actions {
  display: flex;
  gap: 0.5rem;
}

.capture__spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid var(--background-modifier-border);
  border-top-color: var(--background-primary);
  border-radius: 50%;
  animation: capture-spin 0.8s linear infinite;
}

@keyframes capture-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 520px) {
  .capture {
    padding: 0;
    align-items: stretch;
  }

  .capture__dialog {
    max-width: none;
    border: none;
    border-radius: 0;
  }
}
</style>
