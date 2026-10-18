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
- **Un turno que no se pudo escribir.** La pregunta se persiste antes de recuperar y de llamar
  al modelo, así que sobrevive a un fallo posterior de la recuperación o del proveedor. Si la
  escritura del vault falla, el store de chats guarda el error y el envío continúa igual: el
  usuario ve la respuesta en pantalla, pero ningún turno queda en la nota. Está acotado a un
  vault que rechaza escrituras y no está cubierto por un test; se acepta y se deja escrito en
  vez de agregar una comprobación frágil sobre un error que puede venir de antes.

## Tasks

| ID | Alcance | Specs | Commit |
| --- | --- | --- | --- |
| W0 | Backend: `text` aditivo en la respuesta de búsqueda + tests | S3, L4 | `3c2bc88` |
| W1 | `shared/config` + `app/settings.ts` + `app/settings-tab.ts` + `features/configure-llm`, migración del api base, retiro de `configure-api-base` | S4, S5 | `cdbdf81` |
| W2 | `shared/llm`: cliente OpenRouter con streaming, parser SSE puro, mapeo de errores, tests | S3, S5 | `4a3cbd1` |
| W3 | `shared/obsidian` (vault) + `entities/chat` (tipos, frontmatter, regiones, adapter) | S2, S6 | `e4f32b3` |
| W4 | `features/send-message`: builder de prompt puro + orquestación (search → prompt → stream → nota) | S1, S3, S6 | `063f852` |
| W5a | `create-chat`, `select-chat`, `edit-chat-prompt`, `widgets/{chat-list,chat-thread,chat-settings}`, `MessageComposer` | S1, S7 | `aca2464` |
| W5b | `pages/chat` en tres zonas, `pages/corpus`, retiro de `ask-chat`/`chat-panel`/`chat-message`, wiring y docs | S1, S7 | `aca2464` |
| W6 | El arreglo de los dos caminos que perdían datos que encontró la verificación independiente (L6) | S2, S6 | `8d0f51b` |

## Slices (delivery strategy)

The feature is already past 1100 authored lines with W1, so it does not ship as
one pull request. Each slice below is one reviewable unit and one PR; the next
slice branches from the previous one's tip, and each slice carries its own
recorded checks. The line counts are measured with
`git diff --stat origin/master...HEAD` (lock files excluded).

| Slice | Work units | Commits | Review focus | Lines |
| --- | --- | --- | --- | --- |
| **A — El chunk y los ajustes** | W0, W1 | `3c2bc88`, `cdbdf81` (+ 2 docs) | The additive `text` field in the retrieval response, and the credential path: `data.json`, the migration off `localStorage`, and never a key in a URL or a log | 1200 |
| **B — El cliente del modelo y la nota** | W2, W3 | `4a3cbd1`, `e4f32b3` | The SSE parser (a split line must not lose a delta), the note's region surgery (it must never rewrite what the user wrote outside the markers), and the key leaving only in an `Authorization` header | +2252 |
| **C1 — La respuesta** | W4 | `063f852` | The citation frame staying in code and not in the user's prompt, and the orchestration order (retrieve, compose, stream, append) | +894 |
| **C2 — La UI y el arreglo** | W5a, W5b, W6 | `aca2464`, `8d0f51b` | The three zones, the retirement of the old chat slices, and the two data-loss paths the independent verification found | +2251 |

**L5 (2026-10-18, W0-W5 implementados y verificados)** — cada unidad se implementó y
verificó antes de commitear, con el conteo de tests como evidencia:

| Check | Resultado |
| --- | --- |
| `go test ./...` (backend, tras W0) | 13 paquetes `ok`; 3 tests nuevos observados RED primero (el campo no existía) |
| `gofmt -l` / `go vet ./...` | limpios |
| `npm test` (plugin) | 101 → 135 (W1) → 164 (W2) → 206 (W3) → 241 (W4) → 251 (W5a) → 260 (W6) |
| `src/architecture.test.ts` | 7 tests, verde en cada commit: ningún import cruzó una capa |
| `npm run build` (tsc + esbuild) | limpio en cada commit |
| `.vue` nuevos | sin arnés de componentes: se verificó que cada SFC parsea y su template compila con `@vue/compiler-sfc` |
| `pages/corpus` | existe porque retirar la subida y la búsqueda del chat habría borrado dos funciones que nadie pidió perder |

**L6 (2026-10-18, verificación independiente: dos caminos que perdían datos del usuario)** — se
corrió un verificador separado y de sólo lectura contra los dos ítems de riesgo declarados
(la credencial y la escritura en notas del usuario). La credencial pasó entera: la key sólo
viaja en el header `Authorization`, no aparece en URLs, cuerpos ni mensajes de error, no llega
al backend, no se loguea y no está en el diff. La escritura en notas falló, con contraejemplo:

1. `createChatNote` escribía una nota nueva encima de lo que hubiera en el path. Los dos clics
   por defecto en "Nuevo chat" proponen el título "Chat" y por lo tanto el mismo archivo, así
   que el segundo clic consumía el transcript del primero. Arreglado en `8d0f51b`
   (`uniqueChatPath` para la política, `mergeIntoExistingNote` para la garantía), con el input
   del contraejemplo convertido en test de regresión.
