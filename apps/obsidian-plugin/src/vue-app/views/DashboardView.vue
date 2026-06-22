<template>
  <div class="dashboard">
    <MetricsBar />
    <DailyReviewCard
      v-if="showCTA"
      @start="handleDailyReview"
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
          class="dashboard__sync-btn"
          @click="syncActiveNote"
          aria-label="Sincronizar nota"
          title="Sincronizar nota activa"
        >
          Sincronizar nota
        </button>
      </h2>

      <div v-if="topics.length === 0" class="dashboard__state empty-state">
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useRouter } from "vue-router"
import { useStudyStore } from "../stores/study-store"
import { useMetricsStore } from "../stores/metrics-store"
import { storeToRefs } from "pinia"
import { syncConcept, syncFlashcards } from "../services/api"
import { parseFlashcards } from "../services/markdown-parser"
import MetricsBar from "../components/MetricsBar.vue"
import DailyReviewCard from "../components/DailyReviewCard.vue"
import TopicCard from "../components/TopicCard.vue"

const router = useRouter()
const studyStore = useStudyStore()
const metricsStore = useMetricsStore()

const { topics, loading, error } = storeToRefs(studyStore)
const { dailyCardCount } = storeToRefs(metricsStore)

const showCTA = computed(() => dailyCardCount.value > 0)

onMounted(() => {
  studyStore.loadTopics()
})

function retry() {
  studyStore.clearError()
  studyStore.loadTopics()
}

function goToTopic(id: string) {
  studyStore.selectTopic(id)
  router.push({ name: "topic", params: { topicId: id } })
}

function handleDailyReview() {
  if (topics.value.length > 0 && topics.value[0].concepts.length > 0) {
    const topic = topics.value[0]
    const concept = topic.concepts[0]
    studyStore.selectTopic(topic.id)
    studyStore.selectConcept(concept.id)
    router.push({
      name: "flashcards",
      params: { topicId: topic.id, conceptId: concept.id },
    })
  }
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
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
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

.empty-state {
  color: var(--text-muted);
}
</style>
