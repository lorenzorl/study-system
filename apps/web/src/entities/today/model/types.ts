/**
 * Wire shapes and the page's own view of `GET /api/study/today` (S13).
 *
 * The response is a projection, not a table: `due_count` is a number because a due
 * review has no page range to name, and the entries are two kinds that share only
 * the material they belong to and the range. The domain view below keeps that
 * split, so a `read_not_distilled` entry cannot be asked for a title it will never
 * have and a `plan_next` entry cannot be asked for a `read_at`.
 */

export type TodayEntryKind = "read_not_distilled" | "plan_next"

/** One entry as the handler writes it. `title`, `note` and `read_at` are omitted
 * for the kind that has no such field, and `document_title` may be `""` when the
 * corpus no longer knows the material. */
export interface TodayEntryResponse {
  kind: TodayEntryKind
  document_id: string
  document_title: string
  title?: string
  from_page: number
  to_page: number
  note?: string
  read_at?: string
}

export interface TodayResponse {
  due_count: number
  entries: TodayEntryResponse[]
}

/** The two states `PUT /api/study/reading` stores (S3). */
export type ReadingState = "read" | "unread"

export interface SetReadingRequest {
  document_id: string
  from_page: number
  to_page: number
  state: ReadingState
}

/** The row the upsert answers with; the page never reads it, the store only needs
 * the request to have landed. */
export interface ReadingProgressResponse {
  document_id: string
  from_page: number
  to_page: number
  state: ReadingState
  created_at: string
  updated_at: string
}

interface TodayEntryBase {
  documentId: string
  /** `""` when the material is no longer in the corpus; the id is the fallback. */
  documentTitle: string
  fromPage: number
  toPage: number
}

/** A range marked read that nobody has distilled yet, with the way to explain it. */
export interface ReadNotDistilledEntry extends TodayEntryBase {
  kind: "read_not_distilled"
}

/** The next pending chapter of an active plan: the one entry with a toggle. */
export interface PlanNextEntry extends TodayEntryBase {
  kind: "plan_next"
  title: string
  /** `null` when the plan item carries no note. */
  note: string | null
}

export type TodayEntry = ReadNotDistilledEntry | PlanNextEntry

export interface Today {
  dueCount: number
  entries: TodayEntry[]
}
