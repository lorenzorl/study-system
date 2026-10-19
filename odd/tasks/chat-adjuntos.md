# Feature — Adjuntos en el chat: material del corpus por tool-calling, archivos del vault completos

**Status:** fases 1–6 entregadas y verificadas (T1–T13, 2026-10-18); quedan las tareas de seguimiento
anotadas en el Log (topes, README, frame de citas por clase, hallazgos F3/N3/N4)
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

Cada fase se entrega en commits de unidad de ~400 líneas de diff como techo, **uno por tarea
cuando la fase lo supera**, con sus tests y su documentación al lado del comportamiento. La
fase 3 se pasó de ese techo y es la última que se entrega como una sola unidad (ver L11).

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

**L11 (2026-10-18, fase 3 implementada, verificada y commiteada)** — T6 y T7 en el commit
«feat(rag): scope a search to one document and list the corpus».

Verificación independiente (`gentle-ai-verify`, read-only) sobre el diff todavía sin commitear:
T6 y T7 **satisfechos**, **cero blockers** y **sin regresión** en el camino sin filtro, por
tres razones que el verificador comprobó por su cuenta y no aceptó del autor:

1. El SQL sin alcance compuesto (`searchColumns` + `searchSQL`) es **idéntico token a token**
   al de la base `0ca3c2c`, comparado programáticamente.
2. Base contra worktree, con la misma matriz de consultas y límites sobre un corpus fijo:
   salida **byte a byte idéntica** en léxico e híbrido. Las únicas diferencias eran los UUID
   aleatorios de cada corrida, en las mismas posiciones.
3. El camino sin filtro es literalmente la llamada vieja: `SearchInDocument` con id vacío
deriva a `Search`, y el filtro semántico está guardado por `documentID != ""`.

Y comprobó que los tests **sí** cubren el SQL sin alcance, de forma conductual y no textual:
mutando la proyección, la dirección del `ORDER BY`, el `LIMIT` o el orden de los binds, los
tests fallan. Lo único sin cubrir es el desempate por `ordinal`, que ya era así antes.

Dos hallazgos se cerraron antes de commitear: el README normativo seguía documentando la
búsqueda como `(q, limit)` y sin ruta para el listado (S9 exige que la doc acompañe al
comportamiento), y faltaba el test que fija que `limit` acota el conjunto **filtrado en la
etapa semántica** — el léxico ya lo tenía, pero el semántico rankea en Go, y un filtro aplicado
después del truncado habría devuelto vacío. Ese test lo aportó el verificador, y es el que
prueba la propiedad de la que depende el diseño.

Sobre las dos decisiones que el autor tomó por su cuenta y reportó: filtrar la etapa semántica
en Go es correcto — `semanticRanking` ya cargaba todos los vectores, y el `continue` corre antes
del orden y del truncado —, y cambiar el tipo del parámetro exportado de `app.NewSearchUseCase`
a un puerto de la capa `app` es seguro porque `internal/rag/app` es `internal/` de un módulo
único: no puede haber llamadores fuera del repositorio, y dentro todos compilan.

Anotado y no resuelto: `pages` suma `job_pages` sobre **todos** los jobs del documento, lo que
hoy coincide con el conteo del endpoint de estado porque el ingest reusa el último job, pero
divergiría si un documento llegara a acumular dos. Y `ListDocumentsUseCase` hace 2N+1
consultas, aceptable al tamaño actual del corpus.

Control de tamaño: la fase 3 son ~1040 líneas autoradas, muy por encima del techo de ~400. La
mayor parte son cinco archivos de test nuevos, y T6 y T7 son dos comportamientos distintos que
debieron ir en dos commits. Se entrega como una unidad ya verificada y **no se reescribe la
historia**; de acá en adelante cada fase va en un commit por tarea cuando supere el techo.

**L12 (2026-10-18, review nativo del candidato: aprobado)** — linaje `review-d82fa77ea46bbe56`,
target `sha256:7a8ae5…`, riesgo **medium**, 4 lentes (risk, resilience, readability, reliability),
25 archivos y 1683 líneas, con presupuesto de corrección de 200. Los 4 reviewers corrieron (4/4,
relay del host Pi) y el review **aprobó**; la autoridad quedó quemada con
`gentle-ai.review-acknowledged/v1` y la entrega vuelve a la política ordinaria del repositorio.

El candidato revisado es **todo el diff de la rama** contra `master` (fases 1 y 3 más este
documento), no una unidad de commit: es lo que el proveedor acotó al abrir la autoridad, y es
más ancho de lo que esta estrategia de entrega pide.

Doce hallazgos, **todos no bloqueantes** (`disposition: informational`): ninguno abrió una
corrección, ninguno reabre el review, y el proveedor no ofrece transición de corrección para
este candidato. **Su texto completo no viene en el sobre de la facade**: se registran por id,
lente, ubicación y severidad, tal como llegaron, y su lectura completa queda pendiente para
triarlos.

`WARNING` — dos que ya estaban declarados acá y que el review confirmó por su cuenta:

- `R3-topk-inert` (reliability) y `R4-inert-topk` (resilience) — `useSendMessage.ts:87-89,96`.
  Es el `top_k` inerte de L9.
- `R4-latency-nplus1` (resilience) y `R3-corpus-list-2n-plus-1` (reliability) —
  `documents.go:63-89,68-77`. Es el 2N+1 de L11.

`WARNING` nuevo: `R4-count-fail-all` (`documents.go:70-77`) — si falla una consulta de conteo,
falla la lista entera. No es alcanzable por la API hoy; queda como trabajo posterior.

`SUGGESTION`: `R1-001` (`documents_handler.go:20-21`), `R2-001` (`documents.go:40`), `R2-002`
(`prompt.ts:30-37`), `R2-003` (`chunk_repo.go:53-72`), `R2-004` (este archivo, la línea del
`Status`), `R3-pages-count-across-jobs` (`page_repo.go:105-122`, el conteo de páginas por jobs de
L11) y `R4-unscoped-preserve` (`chunk_repo.go:192-196`).

Ninguno de los doce es trabajo de este candidato. El review es una vista externa adicional: no
reemplaza los chequeos funcionales, que ya se corrieron (271/271 en el plugin, 501 en Go), y no
autoriza entrega.

**L13 (2026-10-18, fase 2 implementada y verificada)** — T4 y T5, en dos commits: «feat(chat): give
a chat note a region for its attachments» y «feat(attach-resource): attach a vault file, or refuse
it for its size». RED: 15 de 57 fallaban. GREEN: 299 de 299 en 26 archivos (28 tests nuevos), `tsc`
en 0 y `architecture.test.ts` verde, que es lo que prueba que la slice nueva respeta las capas.

`ChatNote.attachments` quedó **obligatorio**, y eso hizo fallar `tsc` en un test fuera de la
superficie del writer. El writer paró y lo reportó en vez de tocar fuera de su superficie, que es lo
correcto; se arregló ese literal con una línea y el campo **no** se debilitó a opcional. El tipo hizo
su trabajo: encontró un llamador real que el cambio invalidaba.

Decisiones del writer, reportadas: el split usa el primer pipe para la clase y el segundo para la
etiqueta, así un `|` dentro de un título sobrevive; una línea rota se ignora y se pierde al
reescribir la región, igual que en las otras dos regiones, y está fijado por test; un adjunto del
vault ya existente que no se puede leer aporta 0 al total en vez de hacer fallar el adjuntado nuevo
(discutible, fijado por test); y `attachVaultFile` no deduplica rutas repetidas, así que adjuntar dos
veces el mismo archivo cuenta dos veces para el tope — la UI de la fase 6 probablemente lo evite, y si
no, es una línea.

**L14 (2026-10-18, corrección al Contract y una opción de orden)** — la unión se simplifica:
`{ kind: "vault"; path: string; text: string }`, sin `title`. La identidad de un archivo del vault
**es** su ruta, y así la forma coincide con `ChatAttachment`. El bloque de contexto de un archivo se
renderiza como `[n] <ruta>` y su cita como `Fuente: <ruta>`.

Y una consecuencia del diseño que conviene tener presente: **la mitad del vault no necesita la
tool**. Un archivo del vault se manda completo y se cita por ruta, así que S7 se puede entregar sin
tool-calling y sin servidor. Eso abre una opción de orden que se le plantea al usuario antes de
seguir, porque cambia qué se puede usar y cuándo.

**L15 (2026-10-18, fase 4 implementada y verificada)** — T8 en el commit «feat(send-message): carry
context from both attachment classes». Decisión previa del usuario: **A**, la costura primero, sin
cablear en `send()`. RED: 10 de 47 fallaban en `src/features/send-message`. GREEN: 304 de 304 en 26
archivos, `tsc` en 0.

Verificación diferencial contra la base: con entradas solo-corpus y vacías, las funciones exportadas
producen salida **byte a byte idéntica**; cero regresiones en el camino que ya existía. El caso mixto
(nuevo) rinde `["DDD, p. 42", "Notas/Tesis.md"]`, con deduplicación en orden entre las dos clases.

Decisiones del writer, reportadas: renombró los fixtures de test (`PASSAGE` → `CORPUS_BLOCK`) porque
el nombre viejo describía el tipo eliminado; el test que fija la unión vive en su propio `describe`,
porque afirma una propiedad de la forma y no de una función; y el fixture del vault es **a propósito
sin dígitos**, para que "no hay página en ningún lado" no pueda pasar por accidente.

