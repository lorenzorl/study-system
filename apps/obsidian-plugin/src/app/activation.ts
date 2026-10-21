/**
 * Where the study view opens, and how an already-open one is reused.
 *
 * Kept out of the plugin class so the invariant is testable without an Obsidian
 * runtime: the view is a main-area tab, and a second open request focuses the
 * existing tab instead of mounting a second view over the same stores.
 */

/** The slice of Obsidian's `WorkspaceLeaf` this module depends on. */
export interface ViewLeaf {
  setViewState(state: { type: string, active: boolean }): Promise<void>
}

/** The slice of Obsidian's `Workspace` this module depends on. */
export interface ActivationWorkspace {
  getLeavesOfType(type: string): ViewLeaf[]
  revealLeaf(leaf: ViewLeaf): void
  getLeaf(kind: "tab" | "split"): ViewLeaf
}

export async function openStudyView(
  workspace: ActivationWorkspace,
  viewType: string,
): Promise<void> {
  const existing = workspace.getLeavesOfType(viewType)
  if (existing.length > 0) {
    workspace.revealLeaf(existing[0])
    return
  }

  // `"tab"` rather than the sidebar: the view carries a topic → concept →
  // session hierarchy, which needs the width of a main-area leaf.
  const leaf = workspace.getLeaf("tab")
  await leaf.setViewState({ type: viewType, active: true })
  workspace.revealLeaf(leaf)
}
