<template>
  <form class="search-corpus" @submit.prevent="submit">
    <input
      v-model="query"
      type="text"
      class="search-corpus__input"
      placeholder="Buscar en el corpus (ej. agregado)"
      aria-label="Buscar en el corpus"
      :disabled="loading"
    />
    <button
      type="submit"
      class="search-corpus__submit"
      :disabled="loading || !query.trim()"
    >
      Buscar
    </button>
  </form>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia"
import { ref } from "vue"
import { useSearchCorpus } from "../model/useSearchCorpus"

const store = useSearchCorpus()
const { loading } = storeToRefs(store)

const query = ref("")

function submit() {
  if (loading.value || !query.value.trim()) return
  store.search(query.value)
}
</script>

<style scoped>
.search-corpus {
  display: flex;
  gap: 0.5rem;
}

.search-corpus__input {
  flex: 1;
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.search-corpus__input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.search-corpus__submit {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.search-corpus__submit:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}

.search-corpus__submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
