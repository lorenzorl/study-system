import { describe, expect, it } from "vitest"
import { useCardSession } from "./useCardSession"
import type { ReviewCard } from "./useCardSession"

// ─── Review modal session (fixture-driven, no store, no timers) ─────────────

function makeCards(count: number): ReviewCard[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `card-${index + 1}`,
    question: `Pregunta ${index + 1}`,
    answer: `Respuesta ${index + 1}`,
  }))
}

describe("useCardSession", () => {
  it("ignores a grade while the answer is not revealed", () => {
    const session = useCardSession(makeCards(3))

    session.grade(3)

    expect(session.isRevealed.value).toBe(false)
    expect(session.currentIndex.value).toBe(0)
    expect(session.currentCard.value?.id).toBe("card-1")
    expect(session.isComplete.value).toBe(false)
  })

  it("advances to the next card after grading a revealed card", () => {
    const session = useCardSession(makeCards(3))

    session.reveal()
    session.grade(3)

    expect(session.currentIndex.value).toBe(1)
    expect(session.currentCard.value?.id).toBe("card-2")
    expect(session.isRevealed.value).toBe(false)
    expect(session.isComplete.value).toBe(false)
  })

  it("completes the session when the last card is graded", () => {
    const session = useCardSession(makeCards(2))

    session.reveal()
    session.grade(4)
    expect(session.isComplete.value).toBe(false)

    session.reveal()
    session.grade(2)

    expect(session.isComplete.value).toBe(true)
    expect(session.currentIndex.value).toBe(1)
    expect(session.total.value).toBe(2)

    // Grading again after completion changes nothing.
    session.grade(1)
    expect(session.isComplete.value).toBe(true)
    expect(session.currentIndex.value).toBe(1)
  })

  it("starts at initialIndex and restart() returns to it", () => {
    const session = useCardSession(makeCards(8), 4)

    expect(session.currentIndex.value).toBe(4)
    expect(session.currentCard.value?.id).toBe("card-5")

    session.reveal()
    session.grade(3)
    expect(session.currentIndex.value).toBe(5)

    session.restart()

    expect(session.currentIndex.value).toBe(4)
    expect(session.currentCard.value?.id).toBe("card-5")
    expect(session.isRevealed.value).toBe(false)
    expect(session.isComplete.value).toBe(false)
  })
  it("hides the answer again when the card is flipped back, and blocks grading", () => {
    const session = useCardSession(makeCards(2))

    session.reveal()
    session.setRevealed(false)

    expect(session.isRevealed.value).toBe(false)

    session.grade(3)

    expect(session.currentIndex.value).toBe(0)
    expect(session.isComplete.value).toBe(false)
  })

  it("cannot complete a deck with no cards", () => {
    const session = useCardSession([])

    session.reveal()
    session.grade(3)

    expect(session.total.value).toBe(0)
    expect(session.isRevealed.value).toBe(false)
    expect(session.isComplete.value).toBe(false)
  })
})
