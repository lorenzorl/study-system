# Follow-up — los 30 hallazgos advisory de los reviews de `chat-workspaces`

**Status:** in progress
**Created:** 2026-10-18
**Branch:** `fix/review-findings` (por delante de `master` en `8d9c292`, el release 2.4.0)
**Origen:** los cuatro reviews nativos de `odd/tasks/chat-workspaces.md` (L7-L10).

Los 30 hallazgos son **advisory**: ninguno abrió una corrección, ninguno bloqueó el
release. Este documento existe para que no se pierdan y para decidir, uno por uno, qué
merece un cambio y qué no.

## Método (y su límite, dicho de frente)

El proveedor emitió de cada hallazgo el **id, la lente, la severidad y la ubicación**, pero
no el texto de la afirmación: los resultados admitidos viven en el store del proveedor, y en
`.git/gentle-ai/` sólo quedan los payloads **refusados**. Así que cada fila se reconstruye
leyendo el código en la ubicación congelada.

Dos consecuencias que hay que tener presentes al leer la tabla:

1. **La reconstrucción es mía, no la palabra del reviewer.** Donde digo "qué es" estoy
   interpretando la ubicación con la lente y la severidad que el proveedor sí emitió.
2. **Las líneas son del candidato que se revisó, no del código de hoy.** Los slices B y C2
   se revisaron antes y después del arreglo del clobber (`8d0f51b`) y de la corrección del
   slice A (`c2db830`), así que un hallazgo puede apuntar a código que ya cambió. Cada fila
   se juzga contra el estado **entregado**, no contra el candidato.

## Inventario

### Slice A — `apps/obsidian-plugin/src/shared/config/settings.ts` y la credencial

| ID | Lente | Sev | Ubicación (candidato) | Qué es (reconstruido) |
| --- | --- | --- | --- | --- |
| `R1-data-json-round-trip-drops-unknown-keys` | risk | SUGGESTION | `shared/config/settings.ts:66-71` | `normalizeSettings` descarta toda clave que no conoce: si otra versión del plugin (o el usuario) escribió algo en `data.json`, se pierde al guardar |
| `R1-llm-api-key-plaintext-storage` | risk | WARNING | `shared/config/settings.ts:65-72` | La API key se guarda en claro en `data.json`, que vive dentro del vault |
| `R1-search-full-text-exposure` | risk | WARNING | `rag/http/dto.go:27-30` | El campo `text` que agregamos expone el chunk completo a cualquier cliente del endpoint |
| `R3-data-json-whole-file-replace` | reliability | SUGGESTION | `shared/config/settings.ts:66-78` | Cada guardado reescribe el archivo entero de configuración |
| `R3-hybrid-stage-text-unproved` | reliability | WARNING | `rag/app/search_test.go:77-101` | Que un resultado **híbrido** lleve el `text` del chunk no lo fija ningún test |
| `R3-onload-settings-failure-path` | reliability | WARNING | `app/settings-store.ts:29-35` | Si `loadData` o el guardado fallan durante `onload`, no hay camino probado |

### Slice B — el cliente del modelo, la frontera del vault y la nota

| ID | Lente | Sev | Ubicación (candidato) | Qué es (reconstruido) |
| --- | --- | --- | --- | --- |
| `R1-2` | risk | WARNING | `shared/llm/client.ts:185-192` | El camino de error del cliente: qué se hace con el cuerpo del proveedor |
| `R1-3` | risk | SUGGESTION | `shared/obsidian/useObsidian.ts:99-104` | `writeVaultFile` sobrescribe sin preguntar |
| `R3-2` | reliability | WARNING | `shared/llm/client.ts:189-191` | Mismo tramo que `R1-2`, mirado por cobertura |
| `R3-3` | reliability | WARNING | `entities/chat/lib/note.ts:69-70` | `appendChatTurn` cuando falta la región de chat |
| `R3-4` | reliability | WARNING | `entities/chat/lib/note.ts:215-218` | Parseo del turno: qué pasa con un contenido que se parece a un encabezado de rol |
| `R3-5` | reliability | WARNING | `shared/obsidian/useObsidian.ts:139-141` | `onVaultFileChange`: alcance del filtro por carpeta |
| `R4-create-orphans-file` | resilience | WARNING | `entities/chat/model/useChatsStore.ts:60-68` | Si el chat no se puede abrir después de crearlo, queda el archivo huérfano |
| `R4-degraded-list-silent` | resilience | SUGGESTION | `shared/obsidian/useObsidian.ts:126-133` | Una lista degradada no se distingue de una carpeta vacía |
| `R4-partial-content-dropped` | resilience | WARNING | `shared/llm/client.ts:196-211` | Si el stream se corta a mitad, el texto ya recibido se descarta |

