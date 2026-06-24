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

        <form
          v-if="showNewConceptForm"
          class="topic-view__new-form"
          @submit.prevent="handleCreateConcept"
        >
          <input
            ref="newConceptInputRef"
            v-model="newConceptTitle"
            type="text"
            class="topic-view__new-input"
            placeholder="Título del concepto (ej. Aggregates, Value Objects)"
            :disabled="formSubmitting"
          />
          <div class="topic-view__new-actions">
            <button
              type="submit"
              class="topic-view__new-submit"
              :disabled="formSubmitting || !newConceptTitle.trim()"
            >
              <span v-if="formSubmitting" class="spinner"></span>
              <span v-else>Guardar</span>
            </button>
            <button
              type="button"
              class="topic-view__new-cancel"
              :disabled="formSubmitting"
              @click="cancelNewConcept"
            >
              Cancelar
            </button>
          </div>
        </form>

        <div v-if="currentTopic.concepts.length === 0" class="topic-view__state">
          <p>
            Este tema no tiene conceptos. Abrí una nota en esta carpeta y
            sincronizala.
          </p>
        </div>

        <div v-else class="topic-view__grid">
          <ConceptCard
            v-for="concept in currentTopic.concepts"
            :key="concept.id"
            :concept="concept"
            @select="goToConcept"
          />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { useStudyStore } from "../stores/study-store"
import { storeToRefs } from "pinia"
import ConceptCard from "../components/ConceptCard.vue"

const props = defineProps<{
  topicId: string
}>()

const router = useRouter()
const studyStore = useStudyStore()
const { currentTopic, loading, error } = storeToRefs(studyStore)

const showNewConceptForm = ref(false)
const newConceptTitle = ref("")
const newConceptInputRef = ref<HTMLInputElement | null>(null)
const formSubmitting = ref(false)

onMounted(() => {
  studyStore.selectTopic(props.topicId)
})

function goToConcept(conceptId: string) {
  studyStore.selectConcept(conceptId)
  router.push({
    name: "concept",
    params: { topicId: props.topicId, conceptId },
  })
}

function retry() {
  studyStore.clearError()
  studyStore.loadTopics()
}

async function handleCreateConcept() {
  const title = newConceptTitle.value.trim()
  if (!title) return

  formSubmitting.value = true
  studyStore.clearError()

  try {
    await studyStore.createConcept(props.topicId, title)
    showNewConceptForm.value = false
    newConceptTitle.value = ""
  } finally {
    formSubmitting.value = false
  }
}

function cancelNewConcept() {
  showNewConceptForm.value = false
  newConceptTitle.value = ""
  studyStore.clearError()
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

.topic-view__grid {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
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

.topic-view__new-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  margin-bottom: 0.25rem;
}

.topic-view__new-input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.topic-view__new-input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.topic-view__new-input:disabled {
  opacity: 0.5;
}

.topic-view__new-actions {
  display: flex;
  gap: 0.5rem;
}

.topic-view__new-submit {
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

.topic-view__new-submit:hover:not(:disabled) {
  opacity: 0.9;
}

.topic-view__new-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.topic-view__new-cancel {
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

.topic-view__new-cancel:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.topic-view__new-cancel:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
