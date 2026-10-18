<template>
  <section class="chat-settings">
    <p v-if="!note" class="chat-settings__state">
      Abrí o creá un chat para editar su prompt y sus ajustes.
    </p>

    <template v-else>
      <ChatPromptForm />

      <section class="chat-settings__overrides">
        <h3 class="chat-settings__title">Ajustes de este chat</h3>

        <div class="chat-settings__field">
          <label class="chat-settings__label" for="chat-settings-model">
            Modelo
          </label>
          <input
            id="chat-settings-model"
            v-model="draft.model"
            type="text"
            class="chat-settings__input"
            :placeholder="globals.llmModel"
            @change="persist"
          />
          <p class="chat-settings__effective">En uso: {{ modelLabel }}</p>
          <button type="button" class="chat-settings__clear" @click="clearModel">
            Usar el global
          </button>
        </div>

        <div class="chat-settings__field">
          <label class="chat-settings__label" for="chat-settings-temperature">
            Temperatura
          </label>
          <input
            id="chat-settings-temperature"
            v-model="draft.temperature"
            type="number"
            step="0.1"
            min="0"
            max="2"
            class="chat-settings__input"
            :placeholder="String(globals.llmTemperature)"
            @change="persist"
          />
          <p class="chat-settings__effective">En uso: {{ temperatureLabel }}</p>
          <button
            type="button"
            class="chat-settings__clear"
            @click="clearTemperature"
          >
            Usar el global
          </button>
        </div>

        <div class="chat-settings__field">
          <label class="chat-settings__label" for="chat-settings-top-k">
            Fragmentos a recuperar (top k)
          </label>
          <input
            id="chat-settings-top-k"
            v-model="draft.topK"
            type="number"
            min="1"
            max="50"
            class="chat-settings__input"
            :placeholder="String(globals.retrievalTopK)"
            @change="persist"
          />
          <p class="chat-settings__effective">En uso: {{ topKLabel }}</p>
          <button type="button" class="chat-settings__clear" @click="clearTopK">
            Usar el global
          </button>
        </div>

        <span v-if="saved" class="chat-settings__saved" role="status">
          Guardado
        </span>
      </section>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from "vue"
import { storeToRefs } from "pinia"
import { useChatsStore } from "@/entities/chat"
import type { ChatNote, ChatNoteSettings } from "@/entities/chat"
import { getSettings, onSettingsChange } from "@/shared/config"
import { ChatPromptForm } from "@/features/edit-chat-prompt"

const chats = useChatsStore()
const { note } = storeToRefs(chats)

// A copy of the global settings, refreshed when the settings tab changes them,
// so a cleared override shows the global default that is in effect now.
const globals = ref({ ...getSettings() })
const unsubscribe = onSettingsChange((settings) => {
  globals.value = { ...settings }
})

const draft = reactive({ model: "", temperature: "", topK: "" })

const saved = ref(false)
let savedTimer: ReturnType<typeof setTimeout> | null = null

const modelInherited = computed(() => draft.model.trim() === "")
const temperatureInherited = computed(() => draft.temperature.trim() === "")
const topKInherited = computed(() => draft.topK.trim() === "")

const effectiveModel = computed(() =>
  modelInherited.value ? globals.value.llmModel : draft.model.trim(),
)
const effectiveTemperature = computed(() =>
  temperatureInherited.value
    ? globals.value.llmTemperature
    : parseTemperature(draft.temperature) ?? globals.value.llmTemperature,
)
const effectiveTopK = computed(() =>
  topKInherited.value
    ? globals.value.retrievalTopK
    : parseTopK(draft.topK) ?? globals.value.retrievalTopK,
)

const modelLabel = computed(
  () => `${effectiveModel.value} (${sourceLabel(modelInherited.value)})`,
)
const temperatureLabel = computed(
  () => `${effectiveTemperature.value} (${sourceLabel(temperatureInherited.value)})`,
)
const topKLabel = computed(
  () => `${effectiveTopK.value} (${sourceLabel(topKInherited.value)})`,
)

watch(note, (value) => seed(value), { immediate: true })

function seed(value: ChatNote | null): void {
  draft.model = value?.chat.settings.model ?? ""
  draft.temperature = formatNumber(value?.chat.settings.temperature ?? null)
  draft.topK = formatNumber(value?.chat.settings.topK ?? null)
}

async function persist(): Promise<void> {
  const settings: ChatNoteSettings = {
    model: modelInherited.value ? null : draft.model.trim(),
    temperature: parseTemperature(draft.temperature),
    topK: parseTopK(draft.topK),
  }

  await chats.saveSettings(settings)

  // Re-read so the fields show the values the note kept after clamping.
  seed(chats.note)

  saved.value = true
  if (savedTimer !== null) clearTimeout(savedTimer)
  savedTimer = setTimeout(() => {
    saved.value = false
  }, 2000)
}

function clearModel(): void {
  draft.model = ""
  void persist()
}

function clearTemperature(): void {
  draft.temperature = ""
  void persist()
}

function clearTopK(): void {
  draft.topK = ""
  void persist()
}

onBeforeUnmount(() => {
  unsubscribe()
  if (savedTimer !== null) clearTimeout(savedTimer)
})

function sourceLabel(inherited: boolean): string {
  return inherited ? "heredado del global" : "definido en este chat"
}

/** The global range mirrors `normalizeSettings`'s 0..2 for temperature. */
function parseTemperature(value: string): number | null {
  const parsed = parseNumber(value)
  if (parsed === null) return null

  return Math.min(2, Math.max(0, parsed))
}

/** 50 is the backend's MaxSearchLimit; the override never exceeds it. */
function parseTopK(value: string): number | null {
  const parsed = parseNumber(value)
  if (parsed === null) return null

  return Math.min(50, Math.max(1, Math.round(parsed)))
}

function parseNumber(value: string): number | null {
  const trimmed = value.trim()
  if (trimmed === "") return null

  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : null
}

function formatNumber(value: number | null): string {
  return value === null ? "" : String(value)
}
</script>

<style scoped>
.chat-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.chat-settings__state {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.chat-settings__overrides {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.chat-settings__title {
  margin: 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.chat-settings__field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.chat-settings__label {
  font-size: 0.8rem;
  color: var(--text-normal);
}

.chat-settings__input {
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-size: 0.85rem;
  font-family: inherit;
}

.chat-settings__input:focus {
  outline: none;
  border-color: var(--interactive-accent);
}

.chat-settings__effective {
  margin: 0;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.chat-settings__clear {
  align-self: flex-start;
  min-height: 44px;
  padding: 0.25rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: transparent;
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.75rem;
  font-family: inherit;
}

.chat-settings__clear:hover {
  background: var(--background-modifier-hover);
}

.chat-settings__saved {
  font-size: 0.75rem;
  color: var(--text-muted);
}
</style>