### Slice C1 — la orquestación del envío

| ID | Lente | Sev | Ubicación (candidato) | Qué es (reconstruido) |
| --- | --- | --- | --- | --- |
| `R1-error-passthrough` | risk | WARNING | `features/send-message/model/useSendMessage.ts:144-146` | El mensaje del proveedor se muestra tal cual, sin acotar |
| `R3-1` | reliability | WARNING | `useSendMessage.ts:34-57` | Cobertura del orden de los pasos del envío |
| `R3-2` | reliability | WARNING | `odd/tasks/chat-workspaces.md:78-83` | El contrato del feature doc promete más de lo que el código hace |
| `R3-3` | reliability | SUGGESTION | `useSendMessage.ts:40-63` | Cobertura del camino sin chat abierto / sin key |
| `R4-001` | resilience | WARNING | `useSendMessage.ts:35-60` | Qué queda en la nota si la recuperación falla |
| `R4-002` | resilience | WARNING | `useSendMessage.ts:126-128` | Cancelación: qué se limpia y qué no |
| `R4-003` | resilience | WARNING | `useSendMessage.ts:56-58` | Reintento doble: qué pasa si se envía dos veces |
| `R4-004` | resilience | SUGGESTION | `features/send-message/model/prompt.ts:63-70` | `trimHistory` con historial patológico |

### Slice C2 — la UI, el retiro de slices y el arreglo del clobber

| ID | Lente | Sev | Ubicación (candidato) | Qué es (reconstruido) |
| --- | --- | --- | --- | --- |
| `R2-create-chat-duplicates-starting-default` | readability | SUGGESTION | `features/create-chat/model/useCreateChat.ts:23-27` | El prompt inicial por defecto se duplica entre la feature y la configuración |
| `R2-merge-doc-overstates-preservation` | readability | WARNING | `entities/chat/lib/note.ts:141` | El comentario de `mergeIntoExistingNote` promete más preservación de la que ese estado del código daba |
| `R2-saved-badge-contradicts-dirty` | readability | SUGGESTION | `features/edit-chat-prompt/ui/ChatPromptForm.vue:59-67` | El cartel "Guardado" puede quedar visible con cambios sin guardar |
| `R3-001` | reliability | WARNING | `entities/chat/api/chat-note.ts:52-59` | `createChatForTitle` entre el listado y la escritura |
| `R3-002` | reliability | WARNING | `entities/chat/lib/note.ts:151-167` | Regiones duplicadas o marcadores invertidos |
| `R3-003` | reliability | SUGGESTION | `features/send-message/ui/MessageComposer.vue:27-33` | Enter vs Shift+Enter sin test |
| `R3-004` | reliability | SUGGESTION | `widgets/chat-thread/ui/ChatThread.vue:12-27` | El hilo vivo puede duplicar el turno persistido |

## Triage

Tres destinos posibles, y ninguno es "ignorar":

- **ARREGLAR** — es un defecto con consecuencia observable y el arreglo no cambia el diseño.
- **DECISIÓN** — es una consecuencia de algo que **el usuario** eligió (o un cambio de producto).
  No lo toco: lo llevo con la pregunta y el costo, para que se decida.
- **CERRAR** — ya no aplica (lo arregló un commit posterior), o es cobertura que no cambia
  comportamiento (se cierra dejando escrito por qué, no en silencio).

Una regla que me impongo para no inflar el cambio: **un hallazgo de cobertura se cierra con un
test sólo cuando el comportamiento no probado tiene consecuencia para el usuario**; si no, se
cierra con la razón escrita.

## Veredictos

Cada fila se juzgó contra el estado entregado (`8d9c292`), no contra el candidato.
«Consecuencia» es lo que el usuario ve hoy si el defecto se materializa; «—» significa que no
hay consecuencia observable.