Duda que el writer dejó anotada y que es real: el comentario de `EMPTY_CORPUS_MESSAGE` sigue hablando
solo de corpus y de "pasajes", y ahora un chat fundado puede llevar bloques del vault. No se tocó
para mantener la fase sin cambio de comportamiento; hay que revisarlo en la fase 5, que es cuando el
estado intermedio se cablea y ese mensaje puede alcanzarse con material del vault.

**L16 (2026-10-18, segundo review nativo: aprobado)** — linaje `review-3f8ccb22da3fa83f`, target
`sha256:51c25508…`, riesgo **medium**, 4 lentes, 36 archivos y 2566 líneas. Los 4 reviewers corrieron
(4/4) y el review **aprobó**; autoridad quemada con `gentle-ai.review-acknowledged/v1`.

Dos START fueron rechazados antes con `lenses-invalid` y `lineage_created: false` — ninguno creó
linaje ni quemó nada. La causa **no** era la lista de lentes ni su orden, que ya era el canónico: era
el largo del `lensesReason`. Uno de ~640 caracteres falla; el mismo contenido en ~250 pasa. Queda
anotado como restricción del proveedor.

Diecisiete hallazgos, **todos no bloqueantes** (`disposition: informational`); ninguno abre una
corrección y el proveedor no ofrece transición de corrección para este candidato. Igual que en L12, el
texto completo no viene en el sobre de la facade: se registran por id, lente, ubicación y severidad.

`WARNING`:

- `R3-topk-inert` y `R4-topk-inert` (`useSendMessage.ts:87-89,93-94`). El `top_k` inerte de L9, otra
  vez confirmado por dos lentes.
- `R4-corpus-list-2n-plus-1` (`documents.go:68-77`) y `R3-corpus-list-2n-plus-1`
  (`documents.go:68-86`). El 2N+1 de L11.
- `R4-corpus-list-fail-closed` y `R3-count-fail-aborts-list` (`documents.go:69-76`). El
  `R4-count-fail-all` de L12, con dos nombres.
- `R4-grounded-hardcoded-seam` (`useSendMessage.ts:93-94`). Es el `grounded: false` fijo que la fase 4
  dejó a propósito como costura; lo cablea la fase 5.

`SUGGESTION`, con dos nuevos que son accionables:

- **`R1-attachment-region-line-injection`** (`note.ts:266-269`). El más concreto de los nuevos:
  `renderAttachment` recorta los campos pero **no sanea saltos de línea ni el separador**, así que un
  título con un `\n` inyecta una línea extra en la región de adjuntos. Hay que sanear al renderizar.
- **`R1-corpus-list-keyless-posture`** (`router.go:25`). El listado nuevo queda sin key, igual que la
  búsqueda. Es una postura de seguridad a decidir, no un error.
- `R2-001` (`useSendMessage.ts:93-94`), `R2-002` (`documents.go:37-41`), `R2-003`
  (`chunk_repo.go:221-226`), `R2-004` (`search_handler.go:46-50`), `R2-005` (`prompt.ts:186`),
  `R3-pages-count-across-jobs` (`page_repo.go:111-116`), `R4-pages-count-across-jobs`
  (`page_repo.go:105-122`) y `R4-scoped-semantic-full-vector-read` (`search.go:215-220`).

Los dos reviews coinciden: la deuda ya declarada (el `top_k` inerte, el 2N+1, el conteo de páginas por
jobs) más un puñado de sugerencias. Ninguno encontró nada escondido, y el único hallazgo nuevo con
consecuencias reales es la inyección de línea en la región de adjuntos.

**L17 (2026-10-18, fase 5 / T9 implementada y verificada)** — commit «feat(llm): teach the transport
to carry tool calls». RED: 8 de 40 fallaban en `src/shared/llm`. GREEN: 315 de 315 en 26 archivos
(+11), `tsc` en 0. Verificación diferencial: con una request sin `tools` el **cuerpo es byte a byte
idéntico** al de la base, y el camino de contenido también; lo único nuevo es la clave `toolCalls`.

Decisiones del writer, reportadas: las formas de `ToolCall` y `ToolSpec` siguen la **forma del cable**
del proveedor (anidadas), para que el bucle de T10 pueda devolver el turno del assistant tal cual, sin
un paso de traducción; el fragmento se normaliza a `{index, id?, name?, arguments?}` y se descarta si
no trae `index` numérico, porque no se puede ubicar; el `index` se descarta en el resultado armado y el
orden se aplica ordenando, con un test que abre la segunda llamada antes que la primera para que el
orden quede fijado y no dependa de la inserción; y un `arguments` vacío se concatena como cadena vacía.

Dos cosas que T10 tiene que atender, anotadas por el writer: el mock de `streamChatCompletion` en
`useSendMessage.test.ts` devuelve `{content, deltas}` y hay que extenderlo cuando el store lea
`toolCalls`; y el turno del assistant con `tool_calls` tiene que devolverse **tal cual** vino, sin
traducir.

Y una decisión de diseño que queda resuelta acá y conviene dejar escrita: **la capacidad del modelo no
se adivina**. El usuario puede cambiar el modelo cuando quiera, así que no hay lista de modelos que
sirven; la capa manda `tools` cuando el llamador lo pide y reporta lo que el proveedor conteste,
incluido un rechazo. La detección es en tiempo de ejecución, y el mensaje claro para el usuario es
trabajo de T12.

**L18 (2026-10-18, tercer review nativo: aprobado)** — linaje `review-99fa717d052668de`, target
`sha256:0d86e862…`, riesgo **medium**, 4 lentes, 40 archivos y 2989 líneas. 4/4 reviewers,
**aprobado**, autoridad quemada. El START pasó **al primer intento** con la razón corta, lo que confirma
la restricción de L16.

Veinte hallazgos (9 `WARNING`, 11 `SUGGESTION`), todos no bloqueantes. Lo que hay que mirar es que **dos
subieron de severidad** respecto del review anterior:

- `R1-attachment-region-line-injection` (`note.ts:266-269`) pasó de `SUGGESTION` a **`WARNING`**. La
  inyección de línea en la región de adjuntos ya la habían marcado dos reviews; la tercera la subió.
- `R1-keyless-corpus-list` (`router.go:25`) también pasó a **`WARNING`**. El listado sin key.

El resto se concentra en la costura que la fase 5 todavía no cableó: `useSendMessage.ts:93-94` aparece
en **seis** hallazgos (`R2-inert-material-path`, `R3-001`, `R3-007`, `R4-003`, `R4-004`) por el
`grounded: false` y los `blocks: []` fijos, y `documents.go:68-78` en cuatro (`R3-002`, `R3-003`,
`R4-001`, `R4-002`). No es un defecto: es exactamente lo que T10-T12 vienen a conectar. Que cuatro
lentes señalen el mismo punto es la confirmación de que es la costura principal que queda abierta.

`WARNING`: `R2-inert-material-path`, `R3-001`, `R3-002`, `R4-001`, `R4-002`, `R4-003`, `R4-004`.
`SUGGESTION`: `R2-broad-chunk-port` (`documents.go:37-41`), `R2-redundant-docid-branch`
(`search_handler.go:46-50`), `R2-variadic-filter-binding` (`chunk_repo.go:221-226`), `R3-003`
(`documents.go:68-77`), `R3-004` (`search.go:215-220`), `R3-005` (`page_repo.go:111-116`), `R3-006`
(`note.ts:263-268`), `R3-007`, `R4-005` (`search.go:215-220`), `R4-006` (`page_repo.go:111-122`) y
`R4-007` (`shared/llm/client.ts:234-242`, el transporte nuevo).

**L19 (2026-10-18, la inyección arreglada y la fase 5 / T10 implementadas)** — dos commits:
«fix(chat): sanitise the attachment region so a field cannot inject a line» y «feat(send-message): let
the model search the material it was given».

**La inyección.** `toSingleLine` en `note.ts`: colapsa los saltos de línea a un espacio, saca los dos
marcadores de la región y deja el `|` en paz — el parser ya corta por el primer pipe para la clase y el
segundo para el id, así que sacarlo perdería datos al pedo. El parser **no** se hizo más estricto y la
nota sigue siendo editable a mano: se sanea lo que el plugin escribe. El RED mostró la corrupción real:
un título con `\n` producía dos líneas y el round trip dejaba de ser estable, y un campo con el marcador
de fin hacía que la región se cerrara antes y se truncara en la reescritura siguiente. GREEN 63/63 en
`src/entities/chat`, con la preservación de bytes intacta.

**T10.** `searchCorpus` gana el `document_id` opcional que el servidor ya aceptaba desde la fase 3 y el
cliente nunca mandaba. `grounded: boolean` pasa a `MaterialMode` de tres valores (`none`, `searchable`,
`in-message`) con un frame nuevo para el material buscable, porque el frame de citas dice que el
contexto viene a continuación y con la herramienta eso habría sido mentira. `tool.ts` es nuevo: la spec
que recibe el modelo, la decisión de alcance — un id que no está adjunto se rechaza con un mensaje que
el modelo puede leer y **nunca** se ejecuta — y la ejecución. Y el bucle: `tools` si y solo si hay
adjunto del corpus, tope de 3 búsquedas, la última pregunta sin `tools`, cancelar corta el bucle entero,
y el intercambio con la herramienta **no** se escribe en la nota.

