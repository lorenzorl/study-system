// Public API of the `document` entity slice.
export type {
  DocumentPageProgress,
  DocumentState,
  DocumentStatus,
  DocumentSummary,
  UploadDocumentResponse,
} from "./model/types"
export {
  fetchDocumentStatus,
  listDocuments,
  uploadDocument,
} from "./api/document-api"
