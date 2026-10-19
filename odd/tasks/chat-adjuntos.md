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
