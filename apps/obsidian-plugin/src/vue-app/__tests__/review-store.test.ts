import { describe, it, expect, beforeEach, vi } from "vitest"
import { setActivePinia, createPinia } from "pinia"
import { useReviewStore } from "../stores/review-store"
import type { DueCard, ReviewResponse } from "../../types"

const mockFetchDueCards = vi.fn()
const mockSubmitReview = vi.fn()

vi.mock("../services/api", () => ({
  fetchDueCards: (...args: unknown[]) => mockFetchDueCards(...args),
  submitReview: (...args: unknown[]) => mockSubmitReview(...args),
}))

function makeDueCard(overrides?: Partial<DueCard>): DueCard {
  return {
    flashcard_id: "fc-1",
    question: "Test question?",
    answer: "Test answer.",
    concept_title: "Test Concept",
    topic_name: "Test Topic",
    next_review: "2026-06-24T10:00:00Z",
    ...overrides,
  }
}

function makeReviewResponse(overrides?: Partial<ReviewResponse>): ReviewResponse {
  return {
    next_review: "2026-06-26T10:00:00Z",
    stability: 1.8,
    difficulty: 0.5,
    ...overrides,
  }
}

describe("useReviewStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  // ── Initial state ──────────────────────────────────────────────────────────

  it("has correct initial state", () => {
    const store = useReviewStore()
    expect(store.dueCards).toEqual([])
    expect(store.currentIndex).toBe(0)
    expect(store.loading).toBe(false)
    expect(store.error).toBeNull()
    expect(store.completedCount).toBe(0)
    expect(store.isSessionActive).toBe(false)
    expect(store.reviewResult).toBeNull()
    expect(store.currentCard).toBeNull()
    expect(store.totalCount).toBe(0)
    expect(store.hasCards).toBe(false)
    expect(store.isSessionComplete).toBe(false)
  })

  // ── fetchDueCards ──────────────────────────────────────────────────────────

  it("fetchDueCards populates cards on success", async () => {
    const cards: DueCard[] = [
      makeDueCard({ flashcard_id: "fc-1" }),
      makeDueCard({ flashcard_id: "fc-2" }),
      makeDueCard({ flashcard_id: "fc-3" }),
    ]
    mockFetchDueCards.mockResolvedValue(cards)

    const store = useReviewStore()
    await store.fetchDueCards()

    expect(store.dueCards).toHaveLength(3)
    expect(store.loading).toBe(false)
    expect(store.error).toBeNull()
    expect(store.hasCards).toBe(true)
    expect(store.totalCount).toBe(3)
    expect(store.currentIndex).toBe(0)
    expect(mockFetchDueCards).toHaveBeenCalledOnce()
  })

  it("fetchDueCards sets error on failure", async () => {
    mockFetchDueCards.mockRejectedValue(new Error("Network error"))

    const store = useReviewStore()
    await store.fetchDueCards()

    expect(store.dueCards).toEqual([])
    expect(store.loading).toBe(false)
    expect(store.error).toBe("Network error")
    expect(store.hasCards).toBe(false)
  })

  it("fetchDueCards sets error message for non-Error rejections", async () => {
    mockFetchDueCards.mockRejectedValue("unknown failure")

    const store = useReviewStore()
    await store.fetchDueCards()

    expect(store.error).toBe("Failed to load due cards")
  })

  // ── submitReview ───────────────────────────────────────────────────────────

  it("submitReview increments completedCount and returns response", async () => {
    const cards: DueCard[] = [makeDueCard(), makeDueCard()]
    mockFetchDueCards.mockResolvedValue(cards)
    const response = makeReviewResponse()
    mockSubmitReview.mockResolvedValue(response)

    const store = useReviewStore()
    await store.fetchDueCards()

    const result = await store.submitReview(3, 5000)

    expect(result).toEqual(response)
    expect(store.completedCount).toBe(1)
    expect(store.reviewResult).toEqual(response)
    expect(mockSubmitReview).toHaveBeenCalledWith({
      flashcard_id: "fc-1",
      grade: 3,
      duration_ms: 5000,
    })
  })

  it("submitReview handles multiple submissions", async () => {
    const cards: DueCard[] = [makeDueCard(), makeDueCard(), makeDueCard()]
    mockFetchDueCards.mockResolvedValue(cards)
    mockSubmitReview.mockResolvedValue(makeReviewResponse())

    const store = useReviewStore()
    await store.fetchDueCards()

    await store.submitReview(1, 2000)
    expect(store.completedCount).toBe(1)

    await store.submitReview(3, 4000)
    expect(store.completedCount).toBe(2)

    expect(store.isSessionComplete).toBe(false)
  })

  it("isSessionComplete becomes true after last card review", async () => {
    const cards: DueCard[] = [makeDueCard(), makeDueCard()]
    mockFetchDueCards.mockResolvedValue(cards)
    mockSubmitReview.mockResolvedValue(makeReviewResponse())

    const store = useReviewStore()
    await store.fetchDueCards()

    await store.submitReview(3, 5000)
    await store.submitReview(2, 3000)

    expect(store.completedCount).toBe(2)
    expect(store.isSessionComplete).toBe(true)
  })

  // ── advanceCard ────────────────────────────────────────────────────────────

  it("advanceCard increments currentIndex", () => {
    const store = useReviewStore()
    store.dueCards = [makeDueCard(), makeDueCard(), makeDueCard()]

    store.advanceCard()
    expect(store.currentIndex).toBe(1)

    store.advanceCard()
    expect(store.currentIndex).toBe(2)
  })

  it("advanceCard does not go past last card", () => {
    const store = useReviewStore()
    store.dueCards = [makeDueCard(), makeDueCard()]
    store.currentIndex = 1

    store.advanceCard()
    expect(store.currentIndex).toBe(1) // stays at last card
  })

  // ── Session lifecycle ──────────────────────────────────────────────────────

  it("startSession sets isSessionActive to true", () => {
    const store = useReviewStore()
    store.startSession()
    expect(store.isSessionActive).toBe(true)
  })

  it("endSession clears state", () => {
    const store = useReviewStore()
    store.dueCards = [makeDueCard(), makeDueCard()]
    store.currentIndex = 1
    store.isSessionActive = true

    store.endSession()

    expect(store.isSessionActive).toBe(false)
    expect(store.dueCards).toEqual([])
    expect(store.currentIndex).toBe(0)
  })

  it("isSessionComplete is false when no cards", () => {
    const store = useReviewStore()
    expect(store.isSessionComplete).toBe(false)
  })
})
