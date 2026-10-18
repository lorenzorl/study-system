/**
 * The chat slice's I/O adapter.
 *
 * It lives in the `api/` segment because that is the FSD name for "the thing
 * that talks to the outside", but this one talks to the Obsidian vault — plain
 * `.md` files — not to HTTP. Every write is routed through `processVaultFile`
 * so the read-modify-write stays atomic and never races the user's editor.
 */
import {
  chatsFolderPath,
  listVaultFiles,
  onVaultFileChange,
  processVaultFile,
  readVaultFile,
} from "@/shared/obsidian"
import type { ChatNote, ChatNoteSettings, ChatTurn } from "../model/types"
import {
  appendChatTurn,
  createChatNote as buildChatNote,
  parseChatNote,
  setChatPrompt,
  setChatSettings,
} from "../lib/note"

export async function listChatPaths(): Promise<string[]> {
  return listVaultFiles(chatsFolderPath())
}

export async function readChatNote(path: string): Promise<ChatNote> {
  return parseChatNote(await readVaultFile(path), path)
}

export async function createChatNote(
  path: string,
  prompt: string,
  settings: ChatNoteSettings,
): Promise<void> {
  await processVaultFile(path, () => buildChatNote(prompt, settings))
}

export async function writeChatPrompt(
  path: string,
  prompt: string,
): Promise<void> {
  await processVaultFile(path, (data) => setChatPrompt(data, prompt))
}

export async function writeChatSettings(
  path: string,
  settings: ChatNoteSettings,
): Promise<void> {
  await processVaultFile(path, (data) => setChatSettings(data, settings))
}

export async function appendTurn(path: string, turn: ChatTurn): Promise<void> {
  await processVaultFile(path, (data) => appendChatTurn(data, turn))
}

export function subscribeToChats(listener: () => void): () => void {
  return onVaultFileChange(chatsFolderPath(), listener)
}
