<template>
  <div class="dashboard">
    <MetricsBar />
    <DailyReviewCard
      v-if="showCTA"
      @start="handleDailyReview"
    />

    <section class="dashboard__section" v-if="domains.length > 0 || showNewTemaForm">
      <h2 class="dashboard__section-title">
        <span>Temas</span>
        <button
          class="dashboard__add-btn"
          @click="showNewTemaForm = true"
          aria-label="Agregar tema"
        >+</button>
      </h2>

      <form
        v-if="showNewTemaForm"
        class="dashboard__new-tema-form"
        @submit.prevent="saveTema"
      >
        <input
          v-model="newTemaName"
          class="dashboard__input"
          placeholder="Nombre del tema"
          ref="temaNameInput"
          required
        />
        <input
          v-model="newTemaDescription"
          class="dashboard__input"
          placeholder="Descripción (opcional)"
        />
        <div class="dashboard__form-actions">
          <button
            type="submit"
            class="dashboard__btn dashboard__btn--primary"
            :disabled="!newTemaName.trim()"
          >Guardar</button>
          <button
            type="button"
            class="dashboard__btn dashboard__btn--secondary"
            @click="cancelNewTema"
          >Cancelar</button>
        </div>
      </form>

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
      <p>No hay temas aún. ¡Creá el primero!</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from "vue";
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

const showNewTemaForm = ref(false);
const newTemaName = ref("");
const newTemaDescription = ref("");
const temaNameInput = ref<HTMLInputElement | null>(null);

watch(showNewTemaForm, async (val) => {
  if (val) {
    await nextTick();
    temaNameInput.value?.focus();
  }
});

function saveTema() {
  const name = newTemaName.value.trim();
  if (!name) return;
  studyStore.addDomain(name, newTemaDescription.value.trim());
  cancelNewTema();
}

function cancelNewTema() {
  showNewTemaForm.value = false;
  newTemaName.value = "";
  newTemaDescription.value = "";
}

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

.dashboard__add-btn {
  margin-left: auto;
  min-width: 44px;
  min-height: 44px;
  width: 44px;
  height: 44px;
  border: 1px solid var(--background-modifier-border);
  border-radius: 50%;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 1.25rem;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.dashboard__add-btn:hover {
  background: var(--background-modifier-hover);
  border-color: var(--interactive-accent);
}

.dashboard__new-tema-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
}

.dashboard__input {
  min-height: 36px;
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 4px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.dashboard__input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.dashboard__form-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
}

.dashboard__btn {
  min-height: 36px;
  padding: 0.4rem 1rem;
  border-radius: 4px;
  font-size: 0.85rem;
  cursor: pointer;
}

.dashboard__btn--primary {
  border: 1px solid var(--interactive-accent);
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
}

.dashboard__btn--primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.dashboard__btn--secondary {
  border: 1px solid var(--background-modifier-border);
  background: var(--background-secondary);
  color: var(--text-normal);
}
</style>
