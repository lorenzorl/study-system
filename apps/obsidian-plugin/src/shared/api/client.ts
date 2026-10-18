export const DEFAULT_API_BASE = "http://localhost:8080"

/**
 * The active API base URL.
 *
 * Its persistence and its initial load live in `app/settings-store.ts`: this
 * segment (`shared/api`) and `shared/config` are siblings of the same layer, so
 * the boundary test forbids them from importing each other (ARCHITECTURE §3).
 */
let apiBase = DEFAULT_API_BASE

/** Get the current API base URL. */
export function getApiBase(): string {
  return apiBase
}

/** Override the API base URL at runtime (e.g., for mobile: "http://192.168.1.50:8080"). */
export function setApiBase(url: string): void {
  const trimmed = url.trim()

  if (trimmed !== "") {
    apiBase = trimmed
  }
}

/** Reset to default localhost. */
export function resetApiBase(): void {
  apiBase = DEFAULT_API_BASE
}

export class NetworkError extends Error {
  constructor() {
    super("No se pudo conectar al servidor. ¿Está corriendo en localhost:8080?")
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
 * Send a `multipart/form-data` body and read its JSON response.
 *
 * Kept apart from `request` because only the browser may write the multipart
 * Content-Type: it appends the boundary, and a hand-written header would make
 * the server reject the body. Everything else — the base URL, the network error
 * and the `ApiError` mapping — behaves exactly like `request`.
 */
export async function requestMultipart<T>(
  path: string,
  body: FormData,
  init?: Omit<RequestInit, "body" | "headers">,
): Promise<T> {
  let response: Response

  try {
    response = await fetch(`${getApiBase()}${path}`, {
      method: "POST",
      ...init,
      body,
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
