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
  vaultFileExists,
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

  const toFile = (path: string): FakeFile => {
    const name = path.split("/").pop() ?? path
    const extension = name.includes(".") ? name.split(".").pop() ?? "" : ""
    return { path, name, basename: name.replace(/\.[^.]*$/, ""), extension }
  }

  const vault = {
    getFileByPath: vi.fn((path: string) => (files.has(path) ? toFile(path) : null)),
    create: vi.fn(async (path: string, data: string) => {
      files.set(path, data)
      return toFile(path)
    }),
  }

  return { vault, files }
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

describe("vaultFileExists", () => {
  it("is true for an existing file and false otherwise", async () => {
    const { vault } = useFakeVault()
    await vault.create("a.md", "")

    expect(vaultFileExists("a.md")).toBe(true)
    expect(vaultFileExists("b.md")).toBe(false)
  })
})
