import { defineStore } from "pinia"
import { ref, computed } from "vue"
import type { DueCard, ReviewResponse } from "../../types"
import {
  fetchDueCards as apiFetchDueCards,
  submitReview as apiSubmitReview,
} from "../services/api"

export const useReviewStore = defineStore("review", () => {
  const dueCards = ref<DueCard[]>([])
  const currentIndex = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const completedCount = ref(0)
  const isSessionActive = ref(false)
  const reviewResult = ref<ReviewResponse | null>(null)

  const currentCard = computed(() => {
    const card = dueCards.value[currentIndex.value]
    if (!card) return null
    return {
      id: card.flashcard_id,
      question: card.question,
      answer: card.answer,
    }
  })

  const totalCount = computed(() => dueCards.value.length)
  const hasCards = computed(() => dueCards.value.length > 0)
  const isSessionComplete = computed(
    () =>
      completedCount.value === totalCount.value && totalCount.value > 0,
  )

  async function fetchDueCards() {
    loading.value = true
    error.value = null
    try {
      dueCards.value = await apiFetchDueCards()
      currentIndex.value = 0
      completedCount.value = 0
    } catch (e) {
      error.value =
        e instanceof Error ? e.message : "Failed to load due cards"
    } finally {
      loading.value = false
    }
  }

  async function submitReview(grade: number, durationMs: number) {
    const card = dueCards.value[currentIndex.value]
    if (!card) return
    const response = await apiSubmitReview({
      flashcard_id: card.flashcard_id,
      grade,
      duration_ms: durationMs,
    })
    reviewResult.value = response
    completedCount.value++
    return response
  }

  function advanceCard() {
    if (currentIndex.value < dueCards.value.length - 1) {
      currentIndex.value++
    }
  }

  function startSession() {
    isSessionActive.value = true
  }

  function endSession() {
    isSessionActive.value = false
    dueCards.value = []
    currentIndex.value = 0
  }

  return {
    dueCards,
    currentIndex,
    loading,
    error,
    completedCount,
    isSessionActive,
    reviewResult,
    currentCard,
    totalCount,
    hasCards,
    isSessionComplete,
    fetchDueCards,
    submitReview,
    advanceCard,
    startSession,
    endSession,
  }
})
