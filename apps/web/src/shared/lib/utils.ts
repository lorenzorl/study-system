import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

/**
 * Merges class strings, then resolves Tailwind conflicts in favour of the last
 * one. `clsx` handles the conditional forms (`{ "is-active": active }`,
 * arrays, `undefined`) and `twMerge` collapses what `clsx` cannot: a primitive
 * whose default is `px-4` and whose caller passes `px-6` must end up with
 * `px-6`, not with both.
 *
 * Every `shared/ui` primitive calls this, so a primitive is styled by its
 * defaults *and* by whatever the caller adds, without specificity fights and
 * without `!important`. This is the only reason `clsx` and `tailwind-merge` are
 * dependencies.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
