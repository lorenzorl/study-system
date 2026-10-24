import type {
  ResourceResponse,
  SyncResourceRequest,
  SyncResourceResponse,
} from "../model/types"
import { request } from "@/shared/api/client"

export async function fetchResources(): Promise<ResourceResponse[]> {
  return request<ResourceResponse[]>("/api/resources")
}

/**
 * Register a material so the launcher lists it.
 *
 * It is a separate request from the corpus write on purpose: `POST
 * /api/sync/resource` links a resource to a topic by name, which is what the
 * launcher's cards show, and the corpus does not know about topics at all.
 */
export async function syncResource(
  input: SyncResourceRequest,
): Promise<SyncResourceResponse> {
  return request<SyncResourceResponse>("/api/sync/resource", {
    method: "POST",
    body: JSON.stringify(input),
  })
}