RED 20 fallos; GREEN 84/84 en `src/features/send-message` y `src/entities/search-result`. Verificación
diferencial: con un chat sin adjunto del corpus, la URL de `searchCorpus` y los dos frames dan **1961
bytes idénticos** contra la base.

**Un error de proceso mío, y queda escrito.** Commiteé T10 con `tsc` **fallando** sin darme cuenta: mi
comando encadenaba `pnpm exec tsc && echo TSC_OK` bajo `set -e`, y en bash `set -e` **no** corta dentro
de una lista `&&`, así que el fallo quedó enmascarado y el commit siguió igual. El error real era un mock
de test declarado sin parámetros (`vi.fn(async () => …)`), cuyo tipo infiere una tupla vacía, y las tres
líneas que leían `calls[0][0]` no compilaban. Arreglado, y el commit corregido con `--amend`. Lección
para el resto de la feature: para gatillar con `set -e`, la verificación va como comando **suelto**,
nunca dentro de un `&&`.

**Lo que T10 deja anotado y no cablea:**

- **S6 no está servido todavía**: con adjunto del corpus y el servidor caído, el fallo ocurre en la
  primera búsqueda —con el error español y la pregunta a salvo—, no antes de la primera llamada al
  modelo. Es T12.
- **El hueco de S7 es peor que "no pasa nada"**: un chat con **solo** adjuntos del vault recibe el frame
  que dice *"este chat no tiene material adjunto"*, y eso es **falso** — el material está, no se manda.
  Va como tarea propia: enviar completo el texto del vault, con el lector inyectado como puerto.
- `consulted` y el aviso "sin consultar el material" son T11.
- El `top_k` que L9 declaró inerte **volvió a tener efecto** en el camino buscable: la búsqueda usa
  `topK ?? retrievalTopK` como límite. Eso salda parte de la deuda de L9.
- Si el proveedor sigue pidiendo herramientas después de la última pregunta, queda un turno de assistant
  vacío persistido. T11 o T13 pueden querer saltearlo.
- `EMPTY_CORPUS_MESSAGE` sigue hablando solo de "pasajes" (la duda de L15), sin tocar.

**L20 (2026-10-18, cuarto review nativo: rechazado por presupuesto, reabierto con alcance chico, y
APROBADO tras una corrección real)** — el ciclo más importante de la feature. Hay que leerlo entero.

**El candidato entero ya no entra.** El START con el diff acumulado de la rama (43 archivos, 4306
líneas) falló con `lens_context_budget_exceeded` en fase de preflight: **no se creó autoridad**, y el
proveedor dice que reintentar ese mismo candidato no puede funcionar. Es exactamente lo que L12, L16 y
L18 venían anotando —"el candidato es toda la rama, no una unidad"— y el proveedor terminó de
imponerlo. Se reabrió con un `baseRef` explícito al último commit revisado (`2c0f9ba`), y con eso el
candidato bajó a **11 archivos y 1393 líneas**. Linaje `review-563560dd564add3c`.

**El review encontró un bug real y serio, y el bug era mi handoff.** Tres lentes (risk, reliability,
resilience) marcaron el MISMO defecto con severidad **BLOCKER / CRITICAL**: una llamada a
`search_material` **sin `resource_id`** — la forma por defecto, la que el frame invita — se ejecutaba
**sin `document_id`**, o sea buscando en **todo el corpus** en vez de en lo adjunto. El invariante
central de la feature ("el alcance lo fija el chat, no el modelo") no regía en el camino por defecto:
adjuntar un documento no acotaba nada. El refuter confirmó los tres como causados por el candidato, y
el review pasó a `correction_required`.

**Y era mi culpa, no del worker.** Mi handoff decía textualmente "acotada cuando el modelo nombra un
documento, sin acotar cuando no". El worker implementó lo que le pedí. Lección: **una instrucción de
handoff es una especificación, y una especificación equivocada se implementa fielmente.**

**La corrección** (commit «fix(send-message): an unscoped search covers the chat's attachments, not the
corpus», 156 de las 160 líneas declaradas en el plan aceptado): la decisión lleva la **lista** de
documentos a buscar —el nombrado, o todos los adjuntos cuando no nombra nada—; un chat sin adjuntos
del corpus se rechaza de plano; y ejecutar corre **una búsqueda acotada por documento, en paralelo**,
con merge determinista por el score que ya traen los resultados y desempate por documento y chunk. El
test de regresión afirma **los ids que llegaron a `searchCorpus`**, no el texto devuelto, así que no
puede pasar por accidente.

**El validador dirigido confirmó el cierre** y el review quedó **aprobado**; autoridad quemada.

**Quince hallazgos no bloqueantes**, con tres que valen:

- **`R1-tool-sources-into-persisted-turns-unsanitised`** (`useSendMessage.ts:203-207`, WARNING): las
  fuentes que van al turno persistido **no se sanean**. Es la misma clase de defecto que se acaba de
  arreglar en la región de adjuntos, pero del lado de las líneas `Fuente:`. Un título de documento con
  un salto de línea podría inyectar en la nota. Va como tarea propia.
- **`R1-attachment-sanitiser-nested-marker-bypass`** (`note.ts:289-292`, WARNING): el saneador recién
  escrito tiene un bypass con un marcador anidado. Va como tarea propia.
- **`R3-vault-only-chat-gets-no-material-frame`** (`useSendMessage.ts:125`, WARNING): confirma por su
  cuenta el hueco de S7 que ya está anotado como tarea.

Y el resto son del bucle: el tope cuenta rondas y no búsquedas (`R3-cap-counts-rounds-not-searches`),
una búsqueda que falla se lleva el turno entero (`R4-single-search-failure-drops-turn`), quedan turnos
vacíos después del tope (`R4-empty-turn-after-cap`, `R3-final-ask-persists-empty-turn`), el mensaje de
rechazo de `tools` es demasiado amplio (`R3-tool-rejection-message-overbroad`,
`R4-first-request-400-blamed-on-tools`), y cancelar no aborta las búsquedas en vuelo
(`R4-cancel-does-not-abort-search`). `SUGGESTION`: `R1-tool-call-count-uncapped-per-round`,
`R4-uncapped-tool-calls-per-round`, `R2-duplicated-corpus-scope`, `R2-generic-system-message-name`,
`R2-status-check-named-as-tool-rejection`.

**L21 (2026-10-18, quinto review nativo: aprobado, riesgo bajo)** — el candidato sin revisar era **un
solo commit de documentación** (`7d63d51`, 52 líneas de este archivo), así que se acotó el rango con un
`baseRef` explícito a la corrección ya revisada (`766968a`) en lugar de reintentar el diff acumulado de
la rama, que ya se sabe que no entra en el presupuesto (L20).

Dos cosas que el proveedor enseñó acá y conviene no volver a aprender a la mala:

1. **Un candidato sin código es riesgo `low`**, y para riesgo bajo **la selección de lentes es
   inválida**: el START falla con `invalid_request` y dice que hay que omitirlas. Omitidas, el proveedor
   elige por el nivel.
2. Para riesgo bajo **no corre ningún revisor**: el START cerró en `approved` en el acto —cero lentes,
   cero corridas de modelo, sin refuter ni validador—. Un cambio de solo documentación no paga cuatro
   revisiones.

Es el ciclo más barato de la feature, y deja la lección de que el costo del review sigue al riesgo real
y no a la cantidad de archivos.

**L22 (2026-10-18, sexto review nativo: aprobado, riesgo bajo)** — otra vez un candidato de solo
documentación (`39d59a3`, 17 líneas de este archivo). Rango acotado con `baseRef` al commit ya revisado
(`7d63d51`) y **sin lentes**, que es lo que el proveedor exige a riesgo bajo: cerró en `approved` en el
acto, con cero lentes y cero corridas.

El patrón quedó confirmado dos veces: **un candidato sin código no paga revisores**, y el costo del
review sigue al riesgo y no a la cantidad de archivos ni de commits.

Una observación de proceso que vale: **cada commit de `docs(odd)` genera un candidato nuevo**, así que
cada review arrastra su propio ciclo de acuse. Es barato —cero corridas de modelo— pero es un ciclo. Si
molesta, los registros de review se pueden agrupar en un solo commit en vez de uno por ciclo.

**L23 (2026-10-18, séptimo review nativo: aprobado, riesgo bajo)** — otra vez un candidato de solo
documentación (`8a364c1`, 12 líneas de este archivo). Rango acotado, sin lentes, cerrado en `approved`
en el acto.

**Y acá se aplica el arreglo al ciclo que L22 anotó:** este registro **no se commitea solo**. Cada
commit de `docs(odd)` genera un candidato nuevo y por lo tanto otro ciclo de acuse, así que de acá en
adelante los registros de review se agrupan y viajan con el próximo cambio de código. Queda escrito en
el árbol hasta ese commit.

**L24 (2026-10-18, los tres arreglos de la opción A)** — dos commits: «fix(chat): sanitise every line the
note writes, and stop the marker from rebuilding itself» y «feat(send-message): send the vault's text,
not just its name».

