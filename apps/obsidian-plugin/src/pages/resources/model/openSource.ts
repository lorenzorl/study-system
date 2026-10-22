/**
 * Where a material's `source_uri` has to be opened.
 *
 * The middleware stores one opaque string: an external URL for an article or a
 * video, a vault path or an `obsidian://` URI for a note or a local file. The
 * launcher cannot send a vault path to the browser, and it cannot send an https
 * URL to the vault, so the decision is made here and not inside the click handler.
 */
export type SourceTarget = { kind: "external" | "vault"; uri: string } | null

export function sourceTargetOf(uri: string): SourceTarget {
  const trimmed = uri.trim()
  if (trimmed === "") return null

  if (/^https?:\/\//i.test(trimmed)) return { kind: "external", uri: trimmed }

  return { kind: "vault", uri: trimmed }
}
