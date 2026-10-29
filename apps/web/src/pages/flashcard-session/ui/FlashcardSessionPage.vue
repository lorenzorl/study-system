<template>
  <!--
    Every plain action here is the `shared/ui` button: it owns the border, radius,
    background, padding, focus ring, hover and disabled state.

    `variant="outline"` is the secondary action (retry, back, leave, the two
    navigation arrows) and the default variant is the accent-filled primary one
    ("Repasar de nuevo", "Estudiar de nuevo"). The four grade buttons stay
    hand-rolled: their identity is one colour per grade, no variant expresses it,
    and those colours already live in this file's CSS — moving them into utility
    classes would put new colour values in a component, which the palette rule in
    `index.html` forbids. The `review-modal` grade row was left the same way.

    `min-h-[44px]` is the touch target this page has always had. It is a class and
    not a rule because the primitives are sized by utilities; a migration does not
    shrink a 44px target to the primitive's default 36px.
  -->
  <div class="flashcard-session" ref="sessionEl">
    <!-- ═══ Due-Review Mode (FSRS) ═══ -->
    <template v-if="reviewIsSessionActive">
      <!-- Review loading -->
      <div v-if="reviewLoading" class="flashcard-session__state loading-state">
        <span class="spinner"></span> Cargando...
      </div>

      <!-- Review error -->
      <div v-else-if="reviewError" class="flashcard-session__state error-state">
        <p>{{ reviewError }}</p>
        <Button variant="outline" class="min-h-[44px]" @click="retryReview">Reintentar</Button>
      </div>

      <!-- No review cards -->
      <div v-else-if="!reviewCard" class="flashcard-session__empty">
        <p>No hay tarjetas para repasar.</p>
        <Button variant="outline" class="min-h-[44px]" @click="goBackToDue">← Volver</Button>
      </div>

      <!-- Review completion -->
      <div v-else-if="isReviewComplete" class="flashcard-session__complete">
        <div class="flashcard-session__complete-icon">✅</div>
        <h2 class="flashcard-session__complete-title">¡Repaso Completado!</h2>
        <p class="flashcard-session__complete-text">
          Repasaste {{ reviewCompletedCount }} de {{ reviewTotalCount }} tarjeta{{ reviewTotalCount === 1 ? '' : 's' }}.
        </p>
        <div class="flashcard-session__complete-actions">
          <Button class="min-h-[44px]" @click="restartReview">
            Repasar de nuevo
          </Button>
          <Button variant="outline" class="min-h-[44px]" @click="goBackToDue">
            Volver al Panel
          </Button>
        </div>
      </div>

      <!-- Active review card -->
      <template v-else>
        <div class="flashcard-session__progress">
          <span>{{ currentReviewIndex + 1 }} / {{ reviewTotalCount }}</span>
        </div>

        <FlashcardCard :card="reviewCard" @flip="onCardFlip" />

        <!-- Grade buttons (visible when answer revealed) -->
        <div v-if="answerRevealed" class="flashcard-session__grades">
          <button
            class="flashcard-session__grade-btn flashcard-session__grade-btn--again"
            :disabled="isSubmitting"
            @click="grade(1)"
          >
            Otra vez (1)
          </button>
          <button
            class="flashcard-session__grade-btn flashcard-session__grade-btn--hard"
            :disabled="isSubmitting"
            @click="grade(2)"
          >
            Difícil (2)
          </button>
          <button
            class="flashcard-session__grade-btn flashcard-session__grade-btn--good"
            :disabled="isSubmitting"
            @click="grade(3)"
          >
            Bien (3)
          </button>
          <button
            class="flashcard-session__grade-btn flashcard-session__grade-btn--easy"
            :disabled="isSubmitting"
            @click="grade(4)"
          >
            Fácil (4)
          </button>
        </div>

        <!-- Post-grade feedback -->
        <div v-if="showFeedback && reviewResult" class="flashcard-session__feedback">
          <span class="flashcard-session__feedback-date">
            Próximo repaso: {{ formatNextReview(reviewResult.next_review) }}
          </span>
        </div>

        <div class="flashcard-session__keys" v-if="!hasTouch && answerRevealed">
          <span class="flashcard-session__key-hint">Teclas 1–4 para calificar</span>
        </div>
      </template>
    </template>

    <!-- ═══ Concept Mode (existing) ═══ -->
    <template v-else>
      <!-- Loading -->
      <div v-if="loading" class="flashcard-session__state loading-state">
        <span class="spinner"></span> Cargando...
      </div>

      <!-- Error -->
      <div v-else-if="error" class="flashcard-session__state error-state">
        <p>{{ error }}</p>
        <Button variant="outline" class="min-h-[44px]" @click="retry">Reintentar</Button>
      </div>

      <!-- No concept found -->
      <div v-else-if="!concept" class="flashcard-session__empty">
        <p>Concepto no encontrado.</p>
        <Button variant="outline" class="min-h-[44px]" @click="goBackToConcept">
          ← Volver
        </Button>
      </div>

      <!-- No flashcards for this concept -->
      <div v-else-if="flashcards.length === 0" class="flashcard-session__empty">
        <p>No hay tarjetas para estudiar. Sincronizá la nota primero.</p>
        <Button variant="outline" class="min-h-[44px]" @click="goBackToConcept">
          ← Volver al Panel
        </Button>
      </div>

      <!-- Completion state -->
      <div v-else-if="isComplete" class="flashcard-session__complete">
        <div class="flashcard-session__complete-icon">✅</div>
        <h2 class="flashcard-session__complete-title">¡Sesión Completada!</h2>
        <p class="flashcard-session__complete-text">
          Revisaste las {{ flashcards.length }} tarjeta{{ flashcards.length === 1 ? '' : 's' }}
          de {{ concept.title }}.
        </p>
        <div class="flashcard-session__complete-actions">
          <Button class="min-h-[44px]" @click="restartSession">
            Estudiar de nuevo
          </Button>
          <Button variant="outline" class="min-h-[44px]" @click="goBackToConcept">
            Volver al Panel
          </Button>
        </div>
      </div>

      <!-- Active flashcard session -->
      <template v-else>
        <div class="flashcard-session__progress">
          <span>{{ currentIndex + 1 }} / {{ flashcards.length }}</span>
        </div>

        <FlashcardCard :card="flashcards[currentIndex]" />

        <div class="flashcard-session__nav">
          <Button
            variant="outline"
            class="min-h-[44px]"
            @click="prevCard"
            :disabled="currentIndex === 0"
            aria-label="Tarjeta anterior"
          >
            ← Anterior
          </Button>
          <Button
            variant="outline"
            class="min-h-[44px]"
            @click="nextCard"
            aria-label="Siguiente tarjeta"
          >
            {{ currentIndex === flashcards.length - 1 ? 'Finalizar →' : 'Siguiente →' }}
          </Button>
        </div>

        <div class="flashcard-session__keys" v-if="!hasTouch">
          <span class="flashcard-session__key-hint">Teclas ← → para navegar</span>
        </div>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from "vue"
