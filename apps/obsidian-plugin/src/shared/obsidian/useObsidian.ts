import { normalizePath, type App, type Vault } from "obsidian"

// Singleton pattern — set by study-view.ts / main.ts on mount
let _app: App | null = null

const PLUGIN_FOLDER = "Study Dashboard"
const CHATS_FOLDER = `${PLUGIN_FOLDER}/Chats`

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

/** The folder that holds every chat note. */
export function chatsFolderPath(): string {
  return CHATS_FOLDER
}

function requireVault(): Vault {
  const vault = getVault()
  if (!vault) {
    throw new Error("El vault de Obsidian no está disponible.")
  }
  return vault
}

/** Read a vault file's text. Throws when the file does not exist. */
export async function readVaultFile(path: string): Promise<string> {
  const vault = requireVault()
  const normalized = normalizePath(path)
  const file = vault.getFileByPath(normalized)
  if (!file) {
    throw new Error(`No se encontró la nota "${normalized}".`)
  }
  return await vault.read(file)
}

/** Whether a file exists at the given vault path. */
export function vaultFileExists(path: string): boolean {
  const vault = getVault()
  if (!vault) return false
  return vault.getFileByPath(normalizePath(path)) !== null
}

/** Create the parent folder when missing, then create or overwrite the file. */
export async function writeVaultFile(
  path: string,
  content: string,
): Promise<void> {
  const vault = requireVault()
  const normalized = normalizePath(path)
  const parent = normalized.split("/").slice(0, -1).join("/")
  if (parent) {
    await ensureFolder(parent)
  }

  const existing = vault.getFileByPath(normalized)
  if (existing) {
    await vault.modify(existing, content)
    return
  }
  await vault.create(normalized, content)
}

/**
 * Atomic read-modify-write of a vault file, built on `vault.process`. A
 * `read` + `modify` pair would race with the user's own editor and surface
 * Obsidian's "file changed on disk" conflict. Creates the file (and its parent
 * folder) when missing so every write can route through one primitive.
 */
export async function processVaultFile(
  path: string,
  fn: (data: string) => string,
): Promise<string> {
  const vault = requireVault()
  const normalized = normalizePath(path)
  let file = vault.getFileByPath(normalized)

  if (!file) {
    const parent = normalized.split("/").slice(0, -1).join("/")
    if (parent) {
      await ensureFolder(parent)
    }
    await vault.create(normalized, "")
    file = vault.getFileByPath(normalized)
  }

  if (!file) {
    throw new Error(`No se pudo crear la nota "${normalized}".`)
  }

  return await vault.process(file, fn)
}

/** Every `.md` file in the vault, as vault-relative paths, sorted. */
export function listVaultMarkdownFiles(): string[] {
  const vault = getVault()
  if (!vault) return []

  return vault
    .getFiles()
    .filter((file) => file.extension === "md")
    .map((file) => file.path)
    .sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
}

/** The `.md` files directly inside `folder`, as vault-relative paths, sorted. */
export function listVaultFiles(folder: string): string[] {
  const vault = getVault()
  if (!vault) return []

  const normalized = normalizePath(folder)
  const prefix = normalized ? `${normalized}/` : ""

  return vault
    .getFiles()
    .filter((file) => {
      if (file.extension !== "md") return false
      if (!file.path.startsWith(prefix)) return false
      return !file.path.slice(prefix.length).includes("/")
    })
    .map((file) => file.path)
    .sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
}

/**
 * Subscribe to vault changes that touch `folder`. Returns an unsubscribe
 * function that removes the registration.
 */
export function onVaultFileChange(
  folder: string,
  listener: () => void,
): () => void {
  const vault = getVault()
  if (!vault) return () => {}

  const normalized = normalizePath(folder)
  const prefix = normalized ? `${normalized}/` : ""

  const isInside = (candidate: unknown): boolean =>
    typeof candidate === "string" && candidate.startsWith(prefix)

  const handler = (...args: unknown[]): void => {
    const file = args[0] as { path?: string } | undefined
    if (isInside(file?.path) || isInside(args[1])) {
      listener()
    }
  }

  vault.on("modify", handler)
  vault.on("create", handler)
  vault.on("delete", handler)
  vault.on("rename", handler)

  return () => {
    vault.off("modify", handler)
    vault.off("create", handler)
    vault.off("delete", handler)
    vault.off("rename", handler)
  }
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
