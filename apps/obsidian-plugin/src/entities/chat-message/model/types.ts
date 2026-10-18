// ─── Chat message domain types ──────────────────────────────────────────────
// The chat backend endpoint does not exist yet (ARCHITECTURE §8), so this entity
// is the model `features/ask-chat` is scaffolded against. Nothing here talks to
// the network.

/** Who wrote a message: the person asking, or the assistant answering. */
export type ChatRole = "user" | "assistant"

export interface ChatMessage {
  id: string
  role: ChatRole
  content: string
  created_at: string
}
