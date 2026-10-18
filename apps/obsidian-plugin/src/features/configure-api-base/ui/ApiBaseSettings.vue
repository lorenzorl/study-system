<template>
  <details class="dashboard__settings">
    <summary class="dashboard__settings-toggle">⚙️ Configuración</summary>
    <form class="dashboard__settings-form" @submit.prevent="save">
      <label class="dashboard__settings-label" for="api-base-input">
        URL del servidor Go
      </label>
      <input
        id="api-base-input"
        v-model="apiBaseUrl"
        type="text"
        class="dashboard__new-input"
        placeholder="http://192.168.1.50:8080"
      />
      <div class="dashboard__new-actions">
        <button type="submit" class="dashboard__new-submit">Guardar</button>
        <button type="button" class="dashboard__new-cancel" @click="reset">
          Restablecer
        </button>
      </div>
    </form>
  </details>
</template>

<script setup lang="ts">
import { ref } from "vue"
import { getApiBase, resetApiBase, setApiBase } from "@/shared/api/client"

const apiBaseUrl = ref(getApiBase())

function save() {
  const url = apiBaseUrl.value.trim()
  if (url) {
    setApiBase(url)
  }
}

function reset() {
  resetApiBase()
  apiBaseUrl.value = getApiBase()
}
</script>

<style scoped>
.dashboard__settings {
  margin-top: 0.75rem;
  padding: 0.5rem 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
}

.dashboard__settings-toggle {
  min-height: 44px;
  display: flex;
  align-items: center;
  cursor: pointer;
  font-size: 0.8rem;
  color: var(--text-muted);
  user-select: none;
}

.dashboard__settings-toggle::marker {
  font-size: 0.8rem;
}

.dashboard__settings-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 0.5rem;
}

.dashboard__settings-label {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-bottom: -0.25rem;
}

.dashboard__new-input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.dashboard__new-input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.dashboard__new-actions {
  display: flex;
  gap: 0.5rem;
}

.dashboard__new-submit {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  transition: opacity 0.15s ease;
}

.dashboard__new-submit:hover:not(:disabled) {
  opacity: 0.9;
}

.dashboard__new-cancel {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.dashboard__new-cancel:hover:not(:disabled) {
  background: var(--background-modifier-hover);
}
</style>
