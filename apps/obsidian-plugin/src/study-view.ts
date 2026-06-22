import { ItemView, type WorkspaceLeaf } from "obsidian";
import { createApp, type App as VueApp } from "vue";
import { createPinia } from "pinia";
import { router } from "./vue-app/router";
import App from "./vue-app/App.vue";

export const STUDY_VIEW_TYPE = "study-dashboard";

export class StudyView extends ItemView {
  private vueApp: VueApp<Element> | null = null;

  constructor(leaf: WorkspaceLeaf) {
    super(leaf);
  }

  getViewType(): string {
    return STUDY_VIEW_TYPE;
  }

  getDisplayText(): string {
    return "Study";
  }

  getIcon(): string {
    return "graduation-cap";
  }

  async onOpen(): Promise<void> {
    this.vueApp = createApp(App);
    this.vueApp.use(router);
    this.vueApp.use(createPinia());
    this.vueApp.mount(this.contentEl);
  }

  async onClose(): Promise<void> {
    if (this.vueApp) {
      this.vueApp.unmount();
      this.vueApp = null;
    }
  }
}
