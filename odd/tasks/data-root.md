# Step 9 — One root for configuration and data (CLI, stage 4)

**Status:** done
**Created:** 2026-10-20
**Depends on:** step 8 (`internal/cli`, `internal/server`), `platform/config`

## Specs

**S1 — The request.** "sigue con la que mejor te parezca" — the parent chose stage
4 over stage 3, having named the reason twice: "apenas instalás el binario en el
`PATH`, `study-system search` desde otra carpeta lee otro `.env` y otra base. Eso
ya es observable, no una preocupación teórica."

**S2 — Constraints inherited from the system.** No new dependencies. A checked-out
repository keeps working exactly as it did: same `.env`, same `data/study.db`, same
`data/artifacts`. Environment variables keep their names and their precedence
(environment over file over default). The package tests stay green.

## Why

Every path a run used was relative to the process's working directory: `.env`,
`data/study.db`, `data/artifacts`, and the `data/` directory the server created.
That is correct for a command run from the repository and wrong for an installed
binary: `cd /tmp && study-system search foo` reads `/tmp/.env`, finds no database,
and answers "no results" — a wrong answer that looks like a normal empty one.

## Design: one root, absolute, and never silent

`STUDY_HOME` (or the `-home` global flag) names the **root**: the directory `.env`
is read from and `data/` is written under. The root defaults to the working
directory, so the repository layout is unchanged; the defaults for the database and
the artifact store are built from it, which makes them **absolute**.

Two variables select *which* file is read, so they are read from the process
environment only and never from the `.env` file: `STUDY_HOME` and `STUDY_ENV_FILE`.
A selector that came from the thing it selects would be circular.

An explicit `STUDY_DB_DSN` or `STUDY_STORAGE_ROOT` is untouched, and a relative
DSN stays relative to the working directory: that is the operator's own choice.

A run whose root is not the working directory says so — one line on stderr for a
command, one line in the log for the server. The repository layout stays silent, so
the ordinary case prints nothing new.

## Deliberate changes

- `config.DefaultDatabaseDSN` and `config.DefaultStorageRoot` became functions of
  the root instead of constants. Their only consumers were `config`'s own tests.
- The default DSN string is absolute now. It points at the same file when the root
  is the working directory.
- `list` on an empty store prints the absolute path, not `data/artifacts`.

## Tasks

- [x] T1 — `config`: root and env-file resolution, `Options`, `LoadWith`, root-based defaults (S1)
- [x] T2 — `cli`: global `-home`/`-env-file` before the command, the root notice (S1)
- [x] T3 — `server`: create `data/` under the root, log the root when it is not the working directory (S2)
- [x] T4 — tests, README, behavioural verification, commit (S2)

## Log

**L1 (2026-10-20, choice)** — the user delegated the choice of the next stage; the
parent picked stage 4 and recorded why stage 3 can wait: the path problem is
observable today, and stage 3 benefits from having it solved.

**L2 (2026-10-20, implemented)** — `gofmt -l`, `go build ./...`, `go vet ./...`
clean; `go test ./... -count=1` ok in 15 packages, zero failures.

Behaviour, verified by running the built binary rather than by reading it:

| Check | Result |
| --- | --- |
| `search figure -limit 3` from the repository, no `STUDY_HOME` | the same three hits with the same bm25 scores as the recorded baseline |
| `list` from the repository | the same five artifacts |
| `-home <tmp> list` | `data root: <tmp>` on stderr, `no artifacts in <tmp>/data/artifacts` |
| `STUDY_HOME=<tmp> list` | identical to the flag |
| `-home` after the command | rejected: `flag provided but not defined: -home` |
| `STUDY_HOME=<tmp> serve` | `200` on `/api/concepts`, `data root:` in the log, and `<tmp>/data/{study.db,artifacts/incoming}` created |
| `serve` from the repository | no new log line, same listen/shutdown lines |

The tests cover the resolution rules (default, flag, environment, precedence,
relative `-env-file`), the two new dispatch cases, and the notice.

**Risk: item 6** — `main()` is still not executed by a test, and the server's
`data/` creation moved under the root; both were verified by running the binaries,
not by the suite.
