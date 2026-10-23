<template>
  <div class="topic-view">
    <div v-if="loading" class="topic-view__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <div v-else-if="error" class="topic-view__state error-state">
      <p>{{ error }}</p>
      <button class="topic-view__retry-btn" @click="retry">Reintentar</button>
    </div>

    <div v-else-if="!currentTopic" class="topic-view__state">
      <p>Tema no encontrado.</p>
      <button
        class="topic-view__back-btn"
        @click="router.push({ name: 'dashboard' })"
      >
        ← Volver
      </button>
    </div>

    <div v-else class="topic-view__content">
      <div class="topic-view__header">
        <h2 class="topic-view__name">{{ currentTopic.name }}</h2>
      </div>

      <section class="topic-view__section">
        <h3 class="topic-view__section-title">
          <span>Conceptos</span>
          <button
            class="topic-view__add-btn"
            @click="showNewConceptForm = !showNewConceptForm"
            aria-label="Agregar concepto"
            title="Agregar concepto"
          >
            +
          </button>
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

onMounted(() => {
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

.topic-view__back-btn,
.topic-view__retry-btn {
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

.topic-view__back-btn:hover,
.topic-view__retry-btn:hover {
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

.topic-view__add-btn {
  margin-left: auto;
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

.topic-view__add-btn:hover {
  background: var(--background-modifier-hover);
}
</style>
