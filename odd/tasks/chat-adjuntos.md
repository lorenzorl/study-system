# Feature — Adjuntos en el chat: material del corpus por tool-calling, archivos del vault completos

**Status:** implementación autorizada (2026-10-18) — fase 1 en curso
**Created:** 2026-10-18
**Branch:** `feat/chat-adjuntos` (base `master` en `f9fa3b4`)
**Depends on:** `chat-workspaces.md` (el chat ya habla directo con el LLM) y step 5 slice 5.3
(`GET /api/rag/search`), que necesita un filtro por documento que todavía no tiene.

El chat deja de depender del servidor Go para existir: sin adjuntos es una llamada directa
al modelo y nada más; con adjuntos, el modelo decide si consulta el material. Los adjuntos
son de dos clases — documentos del corpus (servidor, por tool) y archivos del vault (se
envían completos) — y cada clase tiene su propia regla de fallo.

---

## Specs

Decisiones del usuario, citadas textualmente. Donde una spec cita una elección, la
implementación no puede re-decidirla en silencio.

**S1 — Chatear sin el servidor.** "quiero poder chatear sin necesidad del servidor, del
servidor solo puedo agregar recursos que esten en el servidor". El chat sin adjuntos no
toca el backend.

**S2 — El servidor es fuente de material, no requisito de ejecución.** "del servidor solo
puedo agregar recursos que esten en el servidor". Sirve documentos; no participa del chat
salvo cuando el material adjunto es suyo.

**S3 — El material del servidor se adjunta al chat y se pide al usarse.** "debe haber
alguna manera de agregar recursos del servidor al chat y cuando se referencien en el chat
es cuando se van a pedir en el servidor".

**S4 — La búsqueda del material es una tool, en turnos.** "el mensaje se envia al llm con la
referencia al adjunto, el llm lo lee y ejecuta una tool que trae hace la busqueda
correspondiente y trae los resultados, despues los resultados se envian al llm otra vez y
alli es donde el llm responde en el chat al usuario".

**S5 — Sin adjuntos no hay tool: se chatea normal.** "no es necesario si no hay archivos
adjuntos en el chat. Entonces si no los hay solo se chatea normal".

**S6 — Con el material del servidor caído, el mensaje se bloquea con un error.** "bloquear
mostrando un error". No se degrada: no se responde sin el material adjunto.

**S7 — También se adjuntan archivos del vault, y esos van completos.** "se pueden enviar
archivos adjuntos que esten dentro de obsidian esos si se enviarian completos".

**S8 — Un archivo que no entra en el contexto se rechaza al adjuntar.** Elección del usuario
entre tres políticas: "Tope con rechazo". No se recorta, no se envía a medias y no se
avisa después: se rechaza antes.

**S9 — Lo que ya regía y no cambia acá.** `ARCHITECTURE.md` rige el plugin (FSD + el guard
de `src/architecture.test.ts`); `chat-workspaces.md` S1–S8 rigen el chat que ya existe
(prompt por nota, key en el plugin, historial en la misma nota); el README de
`apps/go-middleware` rige el backend (contextos por dominio, `domain` sin imports externos).

---

## Contract

Derivado de las specs. No agrega requisitos: dice dónde y cómo se cumple cada una, y qué
ya existe para no construir de más.

### Las dos clases de adjunto

| | Recurso del corpus | Archivo del vault |
| --- | --- | --- |
| Dónde vive | Servidor Go (+ SQLite) | Vault de Obsidian |
| Cómo entra al contexto | Tool `search_material` (S4) | Completo, inyectado (S7) |
| Servidor requerido | Sí | No |
| `tools` en el payload | Sí | No |
| Servidor caído | Bloquea (S6) | Nada |
| Round trips al modelo | 2+ | 1 |
| Cita | `Fuente: <título>, p. <página>` | `Fuente: <ruta de la nota>` |

Un adjunto es de una clase u otra, nunca de las dos. `resource` (el concepto que ya existe
en `entities/resource/`) **no** es ninguna de las dos: es una ficha bibliográfica (título,
tipo, `source_uri`) colgada de un topic, sin texto y sin `document_id`. No se reutiliza.