import { useRouter } from "vue-router"
import { storeToRefs } from "pinia"
import { useSwipe } from "@/shared/lib"
import { Button } from "@/shared/ui/button"
import { FlashcardCard } from "@/entities/flashcard"
import { useTopicsStore } from "@/entities/topic"
import { useTopicSelection } from "@/features/select-topic"
import { useConceptSelection, useCurrentConcept } from "@/features/select-concept"
import { useReviewSession } from "@/features/review-card"

const props = defineProps<{
  topicId: string
  conceptId: string
}>()

const router = useRouter()
const topicsStore = useTopicsStore()
const topicSelection = useTopicSelection()
const conceptSelection = useConceptSelection()

const { loading, error } = storeToRefs(topicsStore)
const { currentTopicId } = storeToRefs(topicSelection)
const { currentFlashcards: flashcards } = storeToRefs(conceptSelection)

const concept = useCurrentConcept(currentTopicId)

const {
  isSessionActive: reviewIsSessionActive,
  currentCard: reviewCard,
  loading: reviewLoading,
  error: reviewError,
  isSessionComplete: isReviewComplete,
  completedCount: reviewCompletedCount,
  totalCount: reviewTotalCount,
  reviewResult,
  currentIndex: currentReviewIndex,
  answerRevealed,
  showFeedback,
  isSubmitting,
  fetchDueCards,
  restartReview,
  endSession,
  onCardFlip,
  grade,
  handleReviewKeydown,
  formatNextReview,
} = useReviewSession()

const sessionEl = ref<HTMLElement | null>(null)
const currentIndex = ref(0)
const isComplete = ref(false)

