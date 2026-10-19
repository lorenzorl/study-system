import { describe, expect, it, vi } from "vitest"
import type { ChatAttachment } from "@/entities/chat"
import { MAX_ATTACHMENT_CHARS, MAX_TOTAL_ATTACHMENT_CHARS } from "@/entities/chat"
import { attachVaultFile } from "./useAttachResource"

const CORPUS: ChatAttachment = {
  kind: "corpus",
  documentId: "d1",
  title: "DDD",
}

/** An injected read over an in-memory map; missing paths throw like the vault. */
function readFrom(files: Record<string, string>) {
  return vi.fn(async (path: string): Promise<string> => {
    const content = files[path]
    if (content === undefined) throw new Error(`No se encontró "${path}".`)
    return content
  })
}

describe("attachVaultFile", () => {
  it("returns the vault attachment when the file fits", async () => {
    const read = readFrom({ "Notas/a.md": "hola" })

    await expect(attachVaultFile("Notas/a.md", [], read)).resolves.toEqual({
      ok: true,
      attachment: { kind: "vault", path: "Notas/a.md" },
    })
    expect(read).toHaveBeenCalledWith("Notas/a.md")
  })

  it("sums the existing vault attachments by reading them", async () => {
    const read = readFrom({
      "Notas/a.md": "x".repeat(1000),
      "Notas/b.md": "y".repeat(100),
    })

    await attachVaultFile("Notas/b.md", [{ kind: "vault", path: "Notas/a.md" }], read)

    expect(read.mock.calls.map(([path]) => path)).toEqual([
      "Notas/a.md",
      "Notas/b.md",
    ])
  })

  it("does not read or count corpus attachments", async () => {
    const read = readFrom({ "Notas/b.md": "y".repeat(10) })

    await attachVaultFile("Notas/b.md", [CORPUS], read)

    expect(read).toHaveBeenCalledTimes(1)
    expect(read).toHaveBeenCalledWith("Notas/b.md")
  })

  it("rejects a file over the per-file cap with a Spanish message", async () => {
    const read = readFrom({ "Notas/grande.md": "x".repeat(MAX_ATTACHMENT_CHARS + 1) })

    const result = await attachVaultFile("Notas/grande.md", [], read)

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error).toContain("Notas/grande.md")
    expect(result.error).toContain(String(MAX_ATTACHMENT_CHARS))
  })

  it("rejects a file that would push the total over the cap", async () => {
    const read = readFrom({
      "Notas/ya.md": "x".repeat(1000),
      "Notas/nueva.md": "y".repeat(MAX_TOTAL_ATTACHMENT_CHARS),
    })

    const result = await attachVaultFile(
      "Notas/nueva.md",
      [{ kind: "vault", path: "Notas/ya.md" }],
      read,
    )

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error).toContain("Notas/nueva.md")
    expect(result.error).toContain(String(MAX_TOTAL_ATTACHMENT_CHARS))
  })

  it("rejects a file that cannot be read instead of throwing", async () => {
    const read = readFrom({})

    const result = await attachVaultFile("Notas/falta.md", [], read)

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error).toContain("Notas/falta.md")
  })

  it("ignores an existing attachment that can no longer be read", async () => {
    // Its text cannot reach the model anyway, so it contributes nothing to the
    // total; the candidate is what is being accepted here.
    const read = vi.fn(async (path: string): Promise<string> => {
      if (path === "Notas/vieja.md") throw new Error("No se encontró.")
      return "y".repeat(10)
    })

    await expect(
      attachVaultFile(
        "Notas/nueva.md",
        [{ kind: "vault", path: "Notas/vieja.md" }],
        read,
      ),
    ).resolves.toEqual({
      ok: true,
      attachment: { kind: "vault", path: "Notas/nueva.md" },
    })
  })
})
