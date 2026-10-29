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
| T2 | S7, S9 | worker | `5bf3c83`, `02e8050` |
| T3 | S6, S10 | worker | `d1a756b` |
| T4 | S2, S3, S5 | worker | `0a61914` |
| T5 | S3, S10 | worker | `ff3ff02`, `4f4aad7` |
| T6 | S4, S8 | inline | pending |
| T7 | S10, all | verifier | this commit |

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
- **L20** (evidence, RDD scope): the receipt-driven review candidate does not match this
  change. `inspect` was called with `workspaceRoot` set to this worktree, because the session
  cwd is the shared tree and the default would have targeted the wrong repository state. It
  returned target `sha256:87aaa428…` over a `base-diff/workspace` projection with
  `base-ref=d70d9a2c` — which is `origin/master` — and `committed-only=true`, covering **204
  files, +7,799/−2,222 across 14 commits**. This feature's actual candidate is **3 files,
  +479, 1 commit** (`db7067f`). The cause is repository state, not the tool: local `master` is
  13 commits ahead of `origin/master` (the whole web-app feature, unpublished), so a projector
  measured against the remote base absorbs that mainline into the candidate. **START was not
  issued.** Relaying a consent envelope for a 204-file review would misrepresent what the human
  is approving, and the contract already names the terminal stop this would earn
  (`lens_context_budget_exceeded`). A second inconsistency was recorded rather than worked
  around: the reminder named target `sha256:67277a2b…` and `inspect` returned
  `sha256:87aaa428…`, two different mutation generations. Resolution belongs to the user —
  publish `master` so the base advances, or review this feature once it does. Neither the base
  ref nor the target identity was reconstructed by hand.
- **L21** (evidence, coordination): session `01a17eb8` fast-forwarded `master` from `9b4373a` to
  `14ca817` (six commits: Tailwind v4 plus the shadcn-vue primitives in `apps/web`, and a fix
  to the FSD guard) with a ref-only update, so no checkout happened and this worktree was not
  touched. Verified rather than assumed: `master` = `14ca817`, this branch = `db7067f`,
  `master...feat/r2-durability` = 6 behind / 1 ahead, and the branch diff is still the same
  three paths, disjoint from `apps/web/**`. No rebase and no merge was taken now: the branch
  carries one commit, integration is not imminent, and rebasing would rewrite `db7067f` and
  invalidate the hash recorded in T1 and L19 for no present gain. It also would not help L20 —
  the review candidate is measured against `origin/master` (`d70d9a2c`), which a local `master`
  advance does not move.
- **L22** (evidence, RDD base — supersedes the resolution L20 proposed): session `01a17eb8`
  reported a third path that neither L20 option covered. `START` accepts an explicit `baseRef`
  plus `committedOnly: true` and scopes the candidate to it, instead of inheriting the
  `origin/master` default: on their branch the default offered 231 files and the explicit base
  offered 37, and the review reached `approved`. This feature's base is
  `9b4373a69633b610a9f518721cfa81c7a6fd8151`, confirmed to equal `git merge-base HEAD master`, so
  it is stable for the whole feature and does not move when `master` advances. Two field notes
  from them are recorded because they cost them a failed attempt: an abbreviated SHA is rejected
  as `base-ref-unresolvable`, and a `candidate-mismatch` traced to a retained untracked selection
  from an older inspect is cleared by a fresh inspect plus `untrackedScope: "exclude"`. A failed
  START creates no lineage and burns no authority, so a retry is safe. **Publishing `master` is
  therefore not a prerequisite for review**, which is what L20 assumed; it remains a separate
  delivery decision. L22 first concluded that the review should be taken "once, at delivery,
  over the whole feature". **That conclusion was wrong and L23 corrects it.**
- **L23** (evidence, RDD protocol — corrects L22): the contract is explicit that the candidate is
  "a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature
  branch", so a single review over `9b4373a..HEAD` with all seven tasks inside is what the rule
  forbids; an oversized candidate risks `lens_context_budget_exceeded`, a terminal stop that costs
  the attempt. The correct rhythm is `assess` after each work-unit commit, with
  `{baseRef: <parent sha>, committedOnly: true}`, because an assess on a clean, fully committed
  tree otherwise answers `unassessable` with "the review assess candidate has no pending changes" —
  the same base problem as L20. Assess is read-only: no lineage, no authority, no review state.
  **T1's own result: `risk: medium`, `changedPaths: 3`, `changedLines: 479`, `reviewDue: false`,
  `reviewDueReason: under_budget`, `candidate.consumed: false`, `outcome_source: unknown`, plan
  `writerSelfVerification: true` / `independentVerifier: false`.** Disposition: **no START for
  T1** — the review is not due, and the writer's own verification stands. The expectation that a
  credential-touching unit would come back `reviewDue: true` did not hold for this one; it is
  re-tested per unit rather than assumed, and T4 and T5 are the units where it is most likely.
