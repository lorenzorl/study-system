import type { Resource, ResourceType } from "@/entities/resource"
import type { MaterialResource } from "./types"

/** A material is opened where it lives; it is never studied in place. */
const MATERIAL_ACTION_LABEL = "Abrir"

const TYPE_LABELS: Record<ResourceType, string> = {
  book: "Libro",
  note: "Nota",
  article: "Artículo",
  video: "Video",
}

const MS_PER_DAY = 86_400_000

/** The stored type as the card's status label. */
export function resourceTypeLabel(type: ResourceType): string {
  return TYPE_LABELS[type]
}

/**
 * Whole days between creation and `now`, or `null` when the timestamp cannot be
 * read.
 *
 * A timestamp in the future (a clock-skewed server) counts as today: the launcher
 * must not print a negative age, and `null` is reserved for "no usable date".
 */
export function addedDaysAgo(createdAt: string, now: Date): number | null {
  const created = Date.parse(createdAt)
  if (Number.isNaN(created)) return null

  const days = Math.floor((now.getTime() - created) / MS_PER_DAY)
  return days < 0 ? 0 : days
}

/**
 * Map the API's resources into launcher cards.
 *
 * The order is the server's, untouched: it is the display order, and filtering
 * must not reshuffle the list under the reader.
 */
export function toMaterialResources(
  resources: readonly Resource[],
  now: Date,
): MaterialResource[] {
  return resources.map((resource) => ({
    id: resource.id,
    kind: "material",
    title: resource.title,
    topicTitle: resource.topicTitle,
    // The middleware links a resource to a topic, never to a concept.
    conceptTitle: "",
    primaryActionLabel: MATERIAL_ACTION_LABEL,
    resourceType: resource.type,
    sourceUri: resource.sourceUri,
    addedDaysAgo: addedDaysAgo(resource.createdAt, now),
  }))
}

/** Per-kind status slot (§2): what the user gains by opening this card. */
export function materialStatus(resource: MaterialResource): string {
  const label = resourceTypeLabel(resource.resourceType)
  if (resource.addedDaysAgo === null) return label
  if (resource.addedDaysAgo === 0) return `${label} · agregado hoy`

  const unit = resource.addedDaysAgo === 1 ? "día" : "días"
  return `${label} · agregado hace ${resource.addedDaysAgo} ${unit}`
}
