# send-message

Turn one question into one answer, built from the corpus and streamed from the
model, and persist both turns into the chat's own note.

This slice replaced `features/ask-chat`. That slice was scaffolded against
`POST /api/chat`, an endpoint the backend never grew: the plugin talks to the
model provider directly, and the backend stays a documents service (S3 in
`odd/tasks/chat-workspaces.md`).

## What one send does, in this order

1. **Guard.** A blank question is ignored. With no API key configured it reports
   that instead of calling, because a request without a key fails at the provider
   with a message the user cannot act on.
2. **Persist the question.** The user's turn goes into the chat's note *before*
   retrieval and before the model call. A question the user asked is a fact
   whether or not anything downstream succeeds, so it must not be the step that
   gets lost.
3. **Retrieve.** `GET /api/rag/search?q=<question>&limit=<top k>` through
   `entities/search-result`. The chat's own `top_k` wins over the global
   `retrievalTopK`.
4. **Compose.** `model/prompt.ts`, which is pure and unit-tested.
5. **Stream.** `POST {llmBaseUrl}/chat/completions` through `shared/llm`, with
   `stream: true`; the deltas accumulate into `answer` so the thread renders the
   answer as it arrives. `cancel()` aborts.
6. **Persist the answer.** Only on success, with one `Fuente: ` line per source.

A failure at step 3 or 5 leaves the user's turn in the note and adds no assistant
turn. An answer that was never received is not written.

## The frame is not editable

`model/prompt.ts` holds `CITATION_FRAME`: answer only from the context, say so
when the context is not enough, never invent a title or a page, close with
`Fuente: <título>, p. <página>`, answer in the language of the question.

The chat's own prompt — the one the user edits in the chat's note, and the reason
this feature exists (S1) — is **appended inside** that frame. It cannot remove
it. A prompt that tells the model to drop the citations still receives them, and
that is the point: a prompt that only works for one chat is not a way to switch
off the guarantee that an answer cites where it came from.

## Where the pieces are

- `model/prompt.ts` — pure: the frame, the numbered context, the citations, the
  history trim. Reads the chunk's `text` (the passage) and never the `snippet`
  (the 12-token marked window a human scans).
- `model/useSendMessage.ts` — the order above, and nothing else.
- `ui/MessageComposer.vue` — the input. It emits `submit` and `cancel`; it never
  calls the store, so the thread widget stays the single place that decides what
  a submit means.

## Limits

- The model is asked once, from the question verbatim. There is no query
  rewriting: a question sharing no vocabulary with an English book relies on the
  semantic stage of the search, the one measured over 16 Spanish queries in
  `odd/tasks/step5-search.md` (L4).
- If writing to the vault fails, the error is recorded on the chat store and the
  send continues: the answer streams to the screen but neither turn reaches the
  note. Recorded in the feature document rather than papered over.
