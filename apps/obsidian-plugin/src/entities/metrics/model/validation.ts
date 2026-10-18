import type { StudyMetrics } from "./types"
import { isNonNegativeInteger, isRate } from "@/shared/lib/guards"

export function isValidStudyMetrics(metrics: StudyMetrics): boolean {
  return (
    isNonNegativeInteger(metrics.dailyCardCount) &&
    isRate(metrics.retentionRate) &&
    isNonNegativeInteger(metrics.currentStreak) &&
    isNonNegativeInteger(metrics.totalReviewed)
  )
}
