import { computed, ref } from "vue"
import type { ComputedRef, Ref } from "vue"

/**
 * One card of a review session. Same shape the `FlashcardCard` entity component
 * renders, kept local so this widget slice depends on nothing but fixtures.
 */
export interface ReviewCard {
  id: string
  question: string
  answer: string
}

export interface CardSession {
  /** The card being reviewed, or `undefined` once the session is complete. */
  currentCard: ComputedRef<ReviewCard | undefined>
  currentIndex: Ref<number>
  total: ComputedRef<number>
  isRevealed: Ref<boolean>
  isComplete: Ref<boolean>
  /** Mirrors the card's own flip state, in both directions. */
  setRevealed: (value: boolean) => void
  reveal: () => void
  grade: (value: number) => void
  restart: () => void
}

/** Keep a caller-supplied index inside the deck, including for an empty deck. */
function clampIndex(index: number, total: number): number {
  if (total === 0) return 0
  return Math.min(Math.max(0, Math.floor(index)), total - 1)
}

/**
 * Session semantics of the review flow (`PRESENTATION-UI.md` §4.5), mirroring
 * `features/review-card`'s `useReviewSession` without its store: reveal gates
 * grading, grading advances, and the last grade completes the session.
 *
 * Deliberately synchronous — no timers, no async, no store — so the browser dev
 * harness can drive it from fixture data.
 */
export function useCardSession(
  cards: readonly ReviewCard[],
  initialIndex = 0,
): CardSession {
  const total = computed(() => cards.length)
  const currentIndex = ref(clampIndex(initialIndex, cards.length))
  const isRevealed = ref(false)
  const isComplete = ref(false)

  const currentCard = computed<ReviewCard | undefined>(() =>
    isComplete.value ? undefined : cards[currentIndex.value],
  )

  function setRevealed(value: boolean) {
    if (isComplete.value || cards.length === 0) return
    isRevealed.value = value
  }

  function reveal() {
    setRevealed(true)
  }

  /** Grades 1-4; ignored while hidden, out of range, or already complete. */
  function grade(value: number) {
    if (isComplete.value || !isRevealed.value) return
    if (cards.length === 0) return
    if (!Number.isInteger(value) || value < 1 || value > 4) return

    if (currentIndex.value < cards.length - 1) {
      currentIndex.value += 1
      isRevealed.value = false
      return
    }

    isComplete.value = true
    isRevealed.value = false
  }

  /** Back to where the session started, not to the first card. */
  function restart() {
    currentIndex.value = clampIndex(initialIndex, cards.length)
    isRevealed.value = false
    isComplete.value = false
  }

  return {
    currentCard,
    currentIndex,
    total,
    isRevealed,
    isComplete,
    setRevealed,
    reveal,
    grade,
    restart,
  }
}
