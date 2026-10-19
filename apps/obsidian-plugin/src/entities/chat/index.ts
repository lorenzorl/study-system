// Public API of the `chat` entity slice.
export type {
  Chat,
  ChatAttachment,
  ChatNote,
  ChatNoteSettings,
  ChatTurn,
  ChatTurnRole,
} from "./model/types"
export { useChatsStore } from "./model/useChatsStore"
export {
  MAX_ATTACHMENT_CHARS,
  MAX_TOTAL_ATTACHMENT_CHARS,
  checkAttachmentLimits,
} from "./lib/attachment-limits"
export type { AttachmentDecision } from "./lib/attachment-limits"
export { setAttachments } from "./lib/note"
export {
  appendTurn,
  createChatNote,
  listChatPaths,
  readChatNote,
  subscribeToChats,
  writeChatPrompt,
  writeChatSettings,
} from "./api/chat-note"
