<template>
  <button class="daily-review" @click="$emit('start')">
    <span class="daily-review__text">
      {{ label }}
    </span>
    <span class="daily-review__arrow">→</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useMetricsStore } from "../stores/metrics-store";
import { storeToRefs } from "pinia";

defineEmits<{
  start: [];
}>();

const metricsStore = useMetricsStore();
const { dailyCardCount } = storeToRefs(metricsStore);

const label = computed(() => {
  const count = dailyCardCount.value;
  if (count === 0) return "Sin tarjetas para repasar";
  return `Estudiar ${count} tarjeta${count === 1 ? "" : "s"} hoy`;
});
</script>

<style scoped>
.daily-review {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  min-height: 56px;
  padding: 0.75rem 1rem;
  border: 2px solid var(--interactive-accent);
  border-radius: 8px;
  background: var(--background-primary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.95rem;
  font-weight: 500;
  transition: background-color 0.15s ease;
}

.daily-review:hover {
  background: var(--background-modifier-hover);
}

.daily-review__arrow {
  font-size: 1.2rem;
  color: var(--interactive-accent);
  min-width: 44px;
  text-align: center;
}
</style>
