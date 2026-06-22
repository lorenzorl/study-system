<template>
  <button class="topic-card" @click="$emit('select', topic.id)">
    <div class="topic-card__content">
      <h3 class="topic-card__name">{{ topic.name }}</h3>
    </div>
    <span class="topic-card__count">
      {{ topic.concepts.length }}
      {{ topic.concepts.length === 1 ? 'concepto' : 'conceptos' }}
    </span>
  </button>
</template>

<script setup lang="ts">
import type { Topic } from "../../types"

defineProps<{
  topic: Topic
}>()

defineEmits<{
  select: [id: string]
}>()
</script>

<style scoped>
.topic-card {
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

.topic-card:hover {
  border-color: var(--interactive-accent);
}

.topic-card__content {
  flex: 1;
  min-width: 0;
}

.topic-card__name {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
}

.topic-card__count {
  font-size: 0.75rem;
  color: var(--text-muted);
  min-width: 44px;
  text-align: right;
  line-height: 44px;
}
</style>
