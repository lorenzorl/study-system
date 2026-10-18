import type { Plugin } from "obsidian"
import { setApiBase } from "@/shared/api/client"
import {
  mergeLegacyApiBase,
  normalizeSettings,
  onSettingsChange,
  setSettingsStore,
  updateSettings,
} from "@/shared/config"

/** The key the plugin used to persist the backend base URL before `data.json`. */
const LEGACY_API_BASE_KEY = "study-api-base"

/**
 * Load the plugin's global settings, adopt the legacy `localStorage` base URL
 * when there is one, and wire the resolved base URL into the HTTP client.
 *
 * This is the one place that bridges `shared/config` and `shared/api`: they are
 * sibling segments of the same layer, so the boundary test forbids them from
 * importing each other (ARCHITECTURE §3).
 */
export async function loadPluginSettings(plugin: Plugin): Promise<void> {
  setSettingsStore({
    load: () => plugin.loadData(),
    save: (data) => plugin.saveData(data),
  })

  const stored = normalizeSettings(await plugin.loadData())
  const resolved = mergeLegacyApiBase(stored, readLegacyApiBase())

  // `updateSettings` normalizes, installs and persists the resolved settings, so
  // a completed migration survives the next load. When there was nothing to
  // migrate, `resolved` is `stored` and this writes the same values back.
  await updateSettings(resolved)

  pushApiBase(resolved.apiBase)
  onSettingsChange((settings) => pushApiBase(settings.apiBase))
}

/** Push a base URL into the HTTP client. */
function pushApiBase(apiBase: string): void {
  setApiBase(apiBase)
}

/**
 * Read the legacy base URL, tolerating an embedding without `localStorage` (or
 * one that blocks it): a missing legacy value must never break plugin load.
 */
function readLegacyApiBase(): string | null {
  try {
    return localStorage.getItem(LEGACY_API_BASE_KEY)
  } catch {
    return null
  }
}
