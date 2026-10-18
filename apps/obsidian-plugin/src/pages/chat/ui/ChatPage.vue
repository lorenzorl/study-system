<template>
  <div class="chat-page">
    <section class="chat-page__section">
      <h2 class="chat-page__title">Chat</h2>
      <ChatPanel />
    </section>

    <section class="chat-page__section">
      <h2 class="chat-page__title">Corpus</h2>
      <UploadDocumentForm />
      <SearchCorpusForm />
      <SearchResults />
    </section>
  </div>
</template>

<script setup lang="ts">
import { UploadDocumentForm } from "@/features/upload-document"
import { SearchCorpusForm } from "@/features/search-corpus"
import { ChatPanel } from "@/widgets/chat-panel"
import { SearchResults } from "@/widgets/search-results"
</script>

<style scoped>
.chat-page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 0.75rem;
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
}

.chat-page__section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.chat-page__title {
  margin: 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
</style>
