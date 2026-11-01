import { describe, it, expect, beforeEach, afterEach, vi } from "vitest"
import { setActivePinia, createPinia } from "pinia"
import { useReviewSession } from "./useReviewSession"
import { useReviewStore } from "@/entities/review"
import type { DueCard, ReviewResponse } from "@/entities/review"

const mockFetchDueCards = vi.fn()
const mockSubmitReview = vi.fn()

vi.mock("@/entities/review/api/review-api", () => ({
  fetchDueCards: (...args: unknown[]) => mockFetchDueCards(...args),
  submitReview: (...args: unknown[]) => mockSubmitReview(...args),
}))

function makeDueCard(overrides?: Partial<DueCard>): DueCard {
  return {
    flashcard_id: "fc-1",
    question: "Question one?",
    answer: "Answer one.",
    concept_title: "Concept",
    topic_name: "Topic",
    next_review: "2026-06-24T10:00:00Z",
    ...overrides,
  }
}

const reviewResponse: ReviewResponse = {
  next_review: "2026-06-26T10:00:00Z",
  stability: 1.8,
  difficulty: 0.5,
}

// The session advances only after the feedback pause, so the assertions have to
// cross it. `advanceTimersByTimeSync` keeps the test synchronous: the graded
// request itself is awaited through `grade()`.
const FEEDBACK_MS = 1500

async function seedSession(cards: DueCard[]) {
  mockFetchDueCards.mockResolvedValue(cards)
  mockSubmitReview.mockResolvedValue(reviewResponse)
  const session = useReviewSession()
  await session.fetchDueCards()
  session.startSession()
  return session
}

describe("useReviewSession", () => {
  // The hook registers `onBeforeUnmount`, so calling it outside a component logs a
  // Vue warning about the missing instance. The hook itself is what is under test
  // here, not its timer cleanup, and the registered callback is never invoked.
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // The defect this pins: `grade()` advanced a local `currentIndex` while the card
  // on screen came from the store's own index, so grading recorded a real review
  // against the card already on screen and the session never left it. Nine reviews
  // of one card in 49 seconds is what that looked like in the database.
  it("after grading, the session shows the next card", async () => {
    const session = await seedSession([
      makeDueCard({ flashcard_id: "fc-1", question: "Question one?" }),
      makeDueCard({ flashcard_id: "fc-2", question: "Question two?" }),
    ])

    expect(session.currentCard.value?.question).toBe("Question one?")

    session.onCardFlip(true)
    await session.grade(3)
    vi.advanceTimersByTime(FEEDBACK_MS)

    expect(session.currentCard.value?.question).toBe("Question two?")
    expect(session.currentIndex.value).toBe(1)
  })

  // The second half of the same defect: `restartReview` reset the local index and
  // left the store's where it was, so "Repasar de nuevo" replayed from the middle.
  it("restartReview returns the session to the first card", async () => {
    const session = await seedSession([
      makeDueCard({ flashcard_id: "fc-1", question: "Question one?" }),
      makeDueCard({ flashcard_id: "fc-2", question: "Question two?" }),
    ])

    session.onCardFlip(true)
    await session.grade(3)
    vi.advanceTimersByTime(FEEDBACK_MS)
    expect(session.currentCard.value?.question).toBe("Question two?")

    session.restartReview()

    expect(session.currentCard.value?.question).toBe("Question one?")
    expect(session.currentIndex.value).toBe(0)
  })

  // The behavior the fix must not break: one grade is one review, of the card that
  // was on screen.
  it("grading submits exactly one review for the card on screen", async () => {
    const session = await seedSession([
      makeDueCard({ flashcard_id: "fc-1", question: "Question one?" }),
      makeDueCard({ flashcard_id: "fc-2", question: "Question two?" }),
    ])

    session.onCardFlip(true)
    await session.grade(3)
    vi.advanceTimersByTime(FEEDBACK_MS)

    session.onCardFlip(true)
    await session.grade(4)
    vi.advanceTimersByTime(FEEDBACK_MS)

    expect(mockSubmitReview).toHaveBeenCalledTimes(2)
    expect(mockSubmitReview.mock.calls[0][0].flashcard_id).toBe("fc-1")
    expect(mockSubmitReview.mock.calls[1][0].flashcard_id).toBe("fc-2")
    expect(useReviewStore().completedCount).toBe(2)
  })
})
