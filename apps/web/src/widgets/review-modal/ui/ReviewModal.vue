<template>
  <!-- Overlay sits above the launcher; the dialog is the only tabbable surface. -->
  <div class="review-modal">
    <div
      ref="dialogEl"
      class="review-modal__dialog"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
    >
      <header class="review-modal__header">
        <h2 class="review-modal__title">Repaso · {{ title }}</h2>
        <p v-if="total > 0" class="review-modal__progress">{{ currentIndex + 1 }} / {{ total }}</p>
      </header>

      <!-- Defensive: the launcher never opens an empty deck, but a zero-card
           session must not masquerade as a completed one. -->
      <div v-if="total === 0" class="review-modal__complete">
        <p class="review-modal__complete-title">No hay tarjetas para repasar</p>
        <div class="review-modal__complete-actions">
          <button
            type="button"
            class="review-modal__btn review-modal__btn--secondary"
            @click="requestClose"
          >
            Cerrar
          </button>
        </div>
      </div>

      <template v-else-if="!isComplete && currentCard">
        <!--
          The card owns the flip: the modal only listens to `flip` and focuses the
          card's own button so Space/Enter reach it.
        -->
        <div ref="cardHost" class="review-modal__card">
          <FlashcardCard
            :key="currentCard.id"
            :card="currentCard"
            @flip="onFlip"
          />
        </div>

        <div class="review-modal__actions" role="group" aria-label="Calificar">
          <button
            v-for="option in GRADES"
            :key="option.value"
            type="button"
            class="review-modal__grade"
            :disabled="!isRevealed"
            @click="handleGrade(option.value)"
          >
            {{ option.value }} {{ option.label }}
          </button>
        </div>
      </template>

      <div v-else ref="completeEl" class="review-modal__complete">
        <p class="review-modal__complete-title">Repaso completado</p>
        <p class="review-modal__complete-progress">
          {{ total }} de {{ total }}
        </p>
        <div class="review-modal__complete-actions">
          <button type="button" class="review-modal__btn" @click="restart">
            Repasar de nuevo
          </button>
          <button
            type="button"
            class="review-modal__btn review-modal__btn--secondary"
            @click="requestClose"
          >
            Cerrar
          </button>
        </div>
      </div>

      <footer class="review-modal__footer">
        <span class="review-modal__source"><slot name="source" /></span>
        <span class="review-modal__hint">Esc para salir</span>
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue"
import { FlashcardCard } from "@/entities/flashcard"
import { useCardSession } from "../model/useCardSession"
import type { ReviewCard } from "../model/useCardSession"

const props = withDefaults(
  defineProps<{
    cards: ReviewCard[]
    title: string
    initialIndex?: number
  }>(),
  { initialIndex: 0 },
)

const emit = defineEmits<{
  close: []
  grade: [{ cardId: string; grade: number }]
}>()

const GRADES = [
  { value: 1, label: "Otra vez" },
  { value: 2, label: "Difícil" },
  { value: 3, label: "Bien" },
  { value: 4, label: "Fácil" },
] as const

const {
  currentCard,
  currentIndex,
  total,
  isRevealed,
  isComplete,
  setRevealed,
  grade,
  restart,
} = useCardSession(props.cards, props.initialIndex)

const dialogEl = ref<HTMLElement | null>(null)
const cardHost = ref<HTMLElement | null>(null)
const completeEl = ref<HTMLElement | null>(null)

/**
 * The card's own interactive element. `FlashcardCard` renders a single button as
 * its root, so this reaches it without exposing anything on the entity.
 */
function cardButton(): HTMLButtonElement | null {
  return cardHost.value?.querySelector<HTMLButtonElement>("button") ?? null
}

function onFlip(flipped: boolean) {
  // The card owns the flip, so the session mirrors it in both directions:
  // flipping back hides the answer, and grading must follow the answer, not the
  // fact that it was revealed once.
  setRevealed(flipped)
}

function handleGrade(value: number) {
  if (!isRevealed.value || isComplete.value) return

  const card = currentCard.value
  if (!card) return

  // Read the card id before grading, since grading advances or completes.
  grade(value)
  emit("grade", { cardId: card.id, grade: value })
}

