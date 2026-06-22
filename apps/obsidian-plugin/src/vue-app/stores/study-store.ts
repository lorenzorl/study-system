import { defineStore } from "pinia";
import { ref, computed } from "vue";
import type { Domain, Concept, Flashcard } from "../../types";
import { MOCK_DOMAINS } from "../../mock-data";

export const useStudyStore = defineStore("study", () => {
  const domains = ref<Domain[]>(MOCK_DOMAINS);
  const currentDomainId = ref<string | null>(null);
  const currentConceptId = ref<string | null>(null);

  const currentDomain = computed<Domain | null>(() => {
    if (!currentDomainId.value) return null;
    return domains.value.find((d) => d.id === currentDomainId.value) ?? null;
  });

  const currentConcept = computed<Concept | null>(() => {
    if (!currentDomain.value || !currentConceptId.value) return null;
    return (
      currentDomain.value.concepts.find(
        (c) => c.id === currentConceptId.value,
      ) ?? null
    );
  });

  const currentFlashcards = computed<Flashcard[]>(() => {
    if (!currentConcept.value) return [];
    return currentConcept.value.flashcards;
  });

  function selectDomain(id: string) {
    currentDomainId.value = id;
    currentConceptId.value = null;
  }

  function selectConcept(id: string) {
    currentConceptId.value = id;
  }

  function addDomain(name: string, description: string) {
    domains.value.push({
      id: crypto.randomUUID(),
      name,
      description,
      concepts: [],
    });
  }

  return {
    domains,
    currentDomainId,
    currentConceptId,
    currentDomain,
    currentConcept,
    currentFlashcards,
    selectDomain,
    selectConcept,
    addDomain,
  };
});
