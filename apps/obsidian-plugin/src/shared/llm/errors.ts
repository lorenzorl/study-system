/**
 * Error mapping for the model provider (OpenRouter-compatible chat completions).
 *
 * Every user-facing message here is Spanish; the identifiers and comments stay
 * in English. The provider's API key never travels through these messages.
 */

/** An error raised by the LLM transport. `status: 0` means no HTTP response. */
export class LlmError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = "LlmError"
    this.status = status
  }
}

/** Base messages per provider status. Anything unlisted falls through below. */
const STATUS_MESSAGES: Record<number, string> = {
  401: "El proveedor rechazó la solicitud: la API key falta o no es válida.",
  402: "El proveedor rechazó la solicitud: la cuenta no tiene crédito.",
  403: "El proveedor rechazó la solicitud: la cuenta no tiene permiso para usar este modelo.",
  404: "El proveedor no encontró el modelo solicitado.",
  429: "El proveedor rechazó la solicitud: demasiadas solicitudes.",
}

const DETAIL_MAX_LENGTH = 300

function baseMessage(status: number): string {
  const known = STATUS_MESSAGES[status]

  if (known !== undefined) return known
  if (status >= 500 && status <= 599) return `El proveedor falló (HTTP ${status}).`

  return `El proveedor devolvió un error inesperado (HTTP ${status}).`
}

/** Collapse to one line and bound the length so an error stays readable. */
function toSingleLine(text: string): string {
  const collapsed = text.replace(/\s+/g, " ").trim()

  return collapsed.length > DETAIL_MAX_LENGTH
    ? `${collapsed.slice(0, DETAIL_MAX_LENGTH)}…`
    : collapsed
}

/**
 * The provider's own error message inside a JSON error body.
 *
 * Returns `""` when the body is not JSON or carries no `error.message` string.
 */
export function extractErrorMessage(body: string): string {
  if (body.trim() === "") return ""

  let parsed: unknown

  try {
    parsed = JSON.parse(body)
  } catch {
    return ""
  }

  if (typeof parsed !== "object" || parsed === null) return ""

  const error = (parsed as { error?: unknown }).error

  if (typeof error !== "object" || error === null) return ""

  const message = (error as { message?: unknown }).message

  return typeof message === "string" ? message : ""
}

/**
 * The user-facing Spanish message for a provider status.
 *
 * When the body carries the provider's own message it is appended, truncated to
 * one line, so a 404 can name the model the provider did not find.
 */
export function describeLlmError(status: number, body: string): string {
  const summary = baseMessage(status)
  const detail = toSingleLine(extractErrorMessage(body))

  return detail === "" ? summary : `${summary} Detalle: ${detail}`
}
