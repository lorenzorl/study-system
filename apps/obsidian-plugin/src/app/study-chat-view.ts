import { ItemView, type WorkspaceLeaf } from "obsidian"
import { createApp, type App as VueApp } from "vue"
import { createPinia } from "pinia"
import { setObsidianApp } from "@/shared/obsidian"
import { ChatPage } from "@/pages/chat"

export const STUDY_CHAT_VIEW_TYPE = "study-chat"

/**
 * The chat as a persistent main-area tab.
 *
 * It mirrors `StudyView`'s structure — an `ItemView` that owns the Vue app it
 * mounts — and differs only in where it is placed: the chat opens in the main
 * area (`getLeaf("tab")`), not the right sidebar.
 */
export class StudyChatView extends ItemView {
  private vueApp: VueApp<Element> | null = null

  constructor(leaf: WorkspaceLeaf) {
    super(leaf)
  }

  getViewType(): string {
    return STUDY_CHAT_VIEW_TYPE
  }

  getDisplayText(): string {
    return "Study Chat"
  }

  getIcon(): string {
    return "message-circle"
  }

  async onOpen(): Promise<void> {
    // Set the Obsidian app singleton before mounting Vue
    setObsidianApp(this.app)

    this.vueApp = createApp(ChatPage)
    this.vueApp.use(createPinia())
    this.vueApp.mount(this.contentEl)
  }

  async onClose(): Promise<void> {
    if (this.vueApp) {
      this.vueApp.unmount()
      this.vueApp = null
    }
  }
}
