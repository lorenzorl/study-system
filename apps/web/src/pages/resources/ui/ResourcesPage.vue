<template>
  <div class="resources">
    <!-- Filter row: search plus kind chips, both driving `filterResources`. -->
    <div class="resources__filters">
      <input
        v-model="query"
        class="resources__search"
        type="search"
        placeholder="Buscar…"
        aria-label="Buscar recursos"
      />
      <!--
        The chips are the `shared/ui` button. They keep their pill shape and the
        palette mapping the CSS had (`secondary` is the grey fill, `outline` the
        white one), and the state is a variant change plus the accent border and
        label, which is exactly what `--active` drew before. `min-h-[44px]` is the
        touch floor this row has always had.
      -->
      <div class="resources__chips" role="group" aria-label="Filtrar por tipo">
        <Button
          v-for="chip in KIND_CHIPS"
          :key="chip.value"
          type="button"
          :variant="kind === chip.value ? 'outline' : 'secondary'"
          :class="[
            'min-h-[44px] gap-1 rounded-full',
            kind === chip.value
              ? 'border-primary text-primary'
              : 'border-border text-muted-foreground',
          ]"
          :aria-pressed="kind === chip.value"
          @click="kind = chip.value"
        >
          <span
            v-if="chip.glyph"
            class="resources__chip-glyph"
            aria-hidden="true"
            >{{ chip.glyph }}</span
          >
          {{ chip.label }}
        </Button>
      </div>
      <!-- The one write the launcher offers: every other action opens a resource. -->
      <Button type="button" class="min-h-[44px]" @click="openCapture($event)">
        Agregar material
      </Button>
    </div>

    <!-- At most one "continue" entry (§6): two would be a list. Its source is the
         due endpoint, so the slot appears only when there is something to resume
         and never shows a fixture. -->
    <section v-if="continueResource" class="resources__continue">
      <h2 class="resources__section-title">Seguir donde quedaste</h2>
      <div class="continue">
        <span class="continue__glyph" aria-hidden="true">{{
          KIND_GLYPHS.flashcards
        }}</span>
        <span class="continue__title">
          {{ continueResource.kindLabel }} · {{ continueResource.title }}
        </span>
        <span class="continue__progress">
          {{ continueResource.done }} / {{ continueResource.total }}
        </span>
        <Button
          type="button"
          class="min-h-[44px] min-w-[44px]"
          @click="openContinue"
        >
          {{ continueResource.primaryActionLabel }}
        </Button>
      </div>
    </section>

    <div v-if="loading" class="resources__state">
      <span class="resources__spinner" aria-hidden="true"></span> Cargando...
    </div>

    <div v-else-if="error" class="resources__state resources__state--error">
      <p class="resources__error">{{ error }}</p>
      <Button
        type="button"
        variant="outline"
        class="min-h-[44px]"
        @click="retry"
      >
        Reintentar
      </Button>
    </div>

    <template v-else>
      <p class="resources__count">{{ countLabel }}</p>

      <div class="resources__grid">
        <article
          v-for="resource in visibleResources"
          :key="resource.id"
          class="resource-card"
        >
          <!-- The kind chip is a `Badge`: the pill is the primitive's, the mapping
               stays the palette's (muted label, accent glyph, default border). -->
          <Badge
            variant="outline"
            class="uppercase tracking-[0.04em] text-muted-foreground"
          >
            <span class="resource-card__glyph" aria-hidden="true">{{
              KIND_GLYPHS[resource.kind]
            }}</span>
            {{ KIND_LABELS[resource.kind] }}
          </Badge>
          <h3 class="resource-card__title">{{ resource.title }}</h3>
          <p class="resource-card__context">{{ resource.topicTitle }} ›</p>
          <p class="resource-card__status">{{ statusOf(resource) }}</p>
          <!--
            `disabled:pointer-events-auto` is deliberate: the primitive's own
            `disabled:pointer-events-none` would swallow the `title`, and that
            tooltip is the whole reason a corpus-only material renders disabled
            instead of silently doing nothing.
          -->
          <Button
            type="button"
            class="min-h-[44px] min-w-[44px] self-start disabled:pointer-events-auto disabled:cursor-not-allowed"
            :aria-label="`${resource.primaryActionLabel}: ${resource.title}`"
            :disabled="!isOpenable(resource)"
            :title="isOpenable(resource) ? undefined : CORPUS_ONLY_HINT"
            @click="openResource(resource, $event)"
          >
            {{ resource.primaryActionLabel }}
          </Button>
        </article>
      </div>

      <p v-if="!hasAnyResource" class="resources__empty">
        Todavía no hay recursos.
      </p>
      <p v-else-if="visibleResources.length === 0" class="resources__empty">
        No hay recursos que coincidan con el filtro.
      </p>
    </template>

    <!-- The capture form owns its fields and reports `saved`; the store it uses
         owns the requests and has already reloaded the list by then, so closing
         is all that is left. -->
    <CaptureMaterialForm
      v-if="captureOpen"
      @saved="closeCapture"
      @cancel="closeCapture"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { storeToRefs } from "pinia"
