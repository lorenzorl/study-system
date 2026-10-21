import type { Resource, ResourceKindFilter } from "./types"

/**
 * Lowercase and strip diacritics, so `atencion` finds `Atención` and the other
 * way round. NFD splits `ó` into `o` + a combining mark, which is then dropped.
 */
function normalize(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .trim()
}

/**
 * Narrow the launcher list by kind and free-text query (`PRESENTATION-UI.md` §6).
 *
 * The query is matched against the title, the topic and the concept; an empty
 * query keeps everything. The result keeps the input order: ordering is explicit
 * and stable, and filtering must not reshuffle the list under the reader.
 */
export function filterResources(
  resources: readonly Resource[],
  kind: ResourceKindFilter,
  query: string,
): Resource[] {
  const needle = normalize(query)

  return resources.filter((resource) => {
    if (kind !== "all" && resource.kind !== kind) return false
    if (!needle) return true

    return [resource.title, resource.topicTitle, resource.conceptTitle].some(
      (field) => normalize(field).includes(needle),
    )
  })
}
