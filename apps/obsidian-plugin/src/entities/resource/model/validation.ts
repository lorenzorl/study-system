import type { SyncResourceRequest, ResourceType } from "./types"
import { isNonEmptyString } from "@/shared/lib/guards"

// ─── SyncResourceRequest Validation ──────────────────────────────────────────

const VALID_RESOURCE_TYPES: ReadonlySet<string> = new Set([
  "book",
  "note",
  "article",
  "video",
])

export function isValidResourceType(type: string): type is ResourceType {
  return VALID_RESOURCE_TYPES.has(type)
}

export function isValidSyncResourceRequest(req: SyncResourceRequest): boolean {
  return (
    isNonEmptyString(req.topic_name) &&
    isNonEmptyString(req.resource_title) &&
    isValidResourceType(req.type) &&
    isNonEmptyString(req.source_uri)
  )
}
