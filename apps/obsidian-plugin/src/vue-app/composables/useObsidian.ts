import type { App } from "obsidian"

// Singleton pattern — set by study-view.ts / main.ts on mount
let _app: App | null = null

export function setObsidianApp(app: App): void {
  _app = app
}

export function useObsidian() {
  function getActiveFilePath(): string | null {
    return _app?.workspace.getActiveFile()?.path ?? null
  }

  async function getActiveFileContent(): Promise<string> {
    const file = _app?.workspace.getActiveFile()
    if (!file) {
      throw new Error("No hay nota activa. Abrí una nota primero.")
    }
    return await _app.vault.read(file)
  }

  function getTopicNameFromPath(path: string): string {
    // "DDD/Agregados.md" → "DDD"
    // Root file → "General"
    const parts = path.split("/")
    if (parts.length < 2) return "General"
    return parts[0] ?? "General"
  }

  function getConceptTitleFromPath(path: string): string {
    // "DDD/Agregados.md" → "Agregados"
    // "Agregados.md" → "Agregados"
    const filename = path.split("/").pop() ?? path
    return filename.replace(/\.md$/, "")
  }

  return {
    getActiveFilePath,
    getActiveFileContent,
    getTopicNameFromPath,
    getConceptTitleFromPath,
  }
}
