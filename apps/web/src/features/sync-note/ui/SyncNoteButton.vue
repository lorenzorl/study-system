<template>
  <button
    class="dashboard__sync-btn"
    @click="emit('sync')"
    aria-label="Sincronizar nota"
    title="Sincronizar nota activa"
  >
    Sincronizar nota
  </button>
</template>

<script setup lang="ts">
const emit = defineEmits<{
  (e: "sync"): void
}>()
</script>

<style scoped>
.dashboard__sync-btn {
  margin-left: auto;
  min-height: 44px;
  padding: 0.25rem 0.75rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.8rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.dashboard__sync-btn:hover {
  opacity: 0.9;
}
</style>
