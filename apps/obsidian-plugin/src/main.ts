import { Plugin } from "obsidian";
import { StudyView, STUDY_VIEW_TYPE } from "./study-view";

export default class FlashcardPlugin extends Plugin {
  onload(): void {
    this.registerView(STUDY_VIEW_TYPE, (leaf) => new StudyView(leaf));

    this.addRibbonIcon("graduation-cap", "Open Study Dashboard", () => {
      this.activateView();
    });

    this.addCommand({
      id: "open-study-dashboard",
      name: "Open Study Dashboard",
      callback: () => {
        this.activateView();
      },
    });
  }

  async activateView(): Promise<void> {
    const { workspace } = this.app;

    // Reveal existing leaf if already open
    const existing = workspace.getLeavesOfType(STUDY_VIEW_TYPE);
    if (existing.length > 0) {
      workspace.revealLeaf(existing[0]);
      return;
    }

    // Create a new leaf in the right sidebar
    const leaf = workspace.getRightLeaf(false);
    if (leaf) {
      await leaf.setViewState({ type: STUDY_VIEW_TYPE, active: true });
      workspace.revealLeaf(leaf);
    }
  }
}
