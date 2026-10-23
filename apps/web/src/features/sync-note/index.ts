// Public API of the `sync-note` feature slice.
export { useSyncNote, syncNoteToBackend } from "./model/useSyncNote"
export type {
  SyncNoteInput,
  SyncNotePayload,
  SyncNoteResult,
} from "./model/useSyncNote"
export { default as SyncNoteButton } from "./ui/SyncNoteButton.vue"
