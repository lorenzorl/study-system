<template>
  <!--
    One page, one question: what do I do today (S14). Everything it shows comes
    from `GET /api/study/today`, so the page decides wording and layout, never what
    is worth doing (S11).
  -->
  <div class="today">
    <!--
      Three states before the day itself (D6, D2): a pending first read, a failed
      one, and a loaded day that happens to be empty. Before this pattern the app
      had two, so a failed load read as an empty result.
    -->
    <div v-if="loading && !today" class="today__state">
      <span class="today__spinner" aria-hidden="true"></span> Cargando...
    </div>

    <div v-else-if="error && !today" class="today__state today__state--error">
      <p class="today__message">{{ error }}</p>
      <Button type="button" variant="outline" class="min-h-[44px]" @click="retry">
        Reintentar
      </Button>
    </div>

    <template v-else-if="today">
      <!--
        A refused write is an error with the day still on screen: the list is what
        the server last said, and replacing it with a bare error would throw away
        the work the retry is meant to keep. The retry re-reads the projection; the
        toggle stays off because only an accepted write can turn it on.
      -->
      <div v-if="error" class="today__state today__state--error" role="alert">
        <p class="today__message">{{ error }}</p>
        <Button type="button" variant="outline" class="min-h-[44px]" @click="retry">
          Reintentar
        </Button>
      </div>

      <!--
        The due row is a count and a way in, and it is absent at zero rather than
        showing "0 para repasar": a review has no range to name, so the count is all
        there is to say (S13).
      -->
      <section v-if="hasDue" class="today__due">
        <p class="today__due-count">{{ dueLabel }}</p>
        <Button type="button" class="min-h-[44px]" @click="goToReview">
          Repasar
        </Button>
      </section>

      <p v-if="isEmpty" class="today__nothing">Hoy no tenés nada para hacer.</p>

      <ul v-else-if="entries.length > 0" class="today__entries">
        <li v-for="entry in entries" :key="entryKey(entry)" class="today__entry">
          <!--
            A read range still has no toggle (it is already read), but it now has
            somewhere to go: the explanation surface exists, and this is the entry it
            was always going to be reached from (S9). The link names the document and
            the range, so opening it again re-reads the stored explanation instead of
            paying for the model a second time.
          -->
          <template v-if="entry.kind === 'read_not_distilled'">
            <p class="today__entry-title">{{ materialName(entry) }}</p>
            <p class="today__entry-range">{{ rangeLabel(entry) }}</p>
            <p class="today__entry-note">
              Leíste estas páginas y todavía no las destilaste.
            </p>
            <RouterLink class="today__entry-link" :to="explainRoute(entry)">
              Destilá estas páginas
            </RouterLink>
          </template>

          <!--
            The plan's next pending chapter, with the one write this page makes.
            `aria-pressed` is the state, as on the launcher's filter chips: the
            primitive owns the frame, the attribute says whether the range is
            already read.
          -->
          <template v-else>
            <p class="today__entry-title">{{ entry.title }}</p>
            <p class="today__entry-range">{{ rangeLabel(entry) }}</p>
            <p v-if="entry.note" class="today__entry-note">{{ entry.note }}</p>
            <Button
              type="button"
              :variant="isRead(entry) ? 'outline' : 'secondary'"
              :class="[
                'min-h-[44px] self-start',
                isRead(entry)
                  ? 'border-primary text-primary'
                  : 'border-border text-muted-foreground',
              ]"
              :aria-pressed="isRead(entry)"
              :disabled="saving"
              @click="toggleRead(entry)"
            >
              Leído
            </Button>
          </template>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from "vue"
import { useRouter } from "vue-router"
import { Button } from "@/shared/ui/button"
import type { PlanNextEntry } from "@/entities/today"
import { useToday } from "../model/useToday"

const router = useRouter()
const {
  today,
  loading,
  error,
  saving,
  entries,
  hasDue,
  isEmpty,
  dueLabel,
  materialName,
  rangeLabel,
  entryKey,
  explainRoute,
  load,
  setRead,
  isRead,
} = useToday()

// Always fetch on mount, even when the store still holds a previous day: "has
// entries" is not "they are current", and a stale plan item is exactly what this
// page must not show (`R3-003` on the review session is the same rule).
onMounted(() => {
  void load()
})

/**
 * The retry covers both failures this page can have: a load that never landed and
 * a write that was refused. In both cases the projection is what has to be read
 * again, and it is also what clears the message.
 */
function retry() {
  void load()
}

function goToReview() {
  router.push({ name: "review" })
}

/** Only `plan_next` entries have a toggle; the template narrows, the handler asserts. */
function toggleRead(entry: PlanNextEntry) {
  void setRead(entry, !isRead(entry))
}
</script>

<style scoped>
/*
 * The actions are `shared/ui` primitives, so their frame, hover and disabled state
 * come from the tokens. What stays here is the day's layout and the two state
 * panels, plus the spinner — the same division the launcher and the session pages
 * already keep.
 */
.today {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
}

.today__due {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 0.875rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-secondary);
}

.today__due-count {
  flex: 1 1 140px;
  margin: 0;
  font-size: 0.9rem;
  font-weight: 500;
}

.today__entries {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.today__entry {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.75rem 0.875rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
}

.today__entry-title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
}

.today__entry-range {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-accent);
}

.today__entry-note {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

/*
 * The read range's one control, and a link rather than a button on purpose: it
 * navigates to a page, so it is the browser's own element and the browser's own
 * affordances (middle click, open in a tab, copy the URL).
 */
.today__entry-link {
  align-self: flex-start;
  margin-top: 0.25rem;
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--text-accent);
  text-decoration: none;
}

.today__entry-link:hover {
  text-decoration: underline;
}

.today__nothing {
  margin: 0;
  padding: 1.5rem 0.5rem;
  text-align: center;
  color: var(--text-muted);
}

.today__state {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1.5rem 0.5rem;
  color: var(--text-muted);
}

.today__state--error {
  flex-direction: column;
  align-items: flex-start;
  color: var(--text-error, #d64545);
}

.today__message {
  margin: 0;
}

.today__spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid var(--background-modifier-border);
  border-top-color: var(--text-accent);
  border-radius: 50%;
  animation: today-spin 0.8s linear infinite;
}

@keyframes today-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
