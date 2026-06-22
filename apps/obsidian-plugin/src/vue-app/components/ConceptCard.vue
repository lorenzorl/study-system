<template>
  <button class="concept-card" @click="$emit('select', concept.id)">
    <div class="concept-card__content">
      <h3 class="concept-card__title">{{ concept.title }}</h3>
      <p class="concept-card__path">{{ concept.file_path }}</p>
    </div>
    <span class="concept-card__count">
      {{ flashcardCount }} tarjeta{{ flashcardCount === 1 ? '' : 's' }}
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from "vue"
import type { ConceptSummary } from "../../types"
import { useStudyStore } from "../stores/study-store"

const props = defineProps<{
  concept: ConceptSummary
}>()

defineEmits<{
  select: [id: string]
}>()

const store = useStudyStore()

const flashcardCount = computed(() => {
  const cached = store.getCachedFlashcards(props.concept.id)
  return cached.length
})
</script>

<style scoped>
.concept-card {
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

.concept-card:hover {
  border-color: var(--interactive-accent);
}

.concept-card__content {
  flex: 1;
  min-width: 0;
}

.concept-card__title {
  margin: 0 0 0.25rem;
  font-size: 0.9rem;
  font-weight: 600;
}

.concept-card__path {
  margin: 0;
  font-size: 0.75rem;
  color: var(--text-muted);
  line-height: 1.3;
}

.concept-card__count {
  font-size: 0.75rem;
  color: var(--text-muted);
  min-width: 44px;
  text-align: right;
  line-height: 44px;
}
</style>
