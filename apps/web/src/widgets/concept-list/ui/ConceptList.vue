<template>
  <div class="concept-list">
    <ConceptCard
      v-for="concept in concepts"
      :key="concept.id"
      :concept="concept"
      :flashcard-count="flashcardCache.getCachedFlashcards(concept.id).length"
      @select="$emit('select', concept.id)"
    />
  </div>
</template>

<script setup lang="ts">
import { ConceptCard } from "@/entities/concept"
import type { ConceptSummary } from "@/entities/concept"
import { useFlashcardCache } from "@/entities/flashcard"

defineProps<{
  concepts: ConceptSummary[]
}>()

defineEmits<{
  select: [id: string]
}>()

const flashcardCache = useFlashcardCache()
</script>

<style scoped>
.concept-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
</style>
