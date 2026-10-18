import type {
  SyncResourceRequest,
  SyncResourceResponse,
} from "../model/types"
import { request } from "@/shared/api/client"

export async function syncResource(
  req: SyncResourceRequest,
): Promise<SyncResourceResponse> {
  return request<SyncResourceResponse>("/api/sync/resource", {
    method: "POST",
    body: JSON.stringify(req),
  })
}