- **L24** (evidence, T1 writer self-verification): `under_budget` is a judgement about cost, not
  an absolution of risk, so the plan's `writerSelfVerification: true` was discharged with
  observable checks instead of an assertion that the code looks careful: (a) `git grep` over the
  committed tree for `sk-or-v1` and for a literal `STUDY_R2_SECRET_ACCESS_KEY=` assignment returns
  nothing, so no credential is committed; (b) `git check-ignore -v .env` still matches
  `.gitignore:6`, so the file that does hold one stays out of the tree; (c) `Validate` builds its
  message from variable **names** and the endpoint value only — it never interpolates
  `SecretAccessKey`; (d) `TestReplica_StringNeverPrintsTheSecret` passes, covering `%v`, `%+v`,
  `%s` of the settings and `%+v` of the whole `Config`.
- **L25** (decision, escalation): T5 replaces the live database, which is Task Size high-risk item
  1 ("hard to undo"), and that consequence is not measured by a line count. Its `assess` will pass
  `escalate: {item: 1}` so an independent verifier runs even if the tier comes back `under_budget`.
  Escalation can only raise, never lower, so it costs a verifier and buys the check that matters.
  The same escalation is deliberately **not** pre-applied to T2-T4 and T6: the tier is measured per
  unit, not predicted.
- **L26** (decision, plan shape): a single "review the feature at the end" task was dropped from
  the projection because review is not a task — it is a gate applied inside each work unit
  (L23). Keeping it as a task would have restated the L22 mistake in a different place. The gate
  now lives in T7's definition and in each unit's closure note.
- **L27** (evidence, T2): delegated to `gentle-ai-worker` against the narrow surface
  `apps/go-middleware/internal/platform/r2/` and committed as `c7802d3`. The writer reported
  honestly that RED could not be observed, because the package did not exist and there was no
  compilable pre-implementation state; it substituted four mutation checks against the finished
  tree, each failing exactly one intended test, with the files restored and sha256-verified
  afterwards. That is an applicability exception, not invented lifecycle evidence. It also found
  three real defects during GREEN rather than accommodating them: the double did not expose Host
  as a header (test wrong), `List` addressed `/bucket/prefix` instead of `/bucket` (implementation
  wrong — the prefix is a query parameter there), and `fmt` `%+v` of `*Client` printed the secret
  because fmt cannot call `String` on an unexported field (a real S9 leak, fixed with
  `Client.String`). Parent verification was done independently of the report: only the new
  directory appears in `git status`, `go.mod` and `go.sum` are unchanged, the package tests pass,
  and the **full suite exits 0** with `gofmt` and `vet` clean. Both signing vectors pass
  unmodified. Its own closure note: the authenticated round trip against the real bucket is not
  proven, and T4/T5 need that rehearsal.
- **L28** (risk surfaced, review workload): T2's candidate measures **4 paths / 1419 changed
  lines**, roughly 3.5 times the ~400-line work-unit slice this workflow asks for. `assess` still
  returned `under_budget`, so the tool does not object; the objection is the human reviewer's, and
  it is recorded rather than hidden. Noted here so the choice is visible before T3 adds to the
  branch. The user chose to split it.
- **L29** (evidence, split executed): `c7802d3` was replaced by two commits, both verified.
  `5bf3c83` holds the signer (`sign.go` 246 + `sign_test.go` 117 = **363 lines**, under the ~400
  slice) and was proven self-contained rather than assumed: with the client files stashed away,
  `go build` and the package tests still passed, so the signer is reviewable and runnable on its
  own against the pinned vectors. `02e8050` holds the transport (`r2.go` 568 + `client_test.go`
  488 = 1056 lines). That second slice is still above the slice target and is recorded as such:
  the excess is mostly test code, and cutting further would split one struct's methods across
  commits, which makes review harder rather than easier. The split cost one `reset --mixed` and
  left the working tree byte-identical; full suite, `gofmt` and `vet` are green afterwards.
