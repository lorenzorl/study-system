# Study UI — resource launcher

**Status: proposal, nothing implemented. 2026-10-21.**
Companion to `PRESENTATION.md` (the slide deck). This document is the plugin's own
interface: what the user sees, and what each surface is for.

## 1. The organizing idea

The plugin is not a flashcard app that later gained slides. It is a **library of
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
├── kind        flashcards | deck | exam | (future kinds)
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
| Flashcards | ⏰ | 5 pendientes hoy / al día | Repasar | Modal |
| Deck | ▤ | 12 diapositivas · generado hace 2 días | Presentar | Tab |
| Exam | ✎ | no rendido / 8/10 / vencido | Empezar | Tab |

Consequence: the resource card is not one generic component with a type label. It
has a per-kind status slot. A card that cannot say what the user gains by opening
it is decoration.

**Architectural note.** `ARCHITECTURE.md` §14 removed `entities/resource` and says
it is not to be reintroduced without a new decision. This proposal reintroduces
the slice, with a different shape and a real consumer (the launcher). That
requires an entry superseding §14 for `entities/resource`; it is not the old
`syncResource` scaffold coming back.

## 3. Surface map

| Surface | Job | Lifetime |
| --- | --- | --- |
| Main-area tab | Launcher: continue, then browse resources by topic, with a kind filter | Long, user-driven |
| Review modal | One card at a time, keyboard-first | Short, task-bounded |
| Deck tab | Present a deck (Reveal, full-bleed) | Session-length |
| Exam tab | Answer a question set, then see results | Session-length |
| In-note widget | The resources attached to the note being read | Ambient, per note |
| Status bar | Pending count today | Permanent, one number |
| Right sidebar | Not a destination. Optional single-line "hoy" later | — |

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

`Esc` is not bound to exit: Reveal uses it for the slide overview. Leaving is
Obsidian's own tab bar.

### 4.5 Review modal

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
   status bar ──┐
   ribbon/cmd ──┴─▶ Launcher (tab)
                      │ SEGUIR / filtro / búsqueda
                      ├── ⏰ tarjetas ──▶ Modal de repaso ──Esc──▶ Launcher
                      ├── ▤ diapositivas ──▶ Tab del deck ──▶ Fuentes [[…]]
                      └── ✎ examen ──▶ Tab del examen ──▶ Resultados
   nota .md ──▶ widget ──▶ (las tres, con alcance al concepto)
```

Every kind has exactly one target surface. The launcher never embeds a resource;
it only routes to one.

## 6. Scale rules

- A flat list dies at ~50 resources. Search and the kind filter are the only
  scaling mechanisms, so they ship with the list, not after it.
- Ordering is explicit and stable: due first, then recently updated. Never
  insertion order, so the list does not reshuffle while the user is reading it.
- The "continue" zone shows at most one item. Two "continue" entries is a list.
- A kind with zero resources in a topic is not rendered as an empty card.
- Filter state is per session, not persisted: a user who filtered to decks last
  week does not want to wonder where their cards went.

## 7. Component inventory

| Block | Where | Slice |
| --- | --- | --- |
| Shell + breadcrumb + filters | `app/ui/App.vue` (rewrite header) | app |
| Continue card | `widgets/continue-card` | widgets |
| Resource card (per-kind status slot) | `widgets/resource-card` | widgets |
| Filter + search bar | `widgets/resource-filter` | widgets |
| Review modal | `widgets/review-modal` wrapping the session page | widgets |
| Deck view | `pages/presentation` | pages |
| Exam view | `pages/exam` (future) | pages |
| In-note widget | `widgets/note-resources` + post-processor | widgets + app |
| Resource entity | `entities/resource` (reintroduced, see §2) | entities |
| Status bar item | `app/status-bar.ts` | app |

## 8. Decisions and open questions

Decided 2026-10-21:

1. **Grouping — flat list.** No topic sections. The topic is context on the card
   (`Atención ›`), not a container. Grouping is a later option if scale demands
   it; a flat list is cheaper to build and reads faster.
2. **Aggregation — concept × kind.** One card per resource. A topic with three
   concepts and two kinds produces six cards, and that is accepted: precision
   beats compactness while the corpus is small.

Open:

3. **Deck ownership**: a deck may cover a whole topic, not a concept. Until that
   is settled, decks are modelled as belonging to a concept like everything else.
4. **Exam source**: questions generated from the concept's cards, or an authored
   set. This changes the model (an exam may have no 1:1 card mapping).
5. **Staleness**: a deck generated from notes that changed afterwards. Showing it
   on the card ("fuentes cambiadas") is cheap; deciding what to do about it is not.
6. **Where the `Resource` type lives**: local to the page for now. Promoting it to
   `entities/resource` requires the decision that supersedes `ARCHITECTURE.md`
   §14, and there is no data source yet to justify it.

---

## 9. Delivery slices

| Slice | Commit | Content |
| --- | --- | --- |
| 1 | `d87cfe9` | Study view as a main-area tab, shared Pinia, Spanish labels |
| 2 | `71a9caa` | Resource launcher + review modal, fixture-driven (~1500 authored lines, accepted as one UI unit: the modal wiring lives inside the launcher page and splitting it would leave a commit that does not build) |
| 3 | `f5398a2` | Deck surface: Reveal mount adapter, fixture deck, harness route `present/deck-atencion` (§4.4). Harness-only on purpose |
| next | — | Deck delivery: `deck.md` parser (`markdown-it` + containers), Reveal CSS inside `styles.css`, plugin route, launcher wiring |

Slice 3 is harness-only for a mechanical reason: the production bundle is esbuild
and cannot import CSS, so wiring the page into the plugin router before the
stylesheet strategy exists would break `pnpm build`. It is still the first unit
that owns DOM, which is why it was isolated rather than bundled with the parser.

Slice 2 exceeded the ~400-line review budget on purpose. It is one behavior —
"a list of resources you can act on" — and its two halves cannot be separated
without an intermediate commit that imports a component that does not exist. The
deck surface is where the budget applies again, and it is a separate slice for a
stronger reason: it is the first unit that owns DOM (D8 in `PRESENTATION.md`).