const hasTouch = computed(() => {
  return typeof window !== "undefined" && "ontouchstart" in window
})

// Touch swipe support (concept mode)
useSwipe(sessionEl, {
  onSwipeLeft: () => nextCard(),
  onSwipeRight: () => prevCard(),
})

onMounted(() => {
  if (reviewIsSessionActive.value) {
    window.addEventListener("keydown", handleReviewKeydown)
  } else {
    topicSelection.selectTopic(props.topicId)
    conceptSelection.selectConcept(props.conceptId)
    window.addEventListener("keydown", handleKeydown)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleKeydown)
  window.removeEventListener("keydown", handleReviewKeydown)
})

// ── Concept mode handlers ────────────────────────────────────────────────────

function retry() {
  topicsStore.clearError()
  topicsStore.loadTopics()
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "ArrowRight") {
    e.preventDefault()
    nextCard()
  } else if (e.key === "ArrowLeft") {
    e.preventDefault()
    prevCard()
  }
}

function nextCard() {
  if (currentIndex.value < flashcards.value.length - 1) {
    currentIndex.value++
  } else {
    isComplete.value = true
  }
}

function prevCard() {
  if (currentIndex.value > 0) {
    currentIndex.value--
  }
}

function restartSession() {
  currentIndex.value = 0
  isComplete.value = false
}

function goBackToConcept() {
  router.push({
    name: "concept",
    params: { topicId: props.topicId, conceptId: props.conceptId },
  })
}

// ── Due-Review mode handlers ──────────────────────────────────────────────────

function retryReview() {
  fetchDueCards()
}

function goBackToDue() {
  endSession()
  router.push({ name: "due-cards" })
}
</script>

<style scoped>
/*
 * The plain actions are `shared/ui` primitives now: their border, radius,
 * background, padding, hover, focus ring and disabled state come from the design
 * tokens, not from this file. What stays is what the primitives do not own — the
 * session's column and grids, the card area, the completion panel, the spinner and
 * the key hints.
 *
 * The grade buttons keep their own rules on purpose. Their identity is one colour
 * per grade (`--text-error`, plus three that predate the palette), which no button
 * variant expresses, and the primitive would add a hover background they never
 * had. `review-modal`'s grade row was left hand-rolled for the same reason.
 */
.flashcard-session {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.flashcard-session__progress {
  text-align: center;
  font-size: 0.8rem;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* ── Grade buttons ──────────────────────────────────────────────────────── */

.flashcard-session__grades {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.5rem;
}

.flashcard-session__grade-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.6rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  transition: opacity 0.15s ease, background-color 0.15s ease;
  color: var(--text-on-accent, #fff);
}

.flashcard-session__grade-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.flashcard-session__grade-btn--again {
  background: var(--text-error, #e74c3c);
  border-color: var(--text-error, #e74c3c);
}

.flashcard-session__grade-btn--hard {
  background: #e67e22;
  border-color: #e67e22;
}

.flashcard-session__grade-btn--good {
  background: #27ae60;
  border-color: #27ae60;
}

.flashcard-session__grade-btn--easy {
  background: #2980b9;
  border-color: #2980b9;
}

/* ── Feedback ────────────────────────────────────────────────────────────── */

.flashcard-session__feedback {
  text-align: center;
  padding: 0.5rem;
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.flashcard-session__feedback-date {
  font-size: 0.85rem;
  color: var(--text-muted);
  font-weight: 500;
}

/* ── Navigation (concept mode) ───────────────────────────────────────────── */

.flashcard-session__nav {
  display: flex;
  gap: 0.5rem;
  justify-content: center;
}

.flashcard-session__keys {
  text-align: center;
}

.flashcard-session__key-hint {
  font-size: 0.72rem;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* ── Completion ──────────────────────────────────────────────────────────── */

.flashcard-session__complete {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2rem 1rem;
  text-align: center;
}

.flashcard-session__complete-icon {
  font-size: 2.5rem;
}

.flashcard-session__complete-title {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
}

.flashcard-session__complete-text {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-muted);
}

.flashcard-session__complete-actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  width: 100%;
  margin-top: 0.5rem;
}

/* ── Empty / state ───────────────────────────────────────────────────────── */

.flashcard-session__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
}

.flashcard-session__state {
  padding: 2rem 1rem;
  text-align: center;
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
