<template>
  <div class="dashboard">
    <MetricsBar />
    <DailyReviewCard
      v-if="showCTA"
      @start="handleDailyReview"
    />

    <section class="dashboard__section" v-if="domains.length > 0">
      <h2 class="dashboard__section-title">Domains</h2>
      <div class="dashboard__grid">
        <DomainCard
          v-for="domain in domains"
          :key="domain.id"
          :domain="domain"
          @select="goToDomain"
        />
      </div>
    </section>

    <div v-else class="dashboard__empty">
      <p>No domains available. Start by creating study domains.</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useStudyStore } from "../stores/study-store";
import { useMetricsStore } from "../stores/metrics-store";
import { storeToRefs } from "pinia";
import MetricsBar from "../components/MetricsBar.vue";
import DailyReviewCard from "../components/DailyReviewCard.vue";
import DomainCard from "../components/DomainCard.vue";

const router = useRouter();
const studyStore = useStudyStore();
const metricsStore = useMetricsStore();

const { domains } = storeToRefs(studyStore);
const { dailyCardCount } = storeToRefs(metricsStore);

const showCTA = computed(() => dailyCardCount.value > 0);

function goToDomain(id: string) {
  studyStore.selectDomain(id);
  router.push({ name: "domain", params: { domainId: id } });
}

function handleDailyReview() {
  // Navigate to first domain's first concept flashcards
  if (domains.value.length > 0 && domains.value[0].concepts.length > 0) {
    const domain = domains.value[0];
    const concept = domain.concepts[0];
    studyStore.selectDomain(domain.id);
    studyStore.selectConcept(concept.id);
    router.push({
      name: "flashcards",
      params: { domainId: domain.id, conceptId: concept.id },
    });
  }
}
</script>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.dashboard__section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dashboard__section-title {
  margin: 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.dashboard__grid {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dashboard__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
