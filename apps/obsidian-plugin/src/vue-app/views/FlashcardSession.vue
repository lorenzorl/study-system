<template>
  <div class="flashcard-session" ref="sessionEl">
    <!-- ═══ Due-Review Mode (FSRS) ═══ -->
    <template v-if="reviewStore.isSessionActive">
      <!-- Review loading -->
      <div v-if="reviewLoading" class="flashcard-session__state loading-state">
        <span class="spinner"></span> Cargando...
      </div>

      <!-- Review error -->
      <div v-else-if="reviewError" class="flashcard-session__state error-state">
        <p>{{ reviewError }}</p>
        <button class="flashcard-session__retry-btn" @click="retryReview">Reintentar</button>
      </div>

      <!-- No review cards -->
      <div v-else-if="!reviewCard" class="flashcard-session__empty">
        <p>No hay tarjetas para repasar.</p>
        <button class="flashcard-session__back-btn" @click="goBackToDue">← Volver</button>
      </div>

      <!-- Review completion -->
      <div v-else-if="isReviewComplete" class="flashcard-session__complete">
        <div class="flashcard-session__complete-icon">✅</div>
        <h2 class="flashcard-session__complete-title">¡Repaso Completado!</h2>
        <p class="flashcard-session__complete-text">
          Repasaste {{ reviewStore.completedCount }} de {{ reviewStore.totalCount }} tarjeta{{ reviewStore.totalCount === 1 ? '' : 's' }}.
        </p>
        <div class="flashcard-session__complete-actions">
          <button class="flashcard-session__btn" @click="reviewStore.startSession(); currentReviewIndex = 0;">
            Repasar de nuevo
          </button>
          <button class="flashcard-session__btn flashcard-session__btn--secondary" @click="goBackToDue">
            Volver al Panel
          </button>
        </div>
      </div>

      <!-- Active review card -->
      <template v-else>
        <div class="flashcard-session__progress">
          <span>{{ currentReviewIndex + 1 }} / {{ reviewStore.totalCount }}</span>
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
        <div v-if="showFeedback && reviewStore.reviewResult" class="flashcard-session__feedback">
          <span class="flashcard-session__feedback-date">
            Próximo repaso: {{ formatNextReview(reviewStore.reviewResult.next_review) }}
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
        <button class="flashcard-session__retry-btn" @click="retry">Reintentar</button>
      </div>

      <!-- No concept found -->
      <div v-else-if="!concept" class="flashcard-session__empty">
        <p>Concepto no encontrado.</p>
        <button
          class="flashcard-session__back-btn"
          @click="goBackToConcept"
        >
          ← Volver
        </button>
      </div>

      <!-- No flashcards for this concept -->
      <div v-else-if="flashcards.length === 0" class="flashcard-session__empty">
        <p>No hay tarjetas para estudiar. Sincronizá la nota primero.</p>
        <button
          class="flashcard-session__back-btn"
          @click="goBackToConcept"
        >
          ← Volver al Panel
        </button>
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
          <button class="flashcard-session__btn" @click="restartSession">
            Estudiar de nuevo
          </button>
          <button class="flashcard-session__btn flashcard-session__btn--secondary" @click="goBackToConcept">
            Volver al Panel
          </button>
        </div>
      </div>

      <!-- Active flashcard session -->
      <template v-else>
        <div class="flashcard-session__progress">
          <span>{{ currentIndex + 1 }} / {{ flashcards.length }}</span>
        </div>

        <FlashcardCard :card="flashcards[currentIndex]" />

        <div class="flashcard-session__nav">
          <button
            class="flashcard-session__nav-btn"
            @click="prevCard"
            :disabled="currentIndex === 0"
            :aria-label="'Tarjeta anterior'"
          >
            ← Anterior
          </button>
          <button
            class="flashcard-session__nav-btn"
            @click="nextCard"
            :aria-label="'Siguiente tarjeta'"
          >
            {{ currentIndex === flashcards.length - 1 ? 'Finalizar →' : 'Siguiente →' }}
          </button>
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
import { useStudyStore } from "../stores/study-store"
import { useReviewStore } from "../stores/review-store"
import { useSwipe } from "../composables/useSwipe"
import { storeToRefs } from "pinia"
import FlashcardCard from "../components/FlashcardCard.vue"

const props = defineProps<{
  topicId: string
  conceptId: string
}>()

