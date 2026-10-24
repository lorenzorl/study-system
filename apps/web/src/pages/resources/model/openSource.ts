/**
 * Where a material's `source_uri` has to be opened.
 *
 * The middleware stores one opaque string: an external URL for an article or a
 * video, and a source this app cannot open for everything else — a corpus
 * document captured here (`rag://documents/<id>`) or a legacy Obsidian vault
 * path. The launcher cannot send either of those to the browser, so the decision
 * is made here and not inside the click handler.
 */
export type SourceTarget = { kind: "external" | "corpus"; uri: string } | null

export function sourceTargetOf(uri: string): SourceTarget {
  const trimmed = uri.trim()

  // An empty source is not a corpus material: it names nothing to open, which is
  // a different answer from "it lives in the corpus".
  if (trimmed === "") return null

  if (/^https?:\/\//i.test(trimmed)) return { kind: "external", uri: trimmed }

  return { kind: "corpus", uri: trimmed }
}