| ID | Veredicto | Consecuencia para el usuario | Qué cambió o por qué no |
| --- | --- | --- | --- |
| `R1-data-json-round-trip-drops-unknown-keys` | DECISIÓN | Al guardar un ajuste se pierden las claves que escribió otra versión del plugin | Conservarlas exige que el puerto `SettingsStore` cargue el registro crudo: cambio de interfaz, y resucitar claves viejas es lo que `R3-legacy-api-base-resurrection` ya advirtió |
| `R1-llm-api-key-plaintext-storage` | DECISIÓN (fuera de alcance) | Quien lea el vault lee la key | Elección del usuario; la mitigación es el límite de gasto del proveedor. No se toca |
| `R1-search-full-text-exposure` | DECISIÓN (fuera de alcance) | Cualquier cliente del endpoint lee el chunk entero | Es el campo que el chat necesita (contrato, L4). No se toca |
| `R3-data-json-whole-file-replace` | CERRAR | — | `saveData` es la única escritura que ofrece Obsidian; no hay escritura parcial. El riesgo concreto que representa es el de las claves ajenas, que se lleva aparte |
| `R3-hybrid-stage-text-unproved` | CERRAR (con test) | Si `text` no viajara en un híbrido, el modelo respondería sin contexto | El comportamiento ya es correcto (`fuseResults` arma el híbrido desde el resultado léxico, que trae `Text`); se fija con una aserción en `TestSearchUseCase_ChunkFoundByBothStagesIsHybrid`. No es RED: no hay defecto |
| `R3-onload-settings-failure-path` | DECISIÓN | Si `loadData`/`saveData` rechaza, `onload` rechaza y el plugin no carga | Qué debe pasar (seguir con defaults o fallar fuerte) es producto; el costo es un `try/catch` en `loadPluginSettings` y decidir si se avisa |
| `R1-2` | CERRAR | — | El cuerpo se lee acotado (4000 chars) y se muestra como `error.message` en una línea, 300 chars (`errors.ts:39-42`, `4a3cbd1`); los tests ya fijan que la key no aparece |
| `R1-3` | CERRAR | — | `writeVaultFile` no tiene llamador en el código entregado; toda escritura de chat pasa por `processVaultFile` |
| `R3-2` (B) | CERRAR | — | Mismo tramo que `R1-2`, mirado por cobertura: lo no probado es el tope de lectura y el cuerpo nulo, y una regresión ahí sólo alarga un mensaje |
| `R3-3` (B) | CERRAR | — | La región ausente ya se agrega al final y está probada (`note.test.ts`, desde `e4f32b3`) |
| `R3-4` (B) | DECISIÓN | Un turno cuya línea empiece con `### 🧑` se parte en dos al releer: el historial que va al modelo queda falseado | Verificado con sonda: `appendChatTurn` + `parseChatNote` devuelven 2 turnos. Cualquier arreglo es un escape en el formato de la nota (render + parse), o sea cambio de contrato |
| `R3-5` (B) | CERRAR | — | El filtro es más ancho de lo necesario (también dispara con subcarpetas); el único efecto es una recarga extra de la lista |
| `R4-create-orphans-file` | DECISIÓN | La nota queda creada pero no seleccionada, y el mensaje dice que no se pudo crear | Borrarla es destructivo y el plugin no tiene camino de borrado; decir otra cosa es cambiar el mensaje de producto |
| `R4-degraded-list-silent` | CERRAR | — | Inalcanzable con el wiring entregado: `main.ts:14` instala la app antes de que exista una vista, y `onunload` la limpia cuando ya no hay UI |
| `R4-partial-content-dropped` | DECISIÓN | Si el stream se corta, el texto ya recibido se descarta: ni en la nota ni en pantalla | Qué hacer con media respuesta es producto (guardarla, mostrarla, preguntar). No se toca |
| `R1-error-passthrough` | CERRAR | — | El mensaje del proveedor ya viene acotado (`toSingleLine`, 300 chars, `4a3cbd1`) y se lee sólo `error.message` |
| `R3-1` (C1) | CERRAR | — | El orden está probado: el test afirma la secuencia exacta `append:user, search, stream, append:assistant` |
| `R3-2` (C1) | CERRAR (informe) | El contrato promete que el usuario ve la respuesta en pantalla aunque falle la escritura | El contrato sobrepromete; la discrepancia exacta está abajo y la decide el padre. No se edita el feature doc |
| `R3-3` (C1) | CERRAR | — | Los dos caminos tienen test: falta la key y falta el chat abierto |
| `R4-001` | CERRAR | — | Probado: la pregunta queda persistida y no se anexa turno del asistente si la recuperación o el stream fallan |
| `R4-002` | DECISIÓN | Al cancelar, la respuesta parcial desaparece de la pantalla y la nota queda con la pregunta sola | Qué se limpia y qué se conserva al cancelar es producto; se cruza con `R4-partial-content-dropped` |
| `R4-003` | ARREGLAR | Un segundo envío en vuelo intercalaba dos respuestas, anexaba cuatro turnos por dos preguntas y dejaba el primer `controller` inalcanzable | Guarda de reentrada en `useSendMessage.send`; test RED primero |
| `R4-004` | CERRAR | — | `trimHistory` está probado (4 tests) y `historyTurns` viene clampeado a [0, 50] |
| `R2-create-chat-duplicates-starting-default` | CERRAR | — | El literal está duplicado, pero `ChatNoteSettings` hace que una divergencia sea error de compilación: no cambia comportamiento |
| `R2-merge-doc-overstates-preservation` | CERRAR | — | Lo arregló `8d0f51b`: `setChatSettings` mergea las tres claves propias y conserva las demás |
| `R2-saved-badge-contradicts-dirty` | ARREGLAR | «Guardado» seguía visible hasta 2 s después de volver a editar el borrador | El badge se apaga al primer cambio del borrador. Sin arnés de componentes: sin test |
| `R3-001` | CERRAR | — | Lo arregló `8d0f51b`: `mergeIntoExistingNote` pliega en vez de reemplazar, así que perder la carrera no pierde datos; el botón además se deshabilita mientras crea |
| `R3-002` | CERRAR (con test) | — | Verificado: con marcadores invertidos o regiones duplicadas el plugin escribe y relee su propio prompt bien. Se fija con dos tests |
| `R3-003` | CERRAR | — | Sin arnés de componentes (vitest corre `src/**/*.test.ts` en node): la lógica de Enter vive en el SFC y sólo se puede leer |
| `R3-004` | CERRAR | — | No hay duplicación: `sending` pasa a `false` en el mismo drenaje de microtareas que anexa el turno, así que no hay pintado intermedio. Sin arnés: leído, no probado |

