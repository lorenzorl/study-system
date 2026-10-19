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
        <!-- The third state of the frame: the answer exists, but it is not
             founded on the material the chat has. Next to the sources, because
             both answer the same question about the turn. -->
        <p
          v-if="turn.role === 'assistant' && turn.consulted === false"
          class="chat-thread__badge"
        >
          sin consultar el material
        </p>
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

    <!-- Above the composer, not in the settings pane: an attachment decides what
         the next message carries, so it belongs next to the send button. -->
    <AttachmentPanel :has-chat="note !== null" />

    <MessageComposer
      v-model="draft"
      :busy="sending"
      @submit="onSubmit"
      @cancel="onCancel"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from "vue"
import { storeToRefs } from "pinia"
import { useChatsStore } from "@/entities/chat"
import { AttachmentPanel } from "@/features/attach-resource"
import { MessageComposer, useSendMessage } from "@/features/send-message"

const chatsStore = useChatsStore()
const { note, error: chatError } = storeToRefs(chatsStore)

const sendStore = useSendMessage()
const { sending, answer, error: sendError } = storeToRefs(sendStore)

const turns = computed(() => note.value?.turns ?? [])

// The composer holds this text and never clears it itself: a send that never
// happened has to leave the question where the user can press send again.
const draft = ref("")

// The thread is the one place that decides what a submit means: send the
// question through the feature store and let it own the orchestration.
//
// The text is cleared optimistically and given back when `send` answers that the
// question was never consumed. `send` resolves only when the whole answer is
// done, so a restore cannot race a second submit: the input is disabled while
// `sending` is true, and the restore only happens on a send that was blocked
// before anything was persisted.
async function onSubmit(question: string): Promise<void> {
  draft.value = ""

  const consumed = await sendStore.send(question)

  if (!consumed) draft.value = question
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

/* Same tone as a source line: the badge qualifies the answer, it is not an
   error and must not read like one. */
.chat-thread__badge {
  align-self: flex-start;
  margin: 0;
  padding-left: 0.5rem;
  font-size: 0.75rem;
  font-style: italic;
  color: var(--text-muted);
}

.chat-thread__error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-error);
}
</style>