- **L30** (evidence, T3): delegated to `gentle-ai-worker` with the surface narrowed to
  `apps/go-middleware/internal/platform/backup/`, committed as `d1a756b` (295 production + 483 test
  = 778 lines, about twice the slice target and mostly test code; splitting `Take` from `Validate`
  would make review harder, not easier — recorded, not hidden). Four probes changed the code, and
  each is the kind of thing a reviewer would otherwise have to take on trust:
  (a) `VACUUM INTO` **refuses a non-empty destination but silently overwrites a zero-byte one**, so
  delegating the existence check to SQLite would be a defect; `Take` refuses any existing path
  itself. (b) `VACUUM INTO ?` **does** accept a bound parameter through this driver, so no path is
  ever spliced into SQL text. (c) A `mode=ro` source handle can run it and still sees rows living
  only in the WAL — but **`immutable=1` also "succeeds" while silently dropping those WAL rows**
  (3 of 4 in the probe). That hazard is the single most dangerous finding of this task and is
  recorded so nobody optimises it in later. (d) `PRAGMA integrity_check` answers `ok` for an empty
  database and for a zero-byte file alike, so integrity alone cannot prove a snapshot is usable;
  that is why `Validate` returns a `Report` with the table list and page count and the caller
  asserts its own schema. RED was partly synthetic and the writer said so: with a compiling stub,
  8 of 15 tests failed for the intended reasons and 7 passed vacuously, so those seven clauses were
  proven by mutation (four mutations, each repaired and sha256-verified). The WAL acceptance test
  also pins the counterexample: a naive `.db` copy of the same live database opens cleanly and
  reads zero rows. Parent verification, independent of the report: only the new directory appears
  in `git status`, `go.mod`/`go.sum` unchanged, package tests pass, **full suite exits 0**,
  `gofmt` and `vet` clean. One branch is declared untested rather than faked: removing the file
  when `VACUUM` succeeded but validation failed is unreachable with real output and shares the
  helper that mutation M2 proves load-bearing.
- **L31** (evidence, T4): delegated, committed as `0a61914` — `backup.go` 604 + `backup_test.go`
  564 = 1168 lines, again above the slice target, and the command's own tests are why. Key layout
  is `db/study-20060102T150405Z.db` (fixed width, UTC, so lexicographic order is chronological —
  retention depends on it) plus `artifacts/<id>/payload|meta.json`, all relative to the configured
  prefix, which the client applies in one place. The snapshot is always PUT and never skipped
  because its key is fixed to the second; artifacts are skipped via **one List per prefix instead
  of one Head per file**, so requests do not grow with the artifact count (a test asserts zero Head
  calls). Retention deletes only keys matching its own shape, so a foreign object under `db/`
  survives. `-dry-run` deliberate deviation, stated in the command's help: it **does** take the
  snapshot (outside the repository, removed before returning) because the byte total it prints
  would otherwise be a guess — a `VACUUM INTO` snapshot is not the size of the live `.db` file. It
  uploads nothing and writes nothing under `data/`. Declared risks, none hidden: two backups inside
  the same second address one key and the newer bytes replace the older; retention is best-effort,
  so a failed listing or delete is reported on stderr without changing the exit code, which means a
  long-lived bucket can exceed 14 snapshots while every run still reports success; and opening the
  database read-only still leaves `study.db-shm` plus a zero-byte `study.db-wal` beside it when the
  server is not running — the same side effect `search` already has, and a tension with the help
  text's "only reads it" that a reviewer should look at. Parent verification, independent of the
  report: exactly the three permitted paths changed, the `cli.go` diff is **one added line**, `go.mod`
  and `go.sum` unchanged, `vet`/`gofmt` clean, the cli tests pass, the full suite exits 0, and the
  command was exercised live — `backup` with the four variables unset exits 1 naming all four, which
  is S8 behaving as designed. The writer also ran a differential check of every other command
  against `b71fe62` and found only the intended usage line plus the two now-known `backup`
  invocations, byte-identical everywhere else.
- **L32** (evidence, T5 — the high-risk unit): committed as `ff3ff02`. `assess` was called with
  `escalate: {item: 1}`, and the escalation did exactly what it promises: `risk` went medium →
  `high`, `agentEscalation.applied: true`, and `plan.independentVerifier` flipped false → **true**,
  while `reviewDue` stayed false (`under_budget`). So no native review, but an independent verifier
  ran — the one case where I knew something the line count could not. Before that, my own
  **mutation test** on the most important property: replacing `moveAside`'s source list with the
  database alone made
  `TestRestore_MovesThePreviousDatabaseAndItsSidecarsAsideSoItsWALCannotBeApplied` **fail**
  (`…pre-restore-…-wal: no such file or directory`) and the file was restored byte-identically
  (`sha256 a1d34c1d…` before and after). That is the test catching a deliberately injected bug, not
  a test passing.
