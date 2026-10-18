<template>
  <div class="llm-settings">
    <p v-if="!form.llmApiKey.trim()" class="llm-settings__warning" role="alert">
      El chat no puede responder hasta que configures la API key del LLM.
    </p>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-api-base">
        URL del servidor Go
      </label>
      <input
        id="llm-settings-api-base"
        v-model="form.apiBase"
        type="text"
        class="llm-settings__input"
        placeholder="http://localhost:8080"
      />
    </div>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-base-url">
        URL base del proveedor (OpenRouter)
      </label>
      <input
        id="llm-settings-base-url"
        v-model="form.llmBaseUrl"
        type="text"
        class="llm-settings__input"
        placeholder="https://openrouter.ai/api/v1"
      />
    </div>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-api-key">
        API key del LLM
      </label>
      <input
        id="llm-settings-api-key"
        v-model="form.llmApiKey"
        type="password"
        class="llm-settings__input"
        autocomplete="off"
      />
      <p class="llm-settings__note">
        La key se guarda en el <code>data.json</code> del plugin, dentro del vault.
        Como el archivo vive en el vault, es preferible usar en el proveedor una key
        con límite de gasto.
      </p>
    </div>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-model">
        Modelo del LLM
      </label>
      <input
        id="llm-settings-model"
        v-model="form.llmModel"
        type="text"
        class="llm-settings__input"
        placeholder="qwen/qwen3.5-flash-02-23"
      />
    </div>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-temperature">
        Temperatura
      </label>
      <input
        id="llm-settings-temperature"
        v-model.number="form.llmTemperature"
        type="number"
        step="0.1"
        min="0"
        max="2"
        class="llm-settings__input"
      />
    </div>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-top-k">
        Fragmentos a recuperar (top K)
      </label>
      <input
        id="llm-settings-top-k"
        v-model.number="form.retrievalTopK"
        type="number"
        min="1"
        max="50"
        class="llm-settings__input"
      />
    </div>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-history-turns">
        Turnos de historial
      </label>
      <input
        id="llm-settings-history-turns"
        v-model.number="form.historyTurns"
        type="number"
        min="0"
        max="50"
        class="llm-settings__input"
      />
    </div>

    <div class="llm-settings__field">
      <label class="llm-settings__label" for="llm-settings-prompt">
        Prompt por defecto del chat
      </label>
      <textarea
        id="llm-settings-prompt"
        v-model="form.defaultChatPrompt"
        class="llm-settings__input llm-settings__textarea"
        rows="4"
      ></textarea>
    </div>

    <div class="llm-settings__actions">
      <button type="button" class="llm-settings__submit" @click="save">
        Guardar
      </button>
      <span v-if="saved" class="llm-settings__saved" role="status">Guardado</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from "vue"
import { getSettings, updateSettings, type PluginSettings } from "@/shared/config"

// Editable copy of the settings: the singleton is only written through
// `updateSettings`, which owns normalization and persistence.
const form = reactive<PluginSettings>({ ...getSettings() })

const saved = ref(false)
let savedTimer: ReturnType<typeof setTimeout> | null = null

async function save(): Promise<void> {
  await updateSettings({ ...form })

  // Re-read so the form shows the values `updateSettings` kept after clamping.
  Object.assign(form, getSettings())

  saved.value = true
  if (savedTimer !== null) {
    clearTimeout(savedTimer)
  }
  savedTimer = setTimeout(() => {
    saved.value = false
  }, 2000)
}

onBeforeUnmount(() => {
  if (savedTimer !== null) {
    clearTimeout(savedTimer)
  }
})
</script>

<style scoped>
.llm-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  max-width: 32rem;
}

.llm-settings__warning {
  margin: 0;
  padding: 0.5rem 0.75rem;
  background: var(--background-secondary);
  border: 1px solid var(--background-modifier-border);
  border-left: 3px solid var(--text-warning, var(--interactive-accent));
  border-radius: 6px;
  color: var(--text-normal);
  font-size: 0.8rem;
}

.llm-settings__field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.llm-settings__label {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.llm-settings__input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
  width: 100%;
}

.llm-settings__input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.llm-settings__textarea {
  min-height: 44px;
  resize: vertical;
  line-height: 1.4;
}

.llm-settings__note {
  margin: 0;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.llm-settings__actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.llm-settings__submit {
  min-height: 44px;
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  background: var(--interactive-accent);
  color: var(--text-on-accent, #fff);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  transition: opacity 0.15s ease;
}

.llm-settings__submit:hover {
  opacity: 0.9;
}

.llm-settings__saved {
  font-size: 0.8rem;
  color: var(--text-muted);
}
</style>
