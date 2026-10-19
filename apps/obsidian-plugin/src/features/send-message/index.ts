// Public API of the `send-message` feature slice.
export { useSendMessage } from "./model/useSendMessage"
export { default as MessageComposer } from "./ui/MessageComposer.vue"
export {
  CITATION_FRAME,
  SEARCHABLE_MATERIAL_FRAME,
  buildContext,
  buildMessages,
  buildSearchableSystemMessage,
  buildSources,
  buildSystemMessage,
  trimHistory,
} from "./model/prompt"
export type { ContextBlock, MaterialMode, PromptInput, PromptTurn } from "./model/prompt"
