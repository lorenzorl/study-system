import { beforeEach, describe, expect, it, vi } from "vitest"
import {
  DEFAULT_SETTINGS,
  getSettings,
  mergeLegacyApiBase,
  normalizeSettings,
  onSettingsChange,
  setSettingsStore,
  updateSettings,
  type PluginSettings,
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

/** Install a fresh store and restore every field to its default. */
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

  it("drops unknown keys and defaults the fields a partial object omits", () => {
    const settings = normalizeSettings({ llmModel: "qwen/qwen3.5-flash-02-23", unrelated: true })

    expect(settings).toEqual({ ...DEFAULT_SETTINGS, llmModel: "qwen/qwen3.5-flash-02-23" })
    expect(settings).not.toHaveProperty("unrelated")
  })

  it("keeps the fields a partial object supplies and defaults the rest", () => {
    const settings = normalizeSettings({
      apiBase: "http://192.168.1.50:8080",
      retrievalTopK: 12,
      historyTurns: 0,
    })

    expect(settings).toEqual({
      ...DEFAULT_SETTINGS,
      apiBase: "http://192.168.1.50:8080",
      retrievalTopK: 12,
      historyTurns: 0,
    })
  })

  it.each([
    ["apiBase", 8080],
    ["llmBaseUrl", null],
    ["llmApiKey", 12345],
    ["llmModel", false],
    ["defaultChatPrompt", ["prompt"]],
  ])("falls back to the default instead of coercing a wrong-typed %s", (field, raw) => {
    const settings = normalizeSettings({ [field]: raw })

    expect(settings[field as keyof PluginSettings]).toBe(DEFAULT_SETTINGS[field as keyof PluginSettings])
  })

  it("falls back to the default for a wrong-typed number field", () => {
    const settings = normalizeSettings({
      retrievalTopK: "6",
      historyTurns: "8",
      llmTemperature: "0.2",
    })

    expect(settings.retrievalTopK).toBe(DEFAULT_SETTINGS.retrievalTopK)
    expect(settings.historyTurns).toBe(DEFAULT_SETTINGS.historyTurns)
    expect(settings.llmTemperature).toBe(DEFAULT_SETTINGS.llmTemperature)
  })

  describe("retrievalTopK", () => {
    it("clamps an integer to 1..50", () => {
      expect(normalizeSettings({ retrievalTopK: 0 }).retrievalTopK).toBe(1)
      expect(normalizeSettings({ retrievalTopK: 1 }).retrievalTopK).toBe(1)
      expect(normalizeSettings({ retrievalTopK: 50 }).retrievalTopK).toBe(50)
      expect(normalizeSettings({ retrievalTopK: 51 }).retrievalTopK).toBe(50)
      expect(normalizeSettings({ retrievalTopK: 9999 }).retrievalTopK).toBe(50)
    })

    it("falls back to the default for a non-integer or non-number", () => {
      expect(normalizeSettings({ retrievalTopK: 2.5 }).retrievalTopK).toBe(DEFAULT_SETTINGS.retrievalTopK)
      expect(normalizeSettings({ retrievalTopK: Number.NaN }).retrievalTopK).toBe(DEFAULT_SETTINGS.retrievalTopK)
      expect(normalizeSettings({ retrievalTopK: null }).retrievalTopK).toBe(DEFAULT_SETTINGS.retrievalTopK)
    })
  })

  describe("historyTurns", () => {
    it("clamps an integer to 0..50", () => {
      expect(normalizeSettings({ historyTurns: -3 }).historyTurns).toBe(0)
      expect(normalizeSettings({ historyTurns: 0 }).historyTurns).toBe(0)
      expect(normalizeSettings({ historyTurns: 50 }).historyTurns).toBe(50)
      expect(normalizeSettings({ historyTurns: 100 }).historyTurns).toBe(50)
    })

    it("falls back to the default for a non-integer or non-number", () => {
      expect(normalizeSettings({ historyTurns: 1.5 }).historyTurns).toBe(DEFAULT_SETTINGS.historyTurns)
      expect(normalizeSettings({ historyTurns: Number.NaN }).historyTurns).toBe(DEFAULT_SETTINGS.historyTurns)
      expect(normalizeSettings({ historyTurns: { turns: 4 } }).historyTurns).toBe(DEFAULT_SETTINGS.historyTurns)
    })
  })

  describe("llmTemperature", () => {
    it("clamps a number to 0..2", () => {
      expect(normalizeSettings({ llmTemperature: -1 }).llmTemperature).toBe(0)
      expect(normalizeSettings({ llmTemperature: 0 }).llmTemperature).toBe(0)
      expect(normalizeSettings({ llmTemperature: 1.35 }).llmTemperature).toBe(1.35)
      expect(normalizeSettings({ llmTemperature: 3 }).llmTemperature).toBe(2)
    })

    it("falls back to the default for a non-number", () => {
      expect(normalizeSettings({ llmTemperature: "0.9" }).llmTemperature).toBe(
        DEFAULT_SETTINGS.llmTemperature,
      )
      expect(normalizeSettings({ llmTemperature: Number.NaN }).llmTemperature).toBe(
        DEFAULT_SETTINGS.llmTemperature,
      )
    })
  })

  describe("URL fields", () => {
    it("trims apiBase and llmBaseUrl", () => {
      const settings = normalizeSettings({
        apiBase: "  http://192.168.1.50:8080  ",
        llmBaseUrl: "  https://openrouter.ai/api/v1  ",
      })

      expect(settings.apiBase).toBe("http://192.168.1.50:8080")
      expect(settings.llmBaseUrl).toBe("https://openrouter.ai/api/v1")
    })

    it("falls back to the default when a URL is empty after trimming", () => {
      expect(normalizeSettings({ apiBase: "   " }).apiBase).toBe(DEFAULT_SETTINGS.apiBase)
      expect(normalizeSettings({ apiBase: "" }).apiBase).toBe(DEFAULT_SETTINGS.apiBase)
      expect(normalizeSettings({ llmBaseUrl: "   " }).llmBaseUrl).toBe(DEFAULT_SETTINGS.llmBaseUrl)
    })
  })

  it("keeps every other string field verbatim when it is a string", () => {
    const settings = normalizeSettings({
      llmApiKey: "sk-test",
      llmModel: "openai/gpt-4o-mini",
      defaultChatPrompt: "Respondé en criollo.",
    })

    expect(settings.llmApiKey).toBe("sk-test")
    expect(settings.llmModel).toBe("openai/gpt-4o-mini")
    expect(settings.defaultChatPrompt).toBe("Respondé en criollo.")
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

    await updateSettings({ llmModel: "qwen/qwen3.5-flash-02-23", retrievalTopK: 999 })

    expect(getSettings().llmModel).toBe("qwen/qwen3.5-flash-02-23")
    expect(getSettings().retrievalTopK).toBe(50)
    expect(store.saved).toEqual([
      { ...DEFAULT_SETTINGS, llmModel: "qwen/qwen3.5-flash-02-23", retrievalTopK: 50 },
    ])
    expect(listener).toHaveBeenCalledTimes(1)
    expect(listener.mock.calls[0][0]).toEqual(getSettings())

    unsubscribe()
  })

  it("stops notifying a listener after it unsubscribes", async () => {
    const listener = vi.fn()
    const unsubscribe = onSettingsChange(listener)

    unsubscribe()
    await updateSettings({ llmModel: "openai/gpt-4o-mini" })

    expect(listener).not.toHaveBeenCalled()
  })

  it("keeps the previous settings when no store is registered", async () => {
    setSettingsStore(null as unknown as SettingsStore)

    await expect(updateSettings({ llmModel: "openai/gpt-4o-mini" })).resolves.toBeUndefined()
    expect(getSettings().llmModel).toBe("openai/gpt-4o-mini")
  })
})
