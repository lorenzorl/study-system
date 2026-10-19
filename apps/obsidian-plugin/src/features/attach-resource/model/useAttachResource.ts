/**
 * Loading a vault file as a chat attachment (S7, S8).
 *
 * The vault read is injected so the size policy is testable without touching
 * the vault: the app layer wires the real `readVaultFile` in here. This module
 * never imports the Obsidian boundary.
 */
import type { ChatAttachment } from "@/entities/chat"
import { checkAttachmentLimits } from "@/entities/chat"

export type AttachResult =
  | { ok: true; attachment: ChatAttachment }
  | { ok: false; error: string }

function unreadableMessage(path: string): string {
  return (
    `No se pudo leer "${path}" del vault. ` +
    `Verificá que la nota exista y volvé a intentar.`
  )
}

/**
 * Reads the candidate, sums what the chat's existing vault attachments already
 * take and returns the attachment when the policy accepts it. A file that does
 * not exist or cannot be read is a rejection with a Spanish message, never a
 * thrown error.
 */
export async function attachVaultFile(
  path: string,
  current: ChatAttachment[],
  read: (path: string) => Promise<string>,
): Promise<AttachResult> {
  let currentChars = 0

  for (const attachment of current) {
    if (attachment.kind !== "vault") continue

    try {
      currentChars += (await read(attachment.path)).length
    } catch {
      // An existing attachment that can no longer be read contributes nothing:
      // its text cannot reach the model anyway. The candidate is what is being
      // accepted here, and this read failure must not invent a size for it.
    }
  }

  let content: string
  try {
    content = await read(path)
  } catch {
    return { ok: false, error: unreadableMessage(path) }
  }

  const decision = checkAttachmentLimits(path, content.length, currentChars)
  if (!decision.ok) {
    return { ok: false, error: decision.error }
  }

  return { ok: true, attachment: { kind: "vault", path } }
}