**El bypass del saneador** (`R1-attachment-sanitiser-nested-marker-bypass`): borrar el marcador podía
**fabricarlo** — `<!-- adjuntos:<!-- adjuntos:end -->end -->` perdía el del medio y las dos mitades se
juntaban en el marcador mismo. Ahora la limpieza corre **hasta un punto fijo** (termina, porque cada paso
que cambia solo saca marcadores) y cubre **todos** los marcadores de región, no solo los de adjuntos.
Esa ampliación es una decisión del writer, deliberada: "un campo nunca lleva un marcador" es más simple
y más seguro que un juego de marcadores por región.

**Las citas sin sanear** (`R1-tool-sources-into-persisted-turns-unsanitised`): `renderSource` ahora pasa
por el mismo saneador, así que un título con salto de línea no puede inyectar una línea en la región del
chat. El parser **no** se tocó: sigue ignorando lo que no entiende, y la nota sigue editable a mano.

**El texto del vault** (`R3-vault-only-chat-gets-no-material-frame`): un puerto inyectado
(`vault-text.ts`) lee cada archivo al mandar, re-chequea los topes y **rechaza el envío con un mensaje
claro** si un archivo no se puede leer o ya no entra. El texto va completo como bloque `kind:"vault"` con
el frame de citas, y la cita es la ruta. El lector real se instala en `app/main.ts`, al lado de
`setObsidianApp`. El chat sin adjuntos manda **byte a byte** el mismo pedido que antes, verificado contra
la base.

RED→GREEN: 69/69 en `src/entities/chat`, y 109/109 en `src/features/send-message` más el guard de
arquitectura (que prueba que no se agregó ningún import cruzado entre slices).

**Tres cosas que quedan anotadas de este cambio:**

- **Los topes están duplicados** en `vault-text.ts`: FSD prohíbe importar `features/attach-resource` y
  `shared/` estaba fuera de las superficies. Un test los compara para detectar la deriva, pero cambiarlos
  sigue siendo un cambio de dos archivos. **Moverlos a `shared/` es la solución.**
- **El `README.md` de la slice quedó viejo**: sigue describiendo la recuperación como paso 3 y no menciona
  el puerto del vault ni los mensajes de rechazo. No se pudo tocar (fuera de superficie).
- **El frame de citas exige `Fuente: <título>, p. <página>`**, y una cita del vault no tiene página. El
  código cita bien (solo la ruta), pero el modelo podría agregar una página en su prosa. Esa línea del
  frame tendría que ramificar por clase.

**L25 (2026-10-18, octavo review nativo: aprobado)** — linaje `review-4b48b4fd9e792268`, medium, 9
archivos, 834 líneas. Los 4 reviewers corrieron y el review **aprobó**; autoridad quemada. **No encontró
nada grave**: los tres arreglos de L24 quedaron bien.

Seis hallazgos no bloqueantes. Los que valen:

- **`R4-refusal-retry-duplicates-persisted-question`** (`useSendMessage.ts:128-134`, WARNING): el rechazo
  ocurre **después** de persistir la pregunta, así que reintentar la duplica. Es real, y es la
  consecuencia de la decisión deliberada "la pregunta no se pierde".
- **`R4-unreadable-file-cause-dropped`** (`vault-text.ts:115-119`, WARNING): el mensaje de rechazo no dice
  **por qué** falló la lectura, así que no se distingue "no existe" de "no tengo permiso".
- **`R2-vault-caps-drift-guard-claim`** (`vault-text.test.ts:166-171`, WARNING): el test que dice detectar
  la deriva de los topes **no la detecta de verdad**. Refuerza la tarea de moverlos a `shared/`.
- `R3-001` (`note.ts:311`), `R3-002` (`useSendMessage.ts:143-144`) y `R3-003`
  (`vault-text.ts:115-119`), los tres de confiabilidad.

**L26 (2026-10-18, noveno review nativo: aprobado, riesgo bajo)** — otra vez un candidato de solo
documentación (`bb3209e`, 16 líneas de este archivo). Rango acotado a `bab950f`, sin lentes, cerrado en
`approved` en el acto.

El patrón ya es rutina y quedó escrito tres veces: **un candidato sin código no paga revisores.**

**Este registro queda SIN commitear a propósito**, para no generar un décimo candidato y su ciclo de
acuse. Viaja con el próximo cambio de código, que es la fase 6.

**L27 (2026-10-18, fase 6 / T13: la ruta de datos, en dos commits)** — «feat(chat): give the note an
attachment writer and the app a corpus list» (140 líneas) y «feat(attach-resource): attach and detach
what a chat brings into context» (~480). La unidad iba a ser una sola de ~590 líneas, por encima del
techo, así que se partió por capa y no por capricho: primero **las costuras** —lo que habla con la nota,
con el servidor y con el vault—, después **la interacción** que las usa. Es el mismo corte que L15 ya
había elegido para la fase 4.

**Lo que quedó construido.** `writeChatAttachments` es el adaptador que faltaba: la cirugía de la región
ya existía y estaba revisada desde T4, pero nadie la llamaba, así que los adjuntos solo se podían
escribir a mano. `listDocuments` lee `GET /api/rag/documents`, que el servidor expone desde T7 y ningún
cliente consumía. `listVaultMarkdownFiles` da los candidatos del vault: `listVaultFiles` lista los hijos
directos de una carpeta, que es correcto para la carpeta de chats e incorrecto para un selector que
tiene que cruzar carpetas. Y `useChatAttachments` es la interacción: listar, adjuntar, desadjuntar y
rechazar con un mensaje en español.

**Tres decisiones que valen.**

1. **La nota es la única fuente de verdad.** El store no guarda copia de la lista: `attachments` es un
   `computed` sobre `chats.note`, y `saveAttachments` refresca la nota **solo después** de que la
   escritura salió bien. Así una escritura fallida no deja la UI mostrando un adjunto que la nota no
   tiene, y una nota editada a mano no puede divergir de lo que muestra el panel.
2. **Dedupe por identidad**, `documentId` para el corpus y ruta para el vault. Cierra el punto que L13
   había dejado abierto: adjuntar dos veces el mismo archivo del vault gastaba el tope total dos veces
   por un solo archivo, porque `attachVaultFile` suma leyendo.
3. **El rechazo por tamaño llega al usuario tal cual lo escribió la política**, sin reescribirlo en el
   store. Una sola fuente para el mensaje de S8.

**Dos defectos que encontraron los chequeos, no yo.**

- El test nuevo del chat cerrado falló contra la primera implementación: `attachCorpus` **no** tenía el
  guard que sí tenían `attachVault` y `detach`. Sin él, adjuntar sin chat abierto escribía en la nada
  (la escritura salía por `currentPath`, que sobrevive al `note` nulo) en vez de decir que no hay chat.
- `architecture.test.ts` rechazó el primer test: usaba `await import("@/entities/chat/api/chat-note")`
  para espiar el adaptador, que es exactamente el import interno que el guard prohíbe. Reescrito con
  `vi.hoisted` + `vi.mock`, que además es la forma limpia. **El guard hizo su trabajo y no se debilitó.**

RED: el módulo no existía y la suite no colectaba; después, 1 de 13 con el pinia activo (el guard de
`attachCorpus`); después, 1 de 7 en el guard de arquitectura. GREEN: 26/26 en `features/attach-resource`,
**399/399 en 29 archivos** (+17 sobre los 382 de L25) y `tsc` en 0.

Verificación diferencial: **no aplica como corrida contra la base**, porque ningún cuerpo existente
cambió —el diff son altas más cuatro líneas de import y de exportación—. La evidencia es que los 382
tests previos pasan sin tocarlos y que `listVaultFiles`, `processVaultFile` y `setAttachments` quedaron
byte a byte iguales.

**Lo que este cambio deja anotado, para las tareas que siguen:**

- La UI del panel es el segundo commit de T13 y todavía no existe: hoy el store se puede usar desde una
  consola, no desde la pantalla.
- `loadCorpus` informa el fallo pero no bloquea nada: el chat con adjunto del corpus y el servidor caído
  sigue fallando en la primera búsqueda, que es T12.
- El panel no distingue un documento a medio procesar de uno listo; `pages` y `chunks` ya viajan en
  `DocumentSummary` para que lo pueda mostrar sin pedir el estado de cada uno.

**L28 (2026-10-18, fase 6 / T13: la UI, y con esto la fase cerrada)** — commit «feat(attach-resource): choose
the material from a panel above the composer». `AttachmentPanel.vue` es lo que deja de exigir editar la
nota a mano: la lista de lo adjunto con su clase y su etiqueta, el selector del corpus con el estado de
cada documento, el selector del vault con filtro por ruta, el botón de quitar por adjunto, y los motivos
en español cuando algo se rechaza.

**Dónde va, y por qué.** Arriba del compositor y no en el panel de ajustes: un adjunto es una propiedad
del **próximo mensaje**, no de la configuración del chat. Lo que hay que tener a la vista mientras se
escribe es qué material va a tener la pregunta, el panel de ajustes se colapsa y desaparece en hoja
angosta, y un adjunto olvidado es un fallo **silencioso** —la respuesta vuelve sin material y el usuario
no sabe por qué—. Además la insignia de T11 vive en el mismo lugar, así que la afirmación y su evidencia
quedan juntas en vez de repartidas en dos paneles.

**Dos decisiones de comportamiento.**

1. **El corpus se lista solo cuando se abren los selectores**, nunca al montar. Con el servidor apagado,
   un chat sin adjuntos del corpus no tiene por qué mostrar un error que nadie pidió: eso sería romper S1
   desde la UI. El listado del vault es local y no cuesta nada.