2. Reescribir el frontmatter descartaba todos los campos que el plugin no posee: una nota con
   `tags:` o `title:` propios los perdía al guardar un override. Arreglado en el mismo commit:
   las tres claves propias se mergean dentro del bloque existente y sólo esas tres.

Queda documentado y sin arreglar un caso menor que el mismo verificador reportó: si el prompt
del usuario contiene literalmente `<!-- prompt:end -->`, los bytes sobreviven pero el prompt se
re-parsea truncado. No se sanea el texto del usuario en silencio; se deja escrito acá.

Gaps honestos de cobertura: los `.vue` no tienen arnés de componentes (el suite corre en node y
sólo toma `*.test.ts`), así que el comportamiento visual e interactivo de las tres zonas está
verificado por lectura, no por test; y `app/settings-store.ts` (la migración de `localStorage`)
tampoco tiene test propio — su lógica pura sí (`mergeLegacyApiBase`), y el verificador independiente
la probó end-to-end con un vault y un `localStorage` falsos.

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

**L7 (2026-10-18, review nativo del slice A: aprobado con una corrección)** — el candidato
entero no entró en el presupuesto de contexto del review (`lens_context_budget_exceeded`, 78
paths / 6638 líneas), así que se acordó revisarlo por slice. El slice A (23 paths, 1186 líneas,
base `ab88b3a`) se revisó con `risk` y `reliability` y terminó **aprobado**
(`review-117aada2c5b6067d`, autoridad quemada).

- Dos capturas de la lente de confiabilidad fueron refusadas por admisión, con payloads del
  relay no conformes (`line_suffix_not_integer`; después falta de
  `evidence_class`/`causal_disposition` en un finding severo). La tercera entró, y ninguna
  negativa consumió el slot.
- El refutador no pudo lanzarse porque el rol `review-refuter` no tenía modelo en
  `~/.pi/gentle-ai/models.json`; el usuario agregó ese rol y `review-validator`.
- El refutador corroboró un hallazgo CRITICAL, `R3-legacy-api-base-resurrection`; la
  corrección (`beca505`, integrada a la rama como `c2db830`) consume la clave legacy de
  `localStorage` una sola vez, después de persistir el valor resuelto, así que la migración
  deja de re-aplicarse y no pisa el default que el usuario guardó a propósito. El validador
  dirigido la aceptó.

Advisory del slice A (no bloqueantes; trabajo posterior, nunca motivo para re-correr el review):

| ID | Lente | Sev | Ubicación |
| --- | --- | --- | --- |
| `R1-data-json-round-trip-drops-unknown-keys` | risk | SUGGESTION | `settings.ts:66-71` |
| `R1-llm-api-key-plaintext-storage` | risk | WARNING | `settings.ts:65-72` |
| `R1-search-full-text-exposure` | risk | WARNING | `rag/http/dto.go:27-30` |
| `R3-data-json-whole-file-replace` | reliability | SUGGESTION | `settings.ts:66-78` |
| `R3-hybrid-stage-text-unproved` | reliability | WARNING | `rag/app/search_test.go:77-101` |
| `R3-onload-settings-failure-path` | reliability | WARNING | `app/settings-store.ts:29-35` |

**L8 (2026-10-18, review nativo del slice B: aprobado con una corrección)** — slice B
(`cdbdf81..e4f32b3`, 18 paths, 2268 líneas) con `risk`, `resilience` y `reliability` (3 de 3
admitidas), lineage `review-90875faa44dfa943`, **aprobado**.

- Las lentes de riesgo y confiabilidad encontraron **el mismo defecto** que la verificación
  independiente ya había hallado y arreglado en `8d0f51b`: `createChat` derivaba el path sólo
  del slug y el create descartaba el contenido existente, así que dos chats con el mismo
  título consumían el transcript del primero (`R1-1` y `R3-1`, ambos CRITICAL). El refutador
  lo corroboró. Dos caminos independientes, el mismo hallazgo: eso es lo que el review
  separado por slice compra.
- La corrección (`f3fea57`; 141 líneas de diff contra 130 declaradas y un tope de 200) aplica
  el parche mínimo — `uniqueChatPath`, `mergeIntoExistingNote` y el store usándolos — con dos
  tests de regresión observados RED antes. El validador dirigido la aceptó.
- No hubo nada que integrar a la rama: ya traía el arreglo equivalente en `8d0f51b`.

Advisory del slice B (9, no bloqueantes): `R1-2` (`shared/llm/client.ts:185-192`, WARNING),
`R1-3` (`shared/obsidian/useObsidian.ts:99-104`, SUGGESTION), `R3-2` (`client.ts:189-191`,
WARNING), `R3-3` (`entities/chat/lib/note.ts:69-70`, WARNING), `R3-4` (`note.ts:215-218`,
WARNING), `R3-5` (`useObsidian.ts:139-141`, WARNING), `R4-create-orphans-file`
(`useChatsStore.ts:60-68`, WARNING), `R4-degraded-list-silent` (`useObsidian.ts:126-133`,
SUGGESTION), `R4-partial-content-dropped` (`client.ts:196-211`, WARNING).

**Nota de proceso.** El candidato es siempre `base..HEAD`, así que para acotar cada slice hay
que dejar HEAD en el tip del slice: se hace con un checkout detached por slice y se vuelve a
la rama al terminar. Quedan pendientes de review: **C1** (`e4f32b3..063f852`, ~894 líneas) y
**C2** (`063f852..8d0f51b`, ~2251 líneas).

