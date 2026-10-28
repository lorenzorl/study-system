<template>
  <!--
    Every control here is the `shared/ui` button (`shadcn-migration.md` T8).

    The two error/empty actions are plain buttons, so the primitive owns their box
    and `variant="outline"` names them as secondary; `min-h-[44px]` is the touch
    target this page has always had, kept as a class because the primitives are
    sized by utilities and a migration does not shrink a 44px target to the
    primitive's 36px.

    The two actions are a different shape: each one is an icon + label + meta block
    that fills the column, so the control *is* the frame and it stays the page's own
    CSS, with the box restated in full so the primitive's `h-9`/`justify-center`/
    `font-medium`/`nowrap` cannot reshape it (the same rule T7 applied to the entity
    cards).
  -->
  <div class="concept-view">
    <div v-if="loading" class="concept-view__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <div v-else-if="error" class="concept-view__state error-state">
      <p>{{ error }}</p>
      <Button variant="outline" class="min-h-[44px]" @click="retry">Reintentar</Button>
    </div>

    <div v-else-if="!concept" class="concept-view__state">
      <p>Concepto no encontrado.</p>
      <Button
        variant="outline"
        class="min-h-[44px]"
        @click="router.push({ name: 'topic', params: { topicId: props.topicId } })"
      >
        ← Volver al Tema
      </Button>
    </div>

    <div v-else class="concept-view__content">
      <div class="concept-view__header">
        <h2 class="concept-view__title">{{ concept.title }}</h2>
        <p class="concept-view__path">{{ concept.file_path }}</p>
      </div>

      <div class="concept-view__actions">
        <Button
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
        </Button>

        <Button class="concept-view__action" @click="navigateTo('feynman')">
          <span class="concept-view__action-icon">📝</span>
          <span class="concept-view__action-text">
            Técnica Feynman
            <small class="concept-view__action-meta">
              Explícalo con tus propias palabras
            </small>
          </span>
        </Button>
      </div>

      <div
        v-if="flashcardCount === 0 && !loading && !error"
        class="concept-view__state"
      >
        <p>Este concepto no tiene tarjetas. Sincronizá la nota para generarlas.</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useRouter } from "vue-router"
import { storeToRefs } from "pinia"
import { useFlashcardCache } from "@/entities/flashcard"
import { useTopicsStore } from "@/entities/topic"
import { useTopicSelection } from "@/features/select-topic"
import { useConceptSelection, useCurrentConcept } from "@/features/select-concept"
import { Button } from "@/shared/ui/button"

const props = defineProps<{
  topicId: string
  conceptId: string
}>()

const router = useRouter()
const topicsStore = useTopicsStore()
const topicSelection = useTopicSelection()
const conceptSelection = useConceptSelection()
const flashcardCache = useFlashcardCache()

const { loading, error } = storeToRefs(topicsStore)
const { currentTopicId } = storeToRefs(topicSelection)

const concept = useCurrentConcept(currentTopicId)

const flashcardCount = computed(() => {
  return flashcardCache.getCachedFlashcards(props.conceptId).length
})

onMounted(() => {
  topicSelection.selectTopic(props.topicId)
  conceptSelection.selectConcept(props.conceptId)
})

function retry() {
  topicsStore.clearError()
  topicsStore.loadTopics()
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

/* The frame of an action row: the control *is* the box, so it is stated in full.
 *
 * Everything after `font-size` exists because the root is the primitive now and it
 * has its own opinion about a button: `h-9` would pin the height to 36px, its
 * `justify-content: center` would centre the icon and text, `gap-2` would narrow
 * the icon gutter from 12px to 8px, `rounded-md` (6px) would square the 8px
 * corners, `nowrap` would stop the meta line wrapping, and `font-medium` would
 * make the meta line heavier than the `400` it declares. `variant` is left at its
 * default because the frame owns the paint; what the variant still carries is the
 * disabled state, which the primitive already expresses as `opacity-50`. */
.concept-view__action {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 0.75rem;
  width: 100%;
  height: auto;
  min-height: 56px;
  padding: 0.75rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-weight: normal;
  white-space: normal;
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
