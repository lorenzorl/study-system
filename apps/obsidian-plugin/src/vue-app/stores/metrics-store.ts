import { defineStore } from "pinia"
import { reactive, computed, toRefs } from "vue"
import type { StudyMetrics } from "../../types"

const MOCK_METRICS: StudyMetrics = {
  dailyCardCount: 0,
  retentionRate: 0.78,
  currentStreak: 12,
  totalReviewed: 280,
}

export const useMetricsStore = defineStore("metrics", () => {
  const state = reactive<StudyMetrics>({ ...MOCK_METRICS })

  const dailyCardCount = computed(() => state.dailyCardCount)
  const retentionRate = computed(() => state.retentionRate)
  const currentStreak = computed(() => state.currentStreak)
  const totalReviewed = computed(() => state.totalReviewed)

  // Placeholder for future GET /api/metrics endpoint
  async function fetchMetrics(): Promise<void> {
    // No-op: stub for future implementation
  }

  return {
    ...toRefs(state),
    dailyCardCount,
    retentionRate,
    currentStreak,
    totalReviewed,
    fetchMetrics,
  }
})
