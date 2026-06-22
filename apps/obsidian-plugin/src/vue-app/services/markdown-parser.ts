import type { FlashcardInput } from "../../types"

export interface ParsedFlashcard {
  obsidian_id: string
  question: string
  answer: string
}

const FLASHCARD_LINE_RE = /^(.+?)::(.+?)$/
const ID_COMMENT_RE = /<!--\s*id:\s*(\S+)\s*-->/
const LOOKBACK_WINDOW = 5

/**
 * Extracts flashcards from markdown content.
 *
 * - Flashcards are lines with `::` separator: question::answer
 * - Optional `<!-- id: xxx -->` comments near Q&A lines provide an obsidian_id
 * - If no obsidian_id is found in the 5-line lookback window, one is generated
 *   from the line number
 */
export function parseFlashcards(markdown: string): ParsedFlashcard[] {
  const lines = markdown.split("\n")
  const cards: ParsedFlashcard[] = []

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const match = FLASHCARD_LINE_RE.exec(line)
    if (!match) continue

    const question = (match[1] ?? "").trim()
    const answer = (match[2] ?? "").trim()

    if (!question || !answer) continue

    let obsidianId = ""

    // Scan backwards for an id comment within the lookback window
    for (let j = i - 1; j >= Math.max(0, i - LOOKBACK_WINDOW); j--) {
      const prevLine = lines[j]
      const idMatch = ID_COMMENT_RE.exec(prevLine)
      if (idMatch) {
        obsidianId = (idMatch[1] ?? "").trim()
        break
      }
    }

    // Generate fallback id from line number if none found
    if (!obsidianId) {
      obsidianId = `card-${i + 1}`
    }

    cards.push({ obsidian_id: obsidianId, question, answer })
  }

  return cards
}
