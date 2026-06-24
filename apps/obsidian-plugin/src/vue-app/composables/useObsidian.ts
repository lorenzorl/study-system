import { normalizePath, type App, type Vault } from "obsidian"

// Singleton pattern — set by study-view.ts / main.ts on mount
let _app: App | null = null

const PLUGIN_FOLDER = "Study Dashboard"

/** Returns the Obsidian Vault instance, or null if not yet initialized. */
export function getVault(): Vault | null {
  return _app?.vault ?? null
}

/** Sanitize a topic/concept name for use as a folder name. */
export function sanitizeFolderName(name: string): string {
  return name
    .replace(/\//g, "-")
    .replace(/[\\:*?"<>|]/g, "")
    .trim()
}

/**
 * Ensure a folder exists at the given vault-relative path.
 * Idempotent — does nothing if the folder already exists.
 */
export async function ensureFolder(path: string): Promise<void> {
  const vault = getVault()
  if (!vault) {
    console.warn("[Study Dashboard] Vault not available, skipping folder creation:", path)
    return
  }

  const normalized = normalizePath(path)

  const existing = vault.getFolderByPath(normalized)
  if (existing) return // already exists

  try {
    await vault.createFolder(normalized)
    console.log("[Study Dashboard] Created folder:", normalized)
  } catch (err) {
    console.error("[Study Dashboard] Failed to create folder:", normalized, err)
  }
}

/** Build the full vault path for a topic folder. */
export function topicFolderPath(topicName: string): string {
  return `${PLUGIN_FOLDER}/${sanitizeFolderName(topicName)}`
}

/** Build the full vault path for a concept subfolder. */
export function conceptFolderPath(topicName: string, conceptTitle: string): string {
  return `${PLUGIN_FOLDER}/${sanitizeFolderName(topicName)}/${sanitizeFolderName(conceptTitle)}`
}

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
    return await _app!.vault.read(file)
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
