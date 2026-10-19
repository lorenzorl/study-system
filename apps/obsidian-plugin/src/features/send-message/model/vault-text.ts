/**
 * The vault material of one send: the port that reads a vault file (S7) and the
 * send-time re-check of the size policy (S8).
 *
 * The read is a port because the send path never imports the Obsidian boundary —
 * `readVaultFile` lives in `shared/obsidian` — so the app layer installs the real
 * reader at bootstrap (the same shape as `setSettingsStore` and `setObsidianApp`).
 *
 * The caps come from `entities/chat`, which owns the policy because it owns the
 * attachment: one copy of the numbers, not one per feature.
 */
import type { ChatAttachment } from "@/entities/chat"
import {
  MAX_ATTACHMENT_CHARS,
  MAX_TOTAL_ATTACHMENT_CHARS,
} from "@/entities/chat"
import type { ContextBlock } from "./prompt"

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

/**
 * What the user reads when a file they attached cannot be read.
 *
 * The vault's own reason is carried through when it has one: "the file could not
 * be read" is not a diagnosis, and a note that was moved, a note the user cannot
 * read and a vault that failed are three different problems with one message. When
 * the failure carries no reason of its own, both remaining causes are named instead
 * of leaving it as a dead end.
 */
function unreadableMessage(path: string, cause: unknown): string {
  const reason = cause instanceof Error ? cause.message.trim() : ""
  const why = reason === "" ? "" : ` ${reason}`

  return (
    `No se pudo leer "${path}" del vault, así que el mensaje no se envió.${why} ` +
    `Verificá que la nota exista y que tengas permiso para leerla, y volvé a intentar; ` +
    `si ya no la usás, quitá el adjunto.`
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
    } catch (e) {
      return { ok: false, error: unreadableMessage(path, e) }
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
