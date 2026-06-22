import { Plugin, Notice } from "obsidian"
import { StudyView, STUDY_VIEW_TYPE } from "./study-view"
import { setObsidianApp, useObsidian } from "./vue-app/composables/useObsidian"

export default class FlashcardPlugin extends Plugin {
  onload(): void {
    // Set the app singleton early so commands can access it
    setObsidianApp(this.app)

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
  }

  onunload(): void {
    // Clean up the app singleton
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
    const { syncConcept, syncFlashcards } = await import(
      "./vue-app/services/api"
    )
    const { parseFlashcards } = await import(
      "./vue-app/services/markdown-parser"
    )

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
      const conceptResponse = await syncConcept({
        topic_name: topicName,
        concept_title: conceptTitle,
        file_path: filePath,
      })

      const parsedCards = parseFlashcards(fileContent)

      if (parsedCards.length === 0) {
        new Notice(
          "No se encontraron tarjetas en la nota. Usá el formato pregunta::respuesta.",
        )
        return
      }

      const flashcardResponse = await syncFlashcards({
        concept_id: conceptResponse.concept_id,
        cards: parsedCards,
      })

      new Notice(
        `Sincronizadas ${flashcardResponse.synced} tarjetas de "${conceptTitle}".`,
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
