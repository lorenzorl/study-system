<template>
  <section class="chat-thread">
    <div class="chat-thread__messages">
      <p v-if="turns.length === 0 && !sending" class="chat-thread__state">
        Todavía no hay mensajes. Preguntá algo sobre tu corpus.
      </p>

      <template v-for="(turn, index) in turns" :key="index">
        <div
          :class="['chat-thread__message', `chat-thread__message--${turn.role}`]"
        >
          {{ turn.content }}
        </div>
        <ul
          v-if="turn.role === 'assistant' && turn.sources.length > 0"
          class="chat-thread__sources"
        >
          <li
            v-for="(source, sourceIndex) in turn.sources"
            :key="sourceIndex"
            class="chat-thread__source"
          >
            Fuente: {{ source }}
          </li>
        </ul>
      </template>

      <div
        v-if="sending"
        class="chat-thread__message chat-thread__message--assistant"
      >
        {{ answer }}
      </div>
    </div>

    <p v-if="chatError" class="chat-thread__error">{{ chatError }}</p>
    <p v-if="sendError" class="chat-thread__error">{{ sendError }}</p>

    <MessageComposer :busy="sending" @submit="onSubmit" @cancel="onCancel" />
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { storeToRefs } from "pinia"
import { useChatsStore } from "@/entities/chat"
import { MessageComposer, useSendMessage } from "@/features/send-message"

const chatsStore = useChatsStore()
const { note, error: chatError } = storeToRefs(chatsStore)

const sendStore = useSendMessage()
const { sending, answer, error: sendError } = storeToRefs(sendStore)

const turns = computed(() => note.value?.turns ?? [])

// The thread is the one place that decides what a submit means: send the
// question through the feature store and let it own the orchestration.
function onSubmit(question: string): void {
  sendStore.send(question)
}

function onCancel(): void {
  sendStore.cancel()
}
</script>

<style scoped>
.chat-thread {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.chat-thread__messages {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.chat-thread__state {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.chat-thread__message {
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  font-size: 0.85rem;
  line-height: 1.4;
  white-space: pre-wrap;
  word-break: break-word;
}

.chat-thread__message--user {
  align-self: flex-end;
  max-width: 85%;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
}

.chat-thread__message--assistant {
  align-self: flex-start;
  background: var(--background-secondary);
  color: var(--text-normal);
}

.chat-thread__sources {
  align-self: flex-start;
  margin: 0;
  padding: 0 0 0 0.5rem;
  list-style: none;
}

.chat-thread__source {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.chat-thread__error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-error);
}
</style>
