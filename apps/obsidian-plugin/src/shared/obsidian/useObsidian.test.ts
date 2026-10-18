import { afterEach, describe, it, expect, vi } from "vitest"

// Obsidian is an environment package (provided by the Obsidian runtime), not a real npm
// package. Mock it so vitest can resolve the import from useObsidian.ts.
vi.mock("obsidian", () => ({
  normalizePath: (path: string) => path,
}))

import {
  sanitizeFolderName,
  topicFolderPath,
  conceptFolderPath,
  chatsFolderPath,
  readVaultFile,
  vaultFileExists,
  writeVaultFile,
  processVaultFile,
  listVaultFiles,
  onVaultFileChange,
  setObsidianApp,
} from "./useObsidian"

// ── sanitizeFolderName ──────────────────────────────────────────────────────

describe("sanitizeFolderName", () => {
  it("replaces forward slash with dash", () => {
    expect(sanitizeFolderName("C/C++")).toBe("C-C++")
  })

  it("replaces multiple slashes with dashes", () => {
    expect(sanitizeFolderName("a/b/c")).toBe("a-b-c")
  })

  it("strips illegal characters \\:*?\"<>|", () => {
    expect(sanitizeFolderName("C:\\test*?\"<>|")).toBe("Ctest")
  })

  it("returns clean name unchanged", () => {
    expect(sanitizeFolderName("DDD")).toBe("DDD")
  })

  it("trims leading and trailing whitespace", () => {
    expect(sanitizeFolderName("  DDD  ")).toBe("DDD")
  })

  it("returns empty string for whitespace-only input", () => {
    expect(sanitizeFolderName("   ")).toBe("")
  })

  it("returns empty string for input with only illegal chars", () => {
    expect(sanitizeFolderName("\\:*?\"<>|")).toBe("")
  })

  it("handles 'C/C++ : Advanced' — colon stripped, spaces preserved", () => {
    // Colon is stripped per R4 design; only / is replaced with -.
    // This deviates from spec scenario R4 which expected "C-C++ - Advanced".
    expect(sanitizeFolderName("C/C++ : Advanced")).toBe("C-C++  Advanced")
  })

  it("preserves spaces, dashes, and other valid chars", () => {
    expect(sanitizeFolderName("Study Dashboard - Español")).toBe(
      "Study Dashboard - Español",
    )
  })
})

// ── topicFolderPath & conceptFolderPath ─────────────────────────────────────

describe("topicFolderPath", () => {
  it('returns "Study Dashboard/{name}" for a clean topic name', () => {
    expect(topicFolderPath("DDD")).toBe("Study Dashboard/DDD")
  })

  it("sanitizes the topic name in the path", () => {
    expect(topicFolderPath("C/C++")).toBe("Study Dashboard/C-C++")
  })

  it("handles topic name with leading/trailing whitespace", () => {
    expect(topicFolderPath("  DDD  ")).toBe("Study Dashboard/DDD")
  })
})

describe("conceptFolderPath", () => {
  it('returns "Study Dashboard/{topic}/{concept}" for clean names', () => {
    expect(conceptFolderPath("DDD", "Agregados")).toBe(
      "Study Dashboard/DDD/Agregados",
    )
  })

  it("sanitizes both topic and concept names", () => {
    expect(conceptFolderPath("C/C++", "Pointers/Refs")).toBe(
      "Study Dashboard/C-C++/Pointers-Refs",
    )
  })

  it("handles whitespace in both names", () => {
    expect(conceptFolderPath("  DDD  ", "  Agregados  ")).toBe(
      "Study Dashboard/DDD/Agregados",
    )
  })
})

// ── Vault boundary ──────────────────────────────────────────────────────────

interface FakeFile {
  path: string
  name: string
  basename: string
  extension: string
}

function fakeVaultFixture() {
  const files = new Map<string, string>()
  const folders = new Set<string>()
  const listeners = new Map<string, Set<(...args: unknown[]) => unknown>>()

  const toFile = (path: string): FakeFile => {
    const name = path.split("/").pop() ?? path
    const extension = name.includes(".") ? name.split(".").pop() ?? "" : ""
    return { path, name, basename: name.replace(/\.[^.]*$/, ""), extension }
  }

  const vault = {
    getFileByPath: vi.fn((path: string) => (files.has(path) ? toFile(path) : null)),
    getFolderByPath: vi.fn((path: string) =>
      folders.has(path) ? { path } : null,
    ),
    createFolder: vi.fn(async (path: string) => {
      folders.add(path)
      return { path }
    }),
    create: vi.fn(async (path: string, data: string) => {
      files.set(path, data)
      return toFile(path)
    }),
    modify: vi.fn(async (file: FakeFile, data: string) => {
      files.set(file.path, data)
    }),
    read: vi.fn(async (file: FakeFile) => files.get(file.path) ?? ""),
    process: vi.fn(async (file: FakeFile, fn: (data: string) => string) => {
      const next = fn(files.get(file.path) ?? "")
      files.set(file.path, next)
      return next
    }),
    getFiles: vi.fn(() => [...files.keys()].map(toFile)),
    on: vi.fn((name: string, callback: (...args: unknown[]) => unknown) => {
      if (!listeners.has(name)) listeners.set(name, new Set())
      listeners.get(name)!.add(callback)
      return {}
    }),
    off: vi.fn((name: string, callback: (...args: unknown[]) => unknown) => {
      listeners.get(name)?.delete(callback)
    }),
  }

  const emit = (name: string, ...args: unknown[]) => {
    for (const callback of listeners.get(name) ?? []) callback(...args)
  }

  return { vault, files, folders, emit }
}

