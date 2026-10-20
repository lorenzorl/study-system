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
        <SyncNoteButton @sync="syncActiveNote" />
      </h2>

      <CreateTopicForm
        v-if="showNewTopicForm"
        v-model="newTopicName"
        :submitting="creatingTopic"
        @submit="handleCreateTopic"
        @cancel="cancelNewTopic"
      />

      <div v-if="topics.length === 0" class="dashboard__state">
        <p>No hay temas todavía. Sincronizá una nota para crear el primero.</p>
      </div>

      <TopicList v-else :topics="topics" @select="goToTopic" />
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { storeToRefs } from "pinia"
import { useTopicsStore } from "@/entities/topic"
import { useTopicSelection } from "@/features/select-topic"
import { useConceptSelection } from "@/features/select-concept"
import { CreateTopicForm, useCreateTopic } from "@/features/create-topic"
import { SyncNoteButton, useSyncNote } from "@/features/sync-note"
import { useReviewSession } from "@/features/review-card"
import { MetricsBar } from "@/widgets/metrics-bar"
import { DailyReviewCard } from "@/widgets/daily-review"
import { TopicList } from "@/widgets/topic-list"

const router = useRouter()
const topicsStore = useTopicsStore()
const topicSelection = useTopicSelection()
const conceptSelection = useConceptSelection()
const createTopicStore = useCreateTopic()
const syncNoteStore = useSyncNote()
const reviewSession = useReviewSession()

const { topics, loading: topicsLoading, error: topicsError } =
  storeToRefs(topicsStore)
const { loading: creatingTopic, error: createTopicError } =
  storeToRefs(createTopicStore)
const { loading: syncingNote, error: syncNoteError } =
  storeToRefs(syncNoteStore)
const { dueCards } = reviewSession

// The page-level state block reflects whichever topic/create/sync action runs.
const loading = computed(
  () => topicsLoading.value || creatingTopic.value || syncingNote.value,
)
const error = computed(
  () => topicsError.value ?? createTopicError.value ?? syncNoteError.value,
)

const showCTA = computed(() => dueCards.value.length > 0)

const showNewTopicForm = ref(false)
const newTopicName = ref("")

function clearErrors() {
  topicsStore.clearError()
  createTopicStore.clearError()
  syncNoteStore.clearError()
}

onMounted(() => {
  topicsStore.loadTopics()
  reviewSession.fetchDueCards()
})

function retry() {
  clearErrors()
  topicsStore.loadTopics()
}

function goToTopic(id: string) {
  topicSelection.selectTopic(id)
  conceptSelection.resetConcept()
  router.push({ name: "topic", params: { topicId: id } })
}

function goToDueCards() {
  router.push({ name: "due-cards" })
}

async function handleCreateTopic() {
  const name = newTopicName.value.trim()
  if (!name) return

  clearErrors()

  await createTopicStore.createTopic(name)

  showNewTopicForm.value = false
  newTopicName.value = ""
}

function cancelNewTopic() {
  showNewTopicForm.value = false
  newTopicName.value = ""
  clearErrors()
}

async function syncActiveNote() {
  clearErrors()

  // Dynamic import to avoid making obsidian a hard dependency in the browser dev mode
  try {
    const { useObsidian } = await import("@/shared/obsidian")

    const obsidian = useObsidian()
    const filePath = obsidian.getActiveFilePath()

    if (!filePath) {
      syncNoteStore.error = "No hay nota activa. Abrí una nota primero."
      return
    }

    const topicName = obsidian.getTopicNameFromPath(filePath)
    const conceptTitle = obsidian.getConceptTitleFromPath(filePath)

    let fileContent: string
    try {
      fileContent = await obsidian.getActiveFileContent()
    } catch {
      syncNoteStore.error = "No hay nota activa. Abrí una nota primero."
      return
    }

    const result = await syncNoteStore.syncCurrentNote(
      topicName,
      conceptTitle,
      filePath,
      fileContent,
    )

    // Navigate to flashcards if sync succeeded
    if (result) {
      topicSelection.selectTopic(result.topicId)
      conceptSelection.selectConcept(result.conceptId)

      router.push({
        name: "flashcards",
        params: {
          topicId: result.topicId,
          conceptId: result.conceptId,
        },
      })
    }
  } catch {
    // useObsidian not available in browser dev — show a friendly message
    syncNoteStore.error =
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
</style>
