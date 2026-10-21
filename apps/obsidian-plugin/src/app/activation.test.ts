import { describe, expect, it, vi } from "vitest"
import { openStudyView, type ActivationWorkspace, type ViewLeaf } from "./activation"

const VIEW = "study-dashboard"

function fakeLeaf(): ViewLeaf & { states: Array<{ type: string, active: boolean }> } {
  const states: Array<{ type: string, active: boolean }> = []
  return {
    states,
    setViewState: vi.fn(async (state) => { states.push(state) }),
  }
}

function fakeWorkspace(overrides: Partial<ActivationWorkspace> = {}) {
  const created = fakeLeaf()
  const workspace: ActivationWorkspace = {
    getLeavesOfType: vi.fn(() => []),
    revealLeaf: vi.fn(),
    getLeaf: vi.fn(() => created),
    ...overrides,
  }
  return { workspace, created }
}

describe("openStudyView", () => {
  it("opens in a main-area tab, not in a sidebar leaf", async () => {
    const { workspace, created } = fakeWorkspace()

    await openStudyView(workspace, VIEW)

    expect(workspace.getLeaf).toHaveBeenCalledWith("tab")
    expect(created.states).toEqual([{ type: VIEW, active: true }])
  })

  it("reveals the new leaf only after its view state is set", async () => {
    const order: string[] = []
    const created = fakeLeaf()
    const setViewState = created.setViewState
    created.setViewState = async (state) => {
      order.push("setViewState")
      await setViewState(state)
    }
    const { workspace } = fakeWorkspace({
      getLeaf: vi.fn(() => created),
      revealLeaf: vi.fn(() => { order.push("revealLeaf") }),
    })

    await openStudyView(workspace, VIEW)

    expect(order).toEqual(["setViewState", "revealLeaf"])
  })

  it("reuses an already-open view instead of creating a second leaf", async () => {
    const existing = fakeLeaf()
    const { workspace } = fakeWorkspace({
      getLeavesOfType: vi.fn(() => [existing]),
    })

    await openStudyView(workspace, VIEW)

    expect(workspace.getLeaf).not.toHaveBeenCalled()
    expect(workspace.revealLeaf).toHaveBeenCalledWith(existing)
    expect(existing.states).toEqual([])
  })
})
