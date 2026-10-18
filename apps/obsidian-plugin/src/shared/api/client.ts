export const DEFAULT_API_BASE = "http://localhost:8080"

/** Get the current API base URL. */
export function getApiBase(): string {
  return localStorage.getItem("study-api-base") || DEFAULT_API_BASE
}

/** Override the API base URL at runtime (e.g., for mobile: "http://192.168.1.50:8080"). */
export function setApiBase(url: string): void {
  localStorage.setItem("study-api-base", url)
}

/** Reset to default localhost. */
export function resetApiBase(): void {
  localStorage.removeItem("study-api-base")
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