### El invariante de la tool

`tools` se declara en el payload **si y solo si el chat tiene al menos un adjunto del
corpus**. Un chat con tres notas del vault y ningún documento del corpus: cero tools, cero
servidor, un solo round trip — que es S5, y es el caso que resuelve S1.

### El alcance lo fija el chat, no el modelo

```text
search_material(query: string, resource_id?: string)
  → [{ resource_id, title, page, text }]
```

`resource_id` sirve para **estrechar** dentro de los adjuntos de ese chat. Si el modelo pide
un id que no está adjunto, el cliente rechaza la llamada y le devuelve el error. El modelo
no puede ampliar su propio alcance al corpus entero: sin esta regla, "adjuntar" deja de
significar algo.

### Los tres estados del frame

`CITATION_FRAME` (`src/features/send-message/model/prompt.ts:36`) hoy dice "no respondas con
tu propio conocimiento": es una **obligación**. Con tool-calling tiene que ser una
**decisión**, o el modelo busca siempre y no se gana nada.

| Estado | Frame | Verificación | UI |
| --- | --- | --- | --- |
| Sin adjuntos | Conocimiento propio, sin citas de páginas | — | nada |
| Con adjuntos, buscó | El frame de citas actual | hubo `tool_call` | las fuentes |
| Con adjuntos, **no** buscó | Puede responder, pero **debe decir que no consultó el material** | no hubo `tool_call` | insignia "sin consultar el material" |

El tercer estado se detecta en el cliente de forma determinista — **¿hubo `tool_call` en este
turno?** — y nunca preguntándole al modelo. Es lo que evita que una respuesta de memoria se
disfrace de respuesta fundada.

### La conversación con tools es transitoria

El intercambio con tools vive **solo durante un mensaje, en memoria**. No se escribe en la
nota.

El motivo: `ChatTurn = { role, content, sources }` con `ChatTurnRole = "user" | "assistant"`
(`src/entities/chat/model/types.ts:1-10`) no puede representar un `assistant` con
`tool_calls` seguido de sus resultados, y el proveedor exige ese par dentro del mismo
request. Nada tiene que sobrevivir a un reload, así que no hay nada que persistir. A la nota
van solo los **resultados**: texto, fuentes, y un booleano nuevo `consulted`. La nota sigue
siendo legible y editable a mano, que es una invariante que ya existe en
`entities/chat/lib/note.ts` (lo escrito fuera de las regiones se copia byte a byte).

### El bucle, sus límites y su cancelación

- **Máximo 3 iteraciones.** Al llegar al tope se pide la respuesta final con lo que haya. Sin
  tope, una consulta vaga encadena búsquedas y quema la key del usuario.
- **Cancelar aborta el bucle entero**, no el round en curso. Hoy `controller` se reemplaza
  por request (`useSendMessage.ts:110`); en un bucle hace falta además el corte entre
  iteraciones. Sin eso, cancelar deja el bucle corriendo.
- El guard de un envío a la vez (`sending`) se mantiene tal cual.

### Política de tamaño de los archivos del vault (S8)

"Completos" no escala, y el material que el propio sistema genera lo demuestra: `data/DDD.md`
tiene 917.616 caracteres (~18.448 líneas, del orden de 200k tokens). No entra en la ventana
de ningún modelo.

- **Tope por adjunto: 80.000 caracteres** (~20k tokens en español).
- **Tope del total adjuntado: 120.000 caracteres**, para que cinco archivos aceptados no
  sumen 400k.
- Un adjunto que supera el tope **se rechaza al adjuntar**, con un mensaje que dice el
  tamaño, el tope y qué hacer. No se envía, no se recorta, no se avisa después.
- Los topes son **constantes con nombre**, no ajustes de UI: un knob que nadie calibra es un
  knob que nadie entiende. Si la experiencia muestra que el número está mal, se cambia la
  constante y su test.
