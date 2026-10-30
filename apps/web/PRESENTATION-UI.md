# Study UI — resource launcher

**Status: the launcher, the capture form and the deck surface are implemented.
2026-10-22, kept current for the web app.** The launcher's list comes from
`GET /api/resources`; the other kinds wait for their endpoints. Companion to
`PRESENTATION.md` (the slide deck). This document is the app's own interface:
what the user sees, and what each surface is for.

## 1. The organizing idea

The app is not a flashcard app that later gained slides. It is a **library of
study resources** — card sets, decks, exams, and whatever comes next — and the
main view is the place where the user picks one. Flashcards are one kind of
resource, not the frame everything else has to fit into.

That reframes the hub: its job is **choose**, not "study now". The action zone
still exists, because a library that only lists is a file manager, but it answers
"continue" rather than pretending the user has no context.

## 2. Resource model

```
Resource
├── id
├── kind        flashcards | deck | exam | material | (future kinds)
├── title
├── topicId
├── conceptId?  cards belong to a concept; a deck may cover a whole topic
├── updatedAt
└── state       kind-specific, discriminated union
```

Each kind reports its **own status**, and that status is the reason a card is
worth clicking:

| Kind | Badge | Status line | Primary action | Surface |
| --- | --- | --- | --- | --- |
| Flashcards | ⏰ | 5 pendientes hoy / al día | Repasar | Session (`/study/review`) |
| Deck | ▤ | 12 diapositivas · generado hace 2 días | Presentar | Tab |
| Exam | ✎ | no rendido / 8/10 / vencido | Empezar | Tab |
| Material | ▦ | Libro · agregado hace 3 días | Abrir | Where it lives |

Consequence: the resource card is not one generic component with a type label. It
has a per-kind status slot. A card that cannot say what the user gains by opening
it is decoration.

**Architectural note.** `ARCHITECTURE.md` §14 removed `entities/resource` and says
it is not to be reintroduced without a new decision. This proposal reintroduces
the slice, with a different shape and a real consumer (the launcher): §15 records
that decision. It is not the old `syncResource` scaffold coming back.

**Where the list comes from.** The launcher renders `GET /api/resources` through
`entities/resource`, mapping every stored resource (`book`, `note`, `article`,
`video`) to the `material` kind: an `http(s)` source is the card's action and
opens in a new tab, and the stored type is the status label (`Libro · agregado
hace 3 días`; a timestamp it cannot read drops the age instead of inventing
"hoy"). A source a browser cannot open — a corpus document captured here
(`rag://documents/<id>`) or a legacy vault path from the deleted plugin — leaves
the action disabled with the reason, because a button that does nothing reads as
a bug.

The other three kinds have no endpoint yet. Their model, filter and surfaces stay
in place and they render nothing until one exists, so a flashcard resource card
cannot appear in the list. Cards are reached through the due session instead, and
that is what the launcher's one "continue" entry offers: `GET /api/study/due`
counted into `Repaso · Pendientes de hoy`, whose action starts the session and
routes to it (`pages/resources/model/continueEntry.ts`).

The review modal this proposal drew is **not** built, and it is not an oversight.
There is one review surface: `/study/review`. The modal was a second,
fixture-driven implementation of the same interaction with no data source, and it
could not be reached from anywhere in the app; it was deleted when the continue
entry got a real source (`odd/tasks/backlog.md` D2).

## 3. Surface map

| Surface | Job | Lifetime |
| --- | --- | --- |
| Launcher (`/`) | Continue, then browse resources by topic, with a kind filter; the capture form opens from here | Long, user-driven |
| Review session (`/study/review`) | One card at a time, keyboard-first; the due queue | Session-length |
| Deck page (`/present/:deckId`) | Present a deck (Reveal, full-bleed) | Session-length |
| Topics (`/topics`, `/topic/:topicId`, `/topic/:topicId/concept/:conceptId`) | Browse topics and concepts; paste a note to sync flashcards | Long, user-driven |
| Flashcard session | The concept's cards, or the due queue (`/study/due`) | Session-length |
| Exam page (future) | Answer a question set, then see results | Session-length |

