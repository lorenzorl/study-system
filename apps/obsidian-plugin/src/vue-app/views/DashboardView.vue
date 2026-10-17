<template>
  <div class="dashboard">
    <MetricsBar />
    <DailyReviewCard
      v-if="showCTA"
      :due-count="dueCards.length"
      @click="goToDueCards"
    />

    <div v-if="loading" class="dashboard__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <div v-else-if="error" class="dashboard__state error-state">
      <p>{{ error }}</p>
      <button class="dashboard__retry-btn" @click="retry">Reintentar</button>
    </div>

    <section v-else class="dashboard__section">
      <h2 class="dashboard__section-title">
        <span>Temas</span>
        <button
          class="dashboard__add-btn"
          @click="showNewTopicForm = !showNewTopicForm"
          aria-label="Agregar tema"
          title="Agregar tema"
        >
          +
        </button>
        <button
          class="dashboard__sync-btn"
          @click="syncActiveNote"
          aria-label="Sincronizar nota"
          title="Sincronizar nota activa"
        >
          Sincronizar nota
        </button>
      </h2>

      <form
        v-if="showNewTopicForm"
        class="dashboard__new-form"
        @submit.prevent="handleCreateTopic"
      >
        <input
          ref="newTopicInputRef"
          v-model="newTopicName"
          type="text"
          class="dashboard__new-input"
          placeholder="Nombre del tema (ej. DDD, Matemáticas)"
          :disabled="formSubmitting"
        />
        <div class="dashboard__new-actions">
          <button
            type="submit"
            class="dashboard__new-submit"
            :disabled="formSubmitting || !newTopicName.trim()"
          >
            <span v-if="formSubmitting" class="spinner"></span>
            <span v-else>Guardar</span>
          </button>
          <button
            type="button"
            class="dashboard__new-cancel"
            :disabled="formSubmitting"
            @click="cancelNewTopic"
          >
            Cancelar
          </button>
        </div>
      </form>

      <div v-if="topics.length === 0" class="dashboard__state">
        <p>No hay temas todavía. Sincronizá una nota para crear el primero.</p>
      </div>

      <div v-else class="dashboard__grid">
        <TopicCard
          v-for="topic in topics"
          :key="topic.id"
          :topic="topic"
          @select="goToTopic"
        />
      </div>
    </section>

    <details class="dashboard__settings">
      <summary class="dashboard__settings-toggle">⚙️ Configuración</summary>
      <form class="dashboard__settings-form" @submit.prevent="saveApiBase">
        <label class="dashboard__settings-label" for="api-base-input">
          URL del servidor Go
        </label>
        <input
          id="api-base-input"
          v-model="apiBaseUrl"
          type="text"
          class="dashboard__new-input"
          placeholder="http://192.168.1.50:8080"
        />
        <div class="dashboard__new-actions">
          <button type="submit" class="dashboard__new-submit">
            Guardar
          </button>
          <button
            type="button"
            class="dashboard__new-cancel"
            @click="resetApiBaseUrl"
          >
            Restablecer
          </button>
        </div>
      </form>
    </details>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { useStudyStore } from "../stores/study-store"
import { useReviewStore } from "../stores/review-store"
import { useMetricsStore } from "../stores/metrics-store"
import { storeToRefs } from "pinia"
import { syncConcept, syncFlashcards, setApiBase, resetApiBase, getApiBase } from "../services/api"
import { parseFlashcards } from "../services/markdown-parser"
import MetricsBar from "../components/MetricsBar.vue"
import DailyReviewCard from "../components/DailyReviewCard.vue"
import TopicCard from "../components/TopicCard.vue"

const router = useRouter()
const studyStore = useStudyStore()
const reviewStore = useReviewStore()
const metricsStore = useMetricsStore()

const { topics, loading, error } = storeToRefs(studyStore)
const { dueCards, loading: dueLoading } = storeToRefs(reviewStore)

const showCTA = computed(() => dueCards.value.length > 0)

const showNewTopicForm = ref(false)
const newTopicName = ref("")
const newTopicInputRef = ref<HTMLInputElement | null>(null)
const formSubmitting = ref(false)

const apiBaseUrl = ref(getApiBase())

