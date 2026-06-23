<template>
  <button class="due-card-item" @click="$emit('select')">
    <span class="due-card-item__question">{{ card.question }}</span>
    <div class="due-card-item__meta">
      <span class="due-card-item__concept">{{ card.concept_title }}</span>
      <span class="due-card-item__topic">{{ card.topic_name }}</span>
    </div>
  </button>
</template>

<script setup lang="ts">
import type { DueCard } from "../../types"

defineProps<{
  card: DueCard
}>()

defineEmits<{
  select: []
}>()
</script>

<style scoped>
.due-card-item {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  width: 100%;
  min-height: 56px;
  padding: 0.75rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.9rem;
  text-align: left;
  transition: background-color 0.15s ease;
}

.due-card-item:hover {
  background: var(--background-modifier-hover);
}

.due-card-item__question {
  font-weight: 500;
  line-height: 1.4;
  color: var(--text-normal);
}

.due-card-item__meta {
  display: flex;
  gap: 0.75rem;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.due-card-item__concept {
  min-height: 44px;
  display: flex;
  align-items: center;
}

.due-card-item__topic {
  min-height: 44px;
  display: flex;
  align-items: center;
}
</style>
