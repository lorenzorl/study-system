<template>
  <!--
    Same shape as `ConceptCard`: the whole card is the control, so the frame stays
    the card's own (a row, its padding, radius and palette) and the primitive owns
    the button behaviour. `gap: 0` and `white-space: normal` are stated in the frame
    for the same reason — the control *is* the frame, and the topic name has to be
    free to wrap.

    The concept count is a `Badge` (`shadcn-migration.md` T7): it is a count chip,
    not a control. The primitive owns the pill now, in the palette's muted
    foreground, and the card keeps its own 44px touch floor.
  -->
  <Button class="topic-card" @click="$emit('select', topic.id)">
    <div class="topic-card__content">
      <h3 class="topic-card__name">{{ topic.name }}</h3>
    </div>
    <Badge variant="outline" class="text-muted-foreground">
      {{ topic.concepts.length }}
      {{ topic.concepts.length === 1 ? 'concepto' : 'conceptos' }}
    </Badge>
  </Button>
</template>

<script setup lang="ts">
import { Badge } from "@/shared/ui/badge"
import { Button } from "@/shared/ui/button"
import type { Topic } from "../model/types"

defineProps<{
  topic: Topic
}>()

defineEmits<{
  select: [id: string]
}>()
</script>

<style scoped>
/* The frame: one row, the card's own padding, radius and palette. */
.topic-card {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0;
  width: 100%;
  min-height: 44px;
  height: auto;
  padding: 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-weight: normal;
  text-align: left;
  white-space: normal;
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
  line-height: normal;
  font-weight: 600;
}
</style>