function saveApiBase() {
  const url = apiBaseUrl.value.trim()
  if (url) {
    setApiBase(url)
  }
}

function resetApiBaseUrl() {
  resetApiBase()
  apiBaseUrl.value = getApiBase()
}

onMounted(() => {
  studyStore.loadTopics()
  reviewStore.fetchDueCards()
})

function retry() {
  studyStore.clearError()
  studyStore.loadTopics()
}

function goToTopic(id: string) {
  studyStore.selectTopic(id)
  router.push({ name: "topic", params: { topicId: id } })
}

function goToDueCards() {
  router.push({ name: "due-cards" })
}

async function handleCreateTopic() {
  const name = newTopicName.value.trim()
  if (!name) return

  formSubmitting.value = true
  studyStore.clearError()

  try {
    await studyStore.createTopic(name)
    showNewTopicForm.value = false
    newTopicName.value = ""
  } finally {
    formSubmitting.value = false
  }
}

function cancelNewTopic() {
  showNewTopicForm.value = false
  newTopicName.value = ""
  studyStore.clearError()
}

async function syncActiveNote() {
  // Dynamic import to avoid making obsidian a hard dependency in the browser dev mode
  try {
    const { useObsidian } = await import("../composables/useObsidian")

    const obsidian = useObsidian()
    const filePath = obsidian.getActiveFilePath()

    if (!filePath) {
      studyStore.error = "No hay nota activa. Abrí una nota primero."
      return
    }

    const topicName = obsidian.getTopicNameFromPath(filePath)
    const conceptTitle = obsidian.getConceptTitleFromPath(filePath)

    let fileContent: string
    try {
      fileContent = await obsidian.getActiveFileContent()
    } catch {
      studyStore.error = "No hay nota activa. Abrí una nota primero."
      return
    }

    await studyStore.syncCurrentNote(topicName, conceptTitle, filePath, fileContent)

    // Navigate to flashcards if sync succeeded
    if (!studyStore.error && studyStore.currentConceptId) {
      router.push({
        name: "flashcards",
        params: {
          topicId: studyStore.currentTopicId,
          conceptId: studyStore.currentConceptId,
        },
      })
    }
  } catch {
    // useObsidian not available in browser dev — show a friendly message
    studyStore.error =
      "La sincronización solo funciona dentro de Obsidian. Abrí el plugin en Obsidian para sincronizar notas."
  }
}
</script>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.dashboard__section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dashboard__section-title {
  margin: 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.dashboard__grid {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dashboard__state {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  min-height: 44px;
}

.dashboard__retry-btn {
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

.dashboard__retry-btn:hover {
  background: var(--background-modifier-hover);
}

.dashboard__sync-btn {
  margin-left: auto;
  min-height: 44px;
  padding: 0.25rem 0.75rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.8rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.dashboard__sync-btn:hover {
  opacity: 0.9;
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

.loading-state {
  color: var(--text-muted);
}

.error-state {
  color: var(--text-error);
}

.dashboard__add-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.25rem 0.5rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 1.1rem;
  font-weight: 600;
  line-height: 1;
  transition: background-color 0.15s ease;
}

.dashboard__add-btn:hover {
  background: var(--background-modifier-hover);
}

.dashboard__new-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

.dashboard__new-input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.dashboard__new-input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.dashboard__new-input:disabled {
  opacity: 0.5;
}

.dashboard__new-actions {
  display: flex;
  gap: 0.5rem;
}

.dashboard__new-submit {
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

.dashboard__new-submit:hover:not(:disabled) {
  opacity: 0.9;
}

.dashboard__new-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.dashboard__new-cancel {
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

.dashboard__new-cancel:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.dashboard__new-cancel:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.dashboard__settings {
  margin-top: 0.75rem;
  padding: 0.5rem 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

.dashboard__settings-toggle {
  min-height: 44px;
  display: flex;
  align-items: center;
  cursor: pointer;
  font-size: 0.8rem;
  color: var(--text-muted);
  user-select: none;
}

.dashboard__settings-toggle::marker {
  font-size: 0.8rem;
}

.dashboard__settings-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 0.5rem;
}

.dashboard__settings-label {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-bottom: -0.25rem;
}
</style>
