<template>
  <!--
    The whole row is the control, so it cannot be a `Card`: the primitive is a
    non-interactive column box, and its frame (`gap-6`, `py-6`, `shadow-sm`) does
    not describe a two-line row that is itself clickable. The frame stays the
    item's own scoped rules; the primitive adds the button behaviour.
  -->
  <Button class="due-card-item" @click="$emit('select')">
    <span class="due-card-item__question">{{ card.question }}</span>
    <div class="due-card-item__meta">
      <span class="due-card-item__concept">{{ card.concept_title }}</span>
      <span class="due-card-item__topic">{{ card.topic_name }}</span>
    </div>
  </Button>
</template>

<script setup lang="ts">
import { Button } from "@/shared/ui/button"
import type { DueCard } from "../model/types"

defineProps<{
  card: DueCard
}>()

defineEmits<{
  select: []
}>()
</script>

<style scoped>
/* The frame: a two-line row, its padding, radius and palette.
 *
 * The last rules are here because the root is the primitive now and it has its own
 * opinion about a button's box: `h-9` would pin the height to 36px (and
 * `min-height` would clamp that to 56, with the row overflowing both ends),
 * `justify-content: center` would centre that overflow, `items-center` and
 * `whitespace-nowrap` would reshape children that used to fill the row and wrap,
 * `text-sm`'s unitless leading would move the meta line a pixel, and its
 * `font-medium` would embolden the concept and topic names that never declared a
 * weight of their own (`--text-muted` at 12px, 410 pixels of the capture). In this
 * card the control *is* the frame, so the frame states its box in full. */
.due-card-item {
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
  align-items: stretch;
  gap: 0.35rem;
  width: 100%;
  height: auto;
  min-height: 56px;
  padding: 0.75rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.9rem;
  font-weight: normal;
  line-height: normal;
  text-align: left;
  white-space: normal;
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
