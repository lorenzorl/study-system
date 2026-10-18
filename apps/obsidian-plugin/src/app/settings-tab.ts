import { App, PluginSettingTab, type Plugin } from "obsidian"
import { createApp, type App as VueApp } from "vue"
import { LlmSettingsForm } from "@/features/configure-llm"

/**
 * The plugin's own settings tab.
 *
 * It stays this thin on purpose: the tab is Obsidian glue (a `PluginSettingTab`
 * and a mount point), and everything the user edits is the Vue form inside
 * `features/configure-llm`, which talks to `shared/config`.
 */
export class ChatSettingsTab extends PluginSettingTab {
  private vueApp: VueApp<Element> | null = null

  constructor(app: App, plugin: Plugin) {
    super(app, plugin)

    // `SettingTab` carries the icon as a property, not as a getter. The tab's
    // title comes from the plugin's manifest name, so there is nothing to
    // override for it.
    this.icon = "settings"
  }

  display(): void {
    this.unmountForm()
    this.containerEl.empty()

    const container = this.containerEl.createDiv()

    this.vueApp = createApp(LlmSettingsForm)
    this.vueApp.mount(container)
  }

  hide(): void {
    this.unmountForm()
  }

  private unmountForm(): void {
    if (this.vueApp) {
      this.vueApp.unmount()
      this.vueApp = null
    }
  }
}
