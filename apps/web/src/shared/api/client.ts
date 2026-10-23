/**
 * The API base URL. Empty means "the origin that served this page": the Go
 * binary serves both the SPA and the API, and Vite proxies `/api` to it in
 * development, so every request is same-origin and no CORS allowlist is involved.
 */
export const DEFAULT_API_BASE = ""

/**
 * The active API base URL.
 *
 * Kept as a module-level value rather than read from `window.location` on every
 * request, so a request built in a test needs no DOM. `shared/api` and
 * `shared/config` are siblings of the same layer, so the boundary test forbids
 * them from importing each other (ARCHITECTURE §3); an explicit base arrives
 * through `setApiBase`, not through a settings import.
 */
let apiBase = DEFAULT_API_BASE

/** Get the current API base URL. */
export function getApiBase(): string {
  return apiBase
}

/** Override the API base URL at runtime (e.g., a server on another port). */
export function setApiBase(url: string): void {
  const trimmed = url.trim()

  if (trimmed !== "") {
    apiBase = trimmed
  }
}

/** Reset to the origin that served the app. */
export function resetApiBase(): void {
  apiBase = DEFAULT_API_BASE
}

export class NetworkError extends Error {
  constructor() {
    super("No se pudo conectar al servidor. ¿Sigue corriendo?")
    this.name = "NetworkError"
  }
}

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message)
    this.name = "ApiError"
  }
}

export async function request<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  let response: Response

  try {
    response = await fetch(`${getApiBase()}${path}`, {
      headers: {
        "Content-Type": "application/json",
        ...options?.headers,
      },
      ...options,
    })
  } catch {
    throw new NetworkError()
  }

  return parseJsonResponse<T>(response)
}

/**
 * Read a JSON response, mapping a non-2xx status to an `ApiError` that carries
 * the server's `error` message when the body has one.
 */
async function parseJsonResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let message = `Error del servidor al sincronizar.`
    try {
      const body = await response.json()
      if (body.error) {
        message = body.error
      }
    } catch {
      // use default message
    }
    throw new ApiError(message, response.status)
  }

  return response.json() as Promise<T>
}
