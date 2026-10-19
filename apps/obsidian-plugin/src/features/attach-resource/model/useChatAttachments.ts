/**
 * Choosing what a chat brings into context (S3, S7).
 *
 * The attachments themselves are the note's data — `entities/chat` owns the
 * region and its parser — so this store is the interaction in front of it: list
 * what can be chosen, attach, detach, and report in Spanish whatever refused the
 * operation. The note is the single source of truth; this module keeps no copy
 * of the list, so a note edited by hand cannot drift from what the panel shows.
 *
 * A corpus document is identified by its `document_id` and a vault file by its
 * path, and both are attached once: attaching a path twice would spend the size
 * budget twice for one file, because `attachVaultFile` sums the note's vault
 * attachments by reading them.
 */
import { defineStore } from "pinia"
import { computed, ref } from "vue"
import { useChatsStore } from "@/entities/chat"
import type { ChatAttachment } from "@/entities/chat"
import { listDocuments } from "@/entities/document"
import type { DocumentSummary } from "@/entities/document"
import { listVaultMarkdownFiles, readVaultFile } from "@/shared/obsidian"
import { attachVaultFile } from "./useAttachResource"

const NO_CHAT_ERROR =
  "No hay un chat abierto. Abrí o creá un chat antes de adjuntar."
const CORPUS_LIST_ERROR = "No se pudo listar el corpus."

export const useChatAttachments = defineStore("chat-attachments", () => {
  const chats = useChatsStore()

  /** The corpus, as the server lists it: what can be attached, not what is. */
  const corpus = ref<DocumentSummary[]>([])
  /** The vault's markdown files, as candidate paths. */
  const vaultFiles = ref<string[]>([])
  const loadingCorpus = ref(false)
  const error = ref<string | null>(null)

  /** What this chat has attached now, read from the note itself. */
  const attachments = computed<ChatAttachment[]>(
    () => chats.note?.attachments ?? [],
  )

  /**
   * List the corpus.
   *
   * A failure is reported and not thrown: the panel keeps working with whatever
   * the vault can offer, and a chat that only needs vault files still can attach
   * them with the server down (S1, S7).
   */
  async function loadCorpus(): Promise<void> {
    loadingCorpus.value = true
    error.value = null

    try {
      corpus.value = await listDocuments()
    } catch (e) {
      corpus.value = []
      error.value = `${CORPUS_LIST_ERROR} ${messageOf(e)}`
    } finally {
      loadingCorpus.value = false
    }
  }

  /** The vault listing is local and synchronous; there is nothing to await. */
  function loadVaultFiles(): void {
    vaultFiles.value = listVaultMarkdownFiles()
  }

  async function attachCorpus(document: DocumentSummary): Promise<void> {
    if (!hasChat()) return

    const attachment: ChatAttachment = {
      kind: "corpus",
      documentId: document.id,
      title: document.title,
    }

    if (attachments.value.some((current) => same(current, attachment))) return

    await persist([...attachments.value, attachment])
  }

  /**
   * Attach one vault file, whole.
   *
   * The size policy decides, and its rejection is the message the user reads: a
   * file over the cap is refused here, when it is attached, and never discovered
   * halfway through a send (S8).
   */
  async function attachVault(path: string): Promise<void> {
    if (!hasChat()) return

    if (attachments.value.some((current) => same(current, { kind: "vault", path }))) {
      error.value = `«${path}» ya está adjunto a este chat.`
      return
    }

    // The policy reads every existing vault attachment to sum the total, so it
    // receives the reader rather than this module reaching into the vault alone.
    const result = await attachVaultFile(path, attachments.value, readVaultFile)

    if (!result.ok) {
      error.value = result.error
      return
    }

    await persist([...attachments.value, result.attachment])
  }

  /** Remove one attachment, keeping the rest in the order they were added. */
  async function detach(attachment: ChatAttachment): Promise<void> {
    if (!hasChat()) return

    await persist(attachments.value.filter((current) => !same(current, attachment)))
  }

  function hasChat(): boolean {
    if (chats.note !== null) return true

    error.value = NO_CHAT_ERROR
    return false
  }

  /**
   * Write the next list and let the note decide whether it happened.
   *
   * The store's own error carries the failure, because the write and its reason
   * belong together; clearing it first means a stale rejection cannot be mistaken
   * for this operation's outcome.
   */
  async function persist(next: ChatAttachment[]): Promise<void> {
    error.value = null
    await chats.saveAttachments(next)

    if (chats.error !== null) {
      error.value = chats.error
    }
  }

  function clearError(): void {
    error.value = null
  }

  return {
    corpus,
    vaultFiles,
    attachments,
    loadingCorpus,
    error,
    loadCorpus,
    loadVaultFiles,
    attachCorpus,
    attachVault,
    detach,
    clearError,
  }
})

/** Two attachments are the same when they name the same material. */
function same(a: ChatAttachment, b: ChatAttachment): boolean {
  if (a.kind === "corpus" && b.kind === "corpus") {
    return a.documentId === b.documentId
  }
  if (a.kind === "vault" && b.kind === "vault") {
    return a.path === b.path
  }
  return false
}

function messageOf(e: unknown): string {
  return e instanceof Error && e.message
    ? e.message
    : "Volvé a intentar en unos segundos."
}