- **L33** (independent verification, no blockers): `gentle-ai-verify` was given seven adversarial
  questions and answered them against its own scratch probe suite, not against the author's tests.
  Verdict: WAL property **holds** (live path left with no `-wal`/`-shm`/`-journal`, restored content
  reads back, previous pair byte-identical and reopenable); rollback **holds with caveat** (every
  injected rename failure 2–5 and the commit left the home byte-identical); validate-before-touch
  **holds** (six invalid snapshots, empty stdout, same inode, no leftovers); confirmation **holds**
  (no `-yes` → exit 1 with the tree unchanged; malformed `-from` → zero GETs); temp placement
  **holds** (same `st_dev`, cross-device rename unreachable); artifacts **hold with caveat**.
  Suite exit codes confirmed 0 for both commands. Seven non-blocking findings were recorded, and
  two of them are why L33 is worth more than a rubber stamp:
  **F2 — a crash window between the database move and the sidecar moves leaves `-wal`/`-shm` with
  no database, and the next open creates an empty database and deletes the WAL, so those rows are
  gone.** The verifier probed that state and reports the plan text overstates what is left there.
  The move order narrows the window rather than eliminating it, and the mirror order would be worse.
  **F7 — restore does not detect a running server**: with an open handle, the restore succeeds and a
  later write through the old handle lands in the `.pre-restore-` copy while the live database shows
  the restored rows. Silent divergence. That is S4's "documented, not enforced" bound showing up in
  the one command that could enforce it, and it belongs to T6's documentation.
  The rest: F1 an inverted comment in `putBack` (fixed in this commit — the trace shows sidecars
  restored before the database, the comment claimed the reverse, and a future reader could
  "correct" the order and break the rollback); F3 no parent-directory fsync after the renames; F4 a
  killed run leaves a `.study-restore-*.db` in `data/` with no sweep; F5 orphan sidecars are moved
  aside but the plan prints `previous: none`; F6 a partial artifact failure exits 0, so automation
  cannot see missing PDFs. F2–F6 are queued rather than rushed at the end of a long turn: changing
  the data-destroying path deserves its own deliberation and test, and the verifier also handed over
  four green regression tests (sidecar move failures at renames 2–4, a second restore in the same
  second, orphan sidecars, a hot rollback journal) that cover gaps the current suite does not reach.
- **L34** (evidence, F2 and F7 fixed): committed as `4f4aad7`. **F2 — the data-loss window — is
  closed by removing its premise rather than narrowing it.** After the `-yes` gate and immediately
  before the first move, `PRAGMA wal_checkpoint(TRUNCATE)` folds the log into the database, so the
  window between the database move and the sidecar moves has nothing unique left to lose. The
  checkpoint deliberately runs *after* `-yes`, because it writes to the live database and "without
  -yes nothing changes" is an asserted property; it is skipped when no database exists; and a
  failure is a **warning, not a refusal**, because a locked or damaged database is exactly when a
  restore is needed. `db.Open` is used rather than `composition.OpenDatabase` so the checkpoint
  cannot migrate a schema on a database about to be replaced. The test was proved load-bearing by
  mutation, not by passing: with the checkpoint removed it fails on
  "the WAL must be truncated before the moves, or a crash here loses its rows", and `restore.go`
  was restored byte-identically (`sha256 08ef35d6…`). Two existing tests then **failed**, and that
  failure was the point: they asserted the weaker, older property that the previous `-wal` travelled
  with its bytes. They now assert the stronger one — the WAL-only row is inside the previous
  database file, and the sidecar holds nothing unique. **F7 is fixed too**: the synopsis tells the
  operator to stop the server first and says what happens if they do not.
- **L35** (incident, subagents unavailable): three writer delegations failed in a row with the same
  opaque "assistant reported an error", two of them independent tasks, so it is not task size. The
  worktree was verified untouched after each failure (`git status` empty, build and tests green), so
  nothing was half-written. Reported as unavailable rather than retried into ceremony, and F2 and F7
  were then done inline. Still open from L33: F3 (no parent-directory fsync), F4 (a killed run
  leaves a `.study-restore-*.db` with no sweep), F5 (the plan prints `previous: none` when only
  sidecars exist), F6 (a partial artifact failure exits 0 — a product decision), and the verifier's
  four regression tests, which are handed over below rather than lost.
