import type { ConceptSummary } from "./types"
import { isNonEmptyString } from "@/shared/lib/guards"

export function isValidConceptSummary(concept: ConceptSummary): boolean {
  return (
    isNonEmptyString(concept.id) &&
    isNonEmptyString(concept.title) &&
    isNonEmptyString(concept.file_path)
  )
}
