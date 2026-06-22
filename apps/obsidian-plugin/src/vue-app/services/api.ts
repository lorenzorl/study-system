import type {
  TopicResponse,
  SyncConceptRequest,
  SyncConceptResponse,
  SyncFlashcardsRequest,
  SyncFlashcardsResponse,
} from "../../types"

const API_BASE = "http://localhost:8080"

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

async function request<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  let response: Response

  try {
    response = await fetch(`${API_BASE}${path}`, {
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

export async function fetchTopics(): Promise<TopicResponse[]> {
  return request<TopicResponse[]>("/api/concepts")
}

export async function syncConcept(
  req: SyncConceptRequest,
): Promise<SyncConceptResponse> {
  return request<SyncConceptResponse>("/api/sync/concept", {
    method: "POST",
    body: JSON.stringify(req),
  })
}

export async function syncFlashcards(
  req: SyncFlashcardsRequest,
): Promise<SyncFlashcardsResponse> {
  return request<SyncFlashcardsResponse>("/api/sync/flashcards", {
    method: "POST",
    body: JSON.stringify(req),
  })
}
