# Feature — Agent API: página por rango y escritura de tarjetas idempotente

**Status:** in progress
**Created:** 2026-10-30
**Branch:** `feat/agent-api`
**Depends on:** nada. Antecede al `SKILL.md` del agente y a cualquier MCP (que no se va a construir).

El agente corre en el cliente (Termux + Pi en Android) y habla HTTP contra este
servidor. Para que pueda proponer tarjetas con cita y escribirlas sin duplicar el
mazo, faltan dos capacidades y nada más: leer el texto de un rango de páginas, y
escribir tarjetas cuyo identificador de idempotencia derive el servidor.

## Specs

**S1 — La propuesta tiene que poder citar su fuente.**

> "quiero que se creen propuestas, detallando de donde se saca la informacion para
> proponer esa tarjeta"

Consecuencia: la cita es (`document_id`, `page_number`), y para que sea
verificable el servidor tiene que poder devolver el **texto de esa página**. El
texto ya está guardado en `job_pages.text`; hoy no hay ruta que lo exponga y el
único camino es `data/DDD.md`, que solo existe para libros pasados por `convert`.

**S2 — La superficie es HTTP/JSON, y solo eso.**

> "voy a usar el telefono androud con termux y pi, vamos a evaluar solo ese caso"
> "el mcp no es necesario porque tengo en agente corriendo en el cliente"

Consecuencia: nada de `go run ./cmd/cli` en el camino del agente (compilar Go en el
teléfono es caro y necesita el repo). No se construye MCP. El cliente es un agente
con shell, así que `curl` contra la API es el transporte.

**S3 — Las dos capacidades son exactamente dos.**

> "si comienza por esos endpoints"

1. `GET /api/rag/documents/{documentID}/pages?from=N&to=M` — el texto de cada
   página del rango, con su estado.
2. Escritura de tarjetas en la que el **servidor** deriva el `obsidian_id`.

**S4 — Aditivo: el contrato que la web ya usa no cambia.**

`POST /api/sync/flashcards` con `obsidian_id` explícito sigue funcionando igual.
El `obsidian_id` pasa a ser **opcional**; cuando falta, se deriva. Ningún cliente
existente cambia su comportamiento.

**S5 — El skill se distribuye como Pi package.**

> "como hago para que alguien que tenga instalado pi la pueda descargar facilmente"

Consecuencia: el skill vive en `skills/study-system/` **dentro de este repo**, así
que se instala con `pi install git:…@v1` y su tag no puede discrepar del tag de la
API que documenta. Nada de valores de despliegue dentro del skill: la URL y el
token se leen de `~/.config/study-system/env`, y el primer paso es un smoke test que
falla fuerte si el destino no es el servidor esperado.

## Tasks

| ID | Alcance | Specs | Commit |
| --- | --- | --- | --- |
| T1 | `GET /api/rag/documents/{documentID}/pages`: puerto angosto en `rag/app`, consulta por rango en `rag/sqlite`, handler, DTO, ruta, composición y tests | S1, S2, S3 | `cad3371` |
| T2 | `obsidian_id` opcional y derivado en `study/domain` + `study/app`, con validación y tests | S3, S4 | `b75cee4` |
| T3 | Documentación (tabla de API del README) y cierre: `go vet`, `go test ./...`, `pnpm test` | S1–S4 | `fdda476` |
| T4 | El skill distribuible: `skills/study-system/` con `SKILL.md`, `references/api.md`, `references/cards.md`, y la sección de instalación en el README raíz | S1, S2, S5 | — |

### Slices (delivery strategy)

| Slice | Unidad | Commit | Foco de revisión | Líneas |
| --- | --- | --- | --- | --- |
| **A — la lectura citable** | T1 | `cad3371` | Los límites del rango y el orden en que se aplican; que el puerto angosto no toque `domain.PageOutcomeRepository`; que `ListOutcomes` no cambie | ~1000 |
| **B — la identidad derivada** | T2 | `b75cee4` | La normalización (espaciado y mayúsculas sí, palabras y puntuación no) y que la validación quede acotada al camino derivado | ~280 |

**Slice A pasa el techo de ~400 líneas a propósito, y es el único que lo hace.** Es un
comportamiento solo: la ruta no existe hasta que existen el puerto, el adaptador, el caso
de uso, el DTO y el registro. Separar los tests o el adaptador produciría un commit que no
compila, y el repo ya aceptó ese criterio una vez (`PRESENTATION-UI.md` §9, slice 2). Slice B
no lo necesita y no lo hace.

### Decisiones de diseño, para que no se re-decidan

- **La derivación vive en el servidor, no en el cliente.** El cliente que manda su
  propio id (un agente que improvisa) es exactamente la falla que esto evita: dos
  corridas con ids distintos duplican el mazo.
- **`obsidian_id` es `UNIQUE` global**, no por concepto. El id derivado lleva el
  concepto en la entrada del hash, así que dos conceptos no colisionan.
