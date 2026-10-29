<template>
  <!--
    Both controls here are the `shared/ui` button (`shadcn-migration.md` T8): the
    retry is `variant="outline"` because it is secondary, and the topic action in the
    section header takes the primitive's `icon` size plus the 44px floor the page has
    always had (the primitive's 36px would have shrunk the touch target) and `ml-auto`,
    which is all the page ever owned about its position.
  -->
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
      <Button variant="outline" class="min-h-[44px]" @click="retry">Reintentar</Button>
    </div>

    <section v-else class="dashboard__section">
      <h2 class="dashboard__section-title">
        <span>Temas</span>
        <Button
          variant="outline"
          size="icon"
          class="min-h-[44px] min-w-[44px]"
          @click="showNewTopicForm = !showNewTopicForm"
          aria-label="Agregar tema"
          title="Agregar tema"
        >
          +
        </Button>
        <SyncNoteTrigger />
      </h2>

      <CreateTopicForm
        v-if="showNewTopicForm"
        v-model="newTopicName"
        :submitting="creatingTopic"
        @submit="handleCreateTopic"
        @cancel="cancelNewTopic"
      />

      <div v-if="topics.length === 0" class="dashboard__state">
        <p>No hay temas todavía. Creá el primero con + o sincronizá una nota.</p>
      </div>

      <TopicList v-else :topics="topics" @select="goToTopic" />
    </section>

    <!-- The sync overlay is the trigger's counterpart in the header above: it
         sits outside the heading, where a block element is valid. -->
    <SyncNoteForm @synced="goToSyncedConcept" />
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
import { SyncNoteForm, SyncNoteTrigger } from "@/features/sync-note"
import type { SyncNoteResult } from "@/features/sync-note"
import { useReviewSession } from "@/features/review-card"
import { MetricsBar } from "@/widgets/metrics-bar"
import { DailyReviewCard } from "@/widgets/daily-review"
import { TopicList } from "@/widgets/topic-list"
import { Button } from "@/shared/ui/button"
import { flashcardsRouteFor } from "../model/syncedRoute"

const router = useRouter()
const topicsStore = useTopicsStore()
const topicSelection = useTopicSelection()
const conceptSelection = useConceptSelection()
const createTopicStore = useCreateTopic()
const reviewSession = useReviewSession()

const { topics, loading: topicsLoading, error: topicsError } =
  storeToRefs(topicsStore)
const { loading: creatingTopic, error: createTopicError } =
  storeToRefs(createTopicStore)
const { dueCards } = reviewSession

// The page-level state block reflects whichever topic or create action runs.
const loading = computed(
  () => topicsLoading.value || creatingTopic.value,
)
const error = computed(() => topicsError.value ?? createTopicError.value)

const showCTA = computed(() => dueCards.value.length > 0)

const showNewTopicForm = ref(false)
const newTopicName = ref("")

function clearErrors() {
  topicsStore.clearError()
  createTopicStore.clearError()
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

/**
 * Land the reader on what they just synced, the way the vault flow did.
 * The panel reports the flow's own result; without a resolvable topic the list
 * stays, which the sync has already reloaded and where the concept now is.
 */
function goToSyncedConcept(result: SyncNoteResult) {
  topicSelection.selectTopic(result.topicId)
  conceptSelection.selectConcept(result.conceptId)

  const route = flashcardsRouteFor(result)
  if (route !== null) router.push(route)
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
