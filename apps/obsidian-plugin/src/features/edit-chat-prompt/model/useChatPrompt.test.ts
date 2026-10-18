/**
 * The edit-chat-prompt interaction: the draft is seeded by the caller, `dirty`
 * compares it with the current note's prompt, `save` goes through the store's
 * prompt action, and `reset` restores the note's value.
 */
import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import type { ChatNote } from "@/entities/chat"

const mocks = vi.hoisted(() => {
  const state = { note: null as ChatNote | null }

  const savePrompt = vi.fn(async (prompt: string) => {
    if (state.note) {
      state.note = { ...state.note, chat: { ...state.note.chat, prompt } }
    }
  })

  return { state, savePrompt }
})

vi.mock("@/entities/chat", () => ({
  useChatsStore: () => ({
    get note() {
      return mocks.state.note
    },
    savePrompt: mocks.savePrompt,
  }),
}))

import { useChatPrompt } from "./useChatPrompt"

function noteWithPrompt(prompt: string): ChatNote {
  return {
    chat: {
      path: "Study Dashboard/Chats/ddd.md",
      title: "ddd",
      prompt,
      settings: { model: null, temperature: null, topK: null },
    },
    turns: [],
    attachments: [],
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  mocks.state.note = noteWithPrompt("Prompt original.")
})

describe("useChatPrompt", () => {
  it("seeds the draft from the note and reports it as clean", () => {
    const prompt = useChatPrompt()

    prompt.loadFrom(noteWithPrompt("Prompt original."))

    expect(prompt.draft.value).toBe("Prompt original.")
    expect(prompt.dirty.value).toBe(false)
  })

  it("is dirty once the draft differs from the note's prompt", () => {
    const prompt = useChatPrompt()
    prompt.loadFrom(noteWithPrompt("Prompt original."))

    prompt.draft.value = "Prompt nuevo."

    expect(prompt.dirty.value).toBe(true)
  })

  it("persists the draft through the store's save action", async () => {
    const prompt = useChatPrompt()
    prompt.loadFrom(noteWithPrompt("Prompt original."))
    prompt.draft.value = "Prompt nuevo."

    await prompt.save()

    expect(mocks.savePrompt).toHaveBeenCalledWith("Prompt nuevo.")
    expect(prompt.dirty.value).toBe(false)
  })

  it("reset restores the draft from the current note", () => {
    const prompt = useChatPrompt()
    prompt.loadFrom(noteWithPrompt("Prompt original."))
    prompt.draft.value = "Prompt descartado."

    prompt.reset()

    expect(prompt.draft.value).toBe("Prompt original.")
    expect(prompt.dirty.value).toBe(false)
  })
})