- La unidad es el carácter porque el plugin no tiene tokenizador, y agregar uno sería una
  dependencia nueva para una estimación. **Los números son un punto de partida a validar con
  notas reales, no un valor medido**: el test fija la política, no la calibración.

Límite de alcance declarado: esto cubre **texto**. Un PDF o una imagen *dentro del vault*
queda fuera — extraer texto de un PDF es exactamente lo que hace el ingest del servidor, y
ese trabajo no existe del lado del plugin.

### Bloques de contexto y citas

`PromptPassage` (`prompt.ts`) se vuelve una unión, porque una nota del vault no tiene página
y `CITATION_FRAME` exige `Fuente: <título>, p. <página>`:

```ts
type ContextBlock =
  | { kind: "corpus"; title: string; page: number; text: string }
  | { kind: "vault";  title: string; path: string;  text: string }
```

Los resultados de la tool entran como `kind: "corpus"`. `buildSources` renderiza
`Fuente: DDD, p. 142` o `Fuente: Notas/Tesis.md` según la clase, y sigue deduplicando en
orden de aparición. Los archivos del vault son contexto, así que viajan en el mismo bloque
que los pasajes: `[system, ...historial, user]`, con `buildUserMessage` recibiendo la unión.

### El adjunto como dato

En una **región de la nota**, no en el frontmatter. El frontmatter del chat es plano por
diseño: `parseFrontmatter` devuelve `Record<string, string>` y `KEY_ORDER` son tres claves
sueltas (`entities/chat/lib/frontmatter.ts:15`). Una lista de registros no entra ahí sin
extender el parser a YAML, y el prompt y el transcript ya resolvieron lo mismo con
marcadores: una región más es esa misma respuesta, no una nueva.

```text
<!-- adjuntos:begin -->
- corpus | 279fadac-07d5-442d-97dd-b9d107f97164 | DDD
- vault | Notas/Tesis.md
<!-- adjuntos:end -->
```

Una línea por adjunto, `clase | identidad | etiqueta`: `document_id` para el corpus, ruta del
vault para el archivo. Texto plano y a mano, como el resto de la nota. **No se guardan datos
derivados** (tamaño, título cacheado): el tamaño se calcula leyendo el archivo al adjuntar, y
un dato derivado guardado es un dato que se queda viejo. La lista vive en el cliente: el
servidor no sabe qué chats la referencian.

### Bloqueo por servidor caído (S6)

Con al menos un adjunto del corpus y el servidor inalcanzable, el mensaje **se bloquea antes
de la primera llamada al modelo**, con un error que dice qué material no está disponible.
Se bloquea pronto y no cuando la tool falle: no se gasta un round trip para descubrir lo que
ya se sabía.

Consecuencia aceptada, explícita: un chat que tiene *un* documento del corpus adjunto no
sirve ni para charlar mientras el servidor esté apagado. Es el precio de la predictibilidad
sobre la degradación, y es una elección del usuario (S6), no un efecto colateral.

### Lo que falta en el servidor (dos cosas, no tres)

1. **`document_id` opcional en `GET /api/rag/search`.** Hoy `Search(ctx, query, limit)`
   (`internal/rag/app/search.go:85`, contrato en `internal/rag/http/contracts.go:21`) no
   tiene filtro. Sin esto la tool no puede acotar y adjuntar no cambia nada de lo que el
   modelo recibe.
2. **`GET /api/rag/documents`.** Hoy el router RAG expone solo `search`, `upload` y `status`
   (`internal/rag/http/router.go:17-19`): no hay forma de listar el corpus, así que no hay
   forma de *elegir* qué adjuntar. La respuesta es un array de
   `{ id, title, created_at, pages, chunks }`: lo que la tabla `documents` ya tiene más los
   dos conteos que salen de `job_pages` y `chunks`, para que quien elige un adjunto vea si el
   documento terminó de procesarse sin pedir el estado de cada uno por separado. `artifact_id`,
   `source_uri` y `content_sha256` no salen de la API: son internos del servidor.

