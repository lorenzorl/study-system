import { defineStore } from "pinia"
import { ref } from "vue"
import { chatsFolderPath } from "@/shared/obsidian"
import {
  appendTurn as apiAppendTurn,
  createChatNote as apiCreateChatNote,
  listChatPaths as apiListChatPaths,
  readChatNote as apiReadChatNote,
  writeChatPrompt as apiWriteChatPrompt,
  writeChatSettings as apiWriteChatSettings,
} from "../api/chat-note"
import { slugifyChatTitle } from "../lib/note"
import type { ChatNote, ChatNoteSettings, ChatTurn } from "./types"

const DEFAULT_SETTINGS: ChatNoteSettings = {
  model: null,
  temperature: null,
  topK: null,
}

export const useChatsStore = defineStore("chats", () => {
  const paths = ref<string[]>([])
  const currentPath = ref<string | null>(null)
  const note = ref<ChatNote | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadChats() {
    loading.value = true
    error.value = null
    try {
      paths.value = await apiListChatPaths()
    } catch (e) {
      error.value = messageOf(e, "No se pudieron cargar los chats.")
    } finally {
      loading.value = false
    }
  }

  async function selectChat(path: string) {
    loading.value = true
    error.value = null
    try {
      note.value = await apiReadChatNote(path)
      currentPath.value = path
    } catch (e) {
      error.value = messageOf(e, "No se pudo abrir el chat.")
    } finally {
      loading.value = false
    }
  }

  async function createChat(
    title: string,
    prompt = "",
    settings: ChatNoteSettings = DEFAULT_SETTINGS,
  ) {
    const path = `${chatsFolderPath()}/${slugifyChatTitle(title)}.md`
    loading.value = true
    error.value = null
    try {
      await apiCreateChatNote(path, prompt, settings)
      paths.value = await apiListChatPaths()
      note.value = await apiReadChatNote(path)
      currentPath.value = path
    } catch (e) {
      error.value = messageOf(e, "No se pudo crear el chat.")
    } finally {
      loading.value = false
    }
  }

  async function savePrompt(prompt: string) {
    if (!currentPath.value) return
    error.value = null
    try {
      await apiWriteChatPrompt(currentPath.value, prompt)
      if (note.value) {
        note.value = { ...note.value, chat: { ...note.value.chat, prompt } }
      }
    } catch (e) {
      error.value = messageOf(e, "No se pudo guardar el prompt.")
    }
  }

  async function saveSettings(settings: ChatNoteSettings) {
    if (!currentPath.value) return
    error.value = null
    try {
      await apiWriteChatSettings(currentPath.value, settings)
      if (note.value) {
        note.value = { ...note.value, chat: { ...note.value.chat, settings } }
      }
    } catch (e) {
      error.value = messageOf(e, "No se pudieron guardar los ajustes del chat.")
    }
  }

  async function appendTurn(turn: ChatTurn) {
    if (!currentPath.value) return
    error.value = null
    try {
      await apiAppendTurn(currentPath.value, turn)
      if (note.value) {
        note.value = { ...note.value, turns: [...note.value.turns, turn] }
      }
    } catch (e) {
      error.value = messageOf(e, "No se pudo anexar el turno.")
    }
  }

  function clearError() {
    error.value = null
  }

  return {
    paths,
    currentPath,
    note,
    loading,
    error,
    loadChats,
    selectChat,
    createChat,
    savePrompt,
    saveSettings,
    appendTurn,
    clearError,
  }
})

function messageOf(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback
}
