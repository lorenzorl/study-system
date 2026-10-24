import type {
  TopicResponse,
  CreateTopicResponse,
} from "../model/types"
import { ApiError, request } from "@/shared/api/client"

export async function fetchTopics(): Promise<TopicResponse[]> {
  return request<TopicResponse[]>("/api/concepts")
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
