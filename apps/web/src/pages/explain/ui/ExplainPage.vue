<template>
  <!--
    Explain one page range in your own words and read the gaps it left (U2).

    The range comes from the URL, not from a plan item: `/explain/:documentId/:from/:to`
    is deep-linkable on purpose, so a plan item can point here any day and a reload
    annotates a URL that already names what it explains. `?id=` names the stored run,
    and reading it back costs no model call (S9).
  -->
  <div class="explain">
    <header class="explain__header">
      <p class="explain__range">{{ rangeLabel }}</p>
      <p class="explain__material">{{ documentId }}</p>
    </header>

    <!--
      A deep link's one wait, and it is short: the gaps come from storage. The form is
      not shown yet — there is nothing to refuse, so a field now would invite typing
      over the explanation the URL already names.
    -->
    <div v-if="loading && !explanation" class="explain__state">
      <span class="explain__spinner" aria-hidden="true"></span> Cargando la
      explicación guardada...
    </div>

    <template v-else>
      <!--
        One panel for every failure, worded by `useExplain`: a refusal is the server's
        sentence verbatim, and only a failure worth another call offers the retry.
      -->
      <div
        v-if="errorMessage"
        class="explain__state explain__state--error"
        role="alert"
      >
        <p class="explain__message">{{ errorMessage }}</p>
        <Button
          v-if="canRetry"
          type="button"
          variant="outline"
          class="min-h-[44px]"
          @click="retry"
        >
          Reintentar
        </Button>
      </div>

      <!--
        The form is only here while there is no explanation: once one is stored, the
        gaps are the answer, and a second submit would be a second model call for a
        range that already has one. A refusal keeps it — with the text still in it.
      -->
      <form v-if="!explanation" class="explain__form" @submit.prevent="onSubmit">
        <p class="explain__instructions">
          Contá con tus palabras qué dice este tramo, como si se lo explicaras a
          alguien que no lo leyó. Después vas a ver qué quedó afuera.
        </p>

        <textarea
          v-model="text"
          class="explain__textarea"
          rows="8"
          placeholder="Escribí tu explicación..."
          :aria-label="'Explicá las ' + rangeLabel + ' con tus propias palabras'"
          :disabled="saving"
        ></textarea>

        <!-- The call takes 30 to 60 seconds, and the reader has to know it is working. -->
        <p v-if="saving" class="explain__working" role="status">
          Explicando… esto tarda entre 30 y 60 segundos.
        </p>

        <Button type="submit" class="min-h-[44px] self-start" :disabled="saving">
          Explicar estas páginas
        </Button>
      </form>

      <!--
        The gaps. Each one is a point, the page that states it and the quote that
        proves it, and there is no score beside them: a number on an open response is
        noise, not something to act on (S8).
      -->
      <section v-else class="explain__gaps" aria-labelledby="explain-gaps-title">
        <h3 id="explain-gaps-title" class="explain__gaps-title">Lagunas</h3>
        <p class="explain__gaps-note">
          Cada laguna cita la página donde el texto lo dice.
        </p>

        <p v-if="isEmpty" class="explain__nothing">
          No quedaron lagunas: tu explicación cubre lo que el texto marca.
        </p>

        <ul v-else class="explain__list">
          <li v-for="gap in gaps" :key="gap.id" class="explain__gap">
            <p class="explain__point">{{ gap.point }}</p>
            <p class="explain__page">{{ pageLabel(gap) }}</p>
            <!-- The evidence, verbatim and set apart: a citation, not a paraphrase. -->
            <blockquote class="explain__quote">{{ gap.quote }}</blockquote>

            <!-- S7: discarded stops it being actionable, nothing else about it changes. -->
            <p v-if="isDiscarded(gap)" class="explain__discarded">Descartada</p>

            <Button
              type="button"
              :variant="isDiscarded(gap) ? 'outline' : 'secondary'"
              :class="[
                'min-h-[44px] self-start',
                isDiscarded(gap)
                  ? 'border-primary text-primary'
                  : 'border-border text-muted-foreground',
              ]"
              :aria-pressed="isDiscarded(gap)"
              :disabled="saving"
              @click="toggleGap(gap)"
            >
              {{ isDiscarded(gap) ? "Deshacer" : "Descartar" }}
            </Button>
          </li>
        </ul>

        <!--
          The only way back to the form: a second explanation is a second model call,
          so it is asked for and never inferred. Without it, any URL naming a stored
          explanation would have no way to explain that range again.
        -->
        <Button
          type="button"
          variant="outline"
          class="min-h-[44px] self-start"
          :disabled="saving"
          @click="onStartOver"
        >
          Explicar de nuevo
        </Button>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from "vue"
