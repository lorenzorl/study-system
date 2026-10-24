<template>
  <button
    ref="triggerEl"
    type="button"
    class="sync-note-trigger"
    aria-label="Sincronizar nota"
    title="Sincronizar una nota pegada"
    :aria-expanded="panelOpen"
    @click="openPanel"
  >
    Sincronizar nota
  </button>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from "vue"
import { storeToRefs } from "pinia"
import { useSyncNote } from "../model/useSyncNote"

/**
 * The topics page's one way in for a note, where the vault's sync button sat.
 *
 * It is separate from the overlay it opens so the button can live in the topic
 * header: an overlay is a block element and does not belong inside a heading.
 * The two talk through the store, which also lets the trigger restore focus when
 * the panel closes — the element that had focus still exists here.
 */
const store = useSyncNote()
const { panelOpen } = storeToRefs(store)

const triggerEl = ref<HTMLButtonElement | null>(null)
let wasOpen = false

watch(panelOpen, async (isOpen) => {
  if (isOpen) {
    wasOpen = true
    return
  }

  // Only a panel that actually opened hands focus back; a component mounted
  // while the panel is closed must not steal it.
  if (!wasOpen) return
  wasOpen = false
  await nextTick()
  triggerEl.value?.focus()
})

function openPanel(): void {
  store.openPanel()
}
</script>

<style scoped>
.sync-note-trigger {
  margin-left: auto;
  min-height: 44px;
  padding: 0.25rem 0.75rem;
  border: 1px solid var(--interactive-accent);
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.8rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.sync-note-trigger:hover {
  opacity: 0.9;
}
</style>
