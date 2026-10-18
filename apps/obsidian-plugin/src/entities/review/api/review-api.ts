import type { DueCard, ReviewRequest, ReviewResponse } from "../model/types"
import { request } from "@/shared/api/client"

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