**No** hace falta `GET /api/rag/documents/{id}/content` (bajar el `book.md`): eso era para
material local. Queda para una fase posterior, y el diseño no la bloquea — la tool y una
búsqueda local comparten la misma interfaz `search(query, scope)`, así que cambiaría la
implementación, no el diseño.

### Capacidad del modelo

`tools` requiere un modelo con function calling, y el modelo se elige **por chat**
(`ChatNoteSettings.model`). La variante es real: un chat puede tener un modelo que no soporta
tools. El plugin tiene que fallar con un mensaje claro cuando el proveedor rechaza `tools`, y
no dejar el chat mudo. La capacidad por modelo **no está verificada** y hay que medirla, no
suponerla.

### UI y costo

Con adjuntos del corpus, la primera respuesta del modelo no trae texto: trae una tool call.
No se puede stremear todavía, así que la UI muestra "consultando el material…" y el stream
arranca en la segunda llamada. Cada pregunta con adjuntos del corpus paga 2+ round trips,
incluso cuando el modelo decide no buscar. Sin adjuntos, nada de esto ocurre.

### Lo que ya existe y no hay que construir

- Leer el vault: `readVaultFile` y `vaultFileExists` ya están en el límite de Obsidian
  (`src/shared/obsidian/useObsidian.ts:70,81`) y `features/` puede importarlos sin violar
  `ARCHITECTURE.md` §4.
- El transporte directo al LLM: `src/shared/llm/client.ts:77`.
- El guard de un envío a la vez y la cancelación por `AbortController`.

---

## Tasks

Autorizadas el 2026-10-18. Cada una lleva sus tests (test-first) y su commit de unidad
dentro de su fase.

| ID | Fase | Linked | Qué | Dónde |
| --- | --- | --- | --- | --- |
| T1 | 1 | S1, S5 | `send()` deja de llamar a la recuperación: sin adjuntos no se toca el servidor | `features/send-message/model` |
| T2 | 1 | S1 | El frame "sin material": conocimiento propio, sin exigir líneas `Fuente:` | `features/send-message/model/prompt.ts` |
| T3 | 1 | S1 | Actualizar el test que hoy fija el comportamiento contrario (el gate) | `features/send-message/model` |
| T4 | 2 | S3, S7 | Las dos clases de adjunto en el frontmatter de la nota (parse y serialize) | `entities/chat/lib` |
| T5 | 2 | S7, S8 | Carga del archivo del vault con los topes y el rechazo al adjuntar | `features/*/model` |
| T6 | 3 | S3, S4 | `document_id` opcional en `GET /api/rag/search`, con su filtro en las dos búsquedas | `internal/rag/{app,http,sqlite}` |
| T7 | 3 | S3 | `GET /api/rag/documents` (listar el corpus) | `internal/rag/http` |
| T8 | 4 | S5, S7 | `ContextBlock` como unión y citas por clase (corpus con página, vault con ruta) | `features/send-message/model/prompt.ts` |
| T9 | 5 | S4 | Contrato de mensajes (`tools`, `role: "tool"`) y acumulación de `delta.tool_calls[]` por índice | `shared/llm` |
| T10 | 5 | S4, S5 | El bucle en `send()`: tope de 3, cancelación que corta, invariante de alcance | `features/send-message/model` |
| T11 | 5 | S4 | El tercer estado del frame y el flag `consulted` | `features/send-message/model` |
| T12 | 5 | S6 | Bloqueo temprano con adjunto del corpus y servidor caído, con su mensaje | `features/send-message/model` |
| T13 | 6 | S3, S7 | UI: elegir del corpus, elegir del vault, ver el estado y la insignia | `features/*/ui`, `pages/chat` |

**Riesgo por tarea.** Riesgo alto en T9 (el contrato del transport, que todo el chat usa) y
en T12 (el bloqueo cambia el fallo de un camino que ya existe): cada una va en su propio
commit para que el review nativo pueda tomarla sola, sin el resto de su fase.

## Fases (delivery strategy)

