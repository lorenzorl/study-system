export type ResourceType = "book" | "note" | "article" | "video"

export interface SyncResourceRequest {
  topic_name: string
  resource_title: string
  type: ResourceType
  source_uri: string
}

export interface SyncResourceResponse {
  resource_id: string
}
