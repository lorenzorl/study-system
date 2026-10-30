<template>
  <!--
    Every plain action here is the `shared/ui` button: it owns the border, radius,
    background, padding, focus ring, hover and disabled state.

    `variant="outline"` is the secondary action (retry, back, leave, the two
    navigation arrows) and the default variant is the accent-filled primary one
    ("Repasar de nuevo", "Estudiar de nuevo"). The four grade buttons stay
    hand-rolled: their identity is one colour per grade, and no variant expresses
    four distinct fills. The colours are the palette's (`--text-error`,
    `--text-warning`, `--text-success`, `--text-info`), not this file's, so the
    dark block in `index.html` retunes them like any other token (backlog D3).
    There is one grade row in the app and it lives here.

    `min-h-[44px]` is the touch target this page has always had. It is a class and
    not a rule because the primitives are sized by utilities; a migration does not
    shrink a 44px target to the primitive's default 36px.
  -->
  <div class="flashcard-session" ref="sessionEl">
    <!-- ═══ Due-Review Mode (FSRS) ═══ -->
    <template v-if="isDueMode">
      <!-- Review loading -->
      <div
        v-if="reviewLoading || dueModePending"
        class="flashcard-session__state loading-state"
      >
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
      <div
        v-if="loading || topicsPending"
        class="flashcard-session__state loading-state"
      >
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
import { createLoadIfEmpty, useSwipe } from "@/shared/lib"
import { Button } from "@/shared/ui/button"
import { FlashcardCard } from "@/entities/flashcard"
import { useTopicsStore } from "@/entities/topic"
import { useTopicSelection } from "@/features/select-topic"
import { useConceptSelection, useCurrentConcept } from "@/features/select-concept"
import { useReviewSession } from "@/features/review-card"

const props = defineProps<{
  // Optional because this page is two routes. The concept route passes both; the
  // review route (`/study/review`) passes neither, and that absence is what makes
  // it the due session.
  topicId?: string
  conceptId?: string
}>()
const router = useRouter()
const topicsStore = useTopicsStore()
const topicSelection = useTopicSelection()
const conceptSelection = useConceptSelection()

const { loading, error, isPending: topicsPending } = storeToRefs(topicsStore)

// The same rule as `TopicPage`, `ConceptPage` and `FeynmanPage` (`backlog.md` D1,
// D6): `concept` is resolved out of the topics store, so a cold visit needs the
// store filled, and a warm one must not fetch twice.
const loadTopicsIfEmpty = createLoadIfEmpty({
  isEmpty: () => topicsStore.topics.length === 0 && !topicsStore.loading,
  load: () => topicsStore.loadTopics(),
})
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
  startSession,
  restartReview,
  endSession,
  onCardFlip,
  grade,
  handleReviewKeydown,
  formatNextReview,
} = useReviewSession()

/**
 * Whether this page was mounted for a concept.
 *
 * The concept route (`/topic/:topicId/concept/:conceptId/flashcards`) passes both
 * props; `/study/review` passes neither, and that is the whole difference between
 * the concept session and the due session. That is why the props are declared
 * optional: "no concept" is a supported way to reach this page, not a missing
 * value.
 */
const hasConceptParams = computed(() => !!props.topicId && !!props.conceptId)

/**
 * Which of the two sessions this render is. The due session is also the one the
 * page falls into when it is entered with no concept, which is why it does not
 * depend on the store having been started by someone else first.
 */
const isDueMode = computed(
  () => !hasConceptParams.value || reviewIsSessionActive.value,
)

/**
 * Before the page's own fetch settles there is no session yet, so the due branch
 * has to report loading. Without this the first frame is an empty deck.
 *
 * A failed load is not pending: `reviewError` wins so the error and its retry are
 * what the page shows, instead of a spinner that never resolves
 * (`R3-due-fetch-failure-masked`).
 */
const dueModePending = computed(
  () => isDueMode.value && !reviewIsSessionActive.value && !reviewError.value,
)

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

