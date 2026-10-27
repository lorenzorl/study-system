# Durable replica of the local state in Cloudflare R2

Keep `data/` as the working state and add a durable replica of it in an S3-compatible
bucket (Cloudflare R2): the SQLite database through a transactionally consistent
snapshot, and the stored artifacts through an append-only sync. The public surface is
two subcommands, `backup` and `restore`. No storage or database path changes, and the
server keeps working with no R2 configuration at all.

## Specs

- **S1** — The request: "puedes hacer que se guarden en un proveedor como cloudflare?".
- **S2** — Objective, selected by the user: "Durabilidad: no perder DB ni PDFs". Explicitly
  not a full cloud migration and not remote access to a hosted backend; the analysis
  offered those as separate routes and the user chose this one.
- **S3** — Unit of replication, confirmed by the user after the trade-off was laid out:
  "asi esta bien" to file-level replication. The accepted consequences are stated there
  and are part of the decision: every snapshot is the whole database, restore is
  all-or-nothing at a point in time, and there is no per-row restore and no merge.
- **S4** — One writer. The accepted usage is one process writing the database and other
  devices reading a restored copy. Multi-device concurrent writing is out of scope and
  this design cannot serve it: two SQLite files have no operation log to reconcile, so
  the last snapshot would win silently. The bound is documented, not enforced.
- **S5** — `data/` stays the primary working state; R2 is a replica. `STUDY_HOME`, the
  default `data/study.db` DSN, the artifact layout, and the local-first behaviour are
  unchanged.
- **S6** — The database is copied only through a transactionally consistent snapshot
  (`VACUUM INTO`), never through a file copy. A plain copy of a WAL database can produce
  a torn file that opens and is wrong. Verified before planning: through
  `modernc.org/sqlite v1.53.0` (~SQLite 3.50; `VACUUM INTO` needs 3.27+) a snapshot taken
  while writes were live in the WAL captured both rows and `PRAGMA integrity_check`
  returned `ok`.
- **S7** — No new Go dependency. The module is stdlib plus `uuid`, `testify`, and
  `modernc.org/sqlite`; R2 is S3-compatible, so access is SigV4 over `net/http`. Adding
  the AWS SDK to this project is a decision nobody asked for.
- **S8** — Nothing that works today may require R2 to work. `serve` and every existing
  subcommand boot with the new variables unset; only `backup` and `restore` fail without
  them, naming the variables that are missing.
- **S9** — The secret is never emitted. `STUDY_R2_SECRET_ACCESS_KEY` must not reach a log
  line, an error message, or a committed file — the same rule
  `STUDY_OPENROUTER_API_KEY` already follows (see the URL-credential fix in `4b92c33`).
- **S10** — A snapshot that fails validation is never uploaded, and `restore` validates
  the downloaded file before it replaces the live database and requires explicit
  confirmation before it does. An unverified backup is not a backup.
- **S11** — Surfaces: this work touches `apps/go-middleware/**` and `odd/**` only. It does
  not touch `apps/web/**`, `src/**`, `package.json`, or `pnpm-*`, which another session
  holds (L7).
- **S12** — Existing contracts are preserved. New environment variables are additive
  and optional; no existing variable, DSN, path, command, flag, or exit code changes.

## Tasks

| ID | Spec | Route | Commit |
| --- | --- | --- | --- |
| T1 | S8, S12 | inline | this commit |
| T2 | S7, S9 | inline | pending |
| T3 | S6, S10 | inline | pending |
| T4 | S2, S3, S5 | inline | pending |
| T5 | S3, S10 | inline | pending |
| T6 | S4, S8 | inline | pending |
| T7 | S10, all | inline | pending |

- **T1** — `platform/config`: `STUDY_R2_ACCOUNT_ID`, `STUDY_R2_BUCKET`,
  `STUDY_R2_ACCESS_KEY_ID`, `STUDY_R2_SECRET_ACCESS_KEY`, `STUDY_R2_ENDPOINT`,
  `STUDY_R2_PREFIX`. Optional as a group; validation rejects a partially configured
  group at use time, not at boot, so S8 holds for every existing command.