Cada fase es un commit revisable (~400 líneas de diff como techo), con sus tests y su
documentación al lado del comportamiento.

**Fase 1 — El chat deja de necesitar la recuperación.** T1, T2, T3. Es el objetivo original
del pedido: sin adjuntos, el chat es una llamada directa al modelo. Entrega S1 y S5.
*Tradeoff explícito:* hasta la fase 5 el chat no tiene camino de material, porque el material
llega por los adjuntos (fases 2–5). No es una pérdida: es S5 al pie de la letra —sin adjuntos
se chatea normal—, solo que los adjuntos todavía no existen.

**Fase 2 — El adjunto como dato.** T4, T5. Puro y aditivo: formato de nota, las dos clases y
el rechazo por tamaño. Testeable sin red ni LLM.

**Fase 3 — El servidor.** T6, T7. Aditivo y acotado; nada del plugin lo consume todavía, y no
rompe a los consumidores actuales de `GET /api/rag/search`.

**Fase 4 — Los frames con material.** T8. `prompt.ts` es puro y ya tiene tests colocados.

**Fase 5 — La herramienta.** T9, T10, T11, T12. La fase riesgosa: toca el transport SSE y el
flujo del chat, y es donde el servidor pasa a ser requisito de las respuestas que consultan
material del corpus.

**Fase 6 — La UI.** T13. Cierra la feature: sin ella los adjuntos existen pero no se pueden
elegir.

---

## Log

**L1 (2026-10-18, pedido original)** — "quiero que investigues si puedo configurar un apykey
y hablar con el llm sin que el servidor de go este corriendo".

**L2 (2026-10-18, dirección y encuadre)** — "Vamos disenarlo sin tocar codigo, quiero poder
chatear sin necesidad del servidor, del servidor solo puedo agregar recursos que esten en el
servidor, por ejemplo debe haber alguna manera de agregar recursos del servidor al chat y
cuando se referencien en el chat es cuando se van a pedir en el servidor, que te parece?".

**L3 (2026-10-18, la idea de la tool)** — "Dime que tal esta esta idea, se puede hacer en
turnos el mensaje se envia al llm con la referencia al adjunto, el llm lo lee y ejecuta una
tool que trae hace la busqueda correspondiente y trae los resultados, despues los resultados
se envian al llm otra vez y alli es donde el llm responde en el chat al usuario".

**L4 (2026-10-18, elección y la condición que cierra el diseño)** — "me gusta el
tool-calling, vamos por esa, pero no es necesario si no hay archivos adjuntos en el chat.
Entonces si no los hay solo se chatea normal". La tool condicionada a la existencia de
adjuntos es lo que vuelve alcanzable S1.

**L5 (2026-10-18, fallo y la segunda clase de adjunto)** — "bloquear mostrando un error.
Tambien tienes que tener en cuenta que se pueden enviar archivos adjuntos que esten dentro de
obsidian esos si se enviarian completos". Aparece la clase vault, y con ella una regla de
fallo distinta: un archivo del vault no puede fallar por el servidor.

**L6 (2026-10-18, política de tamaño)** — elección del usuario entre tres políticas para un
archivo que no entra en el contexto: "Tope con rechazo".

**L7 (2026-10-18, hallazgos de la investigación)** — lo que cambia el diseño, con evidencia:

1. **La recuperación es un paso obligatorio, no opcional.** `useSendMessage.ts:87-94` captura
   el fallo de `searchCorpus`, escribe el error y hace `return`: nunca llega a
   `streamChatCompletion`. Está fijado como **intencional** por `useSendMessage.test.ts:176`
   ("persists the user's turn before retrieval can fail"). No es un bug: es una decisión que
   quedó sin revisar.
2. **El transporte al LLM ya es directo y serverless.** `shared/llm/client.ts:77` toma
   `baseUrl` y `apiKey` de los ajustes. La arquitectura ya estaba decidida en
   `chat-workspaces.md` L3, con CORS medido para `app://obsidian.md`, `capacitor://localhost`
   y `localhost:5173`.