- **L36** (evidence, T7 — the rehearsal): `gentle-ai-verify` ran the whole cycle against a local
  S3 double it wrote itself, in `mktemp` directories, reading a **copy** of the real `data/` and
  never writing to the original (source `study.db` sha256 verified unchanged afterwards). The
  cycle: `backup -dry-run` reported 11 objects / 30,133,700 bytes and left the double's storage
  **empty**; `backup` uploaded those 11; a second `backup` uploaded only the snapshot and skipped
  the 10 artifacts as already present; the stored snapshot was opened with sqlite and held
  **3 topics, 9 flashcards, 1180 chunks, 1180 chunk_embeddings** — identical to the copy, with
  `integrity_check ok`; the database and its sidecars were then moved away to simulate the
  disaster; `restore` without `-yes` printed the plan, exited **1** and left a byte-identical
  tree; `restore -yes` restored a database whose sha256 **equals the snapshot's**
  (`dc019bad…`), with the same row counts, no sidecars at the live path, and all 10 artifact
  files byte-identical to the source manifest. A supplemental run with a database already present
  exercised the path the main run could not: the plan named `previous: …pre-restore-…`, that copy
  held the marker the live database did not, and the checkpoint's `TRUNCATE` was observed as a
  0-byte `-wal` travelling with it. **The end-to-end claim now rests on a rehearsal, not on
  inference.**
- **L37** (defect found by the rehearsal, fixed): the rehearsal refuted two user-facing claims, and
  a `-h` that lies is worse than one that omits. `backup -h` said "this command only reads it", but
  opening a WAL database leaves `study.db-shm` (32 KB) and a zero-byte `study.db-wal` beside it when
  the server is not running — the same side effect `search` already has. The synopsis now says so,
  and the README's `-dry-run` line says "uploads nothing" instead of "writing nothing". The help
  test was changed to **assert the disclosure** rather than the old slogan, so the next person
  cannot quietly restore the false claim.
- **L38** (explicitly not proven): stated here so the PR does not imply more than it has. **Real
  Cloudflare R2**: SigV4 acceptance, real error codes and bodies, TLS, and any tolerance in its
  signature checking — none of it has been exercised, because no credentials exist here. **R2
  behaviour**: `ListObjectsV2` pagination past 1000 keys (the double always answered
  `IsTruncated=false`), and eventual consistency. **Untouched paths**: retention's `DELETE` (only
  two snapshots existed, nothing was pruned), `restore -from KEY`, the client's `HEAD`/404-on-missing
  paths, and multipart or large-object limits. **Concurrency**: S4's single-writer bound is
  documented and not enforced; nothing tested two writers. One observation recorded as benign: the
  snapshot is `journal_mode=delete` while the live database is `wal`, and `db.Open` re-applies WAL
  on the next application open.
- **L39** (evidence, verifier tests and F3/F4/F5): the verifier's five functions now live in
  `restore_durability_test.go`, and two deviations from its text are reported rather than buried.
  (a) `TestRestore_MovesOrphanSidecarsAsideWhenNoDatabaseExists` **did not compile as handed over**
  (`declared and not used: walBefore`), so the value is discharged with `_ = walBefore`. (b)
  `TestRestore_MovesAHotRollbackJournalAsideWithTheDatabase` **cannot pass as written any more**,
  and the cause is proved, not guessed: the F2 checkpoint opens the live database with `db.Open`,
  whose DSN pragmas include `journal_mode(WAL)`, and switching journal mode makes SQLite roll the
  hot journal back and delete it before `moveAside` runs. So there are no journal bytes left to
  travel. **Disposition A**, the same one L34 applied to two other superseded assertions: the
  scenario and fixture stay, the last assertion moves to the property that now holds — the aborted
  transaction was rolled back, so nothing can be applied to an empty database. That is the safer
  outcome, and `TestRestore_ConsumesAHotRollbackJournalBeforeTheMoves` pins it. The alternative
  (skip the checkpoint when a `-journal` exists) was rejected: whichever order the moves take, a
  crash can leave a hot journal at the live path with no database, which is the corruption F2
  removed. **F3/F4/F5 are fixed with test-first coverage**: six new tests, and each was observed
  failing first (F3 as `undefined: syncDir`, F4 and F5 as real assertion failures). F3 fsyncs the
  containing directory after the commit rename and deliberately does not return the flush error,
  because the rename already happened. F4 sweeps leftover `.study-restore-*.db` before downloading,
  matching prefix+suffix only, unlinking symlinks rather than following them, and reporting the
  count. F5 names orphan sidecars in the plan and on completion, so an operator can find what
  moved. Declared limits: F4 is proved against a synthetic leftover rather than a real SIGKILL
  mid-download; F3's durability is observable only through its seam, and a directory fsync on
  darwin does not flush the drive's own cache, so power-loss durability is asserted by
  construction; and the sweep silently skips a leftover it cannot unlink. One behaviour note: a
  restore refused without `-yes` now also clears a killed run's leftovers, because the sweep must
  precede the download and the download precedes the gate — the live database and its sidecars are
  untouched, which the existing test still asserts.
