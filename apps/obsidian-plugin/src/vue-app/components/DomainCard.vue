<template>
  <button class="domain-card" @click="$emit('select', domain.id)">
    <div class="domain-card__content">
      <h3 class="domain-card__name">{{ domain.name }}</h3>
      <p class="domain-card__description">{{ domain.description }}</p>
    </div>
    <span class="domain-card__count">
      {{ domain.concepts.length }} {{ domain.concepts.length === 1 ? 'concepto' : 'conceptos' }}
    </span>
  </button>
</template>

<script setup lang="ts">
import type { Domain } from "../../types";

defineProps<{
  domain: Domain;
}>();

defineEmits<{
  select: [id: string];
}>();
</script>

<style scoped>
.domain-card {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  width: 100%;
  min-height: 44px;
  height: auto;
  padding: 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease;
}

.domain-card:hover {
  border-color: var(--interactive-accent);
}

.domain-card__content {
  flex: 1;
  min-width: 0;
}

.domain-card__name {
  margin: 0 0 0.25rem;
  font-size: 0.95rem;
  font-weight: 600;
}

.domain-card__description {
  margin: 0;
  font-size: 0.82rem;
  color: var(--text-muted);
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.domain-card__count {
  font-size: 0.75rem;
  color: var(--text-muted);
  min-width: 44px;
  text-align: right;
  line-height: 44px;
}
</style>
