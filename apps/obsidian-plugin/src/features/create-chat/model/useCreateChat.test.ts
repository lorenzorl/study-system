/**
 * The create-chat interaction.
 *
 * The chat store is mocked at the module boundary so this test can assert what
 * the feature hands it (the starter prompt and the empty overrides) and how it
 * turns a failure into its own Spanish message without throwing.
 */
import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"

const mocks = vi.hoisted(() => {
  const state = { error: null as string | null }

  return {
    state,
    createChat: vi.fn(async () => {}),
    getSettings: vi.fn(() => ({ defaultChatPrompt: "Prompt del global." })),
  }
})

vi.mock("@/entities/chat", () => ({
  useChatsStore: () => ({
    get error() {
      return mocks.state.error
    },
    createChat: mocks.createChat,
  }),
}))

vi.mock("@/shared/config", () => ({
  getSettings: mocks.getSettings,
}))

import { useCreateChat } from "./useCreateChat"

const STARTING_SETTINGS = { model: null, temperature: null, topK: null }

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  mocks.state.error = null
  mocks.createChat.mockImplementation(async () => {})
})

describe("useCreateChat.create", () => {
  it("starts the chat from the global prompt and the default overrides", async () => {
    const store = useCreateChat()

    await store.create("DDD")

    expect(mocks.createChat).toHaveBeenCalledWith(
      "DDD",
      "Prompt del global.",
      STARTING_SETTINGS,
    )
    expect(store.error).toBeNull()
    expect(store.creating).toBe(false)
  })

  it("surfaces the store's error as its own and never throws", async () => {
    mocks.createChat.mockImplementation(async () => {
      mocks.state.error = "No se pudo crear el chat."
    })

    const store = useCreateChat()

    await expect(store.create("DDD")).resolves.toBeUndefined()
    expect(store.error).toBe("No se pudo crear el chat.")
  })

  it("reports a Spanish message when the store itself throws", async () => {
    mocks.createChat.mockImplementation(async () => {
      throw new Error("boom")
    })

    const store = useCreateChat()

    await expect(store.create("DDD")).resolves.toBeUndefined()
    expect(store.error).toBe("No se pudo crear el chat.")
  })

  it("clearError drops the message", async () => {
    mocks.createChat.mockImplementation(async () => {
      mocks.state.error = "No se pudo crear el chat."
    })

    const store = useCreateChat()
    await store.create("DDD")

    store.clearError()

    expect(store.error).toBeNull()
  })
})
