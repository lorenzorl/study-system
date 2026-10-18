import { beforeEach, describe, expect, it, vi } from "vitest"
import { getApiBase } from "@/shared/api/client"
import { DEFAULT_SETTINGS, getSettings, updateSettings } from "@/shared/config"
import { loadPluginSettings } from "./settings-store"

/**
 * The one part of the settings subsystem that runs against the runtime instead
 * of against a pure function, and the case review finding
 * R3-legacy-api-base-resurrection named: the migration out of `localStorage`
 * must happen ONCE. While the legacy key lingers, a user who deliberately saves
 * the default base URL gets the stale legacy one back after the next reload,
 * after the form told them "Guardado".
 */

type PluginLike = Parameters<typeof loadPluginSettings>[0]

const LEGACY_KEY = "study-api-base"
const LEGACY_URL = "http://192.168.1.50:8080"

/** A `Plugin` over one in-memory value, plus the value it holds now. */
function fakePlugin(initial: unknown) {
  let stored = initial

  return {
    plugin: {
      loadData: async () => stored,
      saveData: async (data: unknown) => void (stored = data),
    } as unknown as PluginLike,
    stored: () => stored,
  }
}

/** The two `localStorage` calls the migration makes. */
function fakeLocalStorage(entries: Record<string, string>) {
  const map = new Map(Object.entries(entries))

  return {
    getItem: (key: string): string | null => map.get(key) ?? null,
    removeItem: (key: string): void => void map.delete(key),
  }
}

beforeEach(async () => {
  await updateSettings(DEFAULT_SETTINGS)
})

describe("loadPluginSettings", () => {
  it("adopts the legacy base URL once and consumes the legacy key", async () => {
    const storage = fakeLocalStorage({ [LEGACY_KEY]: LEGACY_URL })
    vi.stubGlobal("localStorage", storage)
    const { plugin, stored } = fakePlugin(null)

    await loadPluginSettings(plugin)

    expect(getSettings().apiBase).toBe(LEGACY_URL)
    expect(getApiBase()).toBe(LEGACY_URL)
    expect((stored() as { apiBase: string }).apiBase).toBe(LEGACY_URL)
    expect(storage.getItem(LEGACY_KEY)).toBeNull()
  })

  it("keeps a deliberately saved default base URL after a reload", async () => {
    vi.stubGlobal("localStorage", fakeLocalStorage({ [LEGACY_KEY]: LEGACY_URL }))

    const first = fakePlugin(null)
    await loadPluginSettings(first.plugin)
    expect(getSettings().apiBase).toBe(LEGACY_URL)

    // What the settings form does when the user saves the default back.
    await updateSettings({ apiBase: DEFAULT_SETTINGS.apiBase })

    await loadPluginSettings(fakePlugin(first.stored()).plugin)

    expect(getSettings().apiBase).toBe(DEFAULT_SETTINGS.apiBase)
    expect(getApiBase()).toBe(DEFAULT_SETTINGS.apiBase)
  })
})
