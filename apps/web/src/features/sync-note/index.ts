// Public API of the `sync-note` feature slice.
export { useSyncNote, syncNoteToBackend } from "./model/useSyncNote"
export type {
  SyncNoteInput,
  SyncNotePayload,
  SyncNoteResult,
} from "./model/useSyncNote"
export { noteFilePath, useNoteForm } from "./model/useNoteForm"
export { default as SyncNoteForm } from "./ui/SyncNoteForm.vue"
export { default as SyncNoteTrigger } from "./ui/SyncNoteTrigger.vue"
