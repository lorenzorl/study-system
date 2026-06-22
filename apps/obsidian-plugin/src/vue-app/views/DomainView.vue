<template>
  <div class="domain-view">
    <div v-if="domain" class="domain-view__content">
      <div class="domain-view__header">
        <h2 class="domain-view__name">{{ domain.name }}</h2>
        <p class="domain-view__description">{{ domain.description }}</p>
      </div>

      <section class="domain-view__section">
        <h3 class="domain-view__section-title">Concepts</h3>
        <div class="domain-view__grid">
          <ConceptCard
            v-for="concept in domain.concepts"
            :key="concept.id"
            :concept="concept"
            @select="goToConcept"
          />
        </div>
      </section>
    </div>

    <div v-else class="domain-view__empty">
      <p>Domain not found.</p>
      <button class="domain-view__back-btn" @click="router.push({ name: 'dashboard' })">
        ← Back to Dashboard
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from "vue";
import { useRouter } from "vue-router";
import { useStudyStore } from "../stores/study-store";
import { storeToRefs } from "pinia";
import ConceptCard from "../components/ConceptCard.vue";

const props = defineProps<{
  domainId: string;
}>();

const router = useRouter();
const studyStore = useStudyStore();
const { currentDomain: domain } = storeToRefs(studyStore);

onMounted(() => {
  studyStore.selectDomain(props.domainId);
});

function goToConcept(conceptId: string) {
  studyStore.selectConcept(conceptId);
  router.push({
    name: "concept",
    params: { domainId: props.domainId, conceptId },
  });
}
</script>

<style scoped>
.domain-view {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.domain-view__header {
  margin-bottom: 0.25rem;
}

.domain-view__name {
  margin: 0 0 0.25rem;
  font-size: 1.1rem;
  font-weight: 600;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.domain-view__description {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-muted);
  line-height: 1.5;
}

.domain-view__section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.domain-view__section-title {
  margin: 0;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.domain-view__grid {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.domain-view__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
}

.domain-view__back-btn {
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

.domain-view__back-btn:hover {
  background: var(--background-modifier-hover);
}
</style>
