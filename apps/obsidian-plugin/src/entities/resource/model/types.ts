/**
 * The stored study resource, as the middleware defines it.
 *
 * `ResourceType` mirrors the four values `POST /api/sync/resource` accepts: the
 * write validates them, so a value outside this union is a server contract
 * break, not a case the launcher has to absorb.
 */
export type ResourceType = "book" | "note" | "article" | "video"

/** The wire shape of one item of `GET /api/resources`. */
export interface ResourceResponse {
  id: string
  title: string
  type: ResourceType
  source_uri: string
  topic_name: string
  created_at: string
}

/**
 * The domain shape the plugin consumes: the wire's snake_case resolved once, at
 * the entity boundary, so no page or widget reads `source_uri` directly.
 */
export interface Resource {
  id: string
  title: string
  type: ResourceType
  sourceUri: string
  topicTitle: string
  createdAt: string
}
