// Public API of the `send-message` feature slice.
export { useSendMessage } from "./model/useSendMessage"
export { default as MessageComposer } from "./ui/MessageComposer.vue"
export {
  CITATION_FRAME,
  buildContext,
  buildMessages,
  buildSources,
  buildSystemMessage,
  trimHistory,
} from "./model/prompt"
export type { ContextBlock, PromptInput, PromptTurn } from "./model/prompt"
