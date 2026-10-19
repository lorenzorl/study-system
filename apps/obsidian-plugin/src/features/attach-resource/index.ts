// Public API of the `attach-resource` feature slice.
export {
  MAX_ATTACHMENT_CHARS,
  MAX_TOTAL_ATTACHMENT_CHARS,
  checkAttachmentLimits,
} from "./model/attachment-limits"
export type { AttachmentDecision } from "./model/attachment-limits"
export { attachVaultFile } from "./model/useAttachResource"
export type { AttachResult } from "./model/useAttachResource"
export { useChatAttachments } from "./model/useChatAttachments"
export { default as AttachmentPanel } from "./ui/AttachmentPanel.vue"
