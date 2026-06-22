<template>
  <button
    class="flashcard-card"
    :class="{ 'flashcard-card--flipped': isFlipped }"
    @click="toggle"
    :aria-label="isFlipped ? 'Show question' : 'Show answer'"
  >
    <div class="flashcard-card__inner">
      <div class="flashcard-card__face flashcard-card__front">
        <p class="flashcard-card__text">{{ card.question }}</p>
        <span class="flashcard-card__hint">Tap to reveal</span>
      </div>
      <div class="flashcard-card__face flashcard-card__back">
        <p class="flashcard-card__text">{{ card.answer }}</p>
        <span class="flashcard-card__hint">Tap to hide</span>
      </div>
    </div>
  </button>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import type { Flashcard } from "../../types";

const props = defineProps<{
  card: Flashcard;
}>();

const isFlipped = ref(false);

function toggle() {
  isFlipped.value = !isFlipped.value;
}

// Reset flip state when card changes
watch(
  () => props.card.id,
  () => {
    isFlipped.value = false;
  },
);
</script>

<style scoped>
.flashcard-card {
  width: 100%;
  min-height: 180px;
  padding: 0;
  border: 1px solid var(--background-modifier-border);
  border-radius: 12px;
  background: transparent;
  cursor: pointer;
  overflow: hidden;
}

.flashcard-card__inner {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 180px;
}


.flashcard-card__face {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 1.25rem;
  border-radius: 12px;
  transition: transform 0.4s ease;
}

.flashcard-card__front {
  background: var(--background-secondary);
  color: var(--text-normal);
  transform: translateX(0);
}

.flashcard-card__back {
  background: var(--background-primary-alt, var(--background-secondary));
  color: var(--text-normal);
  transform: translateX(100%);
}

.flashcard-card--flipped .flashcard-card__front {
  transform: translateX(-100%);
}

.flashcard-card--flipped .flashcard-card__back {
  transform: translateX(0);
}

.flashcard-card__text {
  margin: 0 0 0.75rem;
  font-size: 1rem;
  text-align: center;
  line-height: 1.6;
  white-space: normal;
  user-select: none;
}

.flashcard-card__hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  align-items: center;
}
</style>