function useFakeVault() {
  const fixture = fakeVaultFixture()
  setObsidianApp(
    { vault: fixture.vault } as unknown as Parameters<typeof setObsidianApp>[0],
  )
  return fixture
}

afterEach(() => {
  setObsidianApp(null as unknown as Parameters<typeof setObsidianApp>[0])
})

describe("chatsFolderPath", () => {
  it('returns "Study Dashboard/Chats"', () => {
    expect(chatsFolderPath()).toBe("Study Dashboard/Chats")
  })
})

describe("readVaultFile", () => {
  it("reads an existing file", async () => {
    const { vault } = useFakeVault()
    await vault.create("Study Dashboard/Chats/a.md", "hola")

    await expect(readVaultFile("Study Dashboard/Chats/a.md")).resolves.toBe(
      "hola",
    )
  })

  it("throws a Spanish error when the file does not exist", async () => {
    useFakeVault()

    await expect(
      readVaultFile("Study Dashboard/Chats/missing.md"),
    ).rejects.toThrow('No se encontró la nota "Study Dashboard/Chats/missing.md".')
  })
})

describe("vaultFileExists", () => {
  it("is true for an existing file and false otherwise", async () => {
    const { vault } = useFakeVault()
    await vault.create("a.md", "")

    expect(vaultFileExists("a.md")).toBe(true)
    expect(vaultFileExists("b.md")).toBe(false)
  })
})

describe("writeVaultFile", () => {
  it("creates the parent folder when missing and creates the file", async () => {
    const { files, folders } = useFakeVault()

    await writeVaultFile("Study Dashboard/Chats/a.md", "contenido")

    expect(folders.has("Study Dashboard/Chats")).toBe(true)
    expect(files.get("Study Dashboard/Chats/a.md")).toBe("contenido")
  })

  it("overwrites an existing file", async () => {
    const { vault, files } = useFakeVault()
    await vault.create("a.md", "viejo")

    await writeVaultFile("a.md", "nuevo")

    expect(files.get("a.md")).toBe("nuevo")
  })
})

describe("processVaultFile", () => {
  it("returns the processed content and writes it back", async () => {
    const { vault, files } = useFakeVault()
    await vault.create("a.md", "hola")

    const result = await processVaultFile("a.md", (data: string) => `${data} mundo`)

    expect(result).toBe("hola mundo")
    expect(files.get("a.md")).toBe("hola mundo")
    expect(vault.process).toHaveBeenCalled()
  })

  it("creates the file when it does not exist", async () => {
    const { files } = useFakeVault()

    const result = await processVaultFile(
      "Study Dashboard/Chats/new.md",
      (data: string) => `${data}turn`,
    )

    expect(result).toBe("turn")
    expect(files.get("Study Dashboard/Chats/new.md")).toBe("turn")
  })
})

describe("listVaultFiles", () => {
  it("returns the direct .md files as vault paths, sorted", async () => {
    const { vault } = useFakeVault()
    await vault.create("Study Dashboard/Chats/b.md", "")
    await vault.create("Study Dashboard/Chats/a.md", "")
    await vault.create("Study Dashboard/Chats/sub/c.md", "")
    await vault.create("Study Dashboard/Chats/notes.txt", "")

    expect(listVaultFiles("Study Dashboard/Chats")).toEqual([
      "Study Dashboard/Chats/a.md",
      "Study Dashboard/Chats/b.md",
    ])
  })

  it("returns an empty array when the folder does not exist", () => {
    useFakeVault()

    expect(listVaultFiles("Study Dashboard/Missing")).toEqual([])
  })
})

describe("onVaultFileChange", () => {
  it("fires only for paths inside the folder and stops after unsubscribe", () => {
    const { emit } = useFakeVault()
    const listener = vi.fn()
    const unsubscribe = onVaultFileChange(
      "Study Dashboard/Chats",
      listener,
    )

    emit("modify", { path: "Study Dashboard/Chats/a.md" })
    expect(listener).toHaveBeenCalledTimes(1)

    emit("modify", { path: "Study Dashboard/Other/b.md" })
    expect(listener).toHaveBeenCalledTimes(1)

    emit("create", { path: "Study Dashboard/Chats/new.md" })
    expect(listener).toHaveBeenCalledTimes(2)

    emit("delete", { path: "Study Dashboard/Chats/gone.md" })
    expect(listener).toHaveBeenCalledTimes(3)

    emit(
      "rename",
      { path: "Study Dashboard/Chats/moved.md" },
      "Study Dashboard/Chats/old.md",
    )
    expect(listener).toHaveBeenCalledTimes(4)

    emit(
      "rename",
      { path: "Outside/x.md" },
      "Study Dashboard/Chats/old.md",
    )
    expect(listener).toHaveBeenCalledTimes(5)

    emit("modify", { path: "Study Dashboard/ChatsExtra/x.md" })
    expect(listener).toHaveBeenCalledTimes(5)

    unsubscribe()
    emit("modify", { path: "Study Dashboard/Chats/a.md" })
    expect(listener).toHaveBeenCalledTimes(5)
  })
})
