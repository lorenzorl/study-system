<template>
  <section class="chat-list">
    <NewChatButton />

    <p v-if="loading" class="chat-list__state">Cargando chats…</p>
    <p v-else-if="error" class="chat-list__error">{{ error }}</p>
    <p v-else-if="chats.length === 0" class="chat-list__state">
      Todavía no hay chats. Creá uno para empezar.
    </p>
    <ul v-else class="chat-list__items">
      <li v-for="chat in chats" :key="chat.path">
        <button
          type="button"
          :class="[
            'chat-list__item',
            { 'chat-list__item--active': chat.path === currentPath },
          ]"
          @click="select(chat.path)"
        >
          {{ chat.title }}
        </button>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { storeToRefs } from "pinia"
import { useChatsStore } from "@/entities/chat"
import { NewChatButton } from "@/features/create-chat"
import { useChatSelection } from "@/features/select-chat"

const chatsStore = useChatsStore()
const { paths, currentPath, loading, error } = storeToRefs(chatsStore)
const selection = useChatSelection()

// The store already lists the folder sorted; sorting again keeps the render in
// ascending path order regardless of the source's order.
const chats = computed(() =>
  [...paths.value].sort().map((path) => ({ path, title: titleOf(path) })),
)

function select(path: string): void {
  selection.select(path)
}

function titleOf(path: string): string {
  const file = path.split("/").pop() ?? path
  return file.replace(/\.md$/, "")
}
</script>

<style scoped>
.chat-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.chat-list__state,
.chat-list__error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.chat-list__error {
  color: var(--text-error);
}

.chat-list__items {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.chat-list__item {
  width: 100%;
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  font-family: inherit;
  text-align: left;
  transition: background 0.15s ease;
}

.chat-list__item:hover {
  background: var(--background-modifier-hover);
}

.chat-list__item--active {
  background: var(--background-secondary);
  border-color: var(--interactive-accent);
}
</style>
