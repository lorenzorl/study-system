import { App, Modal } from "obsidian";
import type { Flashcard } from "./types";
import { nextIndex, prevIndex } from "./validation";

export class FlashcardModal extends Modal {
  private flashcards: Flashcard[];
  private currentIndex: number;
  private isFlipped: boolean;

  constructor(app: App, flashcards: Flashcard[]) {
    super(app);
    this.flashcards = flashcards;
    this.currentIndex = 0;
    this.isFlipped = false;
  }

  onOpen(): void {
    this.containerEl.addClass("flashcard-modal");
    this.render();
  }

  private render(): void {
    const { contentEl } = this;
    contentEl.empty();

    if (this.flashcards.length === 0) {
      this.renderEmptyState(contentEl);
      return;
    }

    this.renderCard(contentEl);
    this.renderNav(contentEl);
  }

  private renderEmptyState(container: HTMLElement): void {
    container.createDiv({
      cls: "flashcard-empty",
      text: "No flashcards available.",
    });
  }

  private renderCard(container: HTMLElement): void {
    const card = this.flashcards[this.currentIndex];
    const cardEl = container.createDiv({ cls: "flashcard-card" });

    const faceEl = cardEl.createDiv({
      cls: `flashcard-face ${this.isFlipped ? "is-flipped" : ""}`,
    });
    faceEl.setText(this.isFlipped ? card.answer : card.question);

    cardEl.addEventListener("click", () => {
      this.isFlipped = !this.isFlipped;
      this.render();
    });
  }

  private renderNav(container: HTMLElement): void {
    const total = this.flashcards.length;
    const navEl = container.createDiv({ cls: "flashcard-nav" });

    const prevBtn = navEl.createEl("button", {
      cls: "flashcard-nav-btn",
      text: "← Prev",
    });
    prevBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      this.currentIndex = prevIndex(this.currentIndex, total);
      this.isFlipped = false;
      this.render();
    });

    navEl.createDiv({
      cls: "flashcard-progress",
      text: `${this.currentIndex + 1} / ${total}`,
    });

    const nextBtn = navEl.createEl("button", {
      cls: "flashcard-nav-btn",
      text: "Next →",
    });
    nextBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      this.currentIndex = nextIndex(this.currentIndex, total);
      this.isFlipped = false;
      this.render();
    });
  }

  onClose(): void {
    this.containerEl.empty();
  }
}
