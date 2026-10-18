/**
 * The plugin's global settings: the single source of truth for the backend base
 * URL and the LLM configuration the user edits from the plugin's settings tab.
 *
 * Pure TypeScript: no `obsidian`, no DOM, no other `shared/*` segment. The value
 * is persisted through the injected `SettingsStore` port, which
 * `app/settings-store.ts` implements over the Obsidian `Plugin` API.
 */

export interface PluginSettings {
  apiBase: string
  llmBaseUrl: string
  llmApiKey: string
  llmModel: string
  llmTemperature: number
  retrievalTopK: number
  historyTurns: number
  defaultChatPrompt: string
}

export const DEFAULT_SETTINGS: PluginSettings = {
  apiBase: "http://localhost:8080",
  llmBaseUrl: "https://openrouter.ai/api/v1",
  // An empty key means "not configured": the chat reports it instead of calling.
  llmApiKey: "",
  llmModel: "qwen/qwen3.5-flash-02-23",
  llmTemperature: 0.2,
  retrievalTopK: 6,
  historyTurns: 8,
  defaultChatPrompt:
    "Sos mi tutor de estudio. Respondé con precisión, en el idioma de mi pregunta, y explicá con las palabras del material.",
}

/** The persistence port. `app/settings-store.ts` implements it over the Plugin API. */
export interface SettingsStore {
  load(): Promise<unknown>
  save(data: unknown): Promise<void>
}

type SettingsListener = (settings: PluginSettings) => void

// Singleton pattern — mirrors `shared/obsidian/useObsidian.ts`'s `setObsidianApp`.
let _settings: PluginSettings = createDefaultSettings()
let _store: SettingsStore | null = null
const _listeners = new Set<SettingsListener>()

/** Install the persistence port. Called once by `app/settings-store.ts` on load. */
export function setSettingsStore(store: SettingsStore): void {
  _store = store
}

/** The current settings. Never null: the defaults stand in until something is set. */
export function getSettings(): PluginSettings {
  return _settings
}

/**
 * Apply a patch to the current settings, persist the normalized result through
 * the injected store, and notify every listener.
 *
 * The patch is merged onto the current settings, so a caller only sends what it
 * edits; the result is normalized, so numeric fields come back clamped and a
 * wrong-typed value falls back to its default instead of reaching the store.
 */
export async function updateSettings(patch: Partial<PluginSettings>): Promise<void> {
  const next = normalizeSettings({ ..._settings, ...patch })

  _settings = next

  if (_store) {
    await _store.save(next)
  }

  for (const listener of [..._listeners]) {
    listener(next)
  }
}

/** Subscribe to settings changes. Returns the unsubscribe function. */
export function onSettingsChange(listener: SettingsListener): () => void {
  _listeners.add(listener)

  return () => {
    _listeners.delete(listener)
  }
}

/**
 * Turn whatever was persisted into a complete, valid `PluginSettings`.
 *
 * A non-object is never thrown on: it yields a fresh copy of the defaults.
 * Unknown keys are dropped, a wrong-typed field falls back to its default
 * rather than being coerced, and numeric fields are clamped to the range the
 * consumer accepts.
 */
export function normalizeSettings(raw: unknown): PluginSettings {
  if (!isRecord(raw)) {
    return createDefaultSettings()
  }

  return {
    apiBase: normalizeUrl(raw.apiBase, DEFAULT_SETTINGS.apiBase),
    llmBaseUrl: normalizeUrl(raw.llmBaseUrl, DEFAULT_SETTINGS.llmBaseUrl),
    llmApiKey: normalizeString(raw.llmApiKey, DEFAULT_SETTINGS.llmApiKey),
    llmModel: normalizeString(raw.llmModel, DEFAULT_SETTINGS.llmModel),
    llmTemperature: normalizeNumber(raw.llmTemperature, 0, 2, DEFAULT_SETTINGS.llmTemperature),
    // 50 is the backend's MaxSearchLimit.
    retrievalTopK: normalizeInteger(raw.retrievalTopK, 1, 50, DEFAULT_SETTINGS.retrievalTopK),
    historyTurns: normalizeInteger(raw.historyTurns, 0, 50, DEFAULT_SETTINGS.historyTurns),
    defaultChatPrompt: normalizeString(raw.defaultChatPrompt, DEFAULT_SETTINGS.defaultChatPrompt),
  }
}

/**
 * Adopt the base URL a user set before settings moved into `data.json`.
 *
 * `legacy` is used only when it is a non-empty trimmed string and the stored
 * `apiBase` is still the default, so a value the user already stored is never
 * overwritten. Returns the settings unchanged when there is nothing to migrate.
 */
export function mergeLegacyApiBase(
  settings: PluginSettings,
  legacy: string | null,
): PluginSettings {
  if (typeof legacy !== "string") {
    return settings
  }

  const trimmed = legacy.trim()

  if (trimmed === "" || trimmed === DEFAULT_SETTINGS.apiBase) {
    return settings
  }

  if (settings.apiBase !== DEFAULT_SETTINGS.apiBase) {
    return settings
  }

  return { ...settings, apiBase: trimmed }
}

/** A fresh copy of the defaults, so a caller can never mutate the shared object. */
function createDefaultSettings(): PluginSettings {
  return { ...DEFAULT_SETTINGS }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

/** Only strings survive; a non-string falls back to the default. */
function normalizeString(value: unknown, fallback: string): string {
  return typeof value === "string" ? value : fallback
}

/** A URL is trimmed; empty after trimming falls back to the default. */
function normalizeUrl(value: unknown, fallback: string): string {
  if (typeof value !== "string") {
    return fallback
  }

  const trimmed = value.trim()

  return trimmed === "" ? fallback : trimmed
}

/** A non-number falls back to the default; a number is clamped to the range. */
function normalizeNumber(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return fallback
  }

  return Math.min(max, Math.max(min, value))
}

/** A non-integer or non-number falls back to the default; an integer is clamped. */
function normalizeInteger(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== "number" || !Number.isInteger(value)) {
    return fallback
  }

  return Math.min(max, Math.max(min, value))
}
