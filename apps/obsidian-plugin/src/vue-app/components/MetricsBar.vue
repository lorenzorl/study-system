<template>
  <div class="metrics-bar">
    <div class="metrics-bar__item">
      <span class="metrics-bar__label">Retención</span>
      <div
        class="metrics-bar__bar"
        role="progressbar"
        :aria-valuenow="displayPercentage"
        aria-valuemin="0"
        aria-valuemax="100"
      >
        <div
          class="metrics-bar__fill"
          :style="{ width: displayPercentage + '%' }"
        ></div>
      </div>
      <span class="metrics-bar__value">{{ displayPercentage }}%</span>
    </div>
    <div class="metrics-bar__item">
      <span class="metrics-bar__label">Racha</span>
      <span class="metrics-bar__streak">{{ streak }} {{ streak === 1 ? 'día' : 'días' }}</span>
    </div>
    <div class="metrics-bar__item">
      <span class="metrics-bar__label">Repasadas</span>
      <span class="metrics-bar__value">{{ total }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useMetricsStore } from "../stores/metrics-store";
import { storeToRefs } from "pinia";

const metricsStore = useMetricsStore();
const { retentionRate, currentStreak, totalReviewed } =
  storeToRefs(metricsStore);

const displayPercentage = computed(() =>
  Math.round(retentionRate.value * 100),
);
const streak = computed(() => currentStreak.value);
const total = computed(() => totalReviewed.value);
</script>

<style scoped>
.metrics-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
}

.metrics-bar__item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  min-width: 44px;
  min-height: 44px;
}

.metrics-bar__item:first-child {
  flex: 1;
}

.metrics-bar__item:nth-child(2) {
  border-left: 1px solid var(--background-modifier-border);
  padding-left: 0.5rem;
}

.metrics-bar__label {
  font-size: 0.75rem;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  min-width: 44px;
}

.metrics-bar__bar {
  flex: 1;
  height: 8px;
  background: var(--background-modifier-border);
  border-radius: 4px;
  overflow: hidden;
  min-width: 60px;
}

.metrics-bar__fill {
  height: 100%;
  background: var(--interactive-accent);
  border-radius: 4px;
  transition: width 0.4s ease;
}

.metrics-bar__value {
  font-size: 0.85rem;
  font-weight: 600;
  min-width: 44px;
  text-align: right;
}

.metrics-bar__streak {
  font-size: 0.85rem;
  font-weight: 600;
  min-width: 44px;
}

@media (max-width: 480px) {
  .metrics-bar__item:first-child {
    flex: 1 1 100%;
  }

  .metrics-bar__item:not(:first-child) {
    flex: 1;
  }

  .metrics-bar__item:nth-child(2) {
    border-left: none;
    border-top: 1px solid var(--background-modifier-border);
    padding-left: 0;
    padding-top: 0.5rem;
  }
}
</style>