function requestClose() {
  emit("close")
}

/** Reveal through the card's own button, so the flip stays owned by the entity. */
function activateCard() {
  if (isRevealed.value || isComplete.value) return
  cardButton()?.click()
}

function isSpaceOrEnter(event: KeyboardEvent): boolean {
  return event.key === " " || event.key === "Spacebar" || event.key === "Enter"
}

/** Keys only act while the dialog (or something inside it) has focus. */
function focusIsInsideDialog(): boolean {
  const focused = document.activeElement
  return !!focused && !!focused.closest('[role="dialog"]')
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    event.preventDefault()
    requestClose()
    return
  }

  if (isComplete.value) return

  if (!focusIsInsideDialog()) return

  if (!isRevealed.value) {
    // Space/Enter reveal; preventDefault stops the native click that would flip twice.
    if (isSpaceOrEnter(event)) {
      event.preventDefault()
      activateCard()
    }
    return
  }

  if (GRADES.some((option) => String(option.value) === event.key)) {
    event.preventDefault()
    handleGrade(Number(event.key))
    return
  }

  // While revealed, keep the card from flipping back under the grading row.
  if (isSpaceOrEnter(event) && event.target === cardButton()) {
    event.preventDefault()
  }
}

onMounted(() => {
  window.addEventListener("keydown", onKeydown)
  // Focus the card itself, so the keyboard can reveal it immediately.
  ;(cardButton() ?? dialogEl.value)?.focus()
})

// Advancing replaces the card (and disables the grade buttons), so the keyboard
// needs focus back on the card to keep revealing with Space/Enter.
watch(currentIndex, () => {
  void nextTick(() => cardButton()?.focus())
})

// Completing unmounts the card, so move focus to the first completion action.
watch(isComplete, (complete) => {
  if (!complete) {
    // Restarting a one-card deck keeps the index, so no `currentIndex` change
    // fires; focus has to come back to the card from here.
    void nextTick(() => cardButton()?.focus())
    return
  }
  void nextTick(() =>
    completeEl.value?.querySelector<HTMLButtonElement>("button")?.focus(),
  )
})

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown)
})
</script>

<style scoped>
.review-modal {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  /* Neutral scrim: Obsidian has no variable for modal dimming. */
  background: rgba(0, 0, 0, 0.5);
}

.review-modal__dialog {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  width: 100%;
  max-width: 640px;
  max-height: 100%;
  overflow-y: auto;
  padding: 1rem 1.125rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 10px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
  box-shadow: 0 8px 24px var(--background-modifier-box-shadow, rgba(0, 0, 0, 0.3));
}

.review-modal__header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem;
}

.review-modal__title {
  margin: 0;
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--text-normal);
}

.review-modal__progress {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.review-modal__card {
  display: flex;
}

.review-modal__actions {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0.5rem;
}

.review-modal__grade {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 0.375rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-family: inherit;
  font-size: 0.85rem;
  font-weight: 500;
  transition: background-color 0.15s ease, opacity 0.15s ease;
}

.review-modal__grade:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.review-modal__grade:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.review-modal__complete {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 1.5rem 0.5rem;
  text-align: center;
}

.review-modal__complete-title {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 600;
  color: var(--text-normal);
}

.review-modal__complete-progress {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-muted);
}

.review-modal__complete-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.5rem;
  width: 100%;
  margin-top: 0.5rem;
}

.review-modal__btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, var(--background-primary));
  cursor: pointer;
  font-family: inherit;
  font-size: 0.85rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.review-modal__btn:hover {
  opacity: 0.85;
}

.review-modal__btn--secondary {
  border-color: var(--background-modifier-border);
  background: var(--background-secondary);
  color: var(--text-normal);
}

.review-modal__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.review-modal__source {
  color: var(--text-accent);
}

.review-modal__hint {
  margin-left: auto;
}

@media (max-width: 520px) {
  .review-modal {
    padding: 0;
    align-items: stretch;
  }

  .review-modal__dialog {
    max-width: none;
    border: none;
    border-radius: 0;
  }

  /* The action row wraps to two lines instead of shrinking the labels. */
  .review-modal__actions {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
