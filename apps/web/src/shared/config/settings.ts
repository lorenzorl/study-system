/**
 * The app's global settings: the single source of truth for the backend base
 * URL.
 *
 * An empty `apiBase` means "the origin that served this page", which is the only
 * deployment the app has: the Go binary serves the SPA and the API from the same
 * port, and Vite proxies `/api` to it in development. Nothing is persisted on
 * load — a stored origin would go stale the moment the port changed — so the
 * `SettingsStore` port exists for a future settings surface that writes an
 * explicit user choice through it.
 *
 * Pure TypeScript: no DOM, no other `shared/*` segment.
 */

export interface AppSettings {
  apiBase: string
}

export const DEFAULT_SETTINGS: AppSettings = {
  apiBase: "",
}

/** The persistence port, implemented by whatever surface owns an explicit choice. */
export interface SettingsStore {
  load(): Promise<unknown>
  save(data: unknown): Promise<void>
}

type SettingsListener = (settings: AppSettings) => void

let _settings: AppSettings = createDefaultSettings()
let _store: SettingsStore | null = null
const _listeners = new Set<SettingsListener>()

/** Install the persistence port. */
export function setSettingsStore(store: SettingsStore): void {
  _store = store
}

/** The current settings. Never null: the defaults stand in until something is set. */
export function getSettings(): AppSettings {
  return _settings
}

/**
 * Apply a patch to the current settings, persist the normalized result through
 * the injected store, and notify every listener.
 *
 * The patch is merged onto the current settings, so a caller only sends what it
 * edits; the result is normalized, so an empty URL falls back to the default
 * instead of reaching the store.
 */
export async function updateSettings(patch: Partial<AppSettings>): Promise<void> {
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
 * Turn whatever was persisted into a complete, valid `AppSettings`.
 *
 * A non-object is never thrown on: it yields a fresh copy of the defaults.
 * Unknown keys are dropped — including the `llm*` and chat fields an older
 * version stored — and a wrong-typed field falls back to its default rather
 * than being coerced.
 */
export function normalizeSettings(raw: unknown): AppSettings {
  if (!isRecord(raw)) {
    return createDefaultSettings()
  }

  return {
    apiBase: normalizeUrl(raw.apiBase, DEFAULT_SETTINGS.apiBase),
  }
}

/** A fresh copy of the defaults, so a caller can never mutate the shared object. */
function createDefaultSettings(): AppSettings {
  return { ...DEFAULT_SETTINGS }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

/** A URL is trimmed; empty after trimming falls back to the default. */
function normalizeUrl(value: unknown, fallback: string): string {
  if (typeof value !== "string") {
    return fallback
  }

  const trimmed = value.trim()

  return trimmed === "" ? fallback : trimmed
}
