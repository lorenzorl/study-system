---
name: study-plan
description: Classify an uploaded book and recommend how to study it — what kind of knowledge it carries, which chapters are the core, in what order, with what retrieval test, and what to skip. Use after a book is uploaded, or when the user asks where to start with a book already ingested (acabo de subir un libro, cómo estudio este libro, por dónde empiezo, qué leo primero, qué me conviene estudiar, cómo armo un plan).
compatibility: Needs network access to a running study-system server and its access credentials.
---

# Study plan

A book in the corpus is raw material, not a plan. This skill reads its skeleton,
decides what kind of knowledge it carries, and returns how to study it. It writes
nothing to the server.

**The premise that makes this worth doing:** the unit of study is not a chapter, it is
the decision the reader cannot yet make well. Everything below is in service of
finding that decision and pointing at the pages that change it.

## 1. Connect, and stop if the connection is wrong

Every command runs in a fresh shell, so source the environment first:

```bash
set -a; . "${STUDY_ENV_FILE:-$HOME/.config/study-system/env}"; set +a
```

Then prove the connection before doing anything else:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "CF-Access-Client-Id: $STUDY_ACCESS_CLIENT_ID" \
  -H "CF-Access-Client-Secret: $STUDY_ACCESS_CLIENT_SECRET" \
  "$STUDY_BASE_URL/api/rag/documents"
```

- `200` — continue.
- `403` — the access token is wrong or expired. Stop; ask for a new one.
- `000` — the server is unreachable. Stop; report the URL that failed.
- `STUDY_BASE_URL` empty — stop and ask for it.

Never guess a URL, and never fall back to `localhost` unless the user says the server
runs on this machine.

## 2. Find the book

```bash
curl -sS -H "CF-Access-Client-Id: $STUDY_ACCESS_CLIENT_ID" \
  -H "CF-Access-Client-Secret: $STUDY_ACCESS_CLIENT_SECRET" \
  "$STUDY_BASE_URL/api/rag/documents"
