import { createPinia } from "pinia"

/**
 * One Pinia instance shared by every mount of the study view.
 *
 * The view can be opened in more than one leaf (a main-area tab today, and a
 * sidebar card or a second tab later). Creating a Pinia per mount would give
 * each mount its own copy of every store: an edit in one view would be invisible
 * in the other, with no error and no obvious symptom.
 *
 * Sharing it also means closing and reopening the view keeps the session state,
 * which is what a user expects from a tab they left open by accident.
 */
export const pinia = createPinia()
