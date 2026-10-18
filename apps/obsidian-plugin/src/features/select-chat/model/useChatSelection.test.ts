/**
 * The select-chat interaction: switching chats is exactly the store's
 * `selectChat`, and re-selecting the current chat is a no-op.
 */
import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"

const mocks = vi.hoisted(() => ({
  state: { currentPath: null as string | null },
  selectChat: vi.fn(async () => {}),
}))

vi.mock("@/entities/chat", () => ({
  useChatsStore: () => ({
    get currentPath() {
      return mocks.state.currentPath
    },
    selectChat: mocks.selectChat,
  }),
}))

import { useChatSelection } from "./useChatSelection"

const PATH = "Study Dashboard/Chats/ddd.md"

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  mocks.state.currentPath = null
})

describe("useChatSelection.select", () => {
  it("selects a different chat path", () => {
    const selection = useChatSelection()

    selection.select(PATH)

    expect(mocks.selectChat).toHaveBeenCalledWith(PATH)
  })

  it("does nothing when the path is already current", () => {
    mocks.state.currentPath = PATH
    const selection = useChatSelection()

    selection.select(PATH)

    expect(mocks.selectChat).not.toHaveBeenCalled()
  })
})
