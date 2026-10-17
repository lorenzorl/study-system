# Step 3 — Artifact storage

**Status:** done
**Created:** 2026-10-17
**Depends on:** step 1 (`platform/config`), step 2 (`platform/db`)

## Why

The ingestion pipeline needs somewhere to put the document it receives, the page
renders it produces, and the Markdown it generates. There was no such concept:
`data/` held only the SQLite file, and a pipeline without a storage port reads and
writes hardcoded paths — the fault in the standalone `optimizebooks` script, whose
input path, page range, and output path were all literals in the source.

## Delivered

- `internal/platform/storage`: `Store` with `Put`, `Open`, `Path`, `Directory`,
  `Stat`, `Workspace`, `Delete`, `List`.
- `internal/platform/apperror`: `ErrTooLarge`, mapped to HTTP 413 in
  `httpx.WriteErrorFrom`.
- `internal/platform/config`: `STUDY_STORAGE_ROOT`, `STUDY_MAX_UPLOAD_BYTES`,
  plus `ParseMaxUploadBytes` so a flag can apply the same rule.
- `cmd/ingest`: the first pipeline slice, acquire and persist.

## Disk layout

```text
<root>/<artifact-id>/payload     the bytes
<root>/<artifact-id>/meta.json   original name, size, timestamp
<root>/<artifact-id>/work/       per-artifact scratch space
<root>/incoming/                 staging for in-flight writes
```

## Design: traversal removed, not filtered

The artifact identifier is generated and validated against a strict UUID pattern
on every read path. It is the only value that reaches a path. The original
filename never becomes one: it is stored as metadata, reduced to its base name.

The **payload filename is fixed** (`payload`). This was a correction forced by a
test. The first implementation derived the stored extension from the original
name, and fell back to re-deriving it when the recorded name failed validation —
which meant tampering with *both* `meta.json` fields made the artifact unreadable.
Fixing the filename removed the failure mode entirely: the payload location is now
independent of metadata, so tampering cannot redirect a read. The extension was
cosmetic anyway; a tool that needs a format hint reads `Artifact.OriginalName`.

Other invariants:

- Writes stage in `<root>/incoming/<id>/` and are renamed into place, on the same
  filesystem, so the commit is atomic. Any failure path removes the staging
  directory, so a rejected or interrupted transfer leaves nothing behind.
- Content is streamed in 64 KiB chunks with a context check per iteration. The
  ceiling is enforced by reading one byte past it, which detects overflow without
  consuming the rest of the source.

## Verification

| Check | Result |
| --- | --- |
| `gofmt -l`, `go vet ./...` | clean |
| `func Test` (backend) | 160 -> **194** (+29 storage, +5 config) |
| Real PDF, 13,957,202 bytes | stored; **stored sha256 identical to the source** |
| Original file after ingest | sha256 unchanged |
| Over the limit (1 KiB limit, 1.4 MB file) | `ErrTooLarge`, message names the env var, **0 artifacts left** |
| Hostile name `..leading.dots..pdf` | stored under the generated UUID; payload named `payload`; name kept as metadata |
| Directory as input | rejected as invalid input |
| Missing file | clear error |
| `-list` on an empty store | reports emptiness |
| No `-pdf` and no `-list` | usage error |

Round trip is byte-exact for binary content, and the suite covers 11 hostile
original names (`../../etc/passwd`, absolute paths, backslash escapes, `.`, `..`,
empty) and 10 malformed identifiers.

## Notes

- The store owns its root: entries in it that are not valid artifacts are skipped
  by `List` rather than reported, so stray files cannot break listing.
- `Path` exists because external binaries consume files, not readers. It is the
  only way a path leaves the store, and it always comes from a validated id.
- Not built: retention/GC policy, and the HTTP multipart upload endpoint. The
  latter belongs to the RAG context, together with the body-size middleware it
  needs.

## Follow-ups

Recorded in [`backlog.md`](backlog.md), the single source of truth. The artifact
retention policy this step opened lives there as `P1`.
