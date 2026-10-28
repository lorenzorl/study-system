<template>
  <!--
    The one way in for a note is the `shared/ui` button (`shadcn-migration.md` T8). It
    is the primary action of the header it sits in, so it keeps the default variant
    and the 44px touch target as a class; `ml-auto` is all this control ever owned
    about its position.
  -->
  <Button
    ref="triggerEl"
    class="ml-auto min-h-[44px]"
    aria-label="Sincronizar nota"
    title="Sincronizar una nota pegada"
    :aria-expanded="panelOpen"
    @click="openPanel"
  >
    Sincronizar nota
  </Button>
</template>

<script setup lang="ts">
import { nextTick, ref, watch, type ComponentPublicInstance } from "vue"
import { storeToRefs } from "pinia"
import { Button } from "@/shared/ui/button"
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

// The primitive renders its own element, so a template ref on it is the component
// instance: the focusable element is its `$el`. Restoring focus onto the control the
// reader pressed is the reason this ref exists, so it is read through `$el` rather
// than being dropped.
const triggerEl = ref<ComponentPublicInstance | null>(null)
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
  ;(triggerEl.value?.$el as HTMLElement | undefined)?.focus()
})

function openPanel(): void {
  store.openPanel()
}
</script>
