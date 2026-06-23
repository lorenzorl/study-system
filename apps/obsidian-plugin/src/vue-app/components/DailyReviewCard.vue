<template>
  <button class="daily-review" @click="goToDueCards">
    <span class="daily-review__text">
      {{ label }}
    </span>
    <span class="daily-review__arrow">→</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { useRouter } from "vue-router"

const props = defineProps<{
  dueCount: number
  loading?: boolean
}>()

const router = useRouter()

const label = computed(() => {
  if (props.loading) return "Cargando..."
  const count = props.dueCount
  if (count === 0) return "Sin tarjetas para repasar"
  return `Estudiar ${count} tarjeta${count === 1 ? "" : "s"} hoy`
})

function goToDueCards() {
  router.push({ name: "due-cards" })
}
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
