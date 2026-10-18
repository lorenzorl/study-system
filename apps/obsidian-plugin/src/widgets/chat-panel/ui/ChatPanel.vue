<template>
  <section class="chat-panel">
    <ul v-if="messages.length > 0" class="chat-panel__messages">
      <li
        v-for="message in messages"
        :key="message.id"
        :class="['chat-panel__message', `chat-panel__message--${message.role}`]"
      >
        {{ message.content }}
      </li>
    </ul>
    <p v-else class="chat-panel__state">
      Todavía no hay mensajes. Preguntá algo sobre tu corpus.
    </p>

    <p v-if="error" class="chat-panel__error">{{ error }}</p>

    <AskChatForm @submit="onSubmit" />
  </section>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia"
import { AskChatForm, useAskChat } from "@/features/ask-chat"

const chat = useAskChat()
const { messages, error } = storeToRefs(chat)

function onSubmit(question: string) {
  chat.ask(question)
}
</script>

<style scoped>
.chat-panel {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.chat-panel__messages {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.chat-panel__message {
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  font-size: 0.85rem;
  line-height: 1.4;
  white-space: pre-wrap;
  word-break: break-word;
}

.chat-panel__message--user {
  align-self: flex-end;
  max-width: 85%;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
}

.chat-panel__message--assistant {
  align-self: flex-start;
  background: var(--background-secondary);
  color: var(--text-normal);
}

.chat-panel__state {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.chat-panel__error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-error);
}
</style>
