# Writing the plan

The plan is the deliverable. It has to fit on one screen and survive being printed: the
reader will look at it as a checklist for weeks.

## The template

```text
Libro: <título> — <N> páginas, procesado <fecha>
Tipo: <uno de los cuatro>
Test: <qué tiene que poder hacer el lector para decir que lo aprendió>

Objetivo: <la frase del lector, o "reconocimiento: todavía sin objetivo">
Modo: <lineal | por proyecto | reconocimiento | consulta>

Núcleo (≈<N>% del libro)
  <cap. y título>  pp. <a>-<b>   <para qué sirve>
  ...

Orden recomendado
  1. <cap> — <por qué este primero>
  2. ...

Descartar por ahora
  <cap/parte> — <por qué, y qué lo traería de vuelta>

Protocolo por sesión (25 min)
  1. Leer <bloque>: <páginas>
  2. Cerrar el libro y escribir <el output del tipo>
  3. Aplicar a <material propio> : un ejemplo y un contraejemplo
  4. El hueco → una pregunta recuperable
  5. Feynman de 3 min, en voz alta

Medir
  % de repasos pasados · decisiones tomadas sin consultar
```

Every page range comes from the book. A range you did not read is a lie with a number.

## Reading modes

| Mode | When | What it optimizes |
| --- | --- | --- |
| **Linear** | the book is sequential and each chapter assumes the previous one | comprehension of the argument |
| **Project-driven** | the reader has a decision to make now | transfer: the only mode that reliably changes what someone does |
| **Survey** | no objective yet | building a map cheaply, so a future problem has an address |
| **Consult** | reference books, and everything that did not earn study | time |

Say the mode out loud in the plan, because it sets the standard the reader should judge
themselves by. Survey mode with the expectations of project mode is how people conclude
they "can't remember anything".

## The core 20-30%

Almost no book is 100% core. The usual shape:

- 2-4 chapters carry the model or the argument;
- the rest applies it, refines it, or discusses organizations and contexts that a reader
  without that problem cannot use yet.

**Finding it from the inspection:** the chapters the preface promises, the ones the
conclusion summarizes, and the ones the contents give the most subheadings. Cross-check
those three lists: what appears in all three is the core.

**What to skip is part of the plan**, with the condition for coming back: "Part III, when
you have a model of your own to refactor".

## Pain mining — how to manufacture an objective

For a reader who has none ("someone told me this book would help"). Twenty minutes, five
lines, and it must be done **by the reader**, not for them.

> Take your current project and write five situations that already annoyed you.

- "When the business renamed this, I had to touch six files."
- "There is an operation that touches three modules and I do not know where it lives."
- "Everyone edits that module and nobody understands it."
- "The same datum is stored in two places and I do not know which one wins."
- "That transaction scares me because I do not know what else it breaks."

Then: each situation is a candidate objective, and the book has a chapter for each kind.
**The rule that makes it work:** only situations that actually happened, with the example
next to them. If the reader cannot say when it happened, it is a wish, and a wish cannot
choose a chapter.

Stop after the list and let the reader pick one. Picking is theirs.

## Survey mode, honestly

When there is no objective and none can be mined, the plan is a map:

- Read only the chapters that define the vocabulary, not the ones that refine it.
- **No cards.** A card the reader cannot apply rots in the review queue and reads as
  debt.
- **No summaries.** A summary of a chapter that is not yours yet is lost work.
- The output is a **trigger list**: one line per concept, and next to it the situation in
  the reader's work that would make it relevant — or empty.

```text
Aggregate           → when an operation touches several tables and needs a boundary
Bounded context     → when one word means two things in two areas
(—)                 → nothing in my work yet
```

Say plainly what survey mode buys (an address for a future problem) and what it does not
(retention, 20-30% by design). A reader who expected project-mode results from survey
mode concludes they are bad at this.

## The session protocol

One block per session, 25 minutes, always the same:

1. **Read one block** — a chapter or a section, never a whole book.
2. **Close the book.**
3. **Produce from memory**, in the format the type requires: a procedure performed, a
   criterion applied, a fact recalled, an argument reconstructed.
4. **Compare with the book.** The gap between what came out and what is printed is the
   entire value of the session.
5. **The gap becomes a retrievable question.**
6. **Schedule the re-retrieval.** Spacing is what turns a session into memory.

The step everyone skips is 3. Without it, reading is not studying; it is time spent near
a book.

## What to study next

Not the next chapter. **Yesterday's failure.**

The reader applies the chapter, gets stuck, and *that* becomes the question; the review
brings it back weak; they return to the paragraph. The contents give the order in which
failures are comprehensible; the failures give priority.

## What deserves a card

The standard `study-system` already enforces, plus the one that matters for planning:

- one card, one fact, answerable without the book, question not a pointer;
- **a judgment card beats a definition card**: if it can be answered by recognizing the
  book's own sentence, it is not retrieval;
- only what would be demanded under pressure — an exam, a decision, an argument.

Expect 8 to 10 judgment cards for a core chapter. A book that produces 300 cards was not
studied; it was transcribed.
