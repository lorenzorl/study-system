import { defineStore } from "pinia"
import { ref } from "vue"
import { syncResource, useResourcesStore } from "@/entities/resource"
import type { SyncResourceRequest } from "@/entities/resource"
import {
  ingestFile,
  ingestText,
  ingestUrl,
  type IngestedDocument,
} from "../api/corpus-api"

/** Which of the three sources the capture form is filling in. */
export type CaptureMode = "file" | "text" | "url"

/**
 * One capture, as the form hands it over.
 *
 * `topicName` is on every mode because `POST /api/sync/resource` links a
 * resource to a topic by name: the launcher's cards show that name as their
 * context line.
 */
export type CaptureSource =
  | { mode: "file"; topicName: string; file: File }
  | { mode: "text"; topicName: string; title: string; text: string }
  | { mode: "url"; topicName: string; title: string; url: string }

/**
 * The `source_uri` a captured material is registered with.
 *
 * It is a client-side convention, not a server contract. `POST
 * /api/sync/resource` requires a non-empty `source_uri` (and the column is
 * unique), while a material captured here has no address of its own — the corpus
 * document *is* the material. Encoding the document id keeps the row unique per
 * document, which an empty string could not, and lets the launcher tell "lives
 * in the corpus" apart from "a page on the web".
 *
 * Nothing parses this value back: the server never reads it, and the app only
 * asks whether a source is an http(s) URL before opening it.
 */
function corpusSourceUri(documentId: string): string {
  return `rag://documents/${documentId}`
}

/**
 * Name a material after its URL, the way the ingest route does when the caller
 * gives no title: the last path segment, then the host.
 *
 * The registration needs a non-empty title, and a card whose title says
 * something other than the corpus row it came from would describe two different
 * materials, so the same three rules are applied on this side.
 */
function titleForUrl(raw: string): string {
  try {
    const parsed = new URL(raw)
    const segments = parsed.pathname.split("/").filter((part) => part !== "")
    const last = segments.at(-1)

    return last ? decodeURIComponent(last) : parsed.hostname
  } catch {
    return raw
  }
}

/** The registration body for a capture whose document the corpus accepted. */
function registrationOf(
  source: CaptureSource,
  documentId: string,
): SyncResourceRequest {
  const topic_name = source.topicName.trim()

  switch (source.mode) {
    case "file":
      // The upload route names the document after the file, so the card keeps
      // that name: the material and the document are the same thing.
      return {
        topic_name,
        resource_title: source.file.name.trim(),
        type: "book",
        source_uri: corpusSourceUri(documentId),
      }
    case "text":
      return {
        topic_name,
        resource_title: source.title.trim(),
        type: "note",
        source_uri: corpusSourceUri(documentId),
      }
    case "url":
      return {
        topic_name,
        resource_title: source.title.trim() || titleForUrl(source.url.trim()),
        type: "article",
        // The URL the caller typed, not the answer's post-redirect origin: it is
        // the address the reader recognises on the card.
        source_uri: source.url.trim(),
      }
  }
}

/** The corpus write for one mode. */
function ingestOf(source: CaptureSource): Promise<IngestedDocument> {
  switch (source.mode) {
    case "file":
      return ingestFile(source.file)
    case "text":
      return ingestText({ title: source.title.trim(), text: source.text })
    case "url":
      return ingestUrl({ title: source.title.trim(), url: source.url.trim() })
  }
}

/**
 * Add a material to the corpus and list it in the launcher.
 *
 * The store owns the requests; the form owns only its fields. A capture is two
 * writes in a fixed order — the corpus document first, then the resource that
 * points at it — because a registered material whose document is missing would
 * be a card the reader cannot open and a search that finds nothing.
 */
export const useCaptureMaterial = defineStore("capture-material", () => {
  const loading = ref(false)
  const error = ref<string | null>(null)
  const resourcesStore = useResourcesStore()

  async function capture(source: CaptureSource): Promise<boolean> {
    loading.value = true
    error.value = null

    try {
      const document = await ingestOf(source)
      await syncResource(registrationOf(source, document.document_id))
      // One list exists, and `entities/resource` owns it: the capture reloads it
      // instead of appending, so the launcher shows exactly what the server has.
      await resourcesStore.loadResources()

      return true
    } catch (e) {
      // `ApiError` already carries the server's own `error` text, which names the
      // cause (no transcription key, duplicate text, a non-text payload) — a
      // friendlier rewrite here would hide it.
      error.value =
        e instanceof Error
          ? e.message
          : "Error desconocido al agregar el material."

      return false
    } finally {
      loading.value = false
    }
  }

  function clearError(): void {
    error.value = null
  }

  return {
    loading,
    error,
    capture,
    clearError,
  }
})
