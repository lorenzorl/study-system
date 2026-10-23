import type { ResourceResponse } from "../model/types"
import { request } from "@/shared/api/client"

export async function fetchResources(): Promise<ResourceResponse[]> {
  return request<ResourceResponse[]>("/api/resources")
}