### Los dos arreglos

- `R4-003` — RED observado en `useSendMessage.test.ts`: con dos envíos en vuelo, la secuencia
esperada `[append:user, search, stream, append:assistant]` llegó como ocho llamadas
(`append:user, search, stream, append:user, search, stream, append:assistant, append:assistant`)
y `answer` quedaba en `"holahola"`. GREEN tras la guarda `if (sending.value) return`.
- `R2-saved-badge-contradicts-dirty` — sin arnés de componentes, así que sin RED posible: el
cambio es un `watch(draft)` que apaga `saved`. Verificado compilando el SFC con
`@vue/compiler-sfc` y con `npm run build`.

### La discrepancia del contrato (R3-2 de C1)

`odd/tasks/chat-workspaces.md:80-81` dice: «el usuario ve la respuesta en pantalla, pero ningún
turno queda en la nota». El código no lo cumple: `ChatThread.vue:29-33` renderiza `answer` sólo
bajo `v-if="sending"`, y `useSendMessage.ts:125-127` apaga `sending` en el `finally`; cuando la
escritura del vault falla, la respuesta existe únicamente mientras llega el stream y desaparece
al terminar. El resto del párrafo (persistir la pregunta antes de recuperar) sí coincide con
`useSendMessage.ts:66-68`. No se edita el feature doc: lo decide el padre.

## Log

**L1 (2026-10-18, pedido)** — "ok sigue con lo delos hallazgos": trabajar sobre los 30
hallazgos advisory que dejaron los cuatro reviews nativos.

**L2 (2026-10-18, veredictos)** — los 30 hallazgos quedaron clasificados en `## Veredictos`:
**2 ARREGLAR** (`R4-003`, `R2-saved-badge-contradicts-dirty`), **8 DECISIÓN**
(`R1-data-json-round-trip-drops-unknown-keys`, `R1-llm-api-key-plaintext-storage`,
`R1-search-full-text-exposure`, `R3-onload-settings-failure-path`, `R3-4` de B,
`R4-create-orphans-file`, `R4-partial-content-dropped`, `R4-002`) y **20 CERRAR**, de los
cuales tres se cierran dejando un test (`R3-hybrid-stage-text-unproved`, `R3-002`). Los dos
fuera de alcance se reportan sin tocar código. Nada de esto está commiteado todavía.
