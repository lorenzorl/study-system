import { App, PluginSettingTab, Setting, type Plugin } from "obsidian"
import { DEFAULT_SETTINGS, getSettings, updateSettings } from "@/shared/config"

/**
 * The plugin's own settings tab.
 *
 * One field on purpose: the backend base URL. Every request the plugin makes —
 * syncing a note, listing due cards, grading one — goes through it, so it is the
 * only setting left now that the chat, the model configuration and the document
 * corpus are gone. Obsidian's own `Setting` renders it; there is no Vue form
 * behind this tab.
 */
export class StudySettingsTab extends PluginSettingTab {
  /** The value being edited, so a half-typed URL is never persisted. */
  private draft = DEFAULT_SETTINGS.apiBase

  constructor(app: App, plugin: Plugin) {
    super(app, plugin)

    // `SettingTab` carries the icon as a property, not as a getter. The tab's
    // title comes from the plugin's manifest name, so there is nothing to
    // override for it.
    this.icon = "settings"
  }

  display(): void {
    this.containerEl.empty()
    this.draft = getSettings().apiBase

    new Setting(this.containerEl)
      .setName("URL del backend")
      .setDesc(
        "Dirección del servidor de estudio. Por defecto, http://localhost:8080. " +
          "Dejá el campo vacío para volver al valor por defecto.",
      )
      .addText((text) =>
        text
          .setPlaceholder(DEFAULT_SETTINGS.apiBase)
          .setValue(this.draft)
          .onChange((value) => {
            this.draft = value
          }),
      )
      .addButton((button) =>
        button.setButtonText("Guardar").onClick(async () => {
          // `updateSettings` normalizes (trims, empty → default) and persists.
          await updateSettings({ apiBase: this.draft })
          // Re-read so the field shows the value that survived normalization.
          this.draft = getSettings().apiBase
          this.display()
        }),
      )
  }
}
