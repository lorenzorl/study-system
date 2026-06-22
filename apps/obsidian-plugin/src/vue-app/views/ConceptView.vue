<template>
  <div class="concept-view">
    <div v-if="concept" class="concept-view__content">
      <div class="concept-view__header">
        <h2 class="concept-view__name">{{ concept.name }}</h2>
        <p class="concept-view__summary">{{ concept.summary }}</p>
      </div>

      <div class="concept-view__actions">
        <button
          class="concept-view__action"
          @click="navigateTo('flashcards')"
          :disabled="concept.flashcards.length === 0"
        >
          <span class="concept-view__action-icon">🃏</span>
          <span class="concept-view__action-text">
            Flashcards
            <small class="concept-view__action-meta">
              {{ concept.flashcards.length }} cards
            </small>
          </span>
        </button>

        <button class="concept-view__action" @click="navigateTo('feynman')">
          <span class="concept-view__action-icon">📝</span>
          <span class="concept-view__action-text">
            Feynman Technique
            <small class="concept-view__action-meta">
              Explain in your own words
            </small>
          </span>
        </button>
      </div>
    </div>

    <div v-else class="concept-view__empty">
      <p>Concept not found.</p>
      <button
        class="concept-view__back-btn"
        @click="router.push({ name: 'domain', params: { domainId: props.domainId } })"
      >
        ← Back to Domain
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from "vue";
import { useRouter } from "vue-router";
import { useStudyStore } from "../stores/study-store";
import { storeToRefs } from "pinia";

const props = defineProps<{
  domainId: string;
  conceptId: string;
}>();

const router = useRouter();
const studyStore = useStudyStore();
const { currentConcept: concept } = storeToRefs(studyStore);

onMounted(() => {
  studyStore.selectDomain(props.domainId);
  studyStore.selectConcept(props.conceptId);
});

function navigateTo(module: "flashcards" | "feynman") {
  router.push({
    name: module,
    params: { domainId: props.domainId, conceptId: props.conceptId },
  });
}
</script>

<style scoped>
.concept-view {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.concept-view__header {
  margin-bottom: 0.25rem;
}

.concept-view__name {
  margin: 0 0 0.25rem;
  font-size: 1.1rem;
  font-weight: 600;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.concept-view__summary {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-muted);
  line-height: 1.5;
}

.concept-view__actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.concept-view__action {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  min-height: 56px;
  padding: 0.75rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.concept-view__action:hover:not(:disabled) {
  border-color: var(--interactive-accent);
  background: var(--background-modifier-hover);
}

.concept-view__action:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.concept-view__action-icon {
  font-size: 1.3rem;
  min-width: 44px;
  text-align: center;
}

.concept-view__action-text {
  display: flex;
  flex-direction: column;
  font-size: 0.9rem;
  font-weight: 500;
}

.concept-view__action-meta {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-weight: 400;
}

.concept-view__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
}

.concept-view__back-btn {
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

.concept-view__back-btn:hover {
  background: var(--background-modifier-hover);
}
</style>
