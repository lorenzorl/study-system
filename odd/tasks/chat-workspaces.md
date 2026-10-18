# Feature — Chat con workspaces en el plugin (LLM directo, prompt por nota)

**Status:** in progress
**Created:** 2026-10-18
**Branch:** `feat/chat-workspaces`
**Depends on:** step 5 slice 5.3 (`GET /api/rag/search`) and step 6 (upload/status) — la
recuperación ya existe y es lo único que el chat necesita del backend.

El plugin deja de necesitar un endpoint `POST /api/chat`: habla directo con el proveedor
del modelo y el backend Go queda como servicio de documentos. Cada chat es una nota `.md`
del vault con su propio prompt, y las preferencias globales del LLM viven en un
`PluginSettingTab`.

---

## Specs

Decisiones del usuario, citadas textualmente. Donde una spec cita una elección, la
implementación no puede re-decidirla en silencio.

**S1 — Un prompt por chat.** "quiero un chat que pueda configurar con un prompt que solo
funcione para ese chat". El prompt de un chat no puede alcanzar a otro, ni por defecto ni
por herencia implícita.

**S2 — El prompt es una nota del vault.** "en el plugin, como un md". Editable con el
editor de Obsidian, versionable, visible en el explorador de archivos.

**S3 — El cliente habla directo con el modelo.** "el plugin se comunica directamente con el
llm, el servidor es solo para los documentos por ahora". El backend no conoce la API key
del LLM ni recibe prompts de chat; sirve ingesta, chunks, embeddings y
`GET /api/rag/search`.

**S4 — Todo configurable desde la UI del plugin.** "Todo editable desde la UI del plugin".
Incluye la API key: no se edita el `.env` del backend para configurar el chat.

**S5 — La key la guarda el plugin y la manda en cada request.** "El plugin la guarda y la
manda en cada request".

**S6 — El historial vive en la misma nota.** "En la misma nota .md". La nota es el chat:
prompt arriba, transcript abajo, en un solo archivo.

**S7 — La referencia de UI es AnythingLLM.** "teniendo en cuenta que vamos a seguir como
ejemplo la ui de anythingllm". Tres zonas: lista de chats, hilo, settings del chat.

**S8 — Lo que el usuario ya tenía decidido para el resto del sistema, y no cambia acá.**
`ARCHITECTURE.md` rige el plugin (FSD + el guard de `src/architecture.test.ts`); el README
de `apps/go-middleware` rige el backend (contextos por dominio, `domain` sin imports
externos).

---

## Contract

Derivado de las specs y aceptado junto con el plan (ver L3). No agrega requisitos: dice
dónde y cómo se cumple cada spec.

- **Formato de la nota.** `Study Dashboard/Chats/<slug>.md`: frontmatter con los overrides
  del chat (`model`, `temperature`, `top_k`), región `<!-- prompt:begin -->…<!-- prompt:end -->`
  con el system prompt, y región `<!-- chat:begin -->…<!-- chat:end -->` con el transcript.
  Esas tres regiones son las únicas que el plugin escribe (S2, S6). Una cita se
guarda como dato plano (`Domain-Driven Design, p. 42`) y el `Fuente: ` que la
nota muestra es presentación del render, así que leer y escribir un turno
round-tripea sin prefijo ni markdown.
- **Escritura aditiva.** El plugin nunca reescribe el archivo entero: inserta dentro de la
  región, y si la región no existe la agrega al final. Lo que el usuario escriba afuera de
  los marcadores no se toca. Se escribe con `vault.process` (read-modify-write atómico), no
  con `read` + `modify`.
- **El frame de citas no es editable.** Las reglas "respondé solo con el contexto", "si no
  alcanza decilo", "citá documento y página" viven en el código (`prompt.ts`), no en el
  prompt del chat. El prompt del usuario se inserta dentro del frame (S1).
- **La lista de chats es la carpeta.** No hay tabla ni store como fuente de verdad: la
  fuente de verdad son los `.md`.
- **Configuración global.** `PluginSettingTab` propia, persistida en el `data.json` del
  plugin (S4, S5). La API base del backend se migra desde el `localStorage` actual.
- **Sin backend nuevo.** El único cambio en `apps/go-middleware` es aditivo: la respuesta de
  `GET /api/rag/search` suma `text` (el texto completo del chunk) además de `snippet`, porque
  el snippet de FTS5 son 12 tokens y no alcanza como contexto para el modelo (ver L4).

## Tasks

