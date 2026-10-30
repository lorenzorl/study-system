<template>
  <!--
    The explanation action is the `shared/ui` button (`shadcn-migration.md` T8): it is
    the page's one primary action, so it keeps the default variant, and it keeps the
    44px touch target as a class plus `self-start`, which is the one thing the page
    owned about its placement. The textarea is deliberately untouched — the task is the
    controls the primitives absorb, and the field is its own unit.
  -->
  <div class="feynman">
    <!--
      Three states before the form, and each one says something different
      (`backlog.md` D6). Before this the page had only "found" and "not found", so a
      pending first frame and a failed request both read as a missing concept, with
      nothing to press.
    -->
    <div v-if="topicsLoading || topicsPending" class="feynman__empty">
      <span class="spinner"></span> Cargando...
    </div>

    <!--
      `&& !concept`: an error left over from an earlier failed load must not win
      over a concept that does resolve (`R3-feynman-error-overrides-found`). The
      error is only the answer when there is nothing to show.
    -->
    <div
      v-else-if="topicsError && !concept"
      class="feynman__empty feynman__empty--error"
    >
      <p>{{ topicsError }}</p>
      <Button variant="outline" class="min-h-[44px]" @click="retry">
        Reintentar
      </Button>
    </div>

    <div v-else-if="concept" class="feynman__content">
      <div class="feynman__header">
        <h2 class="feynman__name">{{ concept.title }}</h2>
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
        :aria-label="'Explicá ' + concept.title + ' con tus propias palabras'"
      ></textarea>

      <Button
        class="min-h-[44px] self-start"
        @click="explain"
        :disabled="!userExplanation.trim()"
      >
        Explicar este concepto
      </Button>

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
import { ref, onMounted } from "vue"
import { storeToRefs } from "pinia"
import { useTopicsStore } from "@/entities/topic"
import { useTopicSelection } from "@/features/select-topic"
import { useConceptSelection, useCurrentConcept } from "@/features/select-concept"
import { createLoadIfEmpty } from "@/shared/lib/loadIfEmpty"
import { Button } from "@/shared/ui/button"

const props = defineProps<{
  topicId: string
  conceptId: string
}>()

const topicsStore = useTopicsStore()
const { loading: topicsLoading, error: topicsError, isPending: topicsPending } =
  storeToRefs(topicsStore)
const topicSelection = useTopicSelection()
const conceptSelection = useConceptSelection()
const { currentTopicId } = storeToRefs(topicSelection)
const concept = useCurrentConcept(currentTopicId)

const userExplanation = ref("")
const response = ref("")

// The same rule as `TopicPage` and `ConceptPage` (`backlog.md` D1): `concept` is
// resolved out of the topics store, and a cold
// `/topic/:t/concept/:c/feynman` arrives with that store empty. This page has no
// retry control, so the mount-time load is its only way in. Nothing else about
// the page changes: it still renders the not-found paragraph while the store is
// empty and swaps to the form once the concepts arrive.
const loadTopicsIfEmpty = createLoadIfEmpty({
  isEmpty: () => topicsStore.topics.length === 0 && !topicsStore.loading,
  load: () => topicsStore.loadTopics(),
})

onMounted(() => {
  loadTopicsIfEmpty()
  topicSelection.selectTopic(props.topicId)
  conceptSelection.selectConcept(props.conceptId)
})

/**
 * The page's only way back from a failed load (`backlog.md` D6).
 *
 * Clearing the error is what makes the pending state honest again: the store has
 * already settled once, so `isPending` is false and the retry shows the spinner
 * instead of the not-found paragraph.
 */
function retry() {
  topicsStore.clearError()
  // A retry forces the load. It must not go through `loadTopicsIfEmpty`: that guard
  // answers "is it empty and idle", and a retry that it declines would clear the
  // error and leave the page on its not-found state (`R3-retry-guard-swallows-retry`).
  topicsStore.loadTopics()
}

function explain() {
  if (!userExplanation.value.trim() || !concept.value) return

  // Mock response — real RAG-powered analysis coming in v2.1
  response.value = [
    `Great explanation! Here's a summary of what you covered about "${concept.value.title}":`,
    "",
    "✓ You engaged with the concept in your own words — this strengthens neural pathways.",
    "✓ Self-explanation is one of the most effective study techniques (meta-analysis by Dunlosky et al., 2013).",
    "",
    "[This is a placeholder response. RAG-powered analysis with gap detection and guided follow-up is planned for v2.1.]",
  ].join("\n")
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
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
  min-height: 44px;
  display: flex;
  align-items: center;
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
  gap: 0.5rem;
}

/* The failure stacks its message over its retry; the other two are one line. */
.feynman__empty--error {
  flex-direction: column;
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