2. **La lógica que decide qué dicen los números vive en el modelo, no en el `.vue`.** `corpusStateLabel`
   (`model/corpus-state.ts`, 6 tests) traduce `pages`/`chunks` a lo que importa: un documento sin
   fragmentos **no puede** matchear una búsqueda, así que adjuntarlo todavía no cambia la respuesta, y eso
   se dice en vez de mostrar un conteo que parece sano. También cubre el singular y el caso "el listado no
   trae ese documento". Un `.vue` no se puede testear en este repo —`vitest.config.ts` no monta
   `@vitejs/plugin-vue` y no hay un solo test de componente—, así que **lo decidible se movió a donde sí
   se puede fijar con un test**, y el componente quedó de presentación.

**Evidencia.** RED observado en `corpus-state.test.ts` (módulo inexistente) antes de implementarlo.
GREEN: 405/405 en 30 archivos (+6), `tsc` en 0, y `node esbuild.config.mjs production` compila el SFC y
el texto del panel aparece en el bundle. **Lo que no tiene evidencia automática**: el render del panel en
sí —qué se ve al abrirlo, si el filtro del vault acota bien, si el botón de un adjunto ya puesto queda
deshabilitado—. Se verificó leyendo el template y compilando; montarlo en un test pide agregar
`@vitejs/plugin-vue` al config de vitest, que es infraestructura compartida y no es trabajo de esta tarea.
Queda anotado como deuda, no como hecho.

Control de tamaño: 552 líneas, de las que unas 250 son CSS del componente. Es una sola unidad —el
panel, la lógica que extraje de él y el test de esa lógica—, y no se parte: separar un componente de
sus estilos no le ahorra nada al revisor.

**T13 queda hecho**, y con él la fase 6.

**L29 (2026-10-18, candidato de la fase 6 / T13: `assess` dice que no corresponde review)** — el aviso de
RDD marcó un candidato sin revisar (target `sha256:9b2e5451…`) y ofrecía `review.start`. Corrí `assess`
sobre el rango acotado, que es lo que el contrato del proveedor pide antes de cualquier `inspect`:
**riesgo `medium` (nativo), 15 archivos, 1217 líneas, `candidate.consumed: false`,
`nativeReviewOutcome: unknown`, y `reviewDue: false` con razón `under_budget`**. El plan no escaló ningún
ítem de riesgo alto, así que tampoco pide verificador independiente.

**No se arrancó el review, y la razón es del proveedor, no mía.** La regla de entrada es textual: llamar
a `inspect` **sólo** cuando `reviewDue` sea verdadero (riesgo alto) o cuando el usuario pida
explícitamente un review de ese candidato; *"otherwise do not start one"*. El usuario describió el ciclo
de review como parte del trabajo —así que no dejó el target sin revisar a propósito— pero no pidió el
review de **este** candidato, y el propio proveedor determinó que no corresponde: riesgo medio y por
debajo del presupuesto. Arrancarlo igual habría sido pagar cuatro revisores por un candidato que la
política considera no debido, que es la lección de L21/L22 al revés: el costo sigue al riesgo real.

Queda anotado que el aviso de RDD sólo conoce dos disposiciones (lo dejó sin revisar / hay que revisarlo)
y ésta es una tercera: **hay un candidato sin revisar y el proveedor dice que no toca revisarlo**. Para
el próximo ciclo, la respuesta correcta a ese aviso es correr `assess` primero y dejar que `reviewDue`
decida, nunca `inspect` por reflejo.

Un pedido explícito del usuario lo reabre en el acto, acotado a `baseRef=bb3209e` con `committedOnly`.
El candidato queda `consumed: false`, así que la ausencia de review **no** lo marca como cerrado: es un
dato, no un cierre.

**L30 (2026-10-18, fase 5 / T11: el tercer estado del frame)** — commit «feat(send-message): record
whether the answer consulted the material». Delegado a un writer sobre el diseño ya decidido
(el handoff llevaba la superficie completa, incluidos los cinco archivos de test que el campo obligatorio
rompía, para no repetir el round trip de L13).

**Lo que quedó construido.** `ChatTurn.consulted` es **obligatorio**, y eso es deliberado: el compilador
nombra cada literal que tiene que decir en qué estado está, así que un llamador nuevo no puede dejar la
pregunta sin contestar. El invariante, escrito sobre el campo: `true` significa "no había nada que
consultar, o el material se leyó de verdad"; `false` significa "el chat tenía material detrás de la
herramienta y no se ejecutó ninguna búsqueda", y es el **único** valor que muestra la insignia.

**La nota registra sólo lo negativo**, con `<!-- material: sin consultar -->` como última línea del turno
del assistant. Un turno consultado, o uno de un chat sin material, **no escribe nada**: un marcador
ausente no afirma nada, así que una nota vieja o editada a mano nunca muestra la insignia. El parser saca
el marcador **antes** de las líneas `Fuente:`, que es lo que hace que un turno con las dos cosas —un
archivo del vault en el mensaje y un documento del corpus detrás de la herramienta— se lea bien.

`SEARCHABLE_MATERIAL_FRAME` gana una sola regla, y es la que faltaba: el frame decía qué hacer si el
material no cubre la pregunta, pero no qué hacer si el modelo directamente no buscó. La regla nombra la
omisión como una falta que hay que admitir, no como una opción que el frame conceda. Los otros dos frames
no se tocaron.

Y la búsqueda cuenta como ejecutada **sólo si `decideToolCall` aceptó la llamada y `runSearchTool` corrió**:
una llamada rechazada por alcance no leyó nada, así que no puede convertir una respuesta de memoria en una
fundada, y una búsqueda que revienta a mitad tampoco. El mismo booleano va al turno normal y a la respuesta
parcial interrumpida.

**RED** (del writer): 19 de 192 fallaban en 3 de 9 archivos, con los nombres de los tests nuevos y de los
que el campo obligatorio rompía. **GREEN**: 192/192 en los 9 archivos enfocados, `tsc` en 0.
**Verificación del padre sobre los dos cambios juntos**: **417/417 en 30 archivos**, `tsc` en 0, y
`node esbuild.config.mjs production` compila el SFC del hilo con la insignia adentro.

**Verificación diferencial** (la hizo el writer, contra la base en un directorio aparte): con notas de
sólo-usuario, con fuentes, con contenido multilínea y escritas a mano con blancos al final, la nota
renderizada y el parseo salen **byte a byte idénticos** a la base. Ningún turno consultado cambia de forma,
que es la propiedad que importa: el marcador sólo aparece en el estado nuevo.

**Dos ajustes míos sobre lo entregado**, chicos y explícitos:

1. La regla del frame quedó **después** del deber de buscar, no antes. Mi handoff pedía "inmediatamente
   después de la línea que nombra el material", y ahí la admisión se leía como un permiso previo. El
   deber va primero y la admisión es lo que queda cuando falla.
2. Agregué un test que fija la **colisión aceptada**: una respuesta cuya última línea sea exactamente el
   marcador pierde esa línea y se lee como no consultada. Es la misma clase que ya tiene este formato —el
   prefijo `Fuente: ` y los marcadores de región son canales de datos en una nota editable a mano—, lo que
   se pierde es un comentario HTML invisible en la vista de lectura, y el error va hacia avisar de más y
   no hacia callar. Fijarlo con un test lo vuelve una decisión y no un accidente.

**Control de tamaño**: 347 líneas en 9 archivos, dentro del techo. **Lo que no tiene evidencia
automática**: el render de la insignia, igual que en L28 —no hay runner de componentes—; el writer lo
verificó compilando el SFC y leyendo el template, y el bundle la contiene.

**Lo que T11 deja anotado:** la insignia es del turno, así que un chat que dejó de tener adjuntos del
corpus conserva las insignias viejas —es correcto, describe lo que pasó—; y la detección sigue siendo
determinista del lado del cliente, nunca preguntándole al modelo.

**L31 (2026-10-18, fase 5 / T12: el bloqueo temprano, y las dos correcciones que el verificador obligó)** —
un solo commit, porque T12 es riesgo alto declarado y su commit tiene que poder revisarse solo.

**Lo construido.** `preflight.ts` responde una sola pregunta —¿se puede llegar al corpus ahora?— y nada
más. La sonda es `listDocuments()`, que es el mismo contexto RAG que la tool lee: el backend no expone
endpoint de salud (verificado), y listar es la pregunta más barata cuya respuesta sólo existe con el
servidor arriba. Cualquier excepción bloquea, sin importar su tipo: la pregunta es "¿contesta?", y no hay
clase de error que merezca un caso especial. Nada se cachea entre envíos, porque un servidor que volvió
tiene que verse arriba en el envío siguiente. En `send()`, con `searchable`, la sonda corre **antes de
persistir la pregunta y antes de cualquier llamada al modelo**; un chat sin material del corpus no sondea
nunca el servidor (S1/S7).

**El verificador independiente: cero blockers y 8/8 afirmaciones sostenidas**, con mutación y no leyendo:
mover la sonda después de `appendTurn` hace fallar el test; `if (true)` en el guard hace fallar los dos
casos "nunca sondea"; cachear la sonda hace fallar cinco. La pregunta que importaba —¿hay algún camino de
`send()` a una llamada al modelo con corpus sin sonda?— la contestó leyendo el archivo entero:
`searchable` es la **misma** variable `const` que decide la sonda y que declara `tools`, y
`streamChatCompletion` tiene un solo llamador en todo el plugin. Confirmó además que el payload sin
adjuntos y vault-only sale **byte a byte idéntico**, con sonda propia además del test pineado.

