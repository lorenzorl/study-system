# What makes a card worth proposing

A card is a question the user will meet again in three months, alone, with no book
in front of them. Everything here follows from that.

## The four rules

**One fact per card.** A card that asks two things gets answered half-right and the
scheduler learns nothing useful.

**Answerable without the source.** The answer must stand alone. If it needs the
paragraph around it, it is a note, not a card.

**The question is not a pointer.** "What does Evans say on page 27?" tests nothing;
it asks where to look. "What does the ubiquitous language structure itself around?"
tests whether the idea landed.

**Short.** Question under about 120 characters, answer one to three sentences.
Length is where invented detail hides.

## Language

Write the card in the language the concept already uses:

- If the concept has cards, read `GET /api/study/due` or ask; match them. Mixed
  languages in one deck are a defect, not a detail.
- If it has none, use the language the user is writing to you in.
- The source's language never decides the card's language. A Spanish card about an
  English book is normal: translate the idea, and keep the quote in the original
  in the citation.

## The citation

Every card names `document title` and `page`. Quote the sentence the fact came from,
verbatim and short — it is what the user checks against when a card feels wrong,
and it is the only thing standing between a grounded card and a confident invention.

A `transcribed` page was described by a model, not written by the author. Cite it
as such: it supports "the diagram shows the aggregate boundary", not "Evans says".

## Skip these

- Tables of contents, index pages, copyright, dedications, chapter title pages.
- Cross-references: "see Chapter 14". They are navigation, not knowledge.
- Figure captions with no content in them.
- Any page whose `text` is empty or whose `status` is `failed`.
- Anything you would have to complete from your own knowledge of the subject. That
  is the moment to stop, not to be helpful.

## Do not propose a duplicate

If the concept already asks the same thing, do not propose it again. The server's
derived identity makes a repeated write an update rather than a second card, but a
duplicate still costs the user attention and looks like the tool is not paying
attention.

Compare by meaning, not by string: "¿Qué es un agregado?" and "Definí agregado en
DDD" are the same question.

## Good and bad

| | Card |
| --- | --- |
| ✗ | **P:** ¿Qué dice la página 27? **A:** Habla del lenguaje ubicuo. |
| ✓ | **P:** ¿Qué tres usos tiene el lenguaje ubicuo? **A:** Hablar del modelo, escribirlo y codificarlo: el modelo es la columna del lenguaje. |
| ✗ | **P:** ¿Cuáles son los patrones del capítulo 2? **A:** Lenguaje ubicuo, modelo, aislamiento. *(una tarjeta con tres hechos y sin el hecho)* |
| ✓ | **P:** ¿Qué pasa si el vocabulario del código y el de los expertos divergen? **A:** El modelo pierde significado: las traducciones son inexactas y el conocimiento se diluye en el camino. |
| ✗ | **P:** ¿Qué es un agregado? **A:** Ver página 90. |
| ✓ | **P:** ¿Qué dos cosas tiene todo agregado? **A:** Una raíz — una entidad con identidad propia dentro del límite — y una frontera que define qué queda adentro. |

## When in doubt

Propose it and let the user decide, but say what makes you doubt — "la fuente es una
página transcrita de una figura", "este hecho aparece en dos páginas con matices
distintos". A proposal with a named doubt is easier to accept than one that hides it.
