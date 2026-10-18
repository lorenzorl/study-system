// Public API of the `chat` entity slice.
export type {
  Chat,
  ChatNote,
  ChatNoteSettings,
  ChatTurn,
  ChatTurnRole,
} from "./model/types"
export { useChatsStore } from "./model/useChatsStore"
export {
  appendTurn,
  createChatNote,
  listChatPaths,
  readChatNote,
  subscribeToChats,
  writeChatPrompt,
  writeChatSettings,
} from "./api/chat-note"
