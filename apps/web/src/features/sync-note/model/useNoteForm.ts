import { computed, ref } from "vue"
import { storeToRefs } from "pinia"
import { useSyncNote } from "./useSyncNote"
import type { SyncNoteResult } from "./useSyncNote"

/**
 * The identity a pasted note is registered under.
 *
 * `POST /api/sync/concept` requires a `file_path` and the server treats it as the
 * concept's identity (`UpsertByPath`), but this app has no vault and no file: the
 * note is text in a textarea. The topic and the concept are exactly what the
 * reader typed, so `<Tema>/<Concepto>.md` is the closest thing to a stable,
 * human-readable name this app can give a note — the same note synced twice keeps
 * one concept, and `GET /api/concepts` shows a path a person can read.
 */
export function noteFilePath(topicName: string, conceptTitle: string): string {
  return `${topicName.trim()}/${conceptTitle.trim()}.md`
}

/**
 * The paste form's rules, kept out of the component: the vitest environment is
 * `node`, so what makes the form submittable, what it sends and what a refusal
 * leaves behind is only pinnable as plain refs.
 */
export function useNoteForm() {
  const store = useSyncNote()
  const { loading, error, panelOpen } = storeToRefs(store)

  const topicName = ref("")
  const conceptTitle = ref("")
  const markdown = ref("")

  // Three non-empty values, trimmed: a blank field would otherwise create a
  // concept with no name, which the server accepts and nobody can find again.
  const canSubmit = computed(
    () =>
      topicName.value.trim() !== "" &&
      conceptTitle.value.trim() !== "" &&
      markdown.value.trim() !== "",
  )

  /** Run the sync; `null` means it was refused and the fields stay as they are. */
  async function submit(): Promise<SyncNoteResult | null> {
    if (!canSubmit.value || loading.value) return null

    const topic = topicName.value.trim()
    const concept = conceptTitle.value.trim()

    return store.syncCurrentNote(
      topic,
      concept,
      noteFilePath(topic, concept),
      markdown.value.trim(),
    )
  }

  /** Drop what was pasted once the note landed: the reader is on its cards. */
  function reset(): void {
    topicName.value = ""
    conceptTitle.value = ""
    markdown.value = ""
  }

  return {
    topicName,
    conceptTitle,
    markdown,
    canSubmit,
    loading,
    error,
    panelOpen,
    openPanel: store.openPanel,
    closePanel: store.closePanel,
    submit,
    reset,
  }
}