3. **No existe vínculo entre un chat y un documento.** El chat busca en *todo* el corpus
   (`useSendMessage.ts:90`) y el servidor no acepta alcance. No se puede decir hoy "este chat
   estudia este libro".
4. **Referenciar un recurso no reemplaza la búsqueda: la acota.** Un documento no entra en un
   contexto (`data/DDD.md` = 917.616 caracteres). La búsqueda sigue siendo necesaria, y
   mientras viva en el servidor, el material remoto sigue dependiendo de él.
5. **Tool-calling es ortogonal a "sin servidor".** Decide *quién* dispara la búsqueda, no
   *dónde* vive el material. Por eso S5 (la tool solo con adjuntos) es la condición que hace
   que S1 y S4 convivan.
6. **El servidor ya está diseñado para escribir el texto por documento** (`book.md`,
   `internal/rag/app/process.go:16,94`), y el Markdown generado trae `<!-- page N -->` (359
   marcadores en `data/DDD.md`), así que una copia local podría seguir citando página exacta.
   Pero el `book.md` **no está** en los artefactos actuales: en `data/artifacts/*/` solo hay
   `meta.json`, `payload` (el PDF) y `work/` vacío. Queda como deuda, no como bloqueo.
7. **`resource` no es un documento** (ver Contract). Se descartó reutilizarlo.

**L8 (2026-10-18, riesgos declarados y no resueltos)** —

- **Frescura.** Si el servidor re-ingiere un documento, cualquier copia local queda vieja en
  silencio. El hash del artefacto es el único juez. Hoy no hay copia local, así que no muerde;
  mordería en la fase de material local.
- **Doble motor.** Una búsqueda local sería léxica y la del servidor es híbrida: no devuelven
  lo mismo. El turno tendría que registrar con qué motor contestó o las citas se vuelven
  inexplicables al comparar.
- **Capacidad del modelo.** Si el modelo configurado no soporta `tools`, el chat con adjuntos
  del corpus falla. Hay que medirlo por modelo, no suponerlo.

**L9 (2026-10-18, fase 1 implementada y verificada)** — T1, T2 y T3 en el commit
«feat(send-message): stop calling the corpus before answering» de `feat/chat-adjuntos`. RED
observado antes de implementar: 10 de 42 tests fallaban entre `prompt.test.ts` y
`useSendMessage.test.ts`. GREEN: 271 de 271 en los 24 archivos, y `tsc -noEmit -skipLibCheck`
en 0. Verificación diferencial: `searchCorpus` sigue vivo e intacto para la página del corpus
(`features/search-corpus/model/useSearchCorpus.ts:24`), y `buildSystemMessage` conserva su
firma y sus tres tests, así que el frame de citas sigue siendo exactamente el de antes.

Dos superficies quedan inertes, y son deuda visible a propósito:

- **El campo «Fragmentos a recuperar (top K)»** de los ajustes globales, y el `top_k` por
  chat, ya no afectan a nada: su único consumidor era la recuperación del chat. Siguen
  mostrándose en la UI (`LlmSettingsForm.vue:85`, `ChatSettingsPanel.vue:121-122`). Recuperan
  efecto en la fase 5, cuando la tool busque.
- **`buildSources`** quedó exportada sin consumidores (`features/send-message/index.ts:8`),
  con sus tests. La fase 4 la vuelve a usar.

**L10 (2026-10-18, corrección al diseño al preparar la fase 2)** — los adjuntos iban al
frontmatter y no pueden: el parser es plano (`Record<string, string>`). Van a una región con
marcadores, igual que el prompt y el transcript (ver Contract). Se agrega además la forma de
la respuesta de `GET /api/rag/documents`, que estaba sin decidir y un worker habría inventado.

El commit de la fase 1 son 552 líneas autoradas, por encima del techo de ~400 de una unidad
de revisión: la mayor parte es este documento, que entra con la primera fase por ser su
especificación. Las fases 2 a 6 van una por commit, y las dos tareas de riesgo alto (T9 y
T12) van cada una en su propio commit dentro de su fase.