import { useRoute, useRouter } from "vue-router"
import { Button } from "@/shared/ui/button"
import { useExplain } from "../model/useExplain"

const route = useRoute()
const router = useRouter()

// The URL is the source of truth for what is explained: the params are the range,
// and a wrong link is the caller's, not something this page can guess its way out of.
const documentId = String(route.params.documentId ?? "")
const model = useExplain({
  documentId,
  from: Number(route.params.from),
  to: Number(route.params.to),
})

const {
  explanation,
  loading,
  saving,
  text,
  gaps,
  isEmpty,
  rangeLabel,
  errorMessage,
  canRetry,
  pageLabel,
  submit,
  load,
  retry,
  startOver,
  toggleGap,
  isDiscarded,
} = model

/**
 * A reload of a deep link reads what the run stored; without `?id=` there is nothing
 * to read and the form is the page. No model call is made in either case until the
 * reader submits one.
 */
onMounted(() => {
  const id = String(route.query.id ?? "")
  if (id !== "") void load(id)
})

/**
 * On success the id goes into the URL with `replace`: this page is the explanation,
 * and going back should leave it rather than walk through every submit (S9).
 */
async function onSubmit() {
  const id = await submit()
  if (id === null) return

  void router.replace({ query: { ...route.query, id } })
}

/**
 * Start the same range over: the explanation leaves the store and its id leaves the
 * URL, so a reload is the form again and not the answer the reader just dismissed.
 */
function onStartOver() {
  startOver()

  const { id: _dropped, ...rest } = route.query
  void router.replace({ query: rest })
}
</script>

<style scoped>
/*
 * The controls are `shared/ui` primitives, so their frame and disabled state come
 * from the tokens. What stays here is the page's layout, the citation, and the two
 * state panels — the same division `pages/today` keeps.
 */
.explain {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
}

.explain__header {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.explain__range {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 600;
}

.explain__material {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.explain__form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.explain__instructions {
  margin: 0;
  font-size: 0.9rem;
  color: var(--text-muted);
}

.explain__textarea {
  width: 100%;
  padding: 0.625rem 0.75rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
  color: var(--text-normal);
  font-family: inherit;
  font-size: 0.95rem;
  line-height: 1.5;
  resize: vertical;
}

.explain__working {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-accent);
}

.explain__gaps {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.explain__gaps-title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
}

.explain__gaps-note {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.explain__list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.explain__gap {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  padding: 0.75rem 0.875rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 8px;
  background: var(--background-primary);
}

.explain__point {
  margin: 0;
  font-size: 0.95rem;
}

.explain__page {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-accent);
}

/*
 * The citation is the one thing on this page that is not the reader's own words, so
 * it is the one thing that looks different: a rule on the left, the muted colour and
 * the italic face the app already uses for a quotation. It is never translated.
 */
.explain__quote {
  margin: 0;
  padding: 0.25rem 0 0.25rem 0.75rem;
  border-left: 3px solid var(--background-modifier-border);
  color: var(--text-muted);
  font-size: 0.85rem;
  font-style: italic;
  line-height: 1.5;
}

.explain__discarded {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.explain__nothing {
  margin: 0;
  padding: 1rem 0.5rem;
  color: var(--text-muted);
}

.explain__state {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem 0.5rem;
  color: var(--text-muted);
}

.explain__state--error {
  flex-direction: column;
  align-items: flex-start;
  color: var(--text-error, #d64545);
}

.explain__message {
  margin: 0;
}

.explain__spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid var(--background-modifier-border);
  border-top-color: var(--text-accent);
  border-radius: 50%;
  animation: explain-spin 0.8s linear infinite;
}

@keyframes explain-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