**Y encontró dos defectos que eran de mi diseño, no del worker.**

- **F2, el serio.** El `AbortController` se creaba **después** de la sonda, así que `cancel()` era un
  no-op durante ella; con `shared/api/client.ts` sin timeout, un servidor que acepta la conexión y no
  contesta dejaba el compositor trabado en `sending` **para siempre**. T12 había metido un `await` de red
  delante del controller en cada envío con corpus: un modo de fallo nuevo. Ahora hay **un** controller,
  creado antes de la sonda y reusado por el bucle, y la señal llega hasta `fetch` (`listDocuments(signal?)`
  → `request` → `fetch`). Como el cliente compartido convierte **cualquier** fallo de `fetch` —un abort
  incluido— en su propio `NetworkError`, el valor lanzado no distingue una cancelación de un servidor
  muerto: el preflight decide por `signal.aborted` y relanza un `Error` con `name: "AbortError"`, que es
  la forma que `isAbort` ya reconoce. El verificador confirmó esa premisa leyendo el cliente y **no pudo
  construir ningún camino** donde una cancelación real se reporte como "el servidor no responde".
- **F1.** El compositor limpiaba el textarea al emitir, y como el bloqueo ahora ocurre antes de persistir,
  se comía la pregunta tecleada. No fue una decisión: se me pasó que el compositor se vacía sola, y es la
  misma clase de pérdida silenciosa que esta feature ya arregló tres veces. `MessageComposer` pasó a un
  `v-model` cuyo borrador es del padre y ya no se limpia solo; `ChatThread` limpia optimista y **devuelve
  la pregunta** cuando `send()` contesta que no se consumió.

**Re-verificación del delta: cero blockers, 7/7.** Mutación (a) controller devuelto después de la sonda →
fallan los dos tests de cancelación con el síntoma original de F2 (timeout); mutación (b) el camino
bloqueado devolviendo `true` → fallan tres. Mutación (c): borrar el "devolver el texto" en `ChatThread`
**no lo detecta ningún test** —no hay runner de componentes en este repo— y queda dicho así, no como un
pase.

**N1, encontrado en la re-verificación, corregido después.** `send()` prometía "la pregunta se consumió"
aunque `useChatsStore.appendTurn` **se tragaba** el fallo de escritura: el contrato decía algo que no podía
sostener. Ahora `appendTurn` contesta si la nota tomó el turno, y `send()` se detiene si no. Es un cambio
de contrato en `entities/chat` (de `Promise<void>` a `Promise<boolean>`, compatible con sus llamadores) y
trae el primer test de ese store, que no tenía ninguno. Verifiqué que muerde: el store mintiendo → falla su
test; el envío siguiendo tras una escritura fallida → falla el del envío. **Lo hice yo después del informe
del verificador, no un tercero**: es una corrección pequeña y testeada en cuatro lugares, y lo digo para
que se sepa cuál evidencia es independiente y cuál es mía.

**Un accidente de proceso, para no repetirlo.** Al mutar para comprobar que los tests de N1 muerden usé
`set -eo pipefail` y encadené la restauración después del test: el test **falló** —que era el punto— y
`set -e` abortó el script **antes de restaurar**, dejando las mutaciones en el árbol hasta que las vi.
Segunda vez que un idioma de shell me muerde en esta feature (la primera fue L19). La restauración va
**antes** del comando que puede fallar, o en un `trap`.

**RED/GREEN.** Worker: RED 2 fallos → GREEN 203/203 enfocados. Correcciones: RED 8 + 6 → GREEN 219/219.
Padre, sobre todo junto: **447/447 en 33 archivos** (+30 sobre los 417 de L30), `tsc` en 0 y
`node esbuild.config.mjs production` compilando los dos SFC. Diferencial: ningún test previo cambió de
expectativa, y el verificador midió el payload sin adjuntos byte a byte contra la base.

**Control de tamaño:** 406 líneas en archivos modificados más 469 en cuatro archivos nuevos — por encima
del techo de ~400. La mayor parte son tests (283 solo en `useSendMessage.test.ts`, 192 en `preflight.test.ts`),
y T12 no se parte: es riesgo alto y su commit tiene que tomarla entera.

**Lo que queda anotado y agrupado, no arreglado acá.** **F3** —un servidor viejo que responde `/search`
pero no `GET /api/rag/documents` bloquea con un diagnóstico falso: dice "no responde" cuando sí responde—
va con la tarea de L25 que arregla justo eso, un rechazo que no dice la causa real. **F4** —
`checkCorpusReachable([])` rendiría `«undefined»`— es inalcanzable desde `send()` y no se testea un defecto
inalcanzable. **F5** —el README de `send-message` ya es falso en dos frases más— es la tarea 23. **F6** —un
`GET /documents` extra por envío con corpus, sobre el 2N+1 de L11— queda como deuda declarada. **N2** —un
fallo de red que coincide con un cancel en el mismo tick se reporta como cancelación— va en la dirección
benigna y se deja. **N3** —un `send()` que rechace antes del `try` deja el borrador limpio— y **N4** —el
texto devuelto puede caer en otro chat si cambiaste de chat durante un bloqueo— se suman a las tareas
restantes.

**L32 (2026-10-18, tareas 22 y 27: un solo dueño para los topes, y el test que no detectaba la deriva)** —
commit «refactor(chat): give the attachment size policy one owner». La política de tamaño existía **dos
veces**: en `features/attach-resource` (la de verdad) y duplicada dentro de `features/send-message/model/vault-text.ts`,
porque una feature no puede importar a su hermana. Cambiar la política era un cambio de dos archivos, y el
test que decía detectar la deriva **no la detectaba** (L25, `R2-vault-caps-drift-guard-claim`).

**Dónde fueron, y por qué no a `shared/`.** La nota de las tareas restantes decía `shared/`, y me aparté con
evidencia del propio repositorio: `ARCHITECTURE.md` §1 define `shared` como *"reusable, **domain-free**
infrastructure"*, y un tope de tamaño para adjuntos del vault es política de dominio de S8 sobre
`ChatAttachment` — que `entities/chat` ya posee, y al que las dos features ya dependen. Así que el módulo y
su test viven en `entities/chat/lib/attachment-limits.ts`, exportados por la raíz de la slice. Ningún test
tuvo que cambiar de expectativa: el movimiento fue un `mv`, y lo verifiqué comparando los dos archivos
contra `HEAD` con `diff -q` — **byte a byte idénticos**. `git diff` los muestra como renames de 0 líneas.

**El test de deriva se borró**, no se arregló: cuidaba una duplicación que ya no existe. Con eso el total
bajó de 447 a **446** tests, y el número es el esperado, no una pérdida silenciosa.

**Un hueco que el worker encontró y vale más que el movimiento.** Ese test era **el único lugar que fijaba
los números literales**: los seis tests que se movieron usan las constantes de forma simbólica, así que
cambiar `80_000` por `8_000` pasaría igual. El documento, sin embargo, ya había decidido lo contrario —
*"el test fija la política, no la calibración"*—, así que no agregué el pin numérico: sería contradecir una
decisión escrita para tapar un agujero que se descubre en el primer uso real (las notas se rechazan, se ve
en el acto; no es un fallo silencioso). Queda anotado como trueque, con la salida a mano: si molesta, el
arreglo es un test que fije los dos valores junto a la política.

**La decisión no mecánica del cambio.** `useSendMessage.test.ts` mockea `@/entities/chat` con un factory
que ahora **esparce `importOriginal`** y sólo pisa `useChatsStore`. Con el factory pelado, las constantes
importadas quedaban `undefined` y **la re-comprobación de topes al enviar se desactivaba en silencio**: los
tests seguían verdes sin estar probando lo que decían. Es un riesgo de clase para cualquier mock de raíz de
slice que se escriba por reemplazo total, y queda escrito acá.

**Un defecto de proceso mío, otra vez de especificación.** Mi handoff pedía en el punto 6 "actualizá
cualquier test que importara los topes desde la ruta movida", y la lista de superficies permitidas no
incluía los dos archivos de test de `attach-resource`. El worker **paró y preguntó** en vez de ampliar su
alcance, que es lo correcto, y las dos líneas de import las terminé yo como padre. Es el mismo patrón que
L13 —el compilador le marcó un archivo fuera de superficie y paró— con una diferencia: esa vez me salvó el
compilador, ésta la contradicción era mía en el mismo handoff.

**Verificación.** `tsc` en 0 (antes de mi arreglo fallaba con dos `TS2307`, exactamente en esos dos
archivos), **446/446 en 33 archivos**, y `node esbuild.config.mjs production` compilando. Los seis tests de
la política pasan con los cuerpos sin tocar, que es la evidencia de que la política no cambió: mismos
valores, mismos mensajes en español, mismo comportamiento. Por eso `features/attach-resource` ya no reexporta los
topes (el único consumidor de su raíz es `ChatThread.vue`, que importa `AttachmentPanel`).

**Y se cerró el encabezado del documento**, que seguía diciendo *"fase 1 en curso"*: las fases 1–6 y las
tareas T1–T13 están entregadas y verificadas, y lo que queda son las tareas de seguimiento del Log.

