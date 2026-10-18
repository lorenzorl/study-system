// Public API of the `send-message` feature slice.
export { useSendMessage } from "./model/useSendMessage"
export {
  CITATION_FRAME,
  buildContext,
  buildMessages,
  buildSources,
  buildSystemMessage,
  trimHistory,
} from "./model/prompt"
export type { PromptInput, PromptPassage, PromptTurn } from "./model/prompt"
