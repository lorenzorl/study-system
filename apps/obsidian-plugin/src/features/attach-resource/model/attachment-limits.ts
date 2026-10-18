/**
 * The size policy for vault attachments (S8). Pure: no I/O.
 *
 * A vault file travels whole, so a file that does not fit is rejected when it
 * is attached -- never trimmed and never discovered halfway through a send.
 *
 * The caps are named constants and not UI settings on purpose: a knob nobody
 * calibrates is a knob nobody understands. If experience shows the number is
 * wrong, change the constant and its test.
 *
 * The numbers are a starting point to validate with real notes, not a measured
 * value. The tests pin the policy, not the calibration.
 */
export const MAX_ATTACHMENT_CHARS = 80_000
export const MAX_TOTAL_ATTACHMENT_CHARS = 120_000

export type AttachmentDecision = { ok: true } | { ok: false; error: string }

/**
 * Decides whether a candidate file can be attached, given how many characters
 * the chat's vault attachments already take and how many the candidate takes.
 *
 * The per-file cap is checked first: a file that does not fit on its own is a
 * different problem from a file that would overflow the whole chat, and the
 * message says which cap was exceeded.
 */
export function checkAttachmentLimits(
  path: string,
  candidateChars: number,
  currentChars: number,
): AttachmentDecision {
  if (candidateChars > MAX_ATTACHMENT_CHARS) {
    return {
      ok: false,
      error:
        `El archivo "${path}" tiene ${candidateChars} caracteres y supera el tope de ` +
        `${MAX_ATTACHMENT_CHARS} caracteres por adjunto. Partilo en notas más chicas ` +
        `o adjuntá solo el fragmento que necesitás.`,
    }
  }

  const totalChars = currentChars + candidateChars
  if (totalChars > MAX_TOTAL_ATTACHMENT_CHARS) {
    return {
      ok: false,
      error:
        `Adjuntar "${path}" (${candidateChars} caracteres) llevaría el total adjuntado a ` +
        `${totalChars} caracteres, por encima del tope de ${MAX_TOTAL_ATTACHMENT_CHARS} ` +
        `caracteres. Quitá algún adjunto o elegí un archivo más chico.`,
    }
  }

  return { ok: true }
}
