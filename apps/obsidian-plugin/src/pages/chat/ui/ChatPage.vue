<template>
  <div class="chat-page">
    <header class="chat-page__header">
      <h2 class="chat-page__title">Chat</h2>
      <button
        type="button"
        class="chat-page__settings-toggle"
        :aria-expanded="showSettings"
        @click="showSettings = !showSettings"
      >
        {{ showSettings ? "Ocultar ajustes" : "Ajustes del chat" }}
      </button>
    </header>

    <div
      :class="[
        'chat-page__body',
        { 'chat-page__body--settings': showSettings },
      ]"
    >
      <aside class="chat-page__sidebar">
        <ChatList />
      </aside>

      <section class="chat-page__thread">
        <ChatThread />
      </section>

      <aside v-if="showSettings" class="chat-page__settings">
        <ChatSettingsPanel />
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { useChatsStore } from "@/entities/chat"
import { ChatList } from "@/widgets/chat-list"
import { ChatSettingsPanel } from "@/widgets/chat-settings"
import { ChatThread } from "@/widgets/chat-thread"

/**
 * The chat, in the three zones AnythingLLM uses: the list of chats, the thread,
 * and the settings of the current chat.
 *
 * The settings pane starts open because the chat's own prompt is the reason this
 * feature exists (S1); it collapses for a narrow leaf, where the thread matters
 * more than the panel.
 */
const showSettings = ref(true)
const chats = useChatsStore()

onMounted(() => {
  chats.loadChats()
})
</script>

<style scoped>
.chat-page {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  height: 100%;
  min-height: 0;
  padding: 0.75rem;
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
}

.chat-page__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
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

.chat-page__settings-toggle {
  min-height: 44px;
  padding: 0.25rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.8rem;
  font-family: inherit;
  transition: background-color 0.15s ease;
}

.chat-page__settings-toggle:hover {
  background: var(--background-modifier-hover);
}

.chat-page__body {
  display: grid;
  grid-template-columns: minmax(150px, 220px) minmax(0, 1fr);
  gap: 0.75rem;
  flex: 1;
  min-height: 0;
}

.chat-page__body--settings {
  grid-template-columns:
    minmax(150px, 220px)
    minmax(0, 1fr)
    minmax(220px, 300px);
}

.chat-page__sidebar,
.chat-page__settings {
  overflow-y: auto;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
}

.chat-page__thread {
  min-height: 0;
  overflow-y: auto;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

/* A narrow leaf stacks the zones: the thread first, because it is what a
   reader came for. */
@media (max-width: 700px) {
  .chat-page__body,
  .chat-page__body--settings {
    grid-template-columns: 1fr;
  }
}
</style>
