<template>
  <div class="pending-resources">
    <div class="pending-resources__header">
      <h2 class="pending-resources__title">Organizar Recursos</h2>
      <span v-if="!loading && pending.length > 0" class="pending-resources__count">
        {{ pending.length }} recurso{{ pending.length === 1 ? '' : 's' }} pendiente{{ pending.length === 1 ? '' : 's' }}
      </span>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="pending-resources__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <!-- Error -->
    <div v-else-if="error" class="pending-resources__state error-state">
      <p>{{ error }}</p>
      <button class="pending-resources__retry-btn" @click="retry">Reintentar</button>
    </div>

    <!-- Empty -->
    <div v-else-if="pending.length === 0" class="pending-resources__state">
      <p>No hay recursos pendientes para organizar.</p>
    </div>

    <!-- Cards -->
    <div v-else class="pending-resources__list">
      <div
        v-for="doc in pending"
        :key="doc.id"
        class="pending-resources__card"
      >
        <div class="pending-resources__card-header">
          <span class="pending-resources__card-name">{{ doc.name }}</span>
        </div>

        <div class="pending-resources__card-form">
          <label :for="'topic-' + doc.id" class="pending-resources__label">Tema</label>
          <select
            :id="'topic-' + doc.id"
            v-model="selectedTopics[doc.id]"
            class="pending-resources__select"
          >
            <option value="" disabled>Seleccionar tema</option>
            <option
              v-for="topic in topics"
              :key="topic.id"
              :value="topic.id"
            >
              {{ topic.name }}
            </option>
          </select>

          <template v-if="selectedTopics[doc.id]">
            <label :for="'concept-' + doc.id" class="pending-resources__label">Concepto (opcional)</label>
            <select
              :id="'concept-' + doc.id"
              v-model="selectedConcepts[doc.id]"
              class="pending-resources__select"
            >
              <option value="">Sin concepto</option>
              <option
                v-for="concept in conceptsForTopic(selectedTopics[doc.id])"
                :key="concept.id"
                :value="concept.id"
              >
                {{ concept.title }}
              </option>
            </select>
          </template>
        </div>

        <div class="pending-resources__card-actions">
          <span v-if="saveErrors[doc.id]" class="pending-resources__card-error">
            {{ saveErrors[doc.id] }}
          </span>
          <button
            class="pending-resources__save-btn"
            :disabled="!selectedTopics[doc.id] || saving[doc.id]"
            @click="handleSave(doc)"
          >
            <span v-if="saving[doc.id]" class="spinner"></span>
            <span v-else>Guardar</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { storeToRefs } from "pinia"
import { useStudyStore } from "../stores/study-store"
import { fetchPendingDifyDocuments, syncResource } from "../services/api"
import type { PendingDifyDocument, ConceptSummary } from "../../types"

const studyStore = useStudyStore()
const { topics } = storeToRefs(studyStore)

const pending = ref<PendingDifyDocument[]>([])
const loading = ref(true)
const error = ref<string | null>(null)

const selectedTopics = ref<Record<string, string>>({})
const selectedConcepts = ref<Record<string, string>>({})
const saving = ref<Record<string, boolean>>({})
const saveErrors = ref<Record<string, string>>({})

onMounted(() => {
  loadData()
})

async function loadData() {
  loading.value = true
  error.value = null

  try {
    await Promise.all([
      studyStore.loadTopics(),
      fetchPending(),
    ])
  } catch {
    // Individual fetch errors are handled in their own try/catch
  } finally {
    loading.value = false
  }
}

async function fetchPending() {
  try {
    pending.value = await fetchPendingDifyDocuments()
  } catch (e) {
    if (e instanceof Error) {
      error.value = e.message
    } else {
      error.value = "No se pudo conectar al servidor."
    }
  }
}

function retry() {
  loadData()
}

function conceptsForTopic(topicId: string): ConceptSummary[] {
  const topic = topics.value.find((t) => t.id === topicId)
  return topic?.concepts ?? []
}

function getTopicName(topicId: string): string {
  const topic = topics.value.find((t) => t.id === topicId)
  return topic?.name ?? ""
}

async function handleSave(doc: PendingDifyDocument) {
  const topicId = selectedTopics.value[doc.id]
  if (!topicId) return

  saving.value = { ...saving.value, [doc.id]: true }
  saveErrors.value = { ...saveErrors.value, [doc.id]: "" }

  try {
    await syncResource({
      topic_name: getTopicName(topicId),
      resource_title: doc.name,
      type: "book",
      source_uri: "Dify UI",
      dify_document_id: doc.id,
    })

    // Remove card on success
    const idx = pending.value.findIndex((p) => p.id === doc.id)
    if (idx !== -1) {
      pending.value.splice(idx, 1)
    }
  } catch (e) {
    if (e instanceof Error) {
      saveErrors.value = { ...saveErrors.value, [doc.id]: e.message }
    } else {
      saveErrors.value = { ...saveErrors.value, [doc.id]: "Error al guardar el recurso." }
    }
  } finally {
    saving.value = { ...saving.value, [doc.id]: false }
  }
}
</script>

<style scoped>
.pending-resources {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.pending-resources__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 44px;
}

.pending-resources__title {
  margin: 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.pending-resources__count {
  font-size: 0.8rem;
  color: var(--text-muted);
  font-weight: 500;
}

.pending-resources__state {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  min-height: 44px;
}

.loading-state {
  color: var(--text-muted);
}

.error-state {
  color: var(--text-error);
}

.pending-resources__retry-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.pending-resources__retry-btn:hover {
  background: var(--background-modifier-hover);
}

.pending-resources__list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.pending-resources__card {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

.pending-resources__card-header {
  min-height: 44px;
  display: flex;
  align-items: center;
}

.pending-resources__card-name {
  font-size: 0.9rem;
  font-weight: 500;
  color: var(--text-normal);
  word-break: break-word;
}

.pending-resources__card-form {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.pending-resources__label {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-top: 0.25rem;
}

.pending-resources__select {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
  cursor: pointer;
}

.pending-resources__select:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.pending-resources__card-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
  min-height: 44px;
  margin-top: 0.25rem;
}

.pending-resources__card-error {
  font-size: 0.75rem;
  color: var(--text-error);
  flex: 1;
  text-align: left;
}

.pending-resources__save-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--interactive-accent);
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

.pending-resources__save-btn:hover:not(:disabled) {
  opacity: 0.9;
}

.pending-resources__save-btn:disabled {
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