**L33 (2026-10-18, tarea 23: el README de `send-message`, que ya era falso en dos frases)** — commit
«docs(send-message): describe what one send actually does now». El README es la norma de la slice, y estaba
describiendo el paso 3 como *"Retrieve"* con una llamada a `GET /api/rag/search` que ya no ocurre en `send()`,
y afirmando que un fallo deja el turno del usuario en la nota — cierto hasta T11, **falso desde T12**, porque
el bloqueo ocurre antes de persistir. Un documento normativo que dice lo contrario de lo que hace el código
es peor que no tener documento: alguien lo lee y se equivoca con confianza.

**Lo que ahora dice, y por qué cada cosa está ahí.** Los siete pasos reales, con la comprobación del corpus
en el segundo lugar y la lectura del vault en el cuarto (el orden importa: es el que decide qué se pierde
cuando algo falla). Una tabla de qué recibe el modelo según lo que el chat tenga adjunto —los cuatro casos,
con su frame, si declara `tools` y cuántos round trips cuesta—, porque el invariante central de la feature es
justamente ese acoplamiento y antes había que deducirlo leyendo tres archivos. El bucle de la herramienta con
su tope y su invariante de alcance. El tercer estado (`consulted`) con su marcador y su insignia. Y el
contrato de `send()` devolviendo si la pregunta se consumió, junto con el `v-model` del compositor: sin esas
dos cosas juntas no se entiende por qué el texto vuelve o no vuelve.

**Una frase que además cambió de significado, no sólo de redacción.** El README decía que si falla la
escritura al vault el envío continúa. Ahora hay que distinguir: una escritura fallida de la **pregunta**
detiene el envío (N1: sin eso, `send()` prometía "consumida" sobre una nota que no la tenía), y una escritura
fallida de la **respuesta** la pierde —llega a la pantalla, no a la nota—. Está dicho así, con las dos.

**N5, cerrado acá:** la firma de `send()` cambió a `Promise<boolean>` y el compositor ahora exige `v-model`;
los dos son superficie pública de la slice y nada lo documentaba.

Es sólo documentación: no toca código, no cambia comportamiento, y la suite no se vuelve a correr para
verificarla porque no hay nada que verificar. Lo que sí hice fue contrastar cada afirmación contra el código
antes de escribirla, que es el trabajo real de este commit.

**L34 (2026-10-18, tarea 24: el frame de citas nombra la forma de cada clase, y un hueco que encontré
leyéndolo)** — `CITATION_FRAME` exigía `Fuente: <título>, p. <página>` para todo. Un archivo del vault **no
tiene página**: su identidad es la ruta, y así lo renderiza `buildSources` desde T8. O sea que el frame pedía
un formato que, para esa clase, sólo se puede cumplir **inventando una página** — exactamente lo que su
propia tercera regla prohíbe.

Se arregló en el mismo frame, sin cuarta constante y sin cuarto `MaterialMode`: ese frame es el de "el
material está en frente del modelo", y el material en frente puede ser de cualquiera de las dos clases. La
regla ahora nombra las dos formas y dice cuál es cuál. `NO_MATERIAL_FRAME` sigue prohibiendo citas (ahora
dicho como "nada que citar", no "sin páginas") y `SEARCHABLE_MATERIAL_FRAME` sigue pidiendo título y página,
porque ahí el material sí es corpus.

**Dos cuidados que no eran obvios.** La regla sigue siendo **una sola** (el frame tiene 5 reglas y una de
citas: hay un test que lo fija), para que no se convierta en una lista de formatos. Y comprobé que
`CITATION_FRAME` **no queda muerta**: sigue alcanzable desde el modo `in-message`.

**Un caveat sobre el test, que prefiero decir que tapar.** El caso que ata cada forma a su clase lo hace con
aritmética de `indexOf` sobre la oración, o sea fija el **orden de la prosa**. Si alguien reescribe la regla
sin cambiar el significado, el test rompe aunque no haya regresión. Los otros tres casos del bloque son
semánticos y no tienen ese problema. No lo toqué porque pasa y porque afirma una propiedad verdadera, pero
queda anotado.

**Y el hallazgo, que vale más que el arreglo.** Al leer la regla nueva se me hizo evidente un hueco que **no
encontró ninguno de los nueve reviews nativos ni los dos verificadores independientes**: en un **chat mixto**
—un archivo del vault y un documento del corpus— el modo que se elige es `in-message`, así que el frame es
`CITATION_FRAME`, cuya primera regla dice *"Respondé la pregunta usando solamente el contexto que aparece a
continuación"*, **y al mismo tiempo `tools` sí se declara**. El frame le prohíbe al modelo exactamente lo que
la herramienta existe para hacer, y el panel de T13 permite adjuntar las dos clases al mismo chat.

No es un error de diff que un revisor pueda ver leyendo un cambio: es una **ausencia** —falta el frame para
"material en el mensaje **y** material buscable"— y los reviews miran diffs. Va como unidad propia, porque el
arreglo toca también `useSendMessage.ts` al elegir el modo. Es el tipo de cosa que justifica leer el código de
al lado en vez de sólo el diff propio.

**Verificación.** RED del worker: 2 fallos nombrados. GREEN: 62/62 en `prompt.test.ts` + `tool.test.ts`, `tsc`
en 0. Sobre todo junto con la unidad siguiente: **456/456 en 33 archivos**, `tsc` en 0 y el bundle compilando
los SFC. El commit del frame va solo, para que la corrección de formato se lea sin el reordenamiento al lado.

**L35 (2026-10-18, tarea 25 + F3 + N3 + N4: nada se escribe para un envío que no puede proceder)** — commit
«fix(send-message): read the vault before writing the question, and say what really failed». Tres hallazgos
con un mismo tema: los caminos de fallo de un envío.

**1. La lectura del vault pasa antes de persistir.** Un archivo ilegible rechazaba el envío **después** de
escribir la pregunta, así que `send()` contestaba "consumida" y reintentar duplicaba el turno (el hallazgo
`R4-refusal-retry-duplicates-persisted-question` de L25, que T12 sólo había saldado para el bloqueo). Ahora
el orden es **preflight → vault → persistir**, y el contrato se afina hasta poder decirse en una frase:
**`false` significa que no se persistió nada**, `true` que la pregunta llegó a la nota. Revisé cada retorno
contra esa frase. Todo lo que puede rechazar el envío corre antes de la escritura, que es exactamente el
motivo del reordenamiento.

**Un test se reescribió, y lo digo porque es un cambio de comportamiento, no un test que estaba mal.** El
caso "rechaza el envío y conserva la pregunta" afirmaba lo viejo: la pregunta quedaba en la nota y
`send()` contestaba `true`. Ahora fija lo nuevo: **cero** llamadas a `appendTurn`, `send()` contesta `false`,
el único colaborador tocado es la lectura, y el texto vuelve al compositor. Ese `expect` se fue, no se
aflojó.

**2. El mensaje del bloqueo deja de afirmar una causa que no conoce (F3).** Con un servidor viejo que no
tiene `GET /api/rag/documents`, o con un 500, el mensaje decía "el servidor del corpus no responde" — falso:
**sí respondió**. Ahora el branching decide primero por `signal.aborted` (cancelación), después por
`instanceof ApiError` (respondió, y se nombra el status), y al final el transporte, cuyo texto queda **byte a
byte igual** que antes, con sus tests intactos. Un test nuevo fija que decide la **clase** y no la propiedad:
un `Error` pelado con `status: 500` sigue siendo un fallo de transporte.

**3. El borrador, sus dos bordes.** Si `send()` rechaza —algo que se escape de su propio `try`— el texto
vuelve en vez de perderse (N3). Y el borrador se limpia al cambiar de chat, para que una pregunta devuelta no
viaje al chat siguiente (N4).

**Queda un residual, y lo digo yo antes de que lo diga un revisor:** cambiar de chat **mientras un envío está
en vuelo** todavía puede hacer caer el texto en el compositor del chat nuevo, porque el `watch` limpia al
cambiar y la devolución ocurre después del `await`. Cerrarlo pide borradores por chat, o comparar la ruta
capturada antes de devolver y decidir qué hacer con el texto cuando ya no coincide: es una decisión de
producto, no un detalle de implementación, y no la tomé.

**4. El README no se quedó mintiendo.** El reordenamiento volvía falsos los pasos 3 y 4 que acababa de
escribir en L33, así que el mismo commit los corrige, y el párrafo de `send()` ahora dice que `false` significa
que no se persistió nada.

**Verificación.** RED del worker: 5 fallos nombrados. GREEN enfocado: 235/235 en 12 archivos, `tsc` en 0.
Padre sobre las dos unidades juntas: **456/456 en 33 archivos**, `tsc` en 0 y el bundle compilando.

**Lo que esta verificación NO cubre, dicho sin maquillar.** `tsc -noEmit` **no** typechequea archivos `.vue`
—el paquete no tiene `vue-tsc`—, así que el cambio de `ChatThread.vue` queda fuera de lo que el typecheck
prueba; lo que sí lo cubre es el build de esbuild, que compila el SFC. Y no hay runner de componentes: N3 y N4
están verificados leyendo y por los 235 tests, ninguno de los cuales monta el widget.

**L36 (2026-10-18, el chat mixto tiene su frame: el hueco que no era un diff)** — commit «fix(send-message):
give a chat with both material classes its own frame». El hallazgo de L34, corregido.

