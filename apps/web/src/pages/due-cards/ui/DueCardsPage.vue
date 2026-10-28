<template>
  <!--
    Both controls are the `shared/ui` button (`shadcn-migration.md` T8): the retry is
    `variant="outline"` because it is secondary, and "Comenzar repaso" is left at the
    default variant because it is the page's one primary action. `min-h-[44px]` keeps
    the touch target this page has always had as a class, because the primitives are
    sized by utilities and a migration does not shrink a 44px target to 36px.
  -->
  <div class="due-cards-view">
    <!-- Loading -->
    <div v-if="loading" class="due-cards-view__state loading-state">
      <span class="spinner"></span> Cargando...
    </div>

    <!-- Error -->
    <div v-else-if="error" class="due-cards-view__state error-state">
      <p>{{ error }}</p>
      <Button variant="outline" class="min-h-[44px]" @click="retry">Reintentar</Button>
    </div>

    <!-- Empty -->
    <div v-else-if="!hasCards" class="due-cards-view__state">
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
        <Button class="min-h-[44px]" @click="startReview">
          Comenzar repaso
        </Button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from "vue"
import { useRouter } from "vue-router"
import { DueCardItem } from "@/entities/review"
import { useReviewSession } from "@/features/review-card"
import { Button } from "@/shared/ui/button"

const router = useRouter()
const reviewSession = useReviewSession()
const { dueCards, loading, error, totalCount, hasCards } = reviewSession

onMounted(() => {
  reviewSession.fetchDueCards()
})

function retry() {
  reviewSession.fetchDueCards()
}

function startReview() {
  reviewSession.startSession()
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

.due-cards-view__state {
  padding: 2rem 1rem;
  text-align: center;
}

.loading-state {
  color: var(--text-muted);
}

.error-state {
  color: var(--text-error);
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
