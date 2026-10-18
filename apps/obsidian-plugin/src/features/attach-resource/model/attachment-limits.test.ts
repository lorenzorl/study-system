import { describe, expect, it } from "vitest"
import {
  MAX_ATTACHMENT_CHARS,
  MAX_TOTAL_ATTACHMENT_CHARS,
  checkAttachmentLimits,
} from "./attachment-limits"

describe("checkAttachmentLimits", () => {
  it("accepts a candidate that fits under both caps", () => {
    expect(checkAttachmentLimits("Notas/a.md", 10, 20)).toEqual({ ok: true })
  })

  it("accepts a candidate exactly at the per-file cap", () => {
    expect(checkAttachmentLimits("Notas/a.md", MAX_ATTACHMENT_CHARS, 0).ok).toBe(
      true,
    )
  })

  it("accepts a candidate exactly at the total cap", () => {
    const decision = checkAttachmentLimits(
      "Notas/a.md",
      100,
      MAX_TOTAL_ATTACHMENT_CHARS - 100,
    )

    expect(decision.ok).toBe(true)
  })

  it("rejects a candidate over the per-file cap, naming file, size, cap and fix", () => {
    const decision = checkAttachmentLimits(
      "Notas/Tesis.md",
      MAX_ATTACHMENT_CHARS + 1,
      0,
    )

    expect(decision.ok).toBe(false)
    if (decision.ok) return
    expect(decision.error).toContain("Notas/Tesis.md")
    expect(decision.error).toContain(String(MAX_ATTACHMENT_CHARS + 1))
    expect(decision.error).toContain(String(MAX_ATTACHMENT_CHARS))
    expect(decision.error).toMatch(/partir|fragmento|chicas/i)
  })

  it("rejects a candidate that would exceed the total cap, naming the cap and fix", () => {
    const decision = checkAttachmentLimits(
      "Notas/b.md",
      100,
      MAX_TOTAL_ATTACHMENT_CHARS - 50,
    )

    expect(decision.ok).toBe(false)
    if (decision.ok) return
    expect(decision.error).toContain("Notas/b.md")
    expect(decision.error).toContain(String(MAX_TOTAL_ATTACHMENT_CHARS))
    expect(decision.error).toMatch(/quit|eleg|chico/i)
  })

  it("reports the per-file cap when both caps are exceeded", () => {
    const decision = checkAttachmentLimits(
      "Notas/c.md",
      MAX_ATTACHMENT_CHARS + 1,
      MAX_TOTAL_ATTACHMENT_CHARS,
    )

    expect(decision.ok).toBe(false)
    if (decision.ok) return
    expect(decision.error).toContain("por adjunto")
  })
})
