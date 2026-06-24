<template>
  <div class="concept-view">
    <div v-if="loading" class="concept-view__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <div v-else-if="error" class="concept-view__state error-state">
      <p>{{ error }}</p>
      <button class="concept-view__retry-btn" @click="retry">Reintentar</button>
    </div>

    <div v-else-if="!concept" class="concept-view__state empty-state">
      <p>Concepto no encontrado.</p>
      <button
        class="concept-view__back-btn"
        @click="router.push({ name: 'topic', params: { topicId: props.topicId } })"
      >
        ← Volver al Tema
      </button>
    </div>

    <div v-else class="concept-view__content">
      <div class="concept-view__header">
        <h2 class="concept-view__title">{{ concept.title }}</h2>
        <p class="concept-view__path">{{ concept.file_path }}</p>
      </div>

      <div class="concept-view__actions">
        <button
          class="concept-view__action"
          @click="navigateTo('flashcards')"
          :disabled="flashcardCount === 0"
        >
          <span class="concept-view__action-icon">🃏</span>
          <span class="concept-view__action-text">
            Tarjetas
            <small class="concept-view__action-meta">
              {{ flashcardCount }}
              tarjeta{{ flashcardCount === 1 ? '' : 's' }}
            </small>
          </span>
        </button>

        <button class="concept-view__action" @click="navigateTo('feynman')">
          <span class="concept-view__action-icon">📝</span>
          <span class="concept-view__action-text">
            Técnica Feynman
            <small class="concept-view__action-meta">
              Explícalo con tus propias palabras
            </small>
          </span>
        </button>
      </div>

      <div
        v-if="flashcardCount === 0 && !loading && !error"
        class="concept-view__state empty-state"
      >
        <p>Este concepto no tiene tarjetas. Sincronizá la nota para generarlas.</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useRouter } from "vue-router"
import { useStudyStore } from "../stores/study-store"
import { storeToRefs } from "pinia"

const props = defineProps<{
  topicId: string
  conceptId: string
}>()

const router = useRouter()
const studyStore = useStudyStore()
const { currentConcept: concept, loading, error } = storeToRefs(studyStore)

const flashcardCount = computed(() => {
  return studyStore.getCachedFlashcards(props.conceptId).length
})

onMounted(() => {
  studyStore.selectTopic(props.topicId)
  studyStore.selectConcept(props.conceptId)
})

function retry() {
  studyStore.clearError()
  studyStore.loadTopics()
}

function navigateTo(module: "flashcards" | "feynman") {
  router.push({
    name: module,
    params: { topicId: props.topicId, conceptId: props.conceptId },
  })
}
</script>

<style scoped>
.concept-view {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.concept-view__content {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.concept-view__header {
  margin-bottom: 0.25rem;
}

.concept-view__title {
  margin: 0 0 0.25rem;
  font-size: 1.1rem;
  font-weight: 600;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.concept-view__path {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
  font-family: monospace;
}

.concept-view__actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.concept-view__action {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  min-height: 56px;
  padding: 0.75rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.concept-view__action:hover:not(:disabled) {
  border-color: var(--interactive-accent);
  background: var(--background-modifier-hover);
}

.concept-view__action:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.concept-view__action-icon {
  font-size: 1.3rem;
  min-width: 44px;
  text-align: center;
}

.concept-view__action-text {
  display: flex;
  flex-direction: column;
  font-size: 0.9rem;
  font-weight: 500;
}

.concept-view__action-meta {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-weight: 400;
}

.concept-view__state {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  min-height: 44px;
}

.concept-view__back-btn,
.concept-view__retry-btn {
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

.concept-view__back-btn:hover,
.concept-view__retry-btn:hover {
  background: var(--background-modifier-hover);
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
</style>
