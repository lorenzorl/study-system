import { Plugin } from "obsidian";
import { FlashcardModal } from "./modal";
import { SAMPLE_FLASHCARDS } from "./data";

export default class FlashcardPlugin extends Plugin {
  onload(): void {
    this.addCommand({
      id: "show-flashcards",
      name: "Show flashcards",
      callback: () => {
        new FlashcardModal(this.app, SAMPLE_FLASHCARDS).open();
      },
    });
  }
}
