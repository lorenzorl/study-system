import { beforeEach, describe, expect, it, vi } from "vitest"
import {
  DEFAULT_SETTINGS,
  getSettings,
  mergeLegacyApiBase,
  normalizeSettings,
  onSettingsChange,
  setSettingsStore,
  updateSettings,
  type SettingsStore,
} from "./settings"

/** A store that records what was persisted, so the tests can assert the writes. */
function createFakeStore(): SettingsStore & { saved: unknown[] } {
  const saved: unknown[] = []

  return {
    saved,
    load: async () => saved.at(-1) ?? null,
    save: async (data: unknown) => {
      saved.push(data)
    },
  }
}

/** Install a fresh store and restore the only setting to its default. */
async function resetSettings(): Promise<void> {
  setSettingsStore(createFakeStore())
  await updateSettings(DEFAULT_SETTINGS)
}

beforeEach(async () => {
  await resetSettings()
})

describe("normalizeSettings", () => {
  it.each([
    ["undefined", undefined],
    ["null", null],
    ["an array", []],
    ["a string", "settings"],
    ["a number", 42],
  ])("falls back to a fresh copy of the defaults for %s", (_label, raw) => {
    const settings = normalizeSettings(raw)

    expect(settings).toEqual(DEFAULT_SETTINGS)
    expect(settings).not.toBe(DEFAULT_SETTINGS)
  })

  it("drops unknown keys, including the llm fields an older version stored", () => {
    const settings = normalizeSettings({
      llmModel: "qwen/qwen3.5-flash-02-23",
      llmApiKey: "sk-test",
      retrievalTopK: 12,
      historyTurns: 8,
      defaultChatPrompt: "Respondé en criollo.",
      unrelated: true,
    })

    expect(settings).toEqual(DEFAULT_SETTINGS)
    expect(Object.keys(settings)).toEqual(["apiBase"])
  })

  it("keeps the apiBase a partial object supplies", () => {
    const settings = normalizeSettings({ apiBase: "http://192.168.1.50:8080" })

    expect(settings).toEqual({ apiBase: "http://192.168.1.50:8080" })
  })

  it.each([
    ["a number", 8080],
    ["null", null],
    ["a boolean", false],
  ])("falls back to the default instead of coercing a wrong-typed apiBase (%s)", (_label, raw) => {
    const settings = normalizeSettings({ apiBase: raw })

    expect(settings.apiBase).toBe(DEFAULT_SETTINGS.apiBase)
  })

  describe("apiBase", () => {
    it("trims the stored URL", () => {
      expect(normalizeSettings({ apiBase: "  http://192.168.1.50:8080  " }).apiBase).toBe(
        "http://192.168.1.50:8080",
      )
    })

    it("falls back to the default when the URL is empty after trimming", () => {
      expect(normalizeSettings({ apiBase: "   " }).apiBase).toBe(DEFAULT_SETTINGS.apiBase)
      expect(normalizeSettings({ apiBase: "" }).apiBase).toBe(DEFAULT_SETTINGS.apiBase)
    })
  })

  it("never mutates the shared default settings", () => {
    normalizeSettings({ apiBase: "  http://192.168.1.50:8080  ", unrelated: true })

    expect(DEFAULT_SETTINGS.apiBase).toBe("http://localhost:8080")
  })
})

describe("mergeLegacyApiBase", () => {
  it("adopts a trimmed non-empty legacy value while apiBase is still the default", () => {
    const settings = normalizeSettings({})

    const migrated = mergeLegacyApiBase(settings, "  http://192.168.1.50:8080  ")

    expect(migrated.apiBase).toBe("http://192.168.1.50:8080")
    expect(settings.apiBase).toBe(DEFAULT_SETTINGS.apiBase)
  })

  it("never overwrites an apiBase the user already stored", () => {
    const settings = normalizeSettings({ apiBase: "http://10.0.0.7:8080" })

    expect(mergeLegacyApiBase(settings, "http://192.168.1.50:8080")).toBe(settings)
  })

  it.each([
    ["null", null],
    ["an empty string", ""],
    ["a blank string", "   "],
  ])("returns the settings unchanged for %s", (_label, legacy) => {
    const settings = normalizeSettings({})

    expect(mergeLegacyApiBase(settings, legacy)).toBe(settings)
  })

  it("returns the settings unchanged when the legacy value is already the default", () => {
    const settings = normalizeSettings({})

    expect(mergeLegacyApiBase(settings, DEFAULT_SETTINGS.apiBase)).toBe(settings)
  })

  it("is pure — it does not mutate the settings it receives", () => {
    const settings = normalizeSettings({})

    mergeLegacyApiBase(settings, "http://192.168.1.50:8080")

    expect(settings.apiBase).toBe(DEFAULT_SETTINGS.apiBase)
  })
})

describe("settings singleton", () => {
  it("returns the defaults before anything else is set", () => {
    expect(getSettings()).toEqual(DEFAULT_SETTINGS)
  })

  it("merges a patch, persists the normalized result and notifies listeners", async () => {
    const store = createFakeStore()
    setSettingsStore(store)
    const listener = vi.fn()

    const unsubscribe = onSettingsChange(listener)

    await updateSettings({ apiBase: "http://192.168.1.50:8080" })

    expect(getSettings().apiBase).toBe("http://192.168.1.50:8080")
    expect(store.saved).toEqual([{ apiBase: "http://192.168.1.50:8080" }])
    expect(listener).toHaveBeenCalledTimes(1)
    expect(listener.mock.calls[0][0]).toEqual(getSettings())

    unsubscribe()
  })

  it("persists the default when a patch normalizes back to it", async () => {
    const store = createFakeStore()
    setSettingsStore(store)

    await updateSettings({ apiBase: "   " })

    expect(getSettings().apiBase).toBe(DEFAULT_SETTINGS.apiBase)
    expect(store.saved).toEqual([{ apiBase: DEFAULT_SETTINGS.apiBase }])
  })

  it("stops notifying a listener after it unsubscribes", async () => {
    const listener = vi.fn()
    const unsubscribe = onSettingsChange(listener)

    unsubscribe()
    await updateSettings({ apiBase: "http://10.0.0.7:8080" })

    expect(listener).not.toHaveBeenCalled()
  })

  it("keeps the previous settings when no store is registered", async () => {
    setSettingsStore(null as unknown as SettingsStore)

    await expect(updateSettings({ apiBase: "http://10.0.0.7:8080" })).resolves.toBeUndefined()
    expect(getSettings().apiBase).toBe("http://10.0.0.7:8080")
  })
})
