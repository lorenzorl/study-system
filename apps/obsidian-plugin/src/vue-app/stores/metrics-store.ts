import { defineStore } from "pinia";
import { reactive, computed, toRefs } from "vue";
import type { StudyMetrics } from "../../types";
import { MOCK_METRICS } from "../../mock-data";

export const useMetricsStore = defineStore("metrics", () => {
  const state = reactive<StudyMetrics>({ ...MOCK_METRICS });

  const dailyCardCount = computed(() => state.dailyCardCount);
  const retentionRate = computed(() => state.retentionRate);
  const currentStreak = computed(() => state.currentStreak);
  const totalReviewed = computed(() => state.totalReviewed);

  return {
    ...toRefs(state),
    dailyCardCount,
    retentionRate,
    currentStreak,
    totalReviewed,
  };
});