/**
 * Set once the component is gone, so the due session's own load cannot start a
 * session — or leave a keydown listener behind — after the page unmounted. The load
 * is an `await` and the route can change while it is in flight
 * (`R3-async-mount-race`).
 *
 * Declared limit (`R4-no-abort-on-unmount`): this stops what happens *after* the
 * await, not the request. `fetchDueCards` writes the store's own `dueCards`,
 * `loading` and `error` as it settles, and the store offers no abort signal; the
 * page therefore cannot promise "no store mutation", only "no session, no
 * listener".
 */
let unmounted = false

onMounted(() => {
  if (isDueMode.value && !reviewIsSessionActive.value) {
    void resolveDueSession()
    return
  }

  attachSessionKeys()
})

/**
 * `/study/review` carries no concept, so the page owns its due session: a deep link
 * or a reload reviews today's cards instead of reporting a concept nobody asked for
 * (backlog D2).
 *
 * It always fetches, even when the store still holds cards from a previous visit
 * (`R3-003`): "has cards" is not "the cards are current", and reusing them showed a
 * queue the user had already graded. One request is the cheaper price.
 */
async function resolveDueSession() {
  await fetchDueCards()

  // A failed load is an error, not an empty deck: the page must not claim a session
  // it could not load (`R3-due-fetch-failure-masked`).
  if (unmounted || reviewError.value) return

  startSession()
  attachSessionKeys()
}

/** The keydown listener belongs to whichever of the two sessions this render is. */
function attachSessionKeys() {
  if (reviewIsSessionActive.value) {
    window.addEventListener("keydown", handleReviewKeydown)
    return
  }

  topicSelection.selectTopic(props.topicId)
  conceptSelection.selectConcept(props.conceptId)
  // D6's first loose end: this page had the same mount gap `TopicPage` had, so a
  // cold `/topic/:t/concept/:c/flashcards` resolved against an empty store. The
  // guard is the shared rule, so a warm path (launcher → topic → concept) does not
  // fetch again.
  loadTopicsIfEmpty()
  window.addEventListener("keydown", handleKeydown)
}

onBeforeUnmount(() => {
  unmounted = true
  window.removeEventListener("keydown", handleKeydown)
  window.removeEventListener("keydown", handleReviewKeydown)
})

// ── Concept mode handlers ────────────────────────────────────────────────────

function retry() {
  topicsStore.clearError()
  loadTopicsIfEmpty()
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
  // Only reachable from the concept session, which is the mode that has both.
  if (!props.topicId || !props.conceptId) return

  router.push({
    name: "concept",
    params: { topicId: props.topicId, conceptId: props.conceptId },
  })
}

// ── Due-Review mode handlers ──────────────────────────────────────────────────

/**
 * Retrying a failed load is also how the page enters its session: the first attempt
 * refused to claim one (`R3-due-fetch-failure-masked`).
 */
async function retryReview() {
  await fetchDueCards()
  if (unmounted || reviewError.value) return

  startSession()
  attachSessionKeys()
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
 * per grade, which no button variant expresses, and the primitive would add a
 * hover background they never had. All four colours are palette tokens
 * (`--text-error`, `--text-warning`, `--text-success`, `--text-info`), so this
 * file declares no colour value and the dark block retunes the row with the rest
 * of the app (backlog D3). The grade row is the app's only one: the launcher's
 * modal that used to carry a second copy is gone (backlog D2).
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
  /*
   * The fallback is the palette ref, not a literal: `--text-on-accent` is defined in
   * `index.html`, and a token that went missing must land on a readable colour
   * instead of inheriting `--text-normal` onto the fill (`R3-fallback-removal-coupling`).
   */
  color: var(--text-on-accent, var(--background-primary));
}

.flashcard-session__grade-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.flashcard-session__grade-btn--again {
  background: var(--text-error);
  border-color: var(--text-error);
}

.flashcard-session__grade-btn--hard {
  background: var(--text-warning);
  border-color: var(--text-warning);
}

.flashcard-session__grade-btn--good {
  background: var(--text-success);
  border-color: var(--text-success);
}

.flashcard-session__grade-btn--easy {
  background: var(--text-info);
  border-color: var(--text-info);
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
