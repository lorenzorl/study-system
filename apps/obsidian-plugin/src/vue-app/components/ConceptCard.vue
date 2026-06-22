<template>
  <button class="concept-card" @click="$emit('select', concept.id)">
    <div class="concept-card__content">
      <h3 class="concept-card__name">{{ concept.name }}</h3>
      <p class="concept-card__summary">{{ concept.summary }}</p>
    </div>
    <span class="concept-card__count">
      {{ concept.flashcards.length }} cards
    </span>
  </button>
</template>

<script setup lang="ts">
import type { Concept } from "../../types";

defineProps<{
  concept: Concept;
}>();

defineEmits<{
  select: [id: string];
}>();
</script>

<style scoped>
.concept-card {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  width: 100%;
  min-height: 44px;
  padding: 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease;
}

.concept-card:hover {
  border-color: var(--interactive-accent);
}

.concept-card__content {
  flex: 1;
  min-width: 0;
}

.concept-card__name {
  margin: 0 0 0.25rem;
  font-size: 0.9rem;
  font-weight: 600;
}

.concept-card__summary {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.concept-card__count {
  font-size: 0.75rem;
  color: var(--text-muted);
  min-width: 44px;
  text-align: right;
  line-height: 44px;
}
</style>
