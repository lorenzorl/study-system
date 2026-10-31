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

## Tasks

| ID | Alcance | Specs | Commit |
| --- | --- | --- | --- |
| T1 | `GET /api/rag/documents/{documentID}/pages`: puerto angosto en `rag/app`, consulta por rango en `rag/sqlite`, handler, DTO, ruta, composición y tests | S1, S2, S3 | — |
| T2 | `obsidian_id` opcional y derivado en `study/domain` + `study/app`, con validación y tests | S3, S4 | — |
| T3 | Documentación (tabla de API del README) y cierre: `go vet`, `go test ./...`, `pnpm test` | S1–S4 | — |

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
