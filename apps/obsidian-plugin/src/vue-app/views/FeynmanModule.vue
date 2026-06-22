<template>
  <div class="feynman">
    <div v-if="concept" class="feynman__content">
      <div class="feynman__header">
        <h2 class="feynman__name">{{ concept.name }}</h2>
        <p class="feynman__summary">{{ concept.summary }}</p>
      </div>

      <div class="feynman__instructions">
        <p>
          Explicá este concepto con tus propias palabras, como si se lo enseñaras a
          alguien nuevo. Esto revela lagunas en tu comprensión.
        </p>
      </div>

      <textarea
        v-model="userExplanation"
        class="feynman__textarea"
        placeholder="Escribe tu explicación aquí..."
        rows="6"
        :aria-label="'Explicá ' + concept.name + ' con tus propias palabras'"
      ></textarea>

      <button
        class="feynman__submit"
        @click="explain"
        :disabled="!userExplanation.trim()"
      >
        Explicar este concepto
      </button>

      <div v-if="response" class="feynman__response">
        <h3 class="feynman__response-title">Retroalimentación</h3>
        <p class="feynman__response-text">{{ response }}</p>
      </div>
    </div>

    <div v-else class="feynman__empty">
      <p>Concepto no encontrado.</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useStudyStore } from "../stores/study-store";
import { storeToRefs } from "pinia";

const props = defineProps<{
  domainId: string;
  conceptId: string;
}>();

const studyStore = useStudyStore();
const { currentConcept: concept } = storeToRefs(studyStore);

const userExplanation = ref("");
const response = ref("");

onMounted(() => {
  studyStore.selectDomain(props.domainId);
  studyStore.selectConcept(props.conceptId);
});

function explain() {
  if (!userExplanation.value.trim() || !concept.value) return;

  // Mock response — real RAG-powered analysis coming in v2.1
  response.value = [
    `Great explanation! Here's a summary of what you covered about "${concept.value.name}":`,
    "",
    "✓ You engaged with the concept in your own words — this strengthens neural pathways.",
    "✓ Self-explanation is one of the most effective study techniques (meta-analysis by Dunlosky et al., 2013).",
    "",
    "[This is a placeholder response. RAG-powered analysis with gap detection and guided follow-up is planned for v2.1.]",
  ].join("\n");
}
</script>

<style scoped>
.feynman {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.feynman__header {
  margin-bottom: 0.25rem;
}

.feynman__name {
  margin: 0 0 0.25rem;
  font-size: 1.1rem;
  font-weight: 600;
  min-height: 44px;
  display: flex;
  align-items: center;
}

.feynman__summary {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-muted);
  line-height: 1.5;
}

.feynman__instructions {
  padding: 0.5rem 0.75rem;
  border-left: 3px solid var(--interactive-accent);
  background: var(--background-secondary);
  border-radius: 0 6px 6px 0;
}

.feynman__instructions p {
  margin: 0;
  font-size: 0.82rem;
  color: var(--text-muted);
  line-height: 1.5;
}

.feynman__textarea {
  width: 100%;
  min-height: 120px;
  padding: 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
  font-size: 0.9rem;
  line-height: 1.6;
  resize: vertical;
  box-sizing: border-box;
}

.feynman__textarea:focus {
  outline: none;
  border-color: var(--interactive-accent);
  box-shadow: 0 0 0 2px rgba(var(--interactive-accent-rgb, 100, 100, 255), 0.2);
}

.feynman__submit {
  min-height: 44px;
  min-width: 44px;
  padding: 0.5rem 1.5rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
  align-self: flex-start;
}

.feynman__submit:hover:not(:disabled) {
  opacity: 0.9;
}

.feynman__submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.feynman__response {
  padding: 0.75rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
}

.feynman__response-title {
  margin: 0 0 0.5rem;
  font-size: 0.85rem;
  font-weight: 600;
}

.feynman__response-text {
  margin: 0;
  font-size: 0.82rem;
  color: var(--text-muted);
  line-height: 1.6;
  white-space: pre-wrap;
}

.feynman__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
