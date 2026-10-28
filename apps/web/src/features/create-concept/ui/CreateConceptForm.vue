<template>
  <form class="topic-view__new-form" @submit.prevent="submit">
    <Input
      v-model="title"
      type="text"
      placeholder="Título del concepto (ej. Agregados, Value Objects)"
      :disabled="submitting"
      class="min-h-[44px]"
    />
    <div class="topic-view__new-actions">
      <Button
        type="submit"
        class="min-h-[44px]"
        :disabled="submitting || !title.trim()"
      >
        <span v-if="submitting" class="spinner"></span>
        <span v-else>Guardar</span>
      </Button>
      <Button
        type="button"
        variant="outline"
        class="min-h-[44px]"
        :disabled="submitting"
        @click="emit('cancel')"
      >
        Cancelar
      </Button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"

const props = defineProps<{
  modelValue: string
  submitting?: boolean
}>()

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void
  (e: "submit"): void
  (e: "cancel"): void
}>()

const title = computed({
  get: () => props.modelValue,
  set: (value: string) => emit("update:modelValue", value),
})

function submit() {
  emit("submit")
}
</script>

<style scoped>
/*
 * The field and the two buttons are `shared/ui` primitives now: their border,
 * radius, background, focus ring and disabled state come from the design tokens,
 * not from this file. What stays here is what the primitives do not own — the
 * panel's own layout and the spinner. The 44px floor stays too: it is the touch
 * target this form has always had, and a migration does not shrink one.
 */
.topic-view__new-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  margin-bottom: 0.25rem;
}

.topic-view__new-actions {
  display: flex;
  gap: 0.5rem;
}

.spinner {
  display: inline-block;
  width: 20px;
  height: 20px;
  border: 2px solid var(--background-modifier-border);
  border-top: 2px solid var(--interactive-accent);
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
