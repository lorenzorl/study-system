import { nextTick, ref } from "vue"

/**
 * The piece of an opener the panel needs back: `HTMLElement` has it, and so does
 * a stand-in in a test, which keeps this rule testable without a DOM.
 */
export interface FocusTarget {
  focus: () => void
}

/**
 * Open and close the launcher's capture panel.
 *
 * It follows the review modal's contract: one overlay at a time, Escape closes
 * it (the form reports that as `cancel`), and focus goes back to the element
 * that opened it — otherwise a keyboard user is left on an element that no
 * longer exists.
 */
export function useCapturePanel() {
  const isOpen = ref(false)
  let opener: FocusTarget | null = null

  function open(target: FocusTarget | null): void {
    opener = target
    isOpen.value = true
  }

  async function close(): Promise<void> {
    isOpen.value = false

    // Taken and cleared before the await, so a second close cannot return focus
    // twice.
    const target = opener
    opener = null

    // Focus moves only once the overlay is gone, or it would land on an element
    // the panel still covers.
    await nextTick()
    target?.focus()
  }

  return { isOpen, open, close }
}