import { useRouter } from "vue-router"
import { useResourcesStore } from "@/entities/resource"
import { useReviewStore } from "@/entities/review"
import { CaptureMaterialForm } from "@/features/capture-material"
import { Badge } from "@/shared/ui/badge"
import { Button } from "@/shared/ui/button"
import { useCapturePanel } from "../model/capturePanel"
import { continueEntryFor } from "../model/continueEntry"
import { deckRouteFor } from "../model/deckRoute"
import { filterResources } from "../model/filter"
import { materialStatus, toMaterialResources } from "../model/materials"
import { sourceTargetOf } from "../model/openSource"
import type {
  ContinueResource,
  Resource,
  ResourceKind,
  ResourceKindFilter,
} from "../model/types"

const KIND_GLYPHS: Record<ResourceKind, string> = {
  flashcards: "⏰",
  deck: "▤",
  exam: "✎",
  material: "▦",
}

const KIND_LABELS: Record<ResourceKind, string> = {
  flashcards: "Tarjetas",
  deck: "Diapositivas",
  exam: "Examen",
  material: "Material",
}

const KIND_CHIPS: { value: ResourceKindFilter; label: string; glyph: string }[] = [
  { value: "all", label: "Todo", glyph: "" },
  { value: "material", label: "Material", glyph: KIND_GLYPHS.material },
  { value: "flashcards", label: "Tarjetas", glyph: KIND_GLYPHS.flashcards },
  { value: "deck", label: "Diapositivas", glyph: KIND_GLYPHS.deck },
  { value: "exam", label: "Exámenes", glyph: KIND_GLYPHS.exam },
]

const router = useRouter()

// The launcher's list is the middleware's, not a fixture set (§2).
const resourcesStore = useResourcesStore()
const { resources, loading, error } = storeToRefs(resourcesStore)

// The one "continue" entry, counted from what is actually due. `GET /api/study/due`
// is the only endpoint that reports a resumable session, and the review session is
// the one surface that resumes it: the launcher links there instead of owning a
// second, fixture-driven session of its own (backlog D2).
const reviewStore = useReviewStore()
const { dueCards } = storeToRefs(reviewStore)
const continueResource = computed<ContinueResource | null>(() =>
  continueEntryFor(dueCards.value),
)

// Filter state is per session, never persisted (§6).
const query = ref("")
const kind = ref<ResourceKindFilter>("all")

const materialResources = computed(() =>
  toMaterialResources(resources.value, new Date()),
)

const hasAnyResource = computed(() => materialResources.value.length > 0)

const visibleResources = computed(() =>
  filterResources(materialResources.value, kind.value, query.value),
)

const countLabel = computed(() =>
  visibleResources.value.length === 1
    ? "1 recurso"
    : `${visibleResources.value.length} recursos`,
)

onMounted(() => {
  resourcesStore.loadResources()
  reviewStore.fetchDueCards()
})

function retry() {
  resourcesStore.clearError()
  resourcesStore.loadResources()
}

/** Each kind reports its own status; that status is why the card is clickable. */
function statusOf(resource: Resource): string {
  switch (resource.kind) {
    case "flashcards":
      if (resource.due === 0) return "al día"
      return `${resource.due} pendiente${resource.due === 1 ? "" : "s"} hoy`
    case "deck": {
      const days = resource.updatedDaysAgo === 1 ? "día" : "días"
      const slides = resource.slides === 1 ? "diapositiva" : "diapositivas"
      return `${resource.slides} ${slides} · generado hace ${resource.updatedDaysAgo} ${days}`
    }
    case "exam":
      if (resource.lastScore === null || resource.of === null) return "no rendido"
      return `${resource.lastScore}/${resource.of}`
    case "material":
      return materialStatus(resource)
  }
}

// ── Review session ───────────────────────────────────────────────────────────

/**
 * `Seguir` opens the real due session.
 *
 * The session store is started before the navigation for the same reason
 * `pages/due-cards` does it: the review route renders its own session when it has
 * no concept, and starting it here means it does not re-fetch what this page just
 * counted.
 */
