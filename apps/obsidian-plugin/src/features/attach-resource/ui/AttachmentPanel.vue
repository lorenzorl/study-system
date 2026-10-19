<template>
  <section v-if="hasChat" class="attachment-panel">
    <div class="attachment-panel__header">
      <h3 class="attachment-panel__title">Material de este chat</h3>
      <button
        type="button"
        class="attachment-panel__toggle"
        :aria-expanded="open"
        @click="toggle"
      >
        {{ open ? "Cerrar" : "Adjuntar material" }}
      </button>
    </div>

    <p v-if="attachments.length === 0" class="attachment-panel__state">
      Sin material adjunto: el chat responde con el conocimiento del modelo.
    </p>

    <ul v-else class="attachment-panel__list">
      <li
        v-for="attachment in attachments"
        :key="keyOf(attachment)"
        class="attachment-panel__item"
      >
        <span class="attachment-panel__class">{{ classLabel(attachment) }}</span>
        <span class="attachment-panel__label">{{ labelOf(attachment) }}</span>
        <span
          v-if="attachment.kind === 'corpus'"
          class="attachment-panel__state-detail"
        >
          {{ stateOf(attachment.documentId) }}
        </span>
        <button
          type="button"
          class="attachment-panel__remove"
          :aria-label="`Quitar ${labelOf(attachment)}`"
          @click="detach(attachment)"
        >
          Quitar
        </button>
      </li>
    </ul>

    <div v-if="open" class="attachment-panel__pickers">
      <section class="attachment-panel__picker">
        <div class="attachment-panel__picker-header">
          <h4 class="attachment-panel__picker-title">Del corpus</h4>
          <button
            type="button"
            class="attachment-panel__refresh"
            :disabled="loadingCorpus"
            @click="loadCorpus"
          >
            {{ loadingCorpus ? "Cargando…" : "Actualizar" }}
          </button>
        </div>

        <p v-if="loadingCorpus" class="attachment-panel__state">Cargando el corpus…</p>
        <p v-else-if="corpus.length === 0" class="attachment-panel__state">
          El corpus está vacío, o el servidor no responde. Podés adjuntar archivos
          del vault igual: se mandan completos y no necesitan el servidor.
        </p>

        <ul v-else class="attachment-panel__candidates">
          <li
            v-for="document in corpus"
            :key="document.id"
            class="attachment-panel__candidate"
          >
            <button
              type="button"
              class="attachment-panel__pick"
              :disabled="isCorpusAttached(document.id)"
              @click="attachCorpus(document)"
            >
              <span class="attachment-panel__label">{{ document.title }}</span>
              <span class="attachment-panel__state-detail">
                {{ corpusStateLabel(document) }}
              </span>
            </button>
          </li>
        </ul>

        <p class="attachment-panel__hint">
          El modelo busca en estos documentos con una herramienta, solo cuando la
          pregunta lo necesita. Por eso el chat necesita el servidor encendido.
        </p>
      </section>

      <section class="attachment-panel__picker">
        <h4 class="attachment-panel__picker-title">Del vault</h4>

        <input
          v-model="filter"
          type="search"
          class="attachment-panel__filter"
          placeholder="Filtrar por ruta"
          aria-label="Filtrar archivos del vault"
        />

        <p v-if="matchingFiles.length === 0" class="attachment-panel__state">
          Ningún archivo coincide.
        </p>

        <ul v-else class="attachment-panel__candidates">
          <li
            v-for="path in matchingFiles"
            :key="path"
            class="attachment-panel__candidate"
          >
            <button
              type="button"
              class="attachment-panel__pick"
              :disabled="isVaultAttached(path)"
              @click="attachVault(path)"
            >
              <span class="attachment-panel__label">{{ path }}</span>
            </button>
          </li>
        </ul>

        <p class="attachment-panel__hint">
          Un archivo del vault viaja completo en cada mensaje, citado por su ruta.
          No necesita el servidor, y un archivo que no entra en el contexto se
          rechaza al adjuntarlo en vez de mandarse a medias.
        </p>
      </section>
    </div>

    <p v-if="error" class="attachment-panel__error" role="alert">{{ error }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from "vue"
import { storeToRefs } from "pinia"
import type { ChatAttachment } from "@/entities/chat"
import type { DocumentSummary } from "@/entities/document"
import { corpusStateLabel } from "../model/corpus-state"
import { useChatAttachments } from "../model/useChatAttachments"

/**
 * Choosing what the chat brings into context (T13).
 *
 * It sits above the composer because an attachment is a property of the next
 * message, not of the chat's settings: what a reader needs in sight while
 * writing is what material the question will have. The list is always visible
 * and the pickers are one click away, so an attachment cannot be forgotten
 * behind a collapsed panel.
 *
 * The corpus is listed only when the pickers are opened. A chat with no corpus
 * attachment never touches the server (S1), and asking for the corpus on mount
 * would turn "open a chat with the server down" into an error nobody asked for.
 */
defineProps<{ hasChat: boolean }>()

const store = useChatAttachments()
const { corpus, vaultFiles, attachments, loadingCorpus, error } = storeToRefs(store)

/** Enough to choose from without turning the panel into a browser. */
const MAX_CANDIDATES = 50

const open = ref(false)
const filter = ref("")

/** The vault candidates that match the filter, capped to a readable list. */
const matchingFiles = computed(() => {
  const needle = filter.value.trim().toLowerCase()
  const matches = needle
    ? vaultFiles.value.filter((path) => path.toLowerCase().includes(needle))
    : vaultFiles.value

  return matches.slice(0, MAX_CANDIDATES)
})

function toggle(): void {
  open.value = !open.value
  if (!open.value) return

  store.loadVaultFiles()
  // One listing per opening is enough: the button refreshes it on demand, and a
  // corpus that changes while the panel is open is not a case worth polling for.
  if (corpus.value.length === 0 && !loadingCorpus.value) {
    void store.loadCorpus()
  }
}

function attachCorpus(document: DocumentSummary): void {
  void store.attachCorpus(document)
}

function attachVault(path: string): void {
  void store.attachVault(path)
}

function detach(attachment: ChatAttachment): void {
  void store.detach(attachment)
}

function isCorpusAttached(documentId: string): boolean {
  return attachments.value.some(
    (current) => current.kind === "corpus" && current.documentId === documentId,
  )
}

function isVaultAttached(path: string): boolean {
  return attachments.value.some(
    (current) => current.kind === "vault" && current.path === path,
  )
}

function keyOf(attachment: ChatAttachment): string {
  return attachment.kind === "corpus"
    ? `corpus:${attachment.documentId}`
    : `vault:${attachment.path}`
}

function classLabel(attachment: ChatAttachment): string {
  return attachment.kind === "corpus" ? "Corpus" : "Vault"
}

function labelOf(attachment: ChatAttachment): string {
  return attachment.kind === "corpus" ? attachment.title : attachment.path
}

/**
 * What this chat's copy of a document is worth right now, by looking the
 * document up in the listing the pickers loaded. `corpusStateLabel` owns the
 * words; this only finds the document.
 */
function stateOf(documentId: string): string {
  return corpusStateLabel(
    corpus.value.find((current) => current.id === documentId),
  )
}
</script>

<style scoped>
.attachment-panel {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
}

.attachment-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.attachment-panel__title {
  margin: 0;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.attachment-panel__toggle,
.attachment-panel__refresh,
.attachment-panel__remove {
  min-height: 44px;
  padding: 0.25rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: transparent;
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.75rem;
  font-family: inherit;
}

.attachment-panel__toggle:hover,
.attachment-panel__refresh:hover:not(:disabled),
.attachment-panel__remove:hover {
  background: var(--background-modifier-hover);
}

.attachment-panel__refresh:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.attachment-panel__list,
.attachment-panel__candidates {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.attachment-panel__item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8rem;
}

.attachment-panel__class {
  flex: 0 0 auto;
  padding: 0.05rem 0.35rem;
  border-radius: 4px;
  background: var(--background-primary);
  color: var(--text-muted);
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.attachment-panel__label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--text-normal);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.attachment-panel__state,
.attachment-panel__hint,
.attachment-panel__state-detail {
  margin: 0;
  color: var(--text-muted);
  font-size: 0.75rem;
  line-height: 1.35;
}

.attachment-panel__state-detail {
  flex: 0 0 auto;
}

.attachment-panel__pickers {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 0.75rem;
}

.attachment-panel__picker {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 0;
}

.attachment-panel__picker-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.attachment-panel__picker-title {
  margin: 0;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-normal);
}

.attachment-panel__filter {
  min-height: 44px;
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.8rem;
  font-family: inherit;
}

.attachment-panel__filter:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.attachment-panel__pick {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  width: 100%;
  min-height: 44px;
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.8rem;
  font-family: inherit;
  text-align: left;
}

.attachment-panel__pick:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.attachment-panel__pick:disabled {
  opacity: 0.5;
  cursor: default;
}

.attachment-panel__error {
  margin: 0;
  color: var(--text-error);
  font-size: 0.75rem;
  line-height: 1.35;
}
</style>
