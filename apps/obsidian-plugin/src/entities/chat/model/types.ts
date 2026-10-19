export type ChatTurnRole = "user" | "assistant"

export interface ChatTurn {
  role: ChatTurnRole
  /** The turn's text, without the citation lines. */
  content: string
  /** One entry per "Fuente: *Title*, p. N" line the assistant wrote. */
  sources: string[]
  /**
   * Whether this turn is founded on the chat's material.
   *
   * `true` means there was nothing to consult, or the material was actually
   * read. `false` means the chat had corpus material behind the tool and no
   * search executed, so the answer is not founded on it; `false` is the only
   * value that shows the badge in the thread.
   *
   * Required, never optional: the compiler then names every literal that has to
   * say which state it is, so a new call site cannot leave the question
   * unanswered. A turn parsed from a note with no marker reads `true`, because
   * an absent marker claims nothing (an older note, or a chat with no material,
   * must never show the badge).
   */
  consulted: boolean
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

/**
 * One thing the chat brings into context, in one of two classes that never mix.
 *
 * A `corpus` attachment is searched on the server through the tool; a `vault`
 * attachment is a note that is sent whole. Keeping them a discriminated union
 * means every consumer has to say which class it handles.
 */
export type ChatAttachment =
  | { kind: "corpus"; documentId: string; title: string }
  | { kind: "vault"; path: string }

export interface ChatNote {
  chat: Chat
  turns: ChatTurn[]
  attachments: ChatAttachment[]
}