function openContinue() {
  reviewStore.startSession()
  router.push({ name: "review" })
}

// ── Capture panel ────────────────────────────────────────────────────────────

const {
  isOpen: captureOpen,
  open: showCapture,
  close: closeCapture,
} = useCapturePanel()

function openCapture(event: MouseEvent) {
  const opener = event.currentTarget
  showCapture(opener instanceof HTMLElement ? opener : null)
}

/**
 * Disabled copy for a material the browser cannot open.
 *
 * It says where the material is and how to reach it, because the alternative —
 * a button that silently does nothing — reads as a bug.
 */
const CORPUS_ONLY_HINT = "Vive en el corpus: buscalo por su título."

/**
 * Whether the card's action has somewhere to go.
 *
 * A material captured here lives in the corpus (`rag://documents/<id>`), and a
 * legacy row may still hold a vault path or an `obsidian://` URI: neither is an
 * address a browser can open, so the action is rendered disabled with the reason
 * instead of doing nothing when clicked.
 */
function isOpenable(resource: Resource): boolean {
  if (resource.kind !== "material") return true

  return sourceTargetOf(resource.sourceUri)?.kind === "external"
}

/**
 * Open a material where it lives: an external URL in a new tab.
 *
 * A corpus material has no address of its own — the corpus document is the
 * material — so it is searched, not opened; `isOpenable` keeps its button
 * disabled and this guard keeps a forced click inert.
 */
function openSource(uri: string) {
  const target = sourceTargetOf(uri)
  if (!target) return

  if (target.kind === "external") {
    window.open(target.uri, "_blank", "noopener")
    return
  }
}

function openResource(resource: Resource, event: MouseEvent) {
  if (resource.kind === "material") {
    openSource(resource.sourceUri)
    return
  }

  if (resource.kind === "flashcards") {
    // No endpoint reports flashcard resources, so this kind never reaches the
    // launcher: the due session is the continue entry's job, not a card's.
    return
  }

  // A deck opens as a tab, not as an overlay (§3): the deck is a session-length
  // surface, and leaving it is a route change the deck's own exit button makes.
  const deckRoute = deckRouteFor(resource)
  if (deckRoute) {
    router.push(deckRoute)
    return
  }

  // TODO(slice: surfaces): open the exam tab.
}
</script>

<style scoped>
.resources {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
}

.resources__filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
}

.resources__search {
  flex: 1 1 140px;
  min-width: 120px;
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-family: inherit;
  font-size: 0.9rem;
}

.resources__search::placeholder {
  color: var(--text-muted);
}

.resources__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem;
}

/*
 * The controls in this page are the `shared/ui` primitives now, so what is left
 * here is layout, the search field (still hand-rolled: the forms unit did not
 * claim it) and the two text glyphs the chips and cards render.
 */
.resources__chip-glyph {
  font-size: 0.9rem;
  line-height: 1;
}

.resources__section-title {
  margin: 0 0 0.375rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.resources__continue {
  display: flex;
  flex-direction: column;
}

.continue {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 0.875rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
}

.continue__glyph {
  font-size: 1rem;
  line-height: 1;
  color: var(--text-accent);
}

.continue__title {
  flex: 1 1 140px;
  font-size: 0.9rem;
  font-weight: 500;
}

.continue__progress {
  font-size: 0.85rem;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.resources__count {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-muted);
}

.resources__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 0.75rem;
}

.resource-card {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.75rem 0.875rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
}

.resource-card__glyph {
  font-size: 0.85rem;
  line-height: 1;
  color: var(--text-accent);
}

.resource-card__title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--text-normal);
}

.resource-card__context {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.resource-card__status {
  margin: 0 0 0.375rem;
  font-size: 0.8rem;
  color: var(--text-accent);
}

.resources__empty {
  margin: 0;
  padding: 1.5rem 0.5rem;
  text-align: center;
  color: var(--text-muted);
}

.resources__state {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1.5rem 0.5rem;
  color: var(--text-muted);
}

.resources__state--error {
  flex-direction: column;
  align-items: flex-start;
  color: var(--text-error, #d64545);
}

.resources__error {
  margin: 0;
}

.resources__spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid var(--background-modifier-border);
  border-top-color: var(--text-accent);
  border-radius: 50%;
  animation: resources-spin 0.8s linear infinite;
}

@keyframes resources-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 520px) {
  .resources__grid {
    grid-template-columns: 1fr;
  }
}
</style>