- **La validación se aplica al camino derivado.** Una tarjeta sin pregunta no
  tiene identidad posible (no se puede hashear algo vacío) y se rechaza. No se
  agrega validación al camino con id explícito: eso cambiaría lo que hoy acepta un
  cliente existente, y S4 lo prohíbe.
- **El rango tiene tope.** El endpoint es la puerta de un libro de 359 páginas y
  ~917 KB de texto: sin tope, un `from=1&to=359` es una descarga de un megabyte
  por pedido. El tope es parte del contrato y está documentado.

## Log

**L1 (2026-10-30) — pedido del usuario, textual:**

> "si comienza por esos endpoints"

En el contexto de la conversación previa, el usuario autorizó los dos endpoints
acordados: `pages` por rango y escritura con `obsidian_id` derivado en el servidor.

**L2 (2026-10-30) — decisiones tomadas en la conversación, textuales:**

> "quiero que las tarjetas se creen automaticamente, para ello quiero que se creen
> propuestas, detallando de donde se saca la informacion para proponer esa tarjeta"

> "para el auth voy a usar cloudflare tunnel + access"

> "voy a usar el telefono androud con termux y pi, vamos a evaluar solo ese caso"

> "el mcp no es necesario porque tengo en agente corriendo en el cliente"

**L3 (2026-10-30) — hallazgo que motiva T1:** `data/DDD.md` es el único camino al
texto por página, y solo existe porque ese libro pasó por `convert`. Un documento
subido por la web no lo tiene. `job_pages.text` sí lo guarda para todos.

**L4 (2026-10-31) — evidencia de cierre de T1 y T2.**

| Check | Resultado |
| --- | --- |
| Test-first T1 | RED: tres suites nuevas fallando al compilar por símbolos inexistentes. GREEN: 23 tests (app 9, http 12, sqlite 2) |
| Test-first T2 | RED: `undefined: domain.CardIdentity` en las dos suites. GREEN: 11 tests nuevos, y los dos tests previos de sync intactos |
| `go vet ./...` | limpio |
| `go test ./...` | todos los paquetes `ok` |
| `go build ./...` | OK, incluidos `cmd/server` y `cmd/cli` |
| `gofmt -l internal/ cmd/` | sin salida |
| Differential check T1 | `ListOutcomes` comparte el nuevo `queryOutcomes`: mismo resultado contra `git archive HEAD` en un árbol aparte (8 passes en ambos) |
| En vivo, lectura | `pages?from=25&to=27` → `200`, `total_pages 359`, páginas 25–27 con el texto real de Evans |
| En vivo, rechazos | `from=0` → `400 "from must be at least 1"`; `1..359` → `400 "at most 50 pages…"`; `355..400` → `400 "to must be at most 359…"`; `from` ausente → `400 "from is required"`; documento inexistente → `404` |
| En vivo, escritura | Contra una copia aislada de la base (`.backup`, puerto 8099): cuatro POST de la misma propuesta —incluida una variante con otro espaciado y mayúsculas y una respuesta corregida— dejan **una** fila con la respuesta corregida, y crean su `card_states` con `next_review` inmediato |
| En vivo, rechazo de escritura | Una tarjeta sin respuesta → `400 "invalid input"` y cero filas escritas |

**Un 500 que fue de mi arnés, no del código, y queda anotado para no repetirlo:** la primera
corrida de escritura devolvió `FOREIGN KEY constraint failed` porque copié `study.db` con `cp`
mientras el servidor tenía la base en WAL, así que la copia no tenía el concepto recién creado.
La derivación había funcionado (el log muestra el `card-…`). El arnés se rehizo con
`sqlite3 .backup`, que sí incluye el WAL.

**L5 (2026-10-31) — el skill, autorizado textualmente:**

> "como hago para que alguien que tenga instalado pi la pueda descargar facilmente"

> "si escribelo asi"

Decisión de distribución: **Pi package dentro de este repo, instalable por git con
tag**. Descartado npm por ahora (publicar y bumpear en cada cambio de contrato sin
tener usuarios externos) y descartada la carpeta suelta en `~/.agents/skills/`
(portable a otros agentes, pero sin versión ni `pi update`). El argumento decisivo no
es la comodidad: el skill documenta contratos de API, así que el tag del skill y el
de la API tienen que ser el mismo número.

Forma entregada: `skills/study-system/SKILL.md` (frontmatter con `name`,
`description` y `compatibility`, sin `license` porque el repo no tiene LICENSE),
`references/api.md` (los endpoints con payloads y mensajes de error textuales) y
`references/cards.md` (las reglas de una tarjeta que vale la pena, con ejemplos
buenos y malos). El `SKILL.md` no lleva ni la URL ni el token: los lee de
`$STUDY_ENV_FILE`, que por defecto es `~/.config/study-system/env`.
