<template>
  <form class="dashboard__new-form" @submit.prevent="submit">
    <Input
      v-model="name"
      type="text"
      placeholder="Nombre del tema (ej. DDD, Matemáticas)"
      :disabled="submitting"
      class="min-h-[44px]"
    />
    <div class="dashboard__new-actions">
      <Button type="submit" :disabled="submitting || !name.trim()">
        <span v-if="submitting" class="spinner"></span>
        <span v-else>Guardar</span>
      </Button>
      <Button
        type="button"
        variant="outline"
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

const name = computed({
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
 * panel's own layout and the spinner.
 */
.dashboard__new-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

.dashboard__new-actions {
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
