<script setup lang="ts">
import type { PrimitiveProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import type { BadgeVariants } from "."
import { cn } from "@/shared/lib/utils"
import { Primitive } from "reka-ui"
import { badgeVariants } from "."

/**
 * Only reka's primitive props are forwarded, one by one — never the whole prop bag.
 *
 * The generator's version did `v-bind="reactiveOmit(props, 'class')"`, which also
 * forwarded `variant`, a prop that exists only to pick a class and is not an
 * attribute HTML defines: every badge rendered `variant="outline"` into the DOM.
 * `Button.vue` never had the bug because it binds `as`/`as-child` explicitly, and
 * this is the same shape. Anything a caller adds (`aria-*`, `role`, …) still
 * reaches the element as a fallthrough attribute.
 */
const props = defineProps<PrimitiveProps & {
  variant?: BadgeVariants["variant"]
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <Primitive
    data-slot="badge"
    :class="cn(badgeVariants({ variant }), props.class)"
    :as="as"
    :as-child="asChild"
  >
    <slot />
  </Primitive>
</template>
