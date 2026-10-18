import { onBeforeUnmount, ref } from "vue"
import { storeToRefs } from "pinia"
import { useReviewStore } from "@/entities/review"

/**
 * Drives the due-review (FSRS) session on top of `entities/review`'s store: the
 * session state, the answer reveal/grading flow and the progress counters. The
 * FSRS behavior itself stays in the entity store — this feature only wraps it.
 */
export function useReviewSession() {
  const reviewStore = useReviewStore()
  const {
    dueCards,
    loading,
    error,
    isSessionActive,
    currentCard,
    totalCount,
    hasCards,
    isSessionComplete,
    completedCount,
    reviewResult,
  } = storeToRefs(reviewStore)

  // ── Review-mode state (local to the session UI) ─────────────────────────────
  const currentIndex = ref(0)
  const answerRevealed = ref(false)
  const answerRevealedAt = ref(0)
  const showFeedback = ref(false)
  const isSubmitting = ref(false)
  let feedbackTimer: ReturnType<typeof setTimeout> | null = null

  onBeforeUnmount(() => {
    if (feedbackTimer) clearTimeout(feedbackTimer)
  })

  function fetchDueCards() {
    return reviewStore.fetchDueCards()
  }

  function startSession() {
    reviewStore.startSession()
  }

  function restartReview() {
    reviewStore.startSession()
    currentIndex.value = 0
  }

  function endSession() {
    reviewStore.endSession()
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

        if (currentIndex.value < totalCount.value - 1) {
          currentIndex.value++
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

  return {
    dueCards,
    loading,
    error,
    isSessionActive,
    currentCard,
    totalCount,
    hasCards,
    isSessionComplete,
    completedCount,
    reviewResult,
    currentIndex,
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
  }
}
