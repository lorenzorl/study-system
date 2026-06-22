<template>
  <div class="flashcard-session" ref="sessionEl">
    <!-- Loading / empty concept -->
    <div v-if="!concept" class="flashcard-session__empty">
      <p>Concepto no encontrado.</p>
      <button
        class="flashcard-session__back-btn"
        @click="goBackToConcept"
      >
        ← Volver
      </button>
    </div>

    <!-- No flashcards for this concept -->
    <div v-else-if="flashcards.length === 0" class="flashcard-session__empty">
      <p>No hay tarjetas para este concepto aún.</p>
      <button
        class="flashcard-session__back-btn"
        @click="goBackToConcept"
      >
        ← Volver al Panel
      </button>
    </div>

    <!-- Completion state -->
    <div v-else-if="isComplete" class="flashcard-session__complete">
      <div class="flashcard-session__complete-icon">✅</div>
      <h2 class="flashcard-session__complete-title">¡Sesión Completada!</h2>
      <p class="flashcard-session__complete-text">
        Revisaste las {{ flashcards.length }} tarjeta{{ flashcards.length === 1 ? '' : 's' }}
        de {{ concept.name }}.
      </p>
      <div class="flashcard-session__complete-actions">
        <button class="flashcard-session__btn" @click="restartSession">
          Estudiar de nuevo
        </button>
        <button class="flashcard-session__btn flashcard-session__btn--secondary" @click="goBackToConcept">
          Volver al Panel
        </button>
      </div>
    </div>

    <!-- Active flashcard session -->
    <template v-else>
      <div class="flashcard-session__progress">
        <span>{{ currentIndex + 1 }} / {{ flashcards.length }}</span>
      </div>

      <FlashcardCard :card="flashcards[currentIndex]" />

      <div class="flashcard-session__nav">
        <button
          class="flashcard-session__nav-btn"
          @click="prevCard"
          :disabled="currentIndex === 0"
          :aria-label="'Tarjeta anterior'"
        >
          ← Anterior
        </button>
        <button
          class="flashcard-session__nav-btn"
          @click="nextCard"
          :aria-label="'Siguiente tarjeta'"
        >
          {{ currentIndex === flashcards.length - 1 ? 'Finalizar →' : 'Siguiente →' }}
        </button>
      </div>

      <div class="flashcard-session__keys" v-if="!hasTouch">
        <span class="flashcard-session__key-hint">Teclas ← → para navegar</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from "vue";
import { useRouter } from "vue-router";
import { useStudyStore } from "../stores/study-store";
import { useSwipe } from "../composables/useSwipe";
import { storeToRefs } from "pinia";
import FlashcardCard from "../components/FlashcardCard.vue";

const props = defineProps<{
  domainId: string;
  conceptId: string;
}>();

const router = useRouter();
const studyStore = useStudyStore();
const { currentConcept: concept, currentFlashcards: flashcards } =
  storeToRefs(studyStore);

const sessionEl = ref<HTMLElement | null>(null);
const currentIndex = ref(0);
const isComplete = ref(false);

const hasTouch = computed(() => {
  return typeof window !== "undefined" && "ontouchstart" in window;
});

// Touch swipe support
useSwipe(sessionEl, {
  onSwipeLeft: () => nextCard(),
  onSwipeRight: () => prevCard(),
});

onMounted(() => {
  studyStore.selectDomain(props.domainId);
  studyStore.selectConcept(props.conceptId);

  window.addEventListener("keydown", handleKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleKeydown);
});

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "ArrowRight") {
    e.preventDefault();
    nextCard();
  } else if (e.key === "ArrowLeft") {
    e.preventDefault();
    prevCard();
  }
}

function nextCard() {
  if (currentIndex.value < flashcards.value.length - 1) {
    currentIndex.value++;
  } else {
    isComplete.value = true;
  }
}

function prevCard() {
  if (currentIndex.value > 0) {
    currentIndex.value--;
  }
}

function restartSession() {
  currentIndex.value = 0;
  isComplete.value = false;
}

function goBackToConcept() {
  router.push({
    name: "concept",
    params: { domainId: props.domainId, conceptId: props.conceptId },
  });
}
</script>

<style scoped>
.flashcard-session {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.flashcard-session__progress {
  text-align: center;
  font-size: 0.8rem;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.flashcard-session__nav {
  display: flex;
  gap: 0.5rem;
  justify-content: center;
}

.flashcard-session__nav-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1.25rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.flashcard-session__nav-btn:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.flashcard-session__nav-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.flashcard-session__keys {
  text-align: center;
}

.flashcard-session__key-hint {
  font-size: 0.72rem;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.flashcard-session__empty,
.flashcard-session__complete {
  padding: 2rem 1rem;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
}

.flashcard-session__complete-icon {
  font-size: 2.5rem;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.flashcard-session__complete-title {
  margin: 0;
  font-size: 1.2rem;
  font-weight: 600;
}

.flashcard-session__complete-text {
  margin: 0;
  font-size: 0.9rem;
  color: var(--text-muted);
}

.flashcard-session__complete-actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  width: 100%;
  max-width: 300px;
}

.flashcard-session__btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.flashcard-session__btn:hover {
  opacity: 0.9;
}

.flashcard-session__btn--secondary {
  background: var(--background-secondary);
  color: var(--text-normal);
  border-color: var(--background-modifier-border);
}

.flashcard-session__back-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.flashcard-session__back-btn:hover {
  background: var(--background-modifier-hover);
}
</style>
