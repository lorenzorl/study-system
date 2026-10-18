import type { Topic } from "./types"
import { isNonEmptyString } from "@/shared/lib/guards"

export function isValidTopic(topic: Topic): boolean {
  return (
    isNonEmptyString(topic.id) &&
    isNonEmptyString(topic.name) &&
    Array.isArray(topic.concepts) &&
    topic.concepts.every(
      (c) =>
        isNonEmptyString(c.id) &&
        isNonEmptyString(c.title) &&
        isNonEmptyString(c.file_path),
    )
  )
}
