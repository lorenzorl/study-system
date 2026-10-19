/**
 * The vault material of one send (S7, S8).
 *
 * Two things are pinned here. The port's behaviour: a vault file goes in whole,
 * identified and cited by its path and never by an invented page. And the
 * send-time re-check of the size policy: the caps were enforced when the file was
 * attached, but a note can grow afterwards, so the only place that can catch a
 * file that no longer fits is the moment it is about to be sent.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  MAX_ATTACHMENT_CHARS,
  MAX_TOTAL_ATTACHMENT_CHARS,
} from "@/entities/chat"
import { readVaultMaterial, setVaultTextReader } from "./vault-text"

const THESIS_PATH = "Notas/Tesis.md"
const THESIS_TEXT = [
  "El estado es efímero y se descarta al recargar.",
  "",
  "## Capítulos",
  "- Uno",
  "- Dos",
].join("\n")
const OTHER_PATH = "Notas/Otra.md"
const OTHER_TEXT = "Una nota corta."

function vaultAttachment(path: string) {
  return { kind: "vault" as const, path }
}

/** A reader over an in-memory vault; a path that is not there cannot be read. */
function fakeVault(files: Record<string, string>) {
  return vi.fn(async (path: string) => {
    const text = files[path]

    if (text === undefined) throw new Error(`No se encontró la nota "${path}".`)

    return text
  })
}

let read: ReturnType<typeof fakeVault>

beforeEach(() => {
  read = fakeVault({ [THESIS_PATH]: THESIS_TEXT, [OTHER_PATH]: OTHER_TEXT })
  setVaultTextReader(read)
})

afterEach(() => {
  setVaultTextReader(null)
})

describe("readVaultMaterial", () => {
  it("reads nothing and returns no blocks when the chat has no vault attachment", async () => {
    const result = await readVaultMaterial([])

    expect(result).toEqual({ ok: true, blocks: [] })
    expect(read).not.toHaveBeenCalled()
  })

  it("ignores corpus attachments: only the vault class is read", async () => {
    const result = await readVaultMaterial([
      { kind: "corpus", documentId: "doc-ddd", title: "Domain-Driven Design" },
    ])

    expect(result).toEqual({ ok: true, blocks: [] })
    expect(read).not.toHaveBeenCalled()
  })

  it("returns the file's whole text as a vault block, identified by its path", async () => {
    const result = await readVaultMaterial([vaultAttachment(THESIS_PATH)])

    expect(result).toEqual({
      ok: true,
      blocks: [{ kind: "vault", path: THESIS_PATH, text: THESIS_TEXT }],
    })
  })

  it("keeps the attachment order, one block per file", async () => {
    const result = await readVaultMaterial([
      vaultAttachment(THESIS_PATH),
      vaultAttachment(OTHER_PATH),
    ])

    expect(result).toEqual({
      ok: true,
      blocks: [
        { kind: "vault", path: THESIS_PATH, text: THESIS_TEXT },
        { kind: "vault", path: OTHER_PATH, text: OTHER_TEXT },
      ],
    })
  })

  it("refuses, naming the file and what to do, when a file cannot be read", async () => {
    const result = await readVaultMaterial([vaultAttachment("Notas/Borrada.md")])

    expect(result.ok).toBe(false)
    if (result.ok) return

    expect(result.error).toContain("No se pudo leer")
    expect(result.error).toContain("Notas/Borrada.md")
    expect(result.error).toContain("no se envió")
    expect(result.error).toContain("Verificá que la nota exista")
    expect(result.error).toContain("volvé a intentar")
  })

  // "The file could not be read" is not a diagnosis: a note that was moved, a note
  // the user cannot read and a vault that failed all look the same, and the reader
  // cannot act on any of them. The reason the vault gave is carried through, so
  // "no existe" and "no tengo permiso" stop being the same message.
  it("carries the vault's own reason, so a missing note differs from a permission problem", async () => {
    read.mockRejectedValueOnce(new Error("No se encontró la nota \"Notas/Tesis.md\"."))

    const result = await readVaultMaterial([vaultAttachment(THESIS_PATH)])

    expect(result.ok).toBe(false)
    if (result.ok) return

    expect(result.error).toContain("No se encontró la nota")
    expect(result.error).toContain(THESIS_PATH)
  })

  it("names both possible causes when the failure carries no reason of its own", async () => {
    // A rejected string, or an error with an empty message, has nothing to quote.
    read.mockRejectedValueOnce("boom")

    const result = await readVaultMaterial([vaultAttachment(THESIS_PATH)])

    expect(result.ok).toBe(false)
    if (result.ok) return

    expect(result.error).toContain("que la nota exista")
    expect(result.error).toContain("permiso")
    expect(result.error).not.toContain("boom")
  })

  it("refuses a file that grew past the per-file cap, saying its size and the cap", async () => {
    const grown = MAX_ATTACHMENT_CHARS + 1

    // The reader hands back a note that grew after it was attached.
    read.mockImplementation(async () => "x".repeat(grown))
    const refused = await readVaultMaterial([vaultAttachment(THESIS_PATH)])

    expect(refused.ok).toBe(false)
    if (refused.ok) return

    expect(refused.error).toContain(THESIS_PATH)
    expect(refused.error).toContain(String(grown))
    expect(refused.error).toContain(String(MAX_ATTACHMENT_CHARS))
    expect(refused.error).toContain("no se envió")
    expect(refused.error).toContain("Partilo en notas más chicas")
  })

  it("accepts a file of exactly the per-file cap", async () => {
    read.mockImplementation(async () => "x".repeat(MAX_ATTACHMENT_CHARS))
    const result = await readVaultMaterial([vaultAttachment(THESIS_PATH)])

    expect(result.ok).toBe(true)
  })

  it("refuses when the files together exceed the total cap", async () => {
    // Each file fits on its own; together they do not.
    const perFile = 70_000

    read.mockImplementation(async () => "x".repeat(perFile))
    const result = await readVaultMaterial([
      vaultAttachment(THESIS_PATH),
      vaultAttachment(OTHER_PATH),
    ])

    expect(result.ok).toBe(false)
    if (result.ok) return

    expect(perFile * 2).toBeGreaterThan(MAX_TOTAL_ATTACHMENT_CHARS)
    expect(result.error).toContain(String(perFile * 2))
    expect(result.error).toContain(String(MAX_TOTAL_ATTACHMENT_CHARS))
    expect(result.error).toContain("no se envió")
    expect(result.error).toContain("Quitá algún adjunto")
  })

  it("refuses instead of crashing when no reader is installed", async () => {
    setVaultTextReader(null)
    const result = await readVaultMaterial([vaultAttachment(THESIS_PATH)])

    expect(result.ok).toBe(false)
    if (result.ok) return

    expect(result.error).toContain(THESIS_PATH)
    expect(result.error).toContain("lector del vault no está disponible")
  })
})
