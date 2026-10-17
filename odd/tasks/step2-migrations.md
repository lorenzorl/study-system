# Step 2 — Versioned migrations, connection PRAGMAs, and safe adoption

**Status:** done
**Created:** 2026-10-17
**Depends on:** step 1 (`platform/config`)

## Why

`study/sqlite/db.go` ran one `CREATE TABLE IF NOT EXISTS` block on every open.
Two consequences: it could not express a schema *change*, and the connection was
opened without PRAGMAs. Measured against this repo's driver before the change:

| DSN | foreign_keys | busy_timeout | journal_mode | orphan insert |
| --- | --- | --- | --- | --- |
| plain | 0 | 0 | delete | **accepted** |
| `_pragma` | 1 | 5000 | wal | **rejected** |

The `REFERENCES` clauses were decorative, and a long write would have contended
with API reads with no WAL and no busy timeout.

## Delivered

- `platform/db` owns the **mechanism**: `Open` (PRAGMAs via DSN, pool tuning) and
  `Migrate` (versioned, transactional runner with a `schema_version` table).
- `platform/db` also owns the driver import, so there is one place that can
  misconfigure a connection.
- `study/sqlite/migrations.go` owns the **study schema**: version 1 is an
  idempotent baseline, version 2 drops the retired `resources.dify_document_id`
  column when present.
- `study/sqlite/db.go` deleted.
- `cmd/server/main.go` opens the database and applies migrations before wiring.

## The adoption risk, and how it was verified

`data/study.db` already existed with 3 topics, 3 concepts, and a `resources`
table still carrying the dropped Dify column. The real binary was run against a
**copy** of it:

| Observation | Before | After |
| --- | --- | --- |
| `schema_version` | absent | `2` (both migrations recorded) |
| `resources.dify_document_id` | present | **dropped** |
| `journal_mode` | delete | `wal` |
| topics | 3 | **3** |
| concepts | 3 | **3** |
| `GET /api/concepts` | n/a | returns `DDD`, `Test`, `so` with their 3 concepts |
| `data/study.db` sha256 | `b7343bf5…` | `b7343bf5…` — **identical, never written** |

## What the PRAGMAs found

Enabling `foreign_keys=ON` immediately failed **6 repository tests** with
`FOREIGN KEY constraint failed`. They were inserting `card_states` and
`review_logs` against invented flashcard IDs (`"flash-uuid-1"`, `"flash-dup"`,
`"flash-update"`, `"flash-logs-1"`, `"flash-multi"`) and a `concepts` row against
an invented `TopicID`. They passed only because the constraint was not enforced.

The production code was correct; the tests were building invalid data. Fixed by
adding `seedTopic` and `seedFlashcard` helpers and giving each test a real parent
chain. The FK switch made the suite stronger rather than weaker.

## Verification

| Check | Result |
| --- | --- |
| `gofmt -l`, `go vet ./...` | clean |
| `func Test` (backend) | 134 -> **160** (+26) |
| `npm test` / `npm run build` | untouched, green (94) |
| Fresh database | ends at version 2, all 6 tables present |
| Legacy database copy | adopted, data intact, legacy column dropped |
| Idempotent re-run | no duplicate rows in `schema_version` |
| Failed migration | rolled back, version stays at last success, later migrations skipped |
| Duplicate / non-positive version | rejected before anything is applied |
| `PRAGMA foreign_keys` | 1, orphan insert rejected |
| Real `data/study.db` | checksum unchanged |

## Design notes

- `foreign_keys` is per connection, so `Open` sets PRAGMAs through the DSN rather
  than issuing a one-off `PRAGMA` against a pooled handle.
- In-memory DSNs pin the pool to one connection: an in-memory database is private
  to the connection that created it, so a second connection would hand out a
  different, empty database. Verified by a concurrent-query test.
- `Migrate` validates the whole set (unique, positive versions, non-nil `Up`)
  before applying anything, so an invalid list cannot be half-applied.
- Forward-only by design: a single-user local database does not need a down path.

## Follow-ups

Recorded in [`backlog.md`](backlog.md), the single source of truth.
