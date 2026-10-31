---
name: study-system
description: Propose study flashcards from the user's own corpus and write only the ones they accept, each citing the document and page it came from. Use when the user asks to study, review, or make cards for a topic or chapter (estudiar, repasar, tarjetas, flashcards, mazos).
compatibility: Needs network access to a running study-system server and its access credentials.
---

# Study system

Turns a page range of the user's own corpus into flashcards, with a source for
every card, and writes only what the user approves.

The server owns the data. This skill never touches a database, a file on the
server, or a repository: it reads and writes through the HTTP API.

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

Never guess a URL, and never fall back to `localhost` unless the user says the
server runs on this machine. Proposing cards against the wrong server is worse
than not starting: the writes land in someone's real study data.

## 2. Resolve where the cards go

Cards live in a concept, and a concept belongs to a topic. Find the destination
before drafting anything:

```bash
curl -sS -H "CF-Access-Client-Id: $STUDY_ACCESS_CLIENT_ID" \
  -H "CF-Access-Client-Secret: $STUDY_ACCESS_CLIENT_SECRET" \
  "$STUDY_BASE_URL/api/concepts"
```

If the concept does not exist, say so and ask whether to create it
(`POST /api/concepts`). Do not create topics or concepts on your own initiative.

## 3. Read the material

Take the page range the user named. If they named a chapter instead of pages, find
the pages first with a search and confirm the range with them — never infer a range
from a table of contents without showing it.

Read at most 50 pages per call and paginate the rest; that is the server's limit,
not a preference.

See `references/api.md` for the exact request and response of every endpoint.

## 4. Draft one card per fact

Read `references/cards.md` before writing a single card. It holds the rules that
decide whether a card is worth putting in front of a human, and the language rule.

The short version: one card states one fact, it is answerable without the book, and
its question is not a pointer ("what does page 27 say?"). Skip contents pages,
figures whose text is a caption, and anything you would have to guess.

## 5. Show the proposal and wait

Present every card like this, and present nothing else until the user answers:

```text
26/34 — DDD.pdf, p. 25
¿Qué es el lenguaje ubicuo?
Un lenguaje compartido por el equipo y los expertos de dominio, estructurado
alrededor del modelo, que se usa en el código, en las conversaciones y en los
documentos.
Fuente: "The UBIQUITOUS LANGUAGE carries knowledge in a dynamic form." (p. 25)
```

Then ask, in the user's language, to accept, edit, or discard — each card or a
batch. **Write nothing before that answer.** Not one card, not "the obvious ones".

## 6. Write only the accepted cards

Send the accepted cards to the concept. Omit `obsidian_id` entirely: the server
derives it from the concept and the question, which is what makes a repeated run an
update instead of a duplicate.

```bash
curl -sS -X POST "$STUDY_BASE_URL/api/sync/flashcards" \
  -H "Content-Type: application/json" \
  -H "CF-Access-Client-Id: $STUDY_ACCESS_CLIENT_ID" \
  -H "CF-Access-Client-Secret: $STUDY_ACCESS_CLIENT_SECRET" \
  -d '{"concept_id":"<id>","cards":[{"question":"…","answer":"…"}]}'
```

`{"synced":1}` is the confirmation. A card written this way becomes due
immediately.

## Rules that are not negotiable

- **No acceptance, no write.** An unapproved card costs the user real review time
  for months, and a wrong one is hard to notice later.
- **No citation you did not read.** Cite the page you actually took the text from.
  A page number you invented is a lie with a footnote.
- **Never re-propose what the user discarded** in this session.
- **Do not edit or delete cards that already exist.** This skill adds and updates
  what it just proposed; anything else is the user's job in the app.