```

One book: use it and say which one you are planning. Several: ask the user which,
naming the titles. Never pick by size or by the newest.

**`pages` is the gate.** A book whose `pages` is `0` has not finished ingesting: say so,
recommend waiting, and stop. A plan built on a book you cannot read is a guess with a
page number.

## 3. Inspect the skeleton before reading a single chapter

This is Adler's *inspectional* pass: 30 to 45 minutes of a human's attention, five
requests here. It exists so the reader does not read the whole book to discover that
only a third of it was theirs.

1. **The stored structure** — `GET /api/rag/documents/{id}/sections`. This is the
   chapter table the ingestion already recovered and validated: one entry per part,
   chapter and section, with `level`, `parent_id`, and the printed `start_page` and
   `end_page`. It is the answer to "what are this book's chapters" and it needs no
   reassembly. Read it first, and plan from it whenever it is not empty.
2. **Front matter** — only when the structure is empty, or when it disagrees with the
   book: `GET /api/rag/documents/{id}/pages?from=1&to=6`. The printed table of contents
   is usually here, arrives mangled by dot leaders, and needs its titles reassembled by
   hand.
3. **Preface or introduction** — the pages between the contents and the first chapter.
   Read them. This is the only place the author says who the book is for and what it
   assumes.
4. **The ending** — `?from=<last-5>&to=<last>`. You cannot plan a route without the
   destination. **The last pages are often back matter**: a bibliography, an appendix, an
   index. Check the contents before treating them as the conclusion, and read the end of
   the last chapter of content instead. (`DDD.pdf` ends with a bibliography on p. 357 and
   an appendix on pp. 358-359; its conclusions start at p. 349.)
4. **Density sample** — three pages from the middle, spread out. Measures how many
   ideas a page carries, and whether the vocabulary is legible at all.
5. **Density sample** — three pages from the middle, spread out. Measures how many
   ideas a page carries, and whether the vocabulary is legible at all.
6. **Vocabulary probe** — one or two `GET /api/rag/search?q=<term>&document_id=<id>`
   calls with the book's own key terms. If the results are about the book's subject,
   the corpus understands it; if they are unrelated, say so.

Rules for this step:

- **The stored structure is the preferred skeleton, and it is normal for it to be
  empty.** An empty array is a document with no recovered structure — a novel, an
  essay, a scan whose headings the cascade could not find — not a failed request. Fall
  back to the printed contents, and say in the plan which of the two you used.
- **Cite the ranges the structure gives you.** When a chapter comes from
  `/sections`, quote its `start_page` and `end_page` rather than reading a page number
  off the printed contents: the stored range was checked against the pages themselves.
- **A book with no printed contents** (a novel, an essay, an argument) has no skeleton
  to read. Say so and work from the front matter, the ending and the reader's own
  question. Do not invent chapters.
- `status: transcribed` means a model described a page, usually a figure or a divider.
  Cite it as a description, never as the author's words.
- Sample pages with `status: extracted` when you need the author's sentence.
- **When both skeletons are missing or mangled**, say that in the plan instead of
  smoothing over it.

## 4. Classify the book — first deliverable

Read `references/classification.md` before answering. Then state, in the user's
language:

```text
Tipo: conceptual (con una cola procedimental)
Por qué: <dos frases con la página que lo muestra>
Test de recuperación: <el que corresponde al tipo>
```

Rules:

- **One dominant type**, with evidence. "It's a bit of everything" is not a
  classification, it is an excuse to study nothing in particular.
- The type is not a label, it decides the **retrieval test**. A book misclassified gets
  practice of the wrong kind and feels productive while it fails.
- If the classification changes what the reader should do (most do), say the consequence
  in the same breath: "so cards of definition are useless here; what works is applying
  the criterion".

## 5. Establish the objective — ask, and wait

One question, and then stop:

> ¿Qué decisión querés poder tomar mejor en tres meses?

- **If the reader answers**, use it to order the chapters: the chapter that serves that
  decision comes first, whatever the book's own order says.
- **If the reader has no objective** — common, and valid, and they may phrase it as
  "me dijeron que este libro ayuda" — run the pain-mining exercise in
  `references/plan.md`: five concrete situations from the reader's own work, each one a
  candidate objective. **Wait for the answer.** Never invent their project, their pain,
  or their objective.
- **If still no objective after that**, recommend **survey mode** and say plainly what
  it buys and what it does not: retention will be low by design, and the output is a
  map, not knowledge.

## 6. Recommend how to read it — second deliverable

Read `references/plan.md` and fill its template. The plan must state:

- the **reading mode**: linear, project-driven, survey, or consult only;
- the **core 20-30%**: which chapters, by title and by the page range the book prints;
- the **order**, when it differs from the book's own, and the reason;
- the **session protocol**: block size, what to produce from memory, the application
  step;
- what to **skip**, and on what condition it comes back;
- the **two numbers** that measure progress.

Present it in the reader's language. Every page range comes from the book.

## 7. Store the plan — the one write this skill makes

The plan stops being text here. Post it so the app can project it, and so the reader
can mark a range read from it:

```bash
curl -sS -X POST \
  -H "CF-Access-Client-Id: $STUDY_ACCESS_CLIENT_ID" \
  -H "CF-Access-Client-Secret: $STUDY_ACCESS_CLIENT_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
        "document_id": "<the id from step 2>",
        "reading_mode": "linear",
        "objective": "<the decision the reader named, verbatim>",
        "source": "sections",
        "items": [
          {"title": "Chapter 2: Ubiquitous Language", "from_page": 24, "to_page": 36,
           "note": "first: it names the vocabulary every later chapter uses"}
        ]
      }' \
  "$STUDY_BASE_URL/api/study/plan"
```

- `reading_mode` is the mode you chose in step 6, one of `linear`, `project`, `survey`,
  `consult`.
- `source` is the skeleton the ranges came from: `sections`, `printed-contents`, or
  `none`. The app stores it so a reader can tell later what the pages were derived from.
- **One item per range you recommend, in the order you recommend reading it.** The
  server keeps that order and numbers the items; do not send a position.
- **Only ranges you would stand behind.** A chapter you told the reader to skip is not an
  item. A plan is partial by design and does not have to cover the book.
- Post **after** the reader chose the first chapter. Posting before that stores a plan
  nobody accepted.
- `201` means it landed, and it supersedes the previous plan for this book: the old plan
  is kept, not deleted. `400` answers a sentence naming the field or the item to fix —
  fix it and post again; nothing was stored. `404` means that document id is not in the
  corpus, so go back to step 2 rather than inventing one.

## 8. Hand off

The plan is stored and where this skill stops. The cards are `study-system`'s job: one
chapter at a time, after the reader picks the first chapter from this plan.

## Rules that are not negotiable

- **No page number you did not read, no chapter the book does not print.** A structure
  you invented is worse than no plan: it sends the reader to the wrong pages with
  confidence.
- **One dominant type, with evidence.** No classification without a page that shows it.
- **Never invent the reader's objective or their project.** Ask, then wait.
- **The plan is a proposal.** The reader chooses the first chapter; do not start one
  because it seems obvious.
- **This skill writes the plan and nothing else.** No cards, no topics, no concepts:
  posting a card here would skip the "no acceptance, no write" rule that
  `study-system` enforces. The plan is the one write, and it carries no card.
- **Do not recommend studying everything.** A book that deserves 30 minutes of
  extraction and nothing more is a normal outcome, and saying so is part of the answer.