- **T2** — `platform/r2`: a stdlib SigV4 client with `Put`, `Get`, `Head`, and a paginated
  `List`, context-aware, with the secret redacted from every error path. Validated
  against the published AWS SigV4 test vectors and an `httptest` S3 double.
- **T3** — `platform/backup`: take the snapshot (`VACUUM INTO` into a temporary path),
  validate it (`PRAGMA integrity_check` plus an open-and-count probe), and report a
  typed error when it fails so nothing downstream can upload it.
- **T4** — `study-system backup [-dry-run]`: snapshot the database, upload it under a
  timestamped key, and sync `data/artifacts/<id>/{payload,meta.json}` under a
  content-addressed key, skipping objects already present. `-dry-run` reports the exact
  upload set and byte count without writing.
- **T5** — `study-system restore [-from KEY] [-yes]`: download and validate the newest
  snapshot (or the named one), print what it will replace, and only then move the live
  database aside and install the snapshot. Without `-yes` it refuses.
- **T6** — Docs: the two variables groups in the README configuration table, a section
  recording the single-writer bound (S4), the restore procedure, and why the database is
  snapshotted rather than copied.
- **T7** — Evidence: `gofmt`, `go vet ./...`, the full test suite, a restore rehearsal
  from a local S3 double, and an explicit statement of what could not be proven here
  (the authenticated round trip against the real bucket needs the user's credentials).

## Log

- **L1** (user): "Actualmente donde se estan guardando los archivos? de el backen en go?"
- **L2** (evidence): answered read-only. The Go backend is `apps/go-middleware`
  (`cmd/`, `internal/{platform,study,rag,composition,server}`); runtime state is
  `apps/go-middleware/data/` — `study.db` plus `-wal`/`-shm`, and
  `data/artifacts/<uuid>/{payload,meta.json}`. Root resolution is `-home`, then
  `STUDY_HOME`, then the working directory (`platform/config/config.go`). All of `data/`
  and `bin/` are gitignored.
- **L3** (user): "puedes hacer que se guarden en un proveedor como cloudflare?"
- **L4** (evidence): the question is three different migrations. `storage.Store` is a
  concrete struct, not a port, and `rag/app/{ingest,process}.go` call `Path()` and
  `Workspace()` to hand a local file to poppler (`exec.CommandContext`), so an object
  store needs a local scratch step; `Put` also relies on `os.Rename` within a single
  filesystem. The database is 49 raw-SQL call sites across 26 repo and migration files
  over `*sql.DB`, and D1 has no `database/sql` wire protocol, no `PRAGMA`, and no WAL.
  Cloudflare Workers cannot run a Go binary at all.
- **L5** (evidence): nothing to install. `litestream`, `rclone`, and `aws` are absent from
  this machine; `sqlite3` 3.50.6 and Go are present. A stdlib SigV4 client therefore fits
  the module's dependency posture better than a foreign daemon or the AWS SDK.
- **L6** (user, decision): "Durabilidad: no perder DB ni PDFs" — the durability route, not
  remote access and not a full cloud migration.
- **L7** (evidence, coordination): the working tree changed under this session and not
  through it. Another Pi session (`01a17eb8`) works in the same cwd on the shadcn-vue
  feature and staged `apps/web/**` and `odd/tasks/shadcn-vue.md`. Those surfaces are
  disjoint from S11; no branch is switched while it works, so its commits cannot land on
  this feature's branch. The one disposable probe this session wrote was
  `/tmp/vacuumprobe`, outside the project, and was removed.
- **L8** (user): "por ahora no modifiques nada, solo estamos viendo si es viable" — the
  session stayed read-only from L1 through L10. Nothing in the repository was written.
- **L9** (user): "antes de seguir una pregunta, la base de datos se va a replicar como archivo?"
- **L10** (evidence, sizing): `study.db` is 20 MB, of which `chunk_embeddings` is 1,180
  rows of 3,072-dimension float32 vectors (12,288 B each, ~13.8 MiB, ~75% of the file);
  `data/artifacts` is 9.2 MB. The bulk of the database is *derived* and regenerable with
  `study-system embed`, so the irreplaceable part is ~5 MB. A full-file snapshot is
  affordable; stripping the vectors was considered and rejected L12.
- **L11** (user, decision): "asi esta bien" to file-level replication, after L10 stated
  that restore is all-or-nothing and that two writers would have no reconciliation. This
  is the decision S3 and S4 record.
- **L12** (decision): vectors stay in the snapshot. Excluding `chunk_embeddings` would cut
  it from 20 MB to ~5 MB, but it would make restore depend on the API key, the network,
  and the embedding model keeping its identifier and dimension. Completeness first at this
  size.
- **L13** (decision): retention keeps the newest 14 snapshots and deletes older ones only
  after a successful upload. Full-file snapshots accumulate linearly and an unbounded
  bucket is an unowned cost; the default is a number, not a promise.
- **L14** (decision): T1–T7 run inline. The Writer rule is not met: the tasks are
  sequential and dependent (T4 needs T2 and T3), so there are not two independent units of
  behaviour each heavier than a subagent start.
- **L15** (risk): items 1 and 2. Item 1 because `restore` replaces stored data
  irreversibly, and item 2 because the work handles a bucket credential. The Verification
  rule therefore applies: independent verification of T3 and T5, the two tasks that can
  lose or leak data.
- **L16** (user): "asi esta bien, ahora sigue con el plan" — authorization to plan.
- **L17** (evidence, T1): `platform/config` reads the six `STUDY_R2_*` variables into
  `Config.Replica`; `Load` never rejects them, and `ReplicaSettings.Validate()` names every
  missing variable at once. 9 new tests in `replica_test.go`, RED observed first (34
  undefined-symbol errors), then green. `gofmt -l` clean, `go vet ./...` clean,
  `go build ./...` OK, **full suite 19 packages, exit 0**. Coverage of the spec: S8 by
  `TestLoad_ReplicaIsAbsentByDefault` and `TestLoad_PartialReplicaSettingsDoNotStopTheProcess`
  (the process boots with zero and with half a bucket); S9 by `TestReplica_StringNeverPrintsTheSecret`,
  which asserts the fixture credential is absent from `%v`, `%+v`, `%s` of the settings **and**
  from `%+v` of the whole `Config` — the realistic leak path; S12 by the untouched existing
  tests in `config_test.go` and the full suite. Design detail worth a later reader's attention:
  `String()` exists because `fmt` honours a `Stringer` on a nested field, so one
  `log.Printf("%+v", cfg)` would otherwise write the bucket key to disk.
- **L18** (blocker, coordination): T1 was implemented and verified but **not committed**. The
  worktree is shared with session `01a17eb8` (L7), which holds `apps/web/**`; creating a branch
  there would put that session's next commit on this feature's branch, and committing on
  `feat/shadcn-vue-primitives` would bury a durability feature in a styling branch.
- **L19** (user, decision): "el problema es que lo estas haciendo en una rama donde se estan
  haciendo modificaciones para la web app, crea un nuevo worktree y haz los cambios alli".
  Resolved L18: this feature now lives in the worktree
  `/Users/lorenzorangel/projects/study-system/.worktrees/r2-durability` on branch
  `feat/r2-durability`, based on `master` (`9b4373a`) rather than on the styling branch, so no
  web-app commit enters this history and no commit of this feature enters that one. The three
  files were copied across and removed from the shared tree, which is back to holding only the
  other session's work. `.worktrees/` was added to `.git/info/exclude` — local to this clone and
  not a tracked `.gitignore` edit — so the shared repository's status stays readable. Verified in
  the new worktree: `gofmt -l` clean, `go vet ./...` clean, `go build ./...` OK, the config
  package green, and the full suite green from the `master` baseline.
