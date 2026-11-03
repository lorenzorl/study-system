// Public API of the `explanation` entity slice.
export type {
  Explanation,
  ExplanationFailure,
  ExplanationFailureSource,
  ExplanationResponse,
  ExplainRangeRequest,
  GapState,
  RangeGap,
  RangeGapResponse,
  SettableGapState,
  SetGapStateRequest,
} from "./model/types"
export { useExplanationStore } from "./model/useExplanationStore"
export {
  explainRange,
  fetchExplanation,
  setGapState,
} from "./api/explanation-api"
