import type {
  SyncConceptRequest,
  SyncConceptResponse,
  CreateConceptResponse,
} from "../model/types"
import { ApiError, request } from "@/shared/api/client"

export async function syncConcept(
  req: SyncConceptRequest,
): Promise<SyncConceptResponse> {
  return request<SyncConceptResponse>("/api/sync/concept", {
    method: "POST",
    body: JSON.stringify(req),
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
