// Public API of the `document` entity slice.
export type {
  DocumentPageProgress,
  DocumentState,
  DocumentStatus,
  UploadDocumentResponse,
} from "./model/types"
export { fetchDocumentStatus, uploadDocument } from "./api/document-api"
