/**
 * The corpus preflight: can the corpus be reached at all right now?
 *
 * One question and nothing else. A chat with corpus material blocks before the
 * first model call when that material is unreachable (S6), and the only thing
 * this module decides is whether the corpus answers — not why it does not, and
 * not what to do about it beyond naming the material and the way out.
 *
 * The probe is `listDocuments()`, the same corpus context the search tool reads;
 * the backend exposes no health route, and listing is the cheapest question whose
 * answer only exists while the server is up. Any thrown value means blocked,
 * whatever its type: the question is "does it answer", so no error class is
 * special and the server's own words never reach the user.
 *
 * Nothing is cached across sends on purpose: a server that came back up between
 * two questions has to be seen as up on the next one.
 */
import { listDocuments } from "@/entities/document"

export type CorpusPreflight = { ok: true } | { ok: false; error: string }

/** How many titles the message names before it counts the rest. */
const MAX_NAMED_TITLES = 3

function quoted(title: string): string {
  return `«${title}»`
}

/**
 * The titles as one Spanish fragment, capped so the message stays readable.
 *
 * `«DDD»`, `«DDD» y «SICP»`, `«DDD», «SICP» y «Refactoring»`. Past the cap the
 * remainder becomes a count — `«DDD», «SICP», «Refactoring» y 1 más` — and the
 * join there is commas only, so the sentence keeps a single `y`.
 */
function listTitles(titles: string[]): string {
  const named = titles.slice(0, MAX_NAMED_TITLES)
  const remaining = titles.length - named.length

  if (remaining > 0) return `${named.map(quoted).join(", ")} y ${remaining} más`

  if (named.length === 1) return quoted(named[0])

  const last = named[named.length - 1]

  return `${named.slice(0, -1).map(quoted).join(", ")} y ${quoted(last)}`
}

/**
 * What the user reads instead of an answer: the material that is not available
 * and the two things that unblock them. Blocking a whole chat because one of its
 * attachments lives on a server that is off is the accepted cost of S6, so the
 * message offers the way out rather than only the diagnosis.
 */
function blockedMessage(titles: string[]): string {
  const single = titles.length === 1

  return (
    `No se pudo consultar ${listTitles(titles)}: el servidor del corpus no responde. ` +
    `Encendé el servidor, o quitá ${single ? "ese adjunto" : "esos adjuntos"} ` +
    `para chatear sin ${single ? "él" : "ellos"}.`
  )
}

/**
 * Ask the corpus whether it is there, and turn "no" into the message to show.
 *
 * `titles` are the corpus attachments of the chat, which is what the message
 * names; the caller only asks when there is at least one, because a chat without
 * corpus material must never touch the server (S1, S7).
 *
 * `signal` is the send's own controller, so pressing Detener can give up on the
 * probe: without it the probe is a network await no one can abort, and a server
 * that accepts the connection and never answers holds the composer forever. A
 * cancelled probe rejects with an `AbortError` — the shape the send path already
 * reads as a cancellation — instead of deciding the corpus is unreachable.
 */
export async function checkCorpusReachable(
  titles: string[],
  signal?: AbortSignal,
): Promise<CorpusPreflight> {
  try {
    await listDocuments(signal)

    return { ok: true }
  } catch {
    // Cancelling is the user's own decision, so it is not a corpus failure to
    // report: saying the server does not answer would send them to start a server
    // that is fine. Decided from the signal and not from the thrown value,
    // because `shared/api/client.ts` maps every `fetch` failure -- an abort
    // included -- to its own `NetworkError`, so the error alone cannot tell the
    // two apart.
    if (signal?.aborted) throw cancelled()

    return { ok: false, error: blockedMessage(titles) }
  }
}

/**
 * The cancellation, in the shape the send path already recognizes: an `Error`
 * named `AbortError`. The message never reaches the user, because a cancellation
 * is reported by saying nothing at all.
 */
function cancelled(): Error {
  const error = new Error("La consulta del corpus se canceló.")

  error.name = "AbortError"
  return error
}