**El defecto, otra vez para que se lea entero.** Un chat podía tener las dos clases adjuntas —una nota y un
libro— y el modo se elegía `blocks.length > 0 ? "in-message" : searchable ? "searchable" : "none"`, así que el
mixto caía en `in-message` y recibía `CITATION_FRAME`, cuya primera regla dice *"Respondé la pregunta usando
solamente el contexto que aparece a continuación"*, **mientras el mismo pedido declaraba `tools`**. El frame le
prohibía al modelo exactamente lo que la herramienta existe para hacer. La mitad del corpus de un chat mixto
estaba contradicha por sus propias instrucciones.

**La corrección.** `MaterialMode` gana `"in-message-and-searchable"` y su frame,
`IN_MESSAGE_AND_SEARCHABLE_FRAME`, que dice las dos cosas: hay material en el mensaje **y** material que no
está, buscá ese otro con la herramienta antes de responder, y citá cada forma como corresponde. Las reglas
que se podían compartir se copiaron **verbatim** y verificado con comparación de strings, no a ojo: la regla
de citas es la de `CITATION_FRAME` carácter por carácter, y la de admisión —"si no llegaste a buscar…"— es la
de `SEARCHABLE_MATERIAL_FRAME`, para que la insignia de `consulted` no pueda derivar de la instrucción que la
produce. `NO_MATERIAL_FRAME` y `SEARCHABLE_MATERIAL_FRAME` no se tocaron.

**La trampa, que era el riesgo real, se arregló en un solo lugar.** `buildUserMessage` devolvía la pregunta
pelada salvo que el modo fuera exactamente `"in-message"`, así que un cuarto modo sin tocarlo habría hecho
**desaparecer el texto del vault del pedido con toda la suite en verde** —el chat habría contestado como si la
nota no estuviera adjunta—. En vez de agregar un segundo `mode === …` en el call site, se extrajo
`carriesBlocksInMessage(mode)`, que es donde un modo futuro podría volver a perder los bloques. El RED del
worker lo muestra literalmente: `expected '¿Qué es el estado?' to be '[1] Notas/Tesis.md …'`.

**Verificación.** RED: 12 fallos nombrados (8 del frame, 2 del builder, 2 de `buildMessages`, y el del pedido
mixto). GREEN enfocado: 248/248 en 12 archivos, `tsc` en 0. Padre sobre todo junto: **469/469 en 33
archivos**, `tsc` en 0, y el bundle compilando. Los pares de no-regresión pasan sin cambios: un chat
vault-only sigue recibiendo `CITATION_FRAME` y **sin** `tools`, uno corpus-only sigue recibiendo
`SEARCHABLE_MATERIAL_FRAME` con la pregunta pelada, y el chat sin adjuntos sigue mandando **byte a byte** el
mismo pedido.

**Dos cosas quedan dichas y no arregladas.** `buildUserMessage` sigue rindiendo
`EMPTY_CORPUS_MESSAGE` cuando un modo con bloques recibe cero bloques; es inalcanzable desde `useSendMessage`
—el modo se deriva de `blocks.length > 0`— pero un llamador futuro de `buildMessages` con el modo mixto y sin
bloques leería una línea engañosa. Y el frame es instrucción: un chat mixto sigue pagando el preflight más uno
o dos round trips aunque la nota sola alcance para responder, y a un modelo que ignore la regla 2 no se lo
puede forzar a buscar. Los dos son trueques ya aceptados por la feature, no nuevos.

**Corregí además una frase que este cambio volvió falsa**: el README decía "the three frames" en "Where the
pieces are". Ahora dice cuatro, una por modo de material. Es el mismo patrón que vengo aplicando: un cambio de
comportamiento arrastra la documentación que describe, o el documento empieza a mentir por acumulación.

**L37 (2026-10-18, el último hallazgo abierto de L25: el rechazo ahora dice por qué)** — commit
«fix(send-message): carry the vault's own reason into the rejection». Cierra
`R4-unreadable-file-cause-dropped`.

**El defecto.** Cuando un archivo del vault no se podía leer, el mensaje decía siempre lo mismo: *"No se pudo
leer «X» del vault, así que el mensaje no se envió. Verificá que la nota exista y volvé a intentar"*. O sea que
**tres problemas distintos** —la nota se movió, no tenés permiso para leerla, el vault falló— producían **una
sola frase**, y la frase afirmaba la primera causa. El `catch {}` se tragaba el motivo, así que el usuario
tenía que adivinar cuál de los tres le tocó.

Es exactamente la misma clase que **F3** del verificador, que arreglé ayer del otro lado del envío: el bloqueo
del corpus decía "no responde" cuando el servidor sí respondía. Los dos eran mensajes que afirmaban una causa
que no conocían. Ahora los dos dicen la verdad que tienen: el bloqueo nombra el status HTTP, el rechazo del
vault cita el motivo que dio el vault.

**La corrección, y qué NO hace.** Se pasa el valor lanzado al mensaje y se incluye su texto **cuando es un
`Error` con texto propio**; cuando no lo es —un string, un error vacío— no se inventa nada y el mensaje nombra
las **dos** causas que quedan (que exista y que tengas permiso), en vez de terminar en un callejón. Decide la
**forma del valor**, no una comparación de textos: no hay `includes("permiso")` en ningún lado, que sería
adivinar el idioma y el proveedor del error.

**Verificación.** RED: 2 fallos nombrados, con el texto viejo en el mensaje de la aserción (`expected 'No se
pudo leer "Notas/Tesis.md" del …' to contain 'No se encontró la nota'`). GREEN: 156/156 enfocados en
`send-message`, **471/471 en 33 archivos**, `tsc` en 0 y el bundle compilando. El test previo que fijaba
"Verificá que la nota exista" y "volvé a intentar" sigue verde sin tocarse: la frase que el usuario ya conocía
no desapareció, se completó.

**Con esto no queda ningún hallazgo abierto de los nueve reviews nativos.** Lo que sigue anotado es deuda
**aceptada y declarada**, no trabajo pendiente: el `top_k` que recuperó efecto en el camino buscable, el 2N+1
del listado del corpus, el conteo de páginas por jobs, la colisión del contenido con los marcadores de la nota
(fijada por test), el GET extra por envío con corpus, el residual de N4 con el borrador al cambiar de chat
durante un envío, y la ausencia de runner de componentes —que es lo único que deja sin cobertura automática el
render del panel y el comportamiento del compositor.

**L38 (2026-10-18, corrección al cierre de L37)** — L37 cierra diciendo que "no queda ningún hallazgo
abierto de los nueve reviews nativos". **Es más de lo que puedo sostener**, y lo corrijo acá en vez de editarlo
allá, para que el registro muestre la afirmación y su corrección en vez de tapar la primera.

Hay **tres hallazgos del octavo review (L25) cuyo texto nunca llegó**: el sobre de la facade trae id, lente,
ubicación y severidad, no el cuerpo. Son `R3-001` (`note.ts:311`), `R3-002` (`useSendMessage.ts:143-144`) y
`R3-003` (`vault-text.ts:115-119`), los tres de confiabilidad. Lo que sí hice fue arreglar causas **candidatas**
en esas tres zonas exactas —el saneador de la nota en L24, el camino de `send()` en T12 y N1, el rechazo del
vault en L37—, pero **no puedo declarar cerrado un hallazgo que nunca leí**. Quedan como **no verificados**, no
como cerrados.

La diferencia importa: "cerrado" dice que alguien miró el defecto y comprobó que ya no está. "No verificado"
dice que hay una zona del código que un tercero marcó y que yo toqué por otras razones. Lo segundo es lo que
pasó.

Para cerrarlos de verdad hace falta el texto, y eso lo tiene el proveedor: si alguien quiere, se puede reabrir
un review acotado a esas tres ubicaciones, o pedir los cuerpos de esos tres hallazgos en lugar de un review
nuevo. Es la salida barata, y no la puedo tomar yo.

**L39 (2026-10-18, la feature entra en `master`)** — a pedido explícito del usuario, `feat/chat-adjuntos` se
integró a `master` con **fast-forward** (`f9fa3b4..9053bc8`): 65 archivos, +8467/−254, sin merge commit. Se usó
`--ff-only` a propósito: si `master` hubiera tenido trabajo propio, el comando fallaba y la decisión de cómo
integrar volvía al usuario, en vez de inventar un commit de merge por mi cuenta.

**Estado verificado sobre `master`, no sobre la rama**: `tsc` en 0, **471/471** en 33 archivos, el bundle
compilando los SFC y **todos los paquetes de Go en verde**. Es la misma foto que la rama porque el
fast-forward no inventa contenido, pero medido en la rama que importa.

**Lo que NO pasó, y conviene que esté escrito:** `master` quedó **35 commits adelante de `origin/master`** y
**nada se empujó**. Publicar sigue siendo decisión del usuario, igual que antes del merge. La rama
`feat/chat-adjuntos` sigue existiendo en `9053bc8`, así que el trabajo se puede revisar o revertir por rama
hasta que se decida publicar.

Con esto el documento de la feature queda cerrado: las nueve specs, las tareas T1–T13, las fases 1–6 y los
seguimientos del Log. Lo que sigue anotado es deuda declarada, y los tres hallazgos de L38 que esperan texto
del proveedor para poder verificarse.
