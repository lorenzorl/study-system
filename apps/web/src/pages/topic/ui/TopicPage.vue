<template>
  <!--
    Every control here is the `shared/ui` button (`shadcn-migration.md` T8).

    The retry and the way out are plain buttons: `variant="outline"` names them as
    secondary, and `min-h-[44px]` keeps the touch target this page has always had as
    a class, because the primitives are sized by utilities and a migration does not
    shrink a 44px target to the primitive's 36px.

    The concept action in the section header is a square icon button, so it takes the
    primitive's `icon` size plus the same 44px floor (36px would have shrunk the
    target) and `ml-auto`, which is all the page ever owned about its position.
  -->
  <div class="topic-view">
    <div v-if="loading" class="topic-view__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <div v-else-if="error" class="topic-view__state error-state">
      <p>{{ error }}</p>
      <Button variant="outline" class="min-h-[44px]" @click="retry">Reintentar</Button>
    </div>

    <div v-else-if="!currentTopic" class="topic-view__state">
      <p>Tema no encontrado.</p>
      <Button
        variant="outline"
        class="min-h-[44px]"
        @click="router.push({ name: 'dashboard' })"
      >
        ← Volver
      </Button>
    </div>

    <div v-else class="topic-view__content">
      <div class="topic-view__header">
        <h2 class="topic-view__name">{{ currentTopic.name }}</h2>
      </div>

      <section class="topic-view__section">
        <h3 class="topic-view__section-title">
          <span>Conceptos</span>
          <Button
            variant="outline"
            size="icon"
            class="ml-auto min-h-[44px] min-w-[44px]"
            @click="showNewConceptForm = !showNewConceptForm"
            aria-label="Agregar concepto"
            title="Agregar concepto"
          >
            +
          </Button>
        </h3>

        <CreateConceptForm
          v-if="showNewConceptForm"
          v-model="newConceptTitle"
          :submitting="creatingConcept"
          @submit="handleCreateConcept"
          @cancel="cancelNewConcept"
        />

        <div v-if="currentTopic.concepts.length === 0" class="topic-view__state">
          <p>
            Este tema no tiene conceptos. Abrí una nota en esta carpeta y
            sincronizala.
          </p>
        </div>

        <ConceptList
          v-else
          :concepts="currentTopic.concepts"
          @select="goToConcept"
        />
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { storeToRefs } from "pinia"
import { useTopicsStore } from "@/entities/topic"
import { useTopicSelection } from "@/features/select-topic"
import { useConceptSelection } from "@/features/select-concept"
import { CreateConceptForm, useCreateConcept } from "@/features/create-concept"
import { ConceptList } from "@/widgets/concept-list"
import { createLoadIfEmpty } from "@/shared/lib/loadIfEmpty"
import { Button } from "@/shared/ui/button"

const props = defineProps<{
  topicId: string
}>()

const router = useRouter()
const topicsStore = useTopicsStore()
const topicSelection = useTopicSelection()
const conceptSelection = useConceptSelection()
const createConceptStore = useCreateConcept()

const { currentTopic } = storeToRefs(topicSelection)
const { loading: topicsLoading, error: topicsError } =
  storeToRefs(topicsStore)
const { loading: creatingConcept, error: createConceptError } =
  storeToRefs(createConceptStore)

const loading = computed(() => topicsLoading.value || creatingConcept.value)
const error = computed(
  () => topicsError.value ?? createConceptError.value,
)

const showNewConceptForm = ref(false)
const newConceptTitle = ref("")

function clearErrors() {
  topicsStore.clearError()
  createConceptStore.clearError()
}

// A cold visit — deep link, reload, bookmark — arrives with an empty store, so
// the page fills it itself: `currentTopic` is only a lookup in `entities/topic`.
// A visit that already came through `/topics` (or any warmer path) still works
// and is *not* reloaded, because the loader only runs while the store is empty
// and idle (`backlog.md` D1, the rule in `shared/lib/loadIfEmpty`). `retry` below
// still calls the store directly, so an explicit retry is never suppressed.
const loadTopicsIfEmpty = createLoadIfEmpty({
  isEmpty: () => topicsStore.topics.length === 0 && !topicsStore.loading,
  load: () => topicsStore.loadTopics(),
})

onMounted(() => {
  loadTopicsIfEmpty()
  topicSelection.selectTopic(props.topicId)
  conceptSelection.resetConcept()
})

function goToConcept(conceptId: string) {
  conceptSelection.selectConcept(conceptId)
  router.push({
    name: "concept",
    params: { topicId: props.topicId, conceptId },
  })
}

function retry() {
  clearErrors()
  topicsStore.loadTopics()
}

async function handleCreateConcept() {
  const title = newConceptTitle.value.trim()
  if (!title) return

  clearErrors()

  await createConceptStore.createConcept(props.topicId, title)

  showNewConceptForm.value = false
  newConceptTitle.value = ""
}

function cancelNewConcept() {
  showNewConceptForm.value = false
  newConceptTitle.value = ""
  clearErrors()
}
</script>

<style scoped>
.topic-view {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.topic-view__content {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.topic-view__header {
  margin-bottom: 0.25rem;
}

.topic-view__name {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.topic-view__section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.topic-view__section-title {
  margin: 0;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.topic-view__state {
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
