import { Plugin, Notice, normalizePath } from "obsidian"
import { StudyView, STUDY_VIEW_TYPE } from "./study-view"
import { loadPluginSettings } from "./settings-store"
import { StudySettingsTab } from "./settings-tab"
import { setObsidianApp, useObsidian } from "@/shared/obsidian"

export default class FlashcardPlugin extends Plugin {
  async onload(): Promise<void> {
    // Set the app singleton early so commands can access it
    setObsidianApp(this.app)

    // Own the plugin's global settings (data.json) and expose its settings tab
    await loadPluginSettings(this)
    this.addSettingTab(new StudySettingsTab(this.app, this))

    this.registerView(STUDY_VIEW_TYPE, (leaf) => new StudyView(leaf))

    this.addRibbonIcon("graduation-cap", "Open Study Dashboard", () => {
      this.activateView()
    })

    this.addCommand({
      id: "open-study-dashboard",
      name: "Open Study Dashboard",
      callback: () => {
        this.activateView()
      },
    })

    this.addCommand({
      id: "sync-active-note",
      name: "Sincronizar nota",
      callback: () => {
        this.syncActiveNote()
      },
    })

    // Ensure the plugin root folder exists in the vault
    const vault = this.app.vault
    const normalized = normalizePath("Study Dashboard")
    if (!vault.getFolderByPath(normalized)) {
      try {
        await vault.createFolder(normalized)
        console.log("[Study Dashboard] Root folder created")
      } catch (err) {
        console.error("[Study Dashboard] Failed to create root folder:", err)
      }
    }
  }

  onunload(): void {
    // SAFETY: clearing the singleton is deliberate; the cast stands in for
    // `App | null`, which `setObsidianApp` does not accept today.
    setObsidianApp(null as unknown as import("obsidian").App)
  }

  async activateView(): Promise<void> {
    const { workspace } = this.app

    // Reveal existing leaf if already open
    const existing = workspace.getLeavesOfType(STUDY_VIEW_TYPE)
    if (existing.length > 0) {
      workspace.revealLeaf(existing[0])
      return
    }

    // Create a new leaf in the right sidebar
    const leaf = workspace.getRightLeaf(false)
    if (leaf) {
      await leaf.setViewState({ type: STUDY_VIEW_TYPE, active: true })
      workspace.revealLeaf(leaf)
    }
  }

  async syncActiveNote(): Promise<void> {
    const { syncNoteToBackend } = await import("@/features/sync-note")

    const obsidian = useObsidian()
    const filePath = obsidian.getActiveFilePath()

    if (!filePath) {
      new Notice("No hay nota activa. Abrí una nota primero.")
      return
    }

    const topicName = obsidian.getTopicNameFromPath(filePath)
    const conceptTitle = obsidian.getConceptTitleFromPath(filePath)

    let fileContent: string
    try {
      fileContent = await obsidian.getActiveFileContent()
    } catch {
      new Notice("No se pudo leer la nota activa.")
      return
    }

    try {
      const result = await syncNoteToBackend({
        topicName,
        conceptTitle,
        filePath,
        fileContent,
      })

      new Notice(
        `Sincronizadas ${result.synced} tarjetas de "${conceptTitle}".`,
      )

      // Open the dashboard view to show results
      await this.activateView()
    } catch (e) {
      const message =
        e instanceof Error
          ? e.message
          : "Error del servidor al sincronizar."
      new Notice(message)
    }
  }
}
