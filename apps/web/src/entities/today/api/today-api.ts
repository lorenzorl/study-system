import type {
  ReadingProgressResponse,
  SetReadingRequest,
  TodayResponse,
} from "../model/types"
import { request } from "@/shared/api/client"

/** The projection of what to do today (S13). Read-only: it writes nothing. */
export async function fetchToday(): Promise<TodayResponse> {
  return request<TodayResponse>("/api/study/today")
}

/**
 * Mark one range read or unread (S3). It is an upsert, so `unread` is stored as a
 * state and not written by deleting the row.
 */
export async function setReading(
  req: SetReadingRequest,
): Promise<ReadingProgressResponse> {
  return request<ReadingProgressResponse>("/api/study/reading", {
    method: "PUT",
    body: JSON.stringify(req),
  })
}
