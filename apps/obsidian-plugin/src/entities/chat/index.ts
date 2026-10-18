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
