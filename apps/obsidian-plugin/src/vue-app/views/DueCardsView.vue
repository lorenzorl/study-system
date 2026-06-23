<template>
  <div class="due-cards-view">
    <!-- Loading -->
    <div v-if="loading" class="due-cards-view__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <!-- Error -->
    <div v-else-if="error" class="due-cards-view__state error-state">
      <p>{{ error }}</p>
      <button class="due-cards-view__retry-btn" @click="retry">Reintentar</button>
    </div>

    <!-- Empty -->
    <div v-else-if="!hasCards" class="due-cards-view__state empty-state">
      <p>No hay tarjetas para repasar hoy.</p>
    </div>

    <!-- Card list -->
    <template v-else>
      <div class="due-cards-view__header">
        <span class="due-cards-view__count">{{ totalCount }} tarjeta{{ totalCount === 1 ? '' : 's' }} para repasar hoy</span>
      </div>

      <div class="due-cards-view__list">
        <DueCardItem
          v-for="card in dueCards"
          :key="card.flashcard_id"
          :card="card"
          @select="startReview"
        />
      </div>

      <div class="due-cards-view__actions">
        <button class="due-cards-view__start-btn" @click="startReview">
          Comenzar repaso
        </button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from "vue"
import { useRouter } from "vue-router"
import { useReviewStore } from "../stores/review-store"
import { storeToRefs } from "pinia"
import DueCardItem from "../components/DueCardItem.vue"

const router = useRouter()
const reviewStore = useReviewStore()
const { dueCards, loading, error, totalCount, hasCards } =
  storeToRefs(reviewStore)

onMounted(() => {
  reviewStore.fetchDueCards()
})

function retry() {
  reviewStore.fetchDueCards()
}

function startReview() {
  reviewStore.startSession()
  router.push({ name: "review" })
}
</script>

<style scoped>
.due-cards-view {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.due-cards-view__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 44px;
}

.due-cards-view__count {
  font-size: 0.85rem;
  color: var(--text-muted);
  font-weight: 500;
}

.due-cards-view__list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.due-cards-view__actions {
  display: flex;
  justify-content: center;
  padding-top: 0.5rem;
}

.due-cards-view__start-btn {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1.5rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.9rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.due-cards-view__start-btn:hover {
  opacity: 0.9;
}

.due-cards-view__retry-btn {
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

.due-cards-view__retry-btn:hover {
  background: var(--background-modifier-hover);
}

.due-cards-view__state {
  padding: 2rem 1rem;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
}

.loading-state {
  color: var(--text-muted);
}

.error-state {
  color: var(--text-error);
}

.empty-state {
  color: var(--text-muted);
}

.spinner {
  display: inline-block;
  width: 20px;
  height: 20px;
  border: 2px solid var(--background-modifier-border);
  border-top: 2px solid var(--interactive-accent);
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
