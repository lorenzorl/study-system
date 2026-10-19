# send-message

Turn one question into one answer, and persist both turns into the chat's own
note. What the model receives is decided by the chat's attachments, and there are
two classes of them: material searched on the server through a tool, and vault
files sent whole inside the message.

This slice replaced `features/ask-chat`. That slice was scaffolded against
`POST /api/chat`, an endpoint the backend never grew: the plugin talks to the
model provider directly, and the backend stays a documents service (S3 in
`odd/tasks/chat-workspaces.md`).

## What one send does, in this order

1. **Guard.** A blank question and a second send while one is in flight are both
   ignored. With no API key configured, or no chat open, it reports that instead
   of calling.
2. **Check the corpus.** Only when the chat has at least one corpus attachment:
   `model/preflight.ts` probes `GET /api/rag/documents`, the same context the tool
   reads. If the corpus does not answer, the send **stops here**, with a Spanish
   message naming the material — before the question is written and before any
   model call. This is what makes a corpus chat's failure predictable instead of
   discovered halfway through an answer (`S6`). A chat with only vault files, or
   with no attachments, never touches the backend.
3. **Read the vault files.** When the chat has vault attachments their text is
   read whole, now and not when they were attached (a note grows). A file that
   cannot be read, or no longer fits the size cap, refuses the send with a message
   saying which file and why — and **writes nothing**: the refusal happens before
   the question is persisted, so the text goes back to the user and a retry after
   fixing the file duplicates nothing.
4. **Persist the question.** The user's turn goes into the note before the model
   call, once every step that can refuse the send has agreed to it. A question the
   user asked is a fact whether or not anything downstream succeeds, so it must
   not be the step that gets lost — and a write the note refuses stops the send,
   because nothing was written to carry on from.
5. **Compose.** `model/prompt.ts`, which is pure and unit-tested.
6. **Stream, and search when asked.** `POST {llmBaseUrl}/chat/completions`
   through `shared/llm`, with `stream: true`; the deltas accumulate into `answer`
   so the thread renders the answer as it arrives. When the chat has corpus
   material, `tools` is declared and the model may ask for `search_material`; see
   the loop below.
7. **Persist the answer**, with one `Fuente: ` line per source and the `consulted`
   flag of the turn.

`cancel()` aborts the probe and every round after it: one `AbortController` per
send, created before the preflight, so a server that accepts the connection and
never answers cannot hold the composer.

## What the model receives depends on the material

| Chat has | Frame | `tools` | Round trips |
| --- | --- | --- | --- |
| No attachments | `NO_MATERIAL_FRAME`: its own knowledge, and no `Fuente:` lines — the format is forbidden by name, because a model with no pages satisfies it by inventing them | no | 1 |
| Vault files only | `CITATION_FRAME`: ground on the context, cite by page | no | 1 |
| Corpus documents | `SEARCHABLE_MATERIAL_FRAME`: grounding is a decision taken by searching, not a duty over material already in front of it | yes | 2+ |
| Both classes | `IN_MESSAGE_AND_SEARCHABLE_FRAME`: material in the message **and** material behind the tool. It merges the other two states instead of choosing one, because the citation frame's "only the context below" would forbid the search the same request declares | yes | 1 or 2+ |

The chat's own prompt — the one the user edits in the chat's note, and the reason
this feature exists (S1) — is **appended inside** the frame. It cannot remove it.

## The tool loop, and the scope invariant

`search_material(query, resource_id?)` returns passages with their title and page.
The **chat's attachments fix what the model may read**: a call that names nothing
covers exactly those, in one scoped search per document merged deterministically
by score; a call that names a document that is not attached is refused, and the
refusal goes back to the model as a tool result so it can adapt. Without that
rule, "attach this book to this chat" would mean nothing.

At most `MAX_TOOL_ITERATIONS` (3) rounds. The last ask omits `tools`, so the turn
ends with an answer instead of an error. The exchange with the tool lives only in
memory: `ChatTurn` cannot represent an assistant turn with `tool_calls`, and none
of it has to survive a reload. Only the results reach the note.

## The third state: consulted or not

A chat with corpus material can end with an answer that never searched. That is
detected here, deterministically — did any search actually run? — and never asked
of the model. A search counts only when the call passed the scope check and the
tool ran.

The note records only the negative, as `<!-- material: sin consultar -->` on the
assistant turn's last line; the thread shows `sin consultar el material` for those
turns. A consulted turn writes nothing, because an absent marker claims nothing:
an older note, or one from a chat with no material, never shows the badge.

## `send()` answers what happened to the question

`send()` returns `Promise<boolean>`: `true` once the question reached the note,
`false` when the send stopped before that (blank, no key, no chat, a blocked
preflight, a vault file that cannot be read or no longer fits its cap, a cancel
during the probe) — and `false` means *nothing* was persisted, not merely that no
answer came. `ui/MessageComposer.vue` holds no text of
its own — the draft is the caller's, through `v-model` — so the thread can give
the question back when it was never consumed. Without that, a blocked send would
take the user's text with it.

## Where the pieces are

- `model/prompt.ts` — pure: the four frames (one per material mode, including the
  mixed chat), the numbered context, the citations,
  the history trim. Reads the chunk's `text` (the passage) and never the `snippet`
  (the 12-token marked window a human scans).
- `model/tool.ts` — the tool's spec, the scope decision and its execution. No
  store, no loop.
- `model/preflight.ts` — one question: can the corpus be reached right now.
- `model/vault-text.ts` — the vault text of a send, through an injected reader so
  it is testable without the vault package.
- `model/useSendMessage.ts` — the order above, and nothing else.
- `ui/MessageComposer.vue` — the input as a `v-model` draft. It emits `submit` and
  `cancel`; it never calls the store, so the thread widget stays the single place
  that decides what a submit means.

## Limits

- **The model is not asked to rewrite the query.** A question sharing no
  vocabulary with an English book relies on the semantic stage of the search, the
  one measured over 16 Spanish queries in `odd/tasks/step5-search.md` (L4).
- **A corpus send costs one extra round trip** for the preflight, on top of the
  list endpoint's own cost. Predictability over latency is the accepted trade.
- **A provider that rejects `tools`** is reported as such when the rejection
  arrives before any answer; the capability of a model is measured at runtime,
  never guessed, because the model is chosen per chat and can change.
- **A failed write of the answer** loses the answer: it streamed to the screen, but
  it did not reach the note. A failed write of the *question* stops the send.
- **The size caps** for vault attachments live in
  `entities/chat/lib/attachment-limits.ts`, with the entity that owns
  `ChatAttachment`, because both this slice and `features/attach-resource` need
  them and a feature may not import a sibling.
