<template>
  <!--
    The whole card is the control, and a control cannot be a `Card`: the primitive
    is a non-interactive box whose frame (`gap-6`, `py-6`, `shadow-sm`, a column)
    is the opposite of this card's row with `justify-between`. So the frame stays
    where it was — the card's own scoped rules keep its padding, radius and
    metrics — and what the primitive adds is everything a button has: keyboard
    focus, the disabled state and the touch floor.

    Three of the primitive's own defaults are stated here instead, because in this
    card the control *is* the frame: `gap: 0` (the primitive opens an 8px hole the
    card never had), `white-space: normal` (the primitive's `nowrap` would stop a
    long `file_path` from wrapping) and `font-weight: normal` (its `font-medium`
    would embolden the path, which never declared a weight).

    The card count is a `Badge`, the same shape `TopicCard` uses for its concept
    count: it is a chip, not a control, and the `line-height: 44px` span that used
    to draw it was doing a touch target's job on a label nobody can touch.
  -->
  <Button class="concept-card" @click="$emit('select', concept.id)">
    <div class="concept-card__content">
      <h3 class="concept-card__title">{{ concept.title }}</h3>
      <p class="concept-card__path">{{ concept.file_path }}</p>
    </div>
    <Badge variant="outline" class="text-muted-foreground">
      {{ flashcardCount }} tarjeta{{ flashcardCount === 1 ? '' : 's' }}
    </Badge>
  </Button>
</template>

<script setup lang="ts">
import { Badge } from "@/shared/ui/badge"
import { Button } from "@/shared/ui/button"
import type { ConceptSummary } from "../model/types"

defineProps<{
  concept: ConceptSummary
  flashcardCount: number
}>()

defineEmits<{
  select: [id: string]
}>()
</script>

<style scoped>
/* The frame: one row, the card's own padding, radius and palette. */
.concept-card {
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
  line-height: normal;
  font-weight: 600;
}

.concept-card__path {
  margin: 0;
  font-size: 0.75rem;
  color: var(--text-muted);
  line-height: 1.3;
}
</style>
