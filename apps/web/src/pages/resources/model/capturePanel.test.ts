import { describe, expect, it, vi } from "vitest"
import { useCapturePanel } from "./capturePanel"

describe("useCapturePanel", () => {
  it("starts closed", () => {
    expect(useCapturePanel().isOpen.value).toBe(false)
  })

  it("opens and closes, returning focus to the element that opened it", async () => {
    const panel = useCapturePanel()
    const opener = { focus: vi.fn() }

    panel.open(opener)
    expect(panel.isOpen.value).toBe(true)

    await panel.close()

    expect(panel.isOpen.value).toBe(false)
    expect(opener.focus).toHaveBeenCalledTimes(1)
  })

  it("closes without an opener", async () => {
    const panel = useCapturePanel()

    panel.open(null)
    await panel.close()

    expect(panel.isOpen.value).toBe(false)
  })

  it("returns focus once per open, not once per close", async () => {
    const panel = useCapturePanel()
    const opener = { focus: vi.fn() }

    panel.open(opener)
    await panel.close()
    await panel.close()

    expect(opener.focus).toHaveBeenCalledTimes(1)
  })
})