| ID | Alcance | Specs | Commit |
| --- | --- | --- | --- |
| W0 | Backend: `text` aditivo en la respuesta de búsqueda + tests | S3, L4 | `feat(rag): return the full chunk text from search` |
| W1 | `shared/config` + `app/settings.ts` + `app/settings-tab.ts` + `features/configure-llm`, migración del api base, retiro de `configure-api-base` | S4, S5 | `feat(plugin): own the plugin settings in data.json` |
| W2 | `shared/llm`: cliente OpenRouter con streaming, parser SSE puro, mapeo de errores, tests | S3, S5 | `feat(llm): stream chat completions from the plugin` |
| W3 | `shared/obsidian` (vault) + `entities/chat` (tipos, frontmatter, regiones, adapter) + `entities/chat-message` | S2, S6 | `feat(chat): store each chat as a vault note` |
| W4 | `features/send-message`: builder de prompt puro + orquestación (search → prompt → stream → nota) | S1, S3, S6 | `feat(chat): answer with corpus citations` |
| W5 | UI: `create-chat`, `select-chat`, `edit-chat-prompt`, `widgets/{chat-list,chat-thread,chat-settings}`, `pages/chat` en 3 zonas, `pages/corpus`, wiring y docs | S1, S7 | `feat(chat): add the three-pane chat UI` |

## Slices (delivery strategy)

The feature is already past 1100 authored lines with W1, so it does not ship as
one pull request. Each slice below is one reviewable unit and one PR; the next
slice branches from the previous one's tip, and each slice carries its own
recorded checks. The line counts are measured with
`git diff --stat origin/master...HEAD` (lock files excluded).

| Slice | Work units | Commits | Review focus | Lines |
| --- | --- | --- | --- | --- |
| **A — El chunk y los ajustes** | W0, W1 | `3c2bc88`, `cdbdf81` | The additive `text` field in the retrieval response, and the credential path: `data.json`, the migration off `localStorage`, and never a key in a URL or a log | 1186 |
| **B — El cliente del modelo y la nota** | W2, W3 | recorded with them | The SSE parser (a split line must not lose a delta), the note's region surgery (it must never rewrite what the user wrote outside the markers), and the key leaving only in an `Authorization` header | — |
| **C — La respuesta y la UI de tres zonas** | W4, W5 | recorded with them | The citation frame staying in code and not in the user's prompt, the orchestration order (retrieve, compose, stream, append), and the wiring | — |

## Log

**L1 (2026-10-18, pedido original)** — "en el plugin, teniendo en cuenta que vamos a seguir
como ejemplo la ui de anythingllm como puedo organizar todo? por ahora solo tenemos laui del
chat pero quiero que me propongas como podemos hacerla como anythingllm, quiero un chat que
pueda configurar con un prompt que solo funcione para ese chat, dime como se puede hacer,
tambien dime donde puedo poner las configuraciones globales del llm".

Encargo explícitamente de propuesta, así que la primera respuesta fue diseño sin código:
tres niveles de configuración (global / chat / mensaje), el prompt como dato de la
conversación y no de la UI, y el árbol FSD destino.

**L2 (2026-10-18, decisiones del usuario)** — cinco respuestas, en orden:

1. Dónde vive el prompt de cada chat: "en el plugin, como un md".
2. Dónde van las configuraciones globales: "Todo editable desde la UI del plugin".
3. Por dónde empezar: "Backend primero: contexto chat/ (recomendado)".
4. La key y el servidor: "El plugin la guarda y la manda en cada request".
5. El historial: "En la misma nota .md".

**L3 (2026-10-18, corrección de arquitectura y autorización)** — el usuario corrigió el eje
del diseño: "el plugin se comunica directamente con el llm, el servidor es solo para los
documentos por ahora". Consecuencia: se cayeron del plan `internal/chat/`, las tablas
`conversations`/`messages` y `platform/llm.Chat`; la decisión 3 quedó sin efecto porque el
backend no cambia; y todo el trabajo se muda al plugin. Se verificó con evidencia real (no
supuesta) que OpenRouter responde CORS a los tres orígenes del cliente:

| Origen | Resultado medido |
| --- | --- |
| `app://obsidian.md` (desktop) | `OPTIONS` → `204`, `access-control-allow-origin: *`, `authorization` en allow-headers |
| `capacitor://localhost` (mobile) | `401` real con `access-control-allow-origin: *` |
| `http://localhost:5173` (harness) | `401` real con `access-control-allow-origin: *` |

Por eso el cliente usa `fetch` (streamea) y no `requestUrl` (esquiva CORS pero no streamea).
El usuario autorizó con "ok implementalo".

**L4 (2026-10-18, hallazgo durante la implementación: el snippet no alcanza como contexto)** —
`GET /api/rag/search` devuelve `snippet`, que para un resultado léxico es
`snippet(chunks_fts, 1, '…', 12)`: una ventana de **12 tokens** con los términos marcados con
`[` y `]`, y para uno semántico un extracto de 240 runas. Ninguno de los dos es el texto del
chunk. Con 12 tokens de contexto el modelo no puede responder: inventa o dice que el contexto
no alcanza. Se agrega `text` (el texto completo del chunk) a la respuesta, de forma aditiva:
`snippet` queda para la UI, `text` para el contexto del modelo. Es un campo más en una
respuesta cuyo único consumidor es este plugin, y el servidor sigue sirviendo documentos,
que es exactamente lo que S3 le deja hacer.
