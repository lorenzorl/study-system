import type {
  SyncFlashcardsRequest,
  SyncFlashcardsResponse,
} from "../model/types"
import { request } from "@/shared/api/client"

export async function syncFlashcards(
  req: SyncFlashcardsRequest,
): Promise<SyncFlashcardsResponse> {
  return request<SyncFlashcardsResponse>("/api/sync/flashcards", {
    method: "POST",
    body: JSON.stringify(req),
  })
}
