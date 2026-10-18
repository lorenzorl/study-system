<template>
  <div class="search-results">
    <p v-if="loading" class="search-results__state">Buscando...</p>
    <p v-else-if="error" class="search-results__error">{{ error }}</p>
    <p v-else-if="results.length === 0" class="search-results__state">
      Sin resultados todavía.
    </p>
    <ul v-else class="search-results__list">
      <li
        v-for="result in results"
        :key="result.chunk_id"
        class="search-results__item"
      >
        <span class="search-results__source">
          {{ result.document_title }} · pág. {{ result.page_number }}
        </span>
        <span class="search-results__snippet">{{ result.snippet }}</span>
        <span class="search-results__scores">
          {{ result.stage }} · bm25 {{ result.bm25.toFixed(2) }} · similitud
          {{ result.similarity.toFixed(2) }}
        </span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia"
import { useSearchCorpus } from "@/features/search-corpus"

const search = useSearchCorpus()
const { results, loading, error } = storeToRefs(search)
</script>

<style scoped>
.search-results__list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.search-results__item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.5rem 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

.search-results__source {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
}

.search-results__snippet {
  font-size: 0.85rem;
  color: var(--text-normal);
  line-height: 1.4;
}

.search-results__scores {
  font-size: 0.7rem;
  color: var(--text-faint);
}

.search-results__state {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.search-results__error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-error);
}
</style>
