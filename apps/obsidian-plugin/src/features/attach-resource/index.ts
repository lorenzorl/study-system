// Public API of the `attach-resource` feature slice. The size policy lives in
// `entities/chat`, which owns the attachment, so it is not re-exported here.
export { attachVaultFile } from "./model/useAttachResource"
export type { AttachResult } from "./model/useAttachResource"
export { useChatAttachments } from "./model/useChatAttachments"
export { default as AttachmentPanel } from "./ui/AttachmentPanel.vue"
