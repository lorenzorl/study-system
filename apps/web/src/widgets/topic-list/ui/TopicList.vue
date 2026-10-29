<template>
  <div class="topic-list">
    <TopicCard
      v-for="topic in topics"
      :key="topic.id"
      :topic="topic"
      @select="$emit('select', topic.id)"
    />
  </div>
</template>

<script setup lang="ts">
import { TopicCard } from "@/entities/topic"
import type { Topic } from "@/entities/topic"

defineProps<{
  topics: Topic[]
}>()

defineEmits<{
  select: [id: string]
}>()
</script>

<style scoped>
.topic-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
</style>
