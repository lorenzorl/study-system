import { describe, it, expect } from "vitest"
import { parseFlashcards } from "./markdown-parser"

// ─── Parser Tests ──────────────────────────────────────────────────────────

describe("parseFlashcards", () => {
  it("extracts a basic :: delimited flashcard", () => {
    const md = "¿Qué es DDD?::Domain-Driven Design"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({
      obsidian_id: "card-1",
      question: "¿Qué es DDD?",
      answer: "Domain-Driven Design",
    })
  })

  it("extracts obsidian_id from <!-- id: --> comment near Q&A line", () => {
    const md =
      "<!-- id: abc123 -->\n¿Qué es DDD?::Domain-Driven Design"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({
      obsidian_id: "abc123",
      question: "¿Qué es DDD?",
      answer: "Domain-Driven Design",
    })
  })

  it("extracts multiple flashcards from markdown", () => {
    const md = [
      "¿Qué es DDD?::Domain-Driven Design",
      "",
      "¿Qué es un Agregado?::Un cluster de objetos",
    ].join("\n")
    const result = parseFlashcards(md)
    expect(result).toHaveLength(2)
  })

  it("skips lines without :: delimiter", () => {
    const md = "This is a regular paragraph.\nAnother line without delimiter.\n¿Pregunta?::Respuesta"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
  })

  it("returns empty array for empty input", () => {
    expect(parseFlashcards("")).toEqual([])
  })

  it("finds id comment within 5-line lookback window", () => {
    const md = [
      "<!-- id: far-id -->",
      "",
      "",
      "",
      "¿Pregunta lejana?::Respuesta lejana",
    ].join("\n")
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0].obsidian_id).toBe("far-id")
  })

  it("generates line-number id when no id comment present", () => {
    const md = "Q1::A1\nQ2::A2"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(2)
    expect(result[0].obsidian_id).toBe("card-1")
    expect(result[1].obsidian_id).toBe("card-2")
  })

  it("skips blank :: lines where question or answer is empty after trim", () => {
    const md = " :: \nQ1::A1"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0].question).toBe("Q1")
  })

  it("trims whitespace from question and answer", () => {
    const md = "  ¿Pregunta?  ::  Respuesta  "
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0].question).toBe("¿Pregunta?")
    expect(result[0].answer).toBe("Respuesta")
  })
})
