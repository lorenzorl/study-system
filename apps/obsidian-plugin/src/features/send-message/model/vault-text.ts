/**
 * The vault material of one send: the port that reads a vault file (S7) and the
 * send-time re-check of the size policy (S8).
 *
 * The read is a port for two reasons that meet here. The send path never imports
 * the Obsidian boundary — `readVaultFile` lives in `shared/obsidian` — and
 * Feature-Sliced Design forbids this slice from reaching into
 * `features/attach-resource`, which owns the caps. So the app layer installs the
 * real reader at bootstrap (the same shape as `setSettingsStore` and
 * `setObsidianApp`), and the caps are restated below.
 */
import type { ChatAttachment } from "@/entities/chat"
import type { ContextBlock } from "./prompt"

/**
 * The size policy, mirrored from
 * `features/attach-resource/model/attachment-limits.ts`.
 *
 * The numbers are restated rather than imported because a slice may not import a
 * sibling of its own layer (`src/architecture.test.ts`). The policy is the same
 * one: 80.000 characters per attachment and 120.000 for all of them together,
 * counted in characters because the plugin has no tokenizer. Change both sides
 * together; `vault-text.test.ts` pins the numbers so a one-sided change fails.
 */
export const MAX_ATTACHMENT_CHARS = 80_000
export const MAX_TOTAL_ATTACHMENT_CHARS = 120_000

/** Reads a vault file's text. `app/main.ts` installs `readVaultFile` here. */
export type VaultTextReader = (path: string) => Promise<string>

// Singleton pattern — mirrors `shared/config`'s `setSettingsStore`: the port is
// installed once by the app layer and read by the send path.
let _readVaultText: VaultTextReader | null = null

/** Install the reader. Called once by the app layer at bootstrap. */
export function setVaultTextReader(reader: VaultTextReader | null): void {
  _readVaultText = reader
}

export type VaultMaterialResult =
  | { ok: true; blocks: ContextBlock[] }
  | { ok: false; error: string }

function readerUnavailableMessage(path: string): string {
  return (
    `No se pudo leer "${path}" del vault: el lector del vault no está disponible. ` +
    `Volvé a abrir el plugin y reintentá.`
  )
}

function unreadableMessage(path: string): string {
  return (
    `No se pudo leer "${path}" del vault, así que el mensaje no se envió. ` +
    `Verificá que la nota exista y volvé a intentar; si ya no la usás, quitá el adjunto.`
  )
}

function tooLargeMessage(path: string, chars: number): string {
  return (
    `El archivo "${path}" del vault tiene ${chars} caracteres y supera el tope de ` +
    `${MAX_ATTACHMENT_CHARS} caracteres por adjunto, así que el mensaje no se envió. ` +
    `Partilo en notas más chicas o quitá el adjunto.`
  )
}

function tooMuchTotalMessage(chars: number): string {
  return (
    `Los archivos del vault de este chat suman ${chars} caracteres y superan el tope de ` +
    `${MAX_TOTAL_ATTACHMENT_CHARS} caracteres adjuntados, así que el mensaje no se envió. ` +
    `Quitá algún adjunto o partí las notas más grandes.`
  )
}

/**
 * Read every vault attachment of the chat and turn it into a context block.
 *
 * The caps are enforced again here, and this is the only place that can do it:
 * a file was checked when it was attached, but a note grows, and the send is the
 * moment before the characters cost money and context window. A file that no
 * longer fits and a file that cannot be read both refuse the send, and refusing
 * is deliberate on both counts:
 *
 * - Sending it over the cap would spend the user's context window and money on
 *   material the model may silently truncate.
 * - Skipping it quietly would answer a question the chat claimed to bring
 *   material for, without that material — the chat would look grounded and be
 *   answering from nothing. That is the exact lie S7 exists to remove.
 *
 * So the failure is loud, names the file, and says what to do. Nothing is sent,
 * nothing is trimmed, and nothing is left behind.
 */
export async function readVaultMaterial(
  attachments: ChatAttachment[],
): Promise<VaultMaterialResult> {
  const paths = attachments
    .filter((attachment): attachment is Extract<ChatAttachment, { kind: "vault" }> =>
      attachment.kind === "vault",
    )
    .map((attachment) => attachment.path)

  // A chat with no vault file does no I/O at all, which is what keeps a chat
  // without material independent from the vault as well as from the server.
  if (paths.length === 0) return { ok: true, blocks: [] }

  const blocks: ContextBlock[] = []
  let totalChars = 0

  for (const path of paths) {
    const read = _readVaultText

    if (read === null) return { ok: false, error: readerUnavailableMessage(path) }

    let text: string

    try {
      text = await read(path)
    } catch {
      return { ok: false, error: unreadableMessage(path) }
    }

    if (text.length > MAX_ATTACHMENT_CHARS) {
      return { ok: false, error: tooLargeMessage(path, text.length) }
    }

    totalChars += text.length

    if (totalChars > MAX_TOTAL_ATTACHMENT_CHARS) {
      return { ok: false, error: tooMuchTotalMessage(totalChars) }
    }

    // The whole text, whole: no trimming, no chunks, and the path as the only
    // identity, because a vault note has no pages to cite.
    blocks.push({ kind: "vault", path, text })
  }

  return { ok: true, blocks }
}
