<template>
  <!--
    The whole card is the control, so it stays one `shared/ui` button while the frame
    stays the card's own CSS (`shadcn-migration.md` T8, the rule T7 set for the entity
    cards). It cannot be a `Card`: the primitive is a non-interactive column box, and
    the click target is the card itself.

    `variant` is left at its default because the frame owns the paint; what is
    restated below is the box, because the primitive's `h-9`/`justify-center`/
    `font-medium`/`nowrap` would otherwise reshape a full-width row.
  -->
  <Button class="daily-review" @click="goToDueCards">
    <span class="daily-review__text">
      {{ label }}
    </span>
    <span class="daily-review__arrow">→</span>
  </Button>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { useRouter } from "vue-router"
import { Button } from "@/shared/ui/button"

const props = defineProps<{
  dueCount: number
  loading?: boolean
}>()

const router = useRouter()

const label = computed(() => {
  if (props.loading) return "Cargando..."
  const count = props.dueCount
  if (count === 0) return "Sin tarjetas para repasar"
  return `Estudiar ${count} tarjeta${count === 1 ? "" : "s"} hoy`
})

function goToDueCards() {
  router.push({ name: "due-cards" })
}
</script>

<style scoped>
/* The frame: a full-width row with its own 2px accent border, radius and palette.
 *
 * The rest of the block is the box stated in full, because the root is the
 * primitive now: `h-9` would pin the height to 36px, `justify-content: center`
 * would drop `space-between`, `gap-2` would open an 8px gap the row never had, and
 * `rounded-md` (6px) would square the 8px corners. `font-size` and `font-weight`
 * are re-declared so the primitive's `text-sm`/`font-medium` cannot move the label. */
.daily-review {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0;
  width: 100%;
  height: auto;
  min-height: 56px;
  padding: 0.75rem 1rem;
  border: 2px solid var(--interactive-accent);
  border-radius: 8px;
  background: var(--background-primary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.95rem;
  font-weight: 500;
  line-height: normal;
  text-align: left;
  white-space: normal;
  transition: background-color 0.15s ease;
}

.daily-review:hover {
  background: var(--background-modifier-hover);
}

.daily-review__arrow {
  font-size: 1.2rem;
  color: var(--interactive-accent);
  min-width: 44px;
  text-align: center;
}
</style>