const router = useRouter()
const studyStore = useStudyStore()
const reviewStore = useReviewStore()

const { currentConcept: concept, currentFlashcards: flashcards, loading, error } =
  storeToRefs(studyStore)

const {
  currentCard: reviewCard,
  loading: reviewLoading,
  error: reviewError,
  isSessionComplete: isReviewComplete,
  completedCount: reviewCompletedCount,
} = storeToRefs(reviewStore)

const sessionEl = ref<HTMLElement | null>(null)
const currentIndex = ref(0)
const isComplete = ref(false)

// ── Due-Review state ─────────────────────────────────────────────────────────
const currentReviewIndex = ref(0)
const answerRevealed = ref(false)
const answerRevealedAt = ref(0)
const showFeedback = ref(false)
const isSubmitting = ref(false)
let feedbackTimer: ReturnType<typeof setTimeout> | null = null

const hasTouch = computed(() => {
  return typeof window !== "undefined" && "ontouchstart" in window
})

// Touch swipe support (concept mode)
useSwipe(sessionEl, {
  onSwipeLeft: () => nextCard(),
  onSwipeRight: () => prevCard(),
})

onMounted(() => {
  if (reviewStore.isSessionActive) {
    window.addEventListener("keydown", handleReviewKeydown)
  } else {
    studyStore.selectTopic(props.topicId)
    studyStore.selectConcept(props.conceptId)
    window.addEventListener("keydown", handleKeydown)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleKeydown)
  window.removeEventListener("keydown", handleReviewKeydown)
  if (feedbackTimer) clearTimeout(feedbackTimer)
})

// ── Concept mode handlers ────────────────────────────────────────────────────

function retry() {
  studyStore.clearError()
  studyStore.loadTopics()
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
  reviewStore.fetchDueCards()
}

function goBackToDue() {
  reviewStore.endSession()
  router.push({ name: "due-cards" })
}

function onCardFlip(flipped: boolean) {
  if (flipped && !answerRevealed.value) {
    answerRevealed.value = true
    answerRevealedAt.value = Date.now()
  }
}

async function grade(value: number) {
  if (isSubmitting.value) return
  isSubmitting.value = true

  const durationMs = Date.now() - answerRevealedAt.value

  try {
    await reviewStore.submitReview(value, durationMs)
    showFeedback.value = true

    feedbackTimer = setTimeout(() => {
      showFeedback.value = false
      answerRevealed.value = false
      isSubmitting.value = false

      if (currentReviewIndex.value < reviewStore.totalCount - 1) {
        currentReviewIndex.value++
      }
    }, 1500)
  } catch {
    // Error handled by reviewStore.error
    isSubmitting.value = false
    showFeedback.value = false
  }
}

function handleReviewKeydown(e: KeyboardEvent) {
  if (!answerRevealed.value || isSubmitting.value) return
  const key = e.key
  if (key === "1" || key === "2" || key === "3" || key === "4") {
    e.preventDefault()
    grade(Number.parseInt(key, 10))
  }
}

function formatNextReview(isoDate: string): string {
  const d = new Date(isoDate)
  return d.toLocaleDateString("es-ES", {
    day: "numeric",
    month: "short",
    year: "numeric",
  })
}
</script>

<style scoped>
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

.flashcard-session__nav-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1.25rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.flashcard-session__nav-btn:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.flashcard-session__nav-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
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

/* ── Empty / Complete / State ────────────────────────────────────────────── */

.flashcard-session__empty,
.flashcard-session__complete,
.flashcard-session__state {
  padding: 2rem 1rem;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
}

.flashcard-session__complete-icon {
  font-size: 2.5rem;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.flashcard-session__complete-title {
  margin: 0;
  font-size: 1.2rem;
  font-weight: 600;
}

.flashcard-session__complete-text {
  margin: 0;
  font-size: 0.9rem;
  color: var(--text-muted);
}

.flashcard-session__complete-actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  width: 100%;
  max-width: 300px;
}

.flashcard-session__btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.flashcard-session__btn:hover {
  opacity: 0.9;
}

.flashcard-session__btn--secondary {
  background: var(--background-secondary);
  color: var(--text-normal);
  border-color: var(--background-modifier-border);
}

.flashcard-session__back-btn,
.flashcard-session__retry-btn {
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

.flashcard-session__back-btn:hover,
.flashcard-session__retry-btn:hover {
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
