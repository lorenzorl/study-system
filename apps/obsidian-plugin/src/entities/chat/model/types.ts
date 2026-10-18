export type ChatTurnRole = "user" | "assistant"

export interface ChatTurn {
  role: ChatTurnRole
  /** The turn's text, without the citation lines. */
  content: string
  /** One entry per "Fuente: *Title*, p. N" line the assistant wrote. */
  sources: string[]
}

export interface ChatNoteSettings {
  model: string | null
  temperature: number | null
  topK: number | null
}

export interface Chat {
  /** Vault-relative path of the note. */
  path: string
  /** The file name without `.md`; the chat's title, derived and never stored twice. */
  title: string
  prompt: string
  settings: ChatNoteSettings
}

export interface ChatNote {
  chat: Chat
  turns: ChatTurn[]
}