Three surfaces the plugin had do not exist here, and not by oversight: the
in-note widget (there is no note being read), the status-bar count and the
right-sidebar destination (a web page has no host chrome to render into).

## 4. Wireframes

### 4.1 Launcher — wide (main area, ~1000px)

Flat list, decided 2026-10-21. The topic stays on the card as context — one line
under the title, `Atención ›` — so the order can be flat without losing where a
resource comes from. Filter chips and search do the narrowing that grouping would
have done structurally.

```text
┌ Estudio ────────────────────────────────────────────────────────── ✕ ┐
│  Buscar…      [ Todo ] [ ⏰ Tarjetas ] [ ▤ Slides ] [ ✎ Exámenes ]   │
│                                                                      │
│  SEGUIR DONDE QUEDASTE                                               │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ ⏰ Repaso · Atención selectiva              4 / 12   [Seguir]  │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  8 recursos                                                          │
│  ┌────────────────────┐ ┌────────────────────┐ ┌────────────────────┐│
│  │ ⏰ Tarjetas        │ │ ▤ Diapositivas     │ │ ✎ Examen           ││
│  │ Atención selectiva │ │ Atención           │ │ Atención           ││
│  │ Atención ›         │ │ Atención ›         │ │ Atención ›         ││
│  │ 5 pendientes       │ │ 12 slides · 2 d    │ │ 10 preguntas       ││
│  │ [ Repasar ]        │ │ [ Presentar ]      │ │ [ Empezar ]        ││
│  └────────────────────┘ └────────────────────┘ └────────────────────┘│
│  ┌────────────────────┐ ┌────────────────────┐                      │
│  │ ⏰ Tarjetas        │ │ ▤ Diapositivas     │                      │
│  │ Carga cognitiva    │ │ Memoria            │                      │
│  │ Memoria ›          │ │ Memoria ›          │                      │
│  │ al día             │ │ 9 slides · 1 sem   │                      │
│  │ [ Repasar ]        │ │ [ Presentar ]      │                      │
│  └────────────────────┘ └────────────────────┘                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 4.2 Launcher — narrow (~390px)

```text
┌ Estudio ────────┐
│ [Todo][⏰][▤][✎] │
│ SEGUIR           │
│ Atención · 4/12  │
│ [ Seguir ]       │
├─────────────────┤
│ 8 recursos       │
│ ⏰ Atención sel. │
│    Atención ›    │
│    5 pendientes  │
│    [ Repasar ]   │
│ ▤ Atención       │
│    12 slides     │
│    [ Presentar ] │
│ ✎ Atención       │
│    no rendido    │
│    [ Empezar ]   │
└─────────────────┘
```

### 4.3 Topic

```text
┌ Estudio ────────────────────────────────────────────────────────── ✕ ┐
│  Recursos › Atención                                                 │
│                                                                      │
│  ATENCIÓN                                    3 conceptos · 3 recursos│
│  [ Repasar tema ]   [ Editar ]   [ Feynman ]                         │
│                                                                      │
│  CONCEPTOS                                                           │
│  ┌────────────────────────────────┐ ┌──────────────────────────────┐ │
│  │ Atención selectiva             │ │ Atención dividida            │ │
│  │ ⏰ 8 tarjetas · 2 pendientes   │ │ ⏰ 6 tarjetas · al día       │ │
│  │ ▤ 6 slides                     │ │ ✎ examen · no rendido        │ │
│  │ [ Abrir ]                      │ │ [ Abrir ]                    │ │
│  └────────────────────────────────┘ └──────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
```

### 4.4 Deck tab

```text
┌ ▤ Atención — presentación ───────────────────────────────────── ✕ ┐
│                                                                   │
│                      ┌─────────────────────────────┐              │
│                      │                             │              │
│                      │     (Reveal, 16:9)          │              │
│                      │                             │              │
│                      └─────────────────────────────┘              │
│   ← →  navegar      F  pantalla completa      Fuentes: [[…]]       │
└───────────────────────────────────────────────────────────────────┘
```

`Esc` is not bound to exit: Reveal uses it for the slide overview. Leaving is the
page's own `Salir` button, which routes back to the launcher, or the browser's
back button.

### 4.5 Review session (`/study/review`)

Designed as an overlay over the launcher; shipped as the route. The layout below
is the one it inherited — a card, a reveal, four grades — but the frame is the
page, not a modal: the route owns its due session (it loads today's cards and
starts itself when it carries no concept), so a deep link or a reload reviews
them. Leaving is `← Volver` to `/study/due`; there is no `Esc` to bind.

```text
        ┌──────────────────────────────────────────────────────┐
        │  Repaso · Atención selectiva                    1/12 │
        │                                                      │
        │   ¿Qué es el filtro atencional?                      │
        │                                                      │
        │   ──────────────────────────────────────────────     │
        │   espacio  →  revelar respuesta                      │
        │                                                      │
        │   (respuesta)                                        │
        │                                                      │
        │   [1 Otra vez] [2 Difícil] [3 Bien] [4 Fácil]        │
        │                                                      │
        │   [[Atención - Kahneman]]            Esc para salir  │
        └──────────────────────────────────────────────────────┘
