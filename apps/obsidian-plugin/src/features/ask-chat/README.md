# ask-chat

Ask a question about the study corpus and receive an answer with citations.

## Status: scaffolded, not served

**The chat endpoint does not exist yet** (ARCHITECTURE §8). This slice is written
against the contract below so the UI, the message model and the call site are
ready, but `useAskChat.ask` makes no network request: it records the question and
reports the missing endpoint. Wiring the request is a one-line change once the
server side lands, plus the `error` line in `model/useAskChat.ts`.

## Contract this slice is written against

```text
POST /api/chat
Content-Type: application/json

{ "question": "¿Qué es un agregado?" }
```

```text
200 OK
Content-Type: application/json

{
  "answer": "Un agregado es …",
  "citations": [
    { "document_id": "…", "document_title": "DDD", "page_number": 42 }
  ]
}
```

`citations` is optional; when present, each citation identifies the document and
the 1-based page the answer drew on. Errors use the same envelope as the other
endpoints — a JSON body with an `error` string — which
`shared/api/client.ts` already maps to `ApiError`.

## Boundaries

The request belongs below `shared/api/client.ts` (ARCHITECTURE §7d/§9): once the
endpoint exists, the call goes in an entity `api/` segment or in this slice's
model, through `request<T>`, never through a raw `fetch`.
