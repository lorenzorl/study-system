#!/usr/bin/env bash
#
# Build the web app and copy it into the server's embedded tree.
#
# The server binary ships the SPA (`internal/server/spa/dist.go`), so the copy is
# what `go build` sees. Run this after any change under `apps/web/src` — or let
# `go generate ./...` run it.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
web="$here/../web"
target="$here/internal/server/spa/dist"

if [ ! -d "$web" ]; then
  echo "sync-web: $web does not exist; nothing to embed" >&2
  exit 1
fi

echo "sync-web: building $web"
(cd "$web" && pnpm install --frozen-lockfile --silent && pnpm build)

# Replace the tree instead of copying over it: a hashed asset from a previous
# build would otherwise stay embedded forever.
rm -rf "$target"
mkdir -p "$target"
cp -R "$web/dist/." "$target/"

echo "sync-web: embedded $(find "$target" -type f | wc -l | tr -d ' ') files into $target"