```

### 4.6 In-note widget

```text
   Atención selectiva.md
   ──────────────────────────────────────────────────────
   …contenido de la nota…

   ┌────────────────────────────────────────────────────┐
   │ En este concepto                                   │
   │ ⏰ 8 tarjetas · 2 pendientes           [ Repasar ] │
   │ ▤ 6 diapositivas                    [ Presentar ] │
   │ ✎ examen · no rendido                 [ Empezar ] │
   └────────────────────────────────────────────────────┘
```

Same three kinds as the launcher, scoped to the note. This is the highest-value
surface: it meets the user where they decide to study.

## 5. Navigation model

```text
   header nav ──┐
   URL ─────────┴─▶ Launcher (/)
                      │ SEGUIR / filtro / búsqueda
                      ├── ⏰ Repaso ──▶ /study/review ──Volver──▶ /study/due
                      ├── ▤ diapositivas ──▶ /present/:deckId ──Salir──▶ Launcher
                      └── ✎ examen ──▶ (sin ruta todavía)
   header nav ────▶ Temas (/topics) ──▶ Tema ──▶ Concepto ──▶ Sesión / Feynman
```

Every kind that has a surface has exactly one target. The launcher never embeds a
resource; it only routes to one. Exams are modelled but unrouted.

## 6. Scale rules

- A flat list dies at ~50 resources. Search and the kind filter are the only
  scaling mechanisms, so they ship with the list, not after it.
- Ordering is explicit and stable: due first, then recently updated. Never
  insertion order, so the list does not reshuffle while the user is reading it.
- The "continue" zone shows at most one item. Two "continue" entries is a list.
- A kind with zero resources in a topic is not rendered as an empty card.
- Filter state is per session, not persisted: a user who filtered to decks last
  week does not want to wonder where their cards went.
- The list has four states, and none of them is a blank area: loading, a failure
  with the server's message and a `Reintentar` button, an empty corpus
  (`Todavía no hay recursos.`) and a filter that matches nothing
  (`No hay recursos que coincidan con el filtro.`).

## 7. Component inventory

| Block | Where | Slice |
| --- | --- | --- |
| Shell + nav | `app/ui/App.vue` | app |
| Launcher, continue slot, resource card, filter + search | `pages/resources` (`ui/ResourcesPage.vue`, `model/filter.ts`, `model/types.ts`) | pages |
| Capture form (file, text, URL) | `features/capture-material` | features |
| Deck view | `pages/presentation` + `widgets/deck-viewer` | pages + widgets |
| Topic and concept pages | `pages/dashboard`, `pages/topic`, `pages/concept` | pages |
| Sync-note form | `features/sync-note` (paste markdown on the topics page) | features |
| Resource entity | `entities/resource` (`ARCHITECTURE.md` §15) | entities |
| Exam view | not started | pages (future) |

## 8. Decisions and open questions

Decided 2026-10-21:

1. **Grouping — flat list.** No topic sections. The topic is context on the card
   (`Atención ›`), not a container. Grouping is a later option if scale demands
   it; a flat list is cheaper to build and reads faster.
2. **Aggregation — concept × kind.** One card per resource. A topic with three
   concepts and two kinds produces six cards, and that is accepted: precision
   beats compactness while the corpus is small.

Decided 2026-10-30:

Decided 2026-10-30:

1. **One review surface.** The review modal is dropped; `/study/review` is the
   session, reached from the launcher's continue entry and from `/study/due`. Two
   surfaces for one interaction is what `backlog.md` D2 was made of.

Open:

3. **Deck ownership**: a deck may cover a whole topic, not a concept. Until that
   is settled, decks are modelled as belonging to a concept like everything else.
4. **Exam source**: questions generated from the concept's cards, or an authored
   set. This changes the model (an exam may have no 1:1 card mapping).
5. **Staleness**: a deck generated from notes that changed afterwards. Showing it
   on the card ("fuentes cambiadas") is cheap; deciding what to do about it is not.
6. **Where the `Resource` type lives** — **decided 2026-10-22**: `entities/resource`
   owns the wire shape of `GET /api/resources` (§15), and the launcher's view
   model stays page-local in `pages/resources/model/types.ts` until a second
   surface reads it. That is the decision `ARCHITECTURE.md` §14 asked for before
   the slice could return.

---

## 9. Delivery slices

This is the delivery record: slices 1–4 landed while the UI still lived in the
Obsidian plugin, and T1 moved the app unchanged to `apps/web`. The plugin paths
named below are history, not where the code is now.

| Slice | Commit | Content |
| --- | --- | --- |
| 1 | `d87cfe9` | Study view as a main-area tab, shared Pinia, Spanish labels |
| 2 | `71a9caa` | Resource launcher + review modal, fixture-driven (~1500 authored lines, accepted as one UI unit: the modal wiring lives inside the launcher page and splitting it would leave a commit that does not build) |
| 3 | `f5398a2` | Deck surface: Reveal mount adapter, fixture deck, harness route `present/deck-atencion` (§4.4). Harness-only on purpose |
| 4 | — | Deck delivery, first half: Reveal's stylesheets imported as text inside a shadow root (`PRESENTATION.md` §3.1), route `present/:deckId`, and the launcher's `Presentar` action wired to it through `deckRouteFor`. No parser yet: the page still renders the fixture deck |
| next | — | Deck delivery, second half: `deck.md` parser (`markdown-it` + containers) and the real deck behind `:deckId` |

Slice 3 was harness-only for a mechanical reason: the plugin's production bundle
was esbuild and could not import CSS, so wiring the page into the router before
the stylesheet strategy existed would have broken its build. It was still the
first unit that owned DOM, which is why it was isolated rather than bundled with
the parser. Slice 4 paid that debt: `.css` is now a `text` module
(`vite.config.ts`) and the styles live in the deck's shadow root, so the page is
in the app graph. It stops at the mount: the deck is still the fixture, because
the parser is a separate behavior with its own tests.

Slice 2 exceeded the ~400-line review budget on purpose. It is one behavior —
"a list of resources you can act on" — and its two halves cannot be separated
without an intermediate commit that imports a component that does not exist. The
deck surface is where the budget applies again, and it is a separate slice for a
stronger reason: it is the first unit that owns DOM (D8 in `PRESENTATION.md`).
