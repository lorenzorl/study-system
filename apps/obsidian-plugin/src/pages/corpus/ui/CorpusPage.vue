<template>
  <div class="corpus-page">
    <section class="corpus-page__section">
      <h2 class="corpus-page__title">Corpus</h2>
      <UploadDocumentForm />
      <SearchCorpusForm />
      <SearchResults />
    </section>
  </div>
</template>

<script setup lang="ts">
import { SearchCorpusForm } from "@/features/search-corpus"
import { UploadDocumentForm } from "@/features/upload-document"
import { SearchResults } from "@/widgets/search-results"

/**
 * The corpus: what the chat answers from.
 *
 * It used to sit under the chat on the same page. It is its own page now because
 * uploading a book and asking a question are different jobs, and the chat needs
 * the full width for three zones.
 */
</script>

<style scoped>
.corpus-page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 0.75rem;
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
}

.corpus-page__section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.corpus-page__title {
  margin: 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
</style>
