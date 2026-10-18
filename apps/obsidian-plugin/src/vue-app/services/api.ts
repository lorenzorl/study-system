import type {
  TopicResponse,
  SyncConceptRequest,
  SyncConceptResponse,
  SyncFlashcardsRequest,
  SyncFlashcardsResponse,
  CreateTopicResponse,
  CreateConceptResponse,
  SyncResourceRequest,
  SyncResourceResponse,
  DueCard,
  ReviewRequest,
  ReviewResponse,
} from "../../types"
import { ApiError, request } from "@/shared/api/client"

// Re-export the shared HTTP client surface so existing importers of this
// module keep working until the Phase 2 entity api segments own these endpoints.
export * from "@/shared/api/client"

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

export async function createTopic(
  name: string,
): Promise<CreateTopicResponse> {
  return request<CreateTopicResponse>("/api/topics", {
    method: "POST",
    body: JSON.stringify({ name }),
  }).catch((e) => {
    if (e instanceof ApiError) {
      if (e.status === 409) {
        throw new ApiError("Ya existe un tema con ese nombre", 409)
      }
      if (e.status === 400) {
        throw new ApiError("El nombre no puede estar vacío", 400)
      }
    }
    throw e
  })
}

export async function createConcept(
  topicId: string,
  title: string,
): Promise<CreateConceptResponse> {
  return request<CreateConceptResponse>("/api/concepts", {
    method: "POST",
    body: JSON.stringify({ topic_id: topicId, title }),
  }).catch((e) => {
    if (e instanceof ApiError) {
      if (e.status === 404) {
        throw new ApiError("El tema no existe", 404)
      }
      if (e.status === 400) {
        throw new ApiError("El título no puede estar vacío", 400)
      }
    }
    throw e
  })
}

export async function syncResource(
  req: SyncResourceRequest,
): Promise<SyncResourceResponse> {
  return request<SyncResourceResponse>("/api/sync/resource", {
    method: "POST",
    body: JSON.stringify(req),
  })
}

export async function fetchDueCards(): Promise<DueCard[]> {
  return request<DueCard[]>("/api/study/due")
}

export async function submitReview(
  req: ReviewRequest,
): Promise<ReviewResponse> {
  return request<ReviewResponse>("/api/study/review", {
    method: "POST",
    body: JSON.stringify(req),
  })
}
