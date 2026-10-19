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
import type { ChatAttachment, ChatNote, ChatNoteSettings, ChatTurn } from "../model/types"
import {
  appendChatTurn,
  createChatNote as buildChatNote,
  mergeIntoExistingNote,
  parseChatNote,
  setAttachments,
  setChatPrompt,
  setChatSettings,
  uniqueChatPath,
} from "../lib/note"

export async function listChatPaths(): Promise<string[]> {
  return listVaultFiles(chatsFolderPath())
}

export async function readChatNote(path: string): Promise<ChatNote> {
  return parseChatNote(await readVaultFile(path), path)
}

/**
 * Create a chat note for a title, at a path that is free.
 *
 * The title is the user's and may repeat; the path is ours and may not. Two
 * default clicks on "Nuevo chat" therefore make two chats, because the second
 * one counts up instead of landing on the first one's file.
 *
 * Returns the path it wrote, so the caller can select the chat it just made
 * rather than recomputing the name and guessing wrong.
 */
export async function createChatForTitle(
  title: string,
  prompt: string,
  settings: ChatNoteSettings,
): Promise<string> {
  const path = uniqueChatPath(await listChatPaths(), chatsFolderPath(), title)

  await createChatNote(path, prompt, settings)

  return path
}

export async function createChatNote(
  path: string,
  prompt: string,
  settings: ChatNoteSettings,
): Promise<void> {
  // An empty file is a new note. A file with content is a note the user already
  // has — the store picks a free path first, so reaching this branch means the
  // path appeared since — and it is folded into rather than replaced.
  await processVaultFile(path, (data) =>
    data.trim() === ""
      ? buildChatNote(prompt, settings)
      : mergeIntoExistingNote(data, prompt, settings),
  )
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

/**
 * Replace the note's attachment region, and nothing else.
 *
 * The whole list travels at once because the region is the list: attaching adds
 * one entry and detaching removes one, and both are a rewrite of what the note
 * should now hold.
 */
export async function writeChatAttachments(
  path: string,
  attachments: ChatAttachment[],
): Promise<void> {
  await processVaultFile(path, (data) => setAttachments(data, attachments))
}

export async function appendTurn(path: string, turn: ChatTurn): Promise<void> {
  await processVaultFile(path, (data) => appendChatTurn(data, turn))
}

export function subscribeToChats(listener: () => void): () => void {
  return onVaultFileChange(chatsFolderPath(), listener)
}
