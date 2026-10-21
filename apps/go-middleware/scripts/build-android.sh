#!/usr/bin/env bash
# Build study-system for Android/Termux, natively on the device.
#
# WHY THIS EXISTS
# ---------------
# Android runs apps under a seccomp filter that answers a forbidden syscall with
# SECCOMP_RET_KILL: the process dies with SIGSYS instead of receiving ENOSYS, so
# no fallback path ever runs. Two consequences shape this script:
#
# 1. modernc.org/libc bundles musl's fstatat, which probes statx (397) first and
#    falls back to fstatat64 (327) only on ENOSYS. This kernel has no statx, so
#    every command that opens SQLite on disk dies before the fallback. The script
#    patches a COPY of the dependency (see "The libc patch" below); the
#    repository's go.mod is never modified.
# 2. A CGO_ENABLED=0 build uses Go's pure-Go resolver, which reads
#    /etc/resolv.conf. Android has no such file (it resolves through netd), so
#    every network call fails with "lookup host on [::1]:53: connection refused".
#    Building on the device with cgo makes net use bionic's getaddrinfo, which
#    works. That is why this script compiles ON the phone rather than
#    cross-compiling from a workstation.
#
# Building with GOOS=android also removes two other seccomp problems for free:
# Go's own android guards cover faccessat2 (439, used by exec.LookPath) and
# pidfd_open (434, used by os.startProcess).
#
# REQUIREMENTS ON THE DEVICE
# --------------------------
#   pkg install golang clang poppler
#   sshd running and reachable (Termux: `sshd`, default port 8022)
#
# USAGE
# -----
#   DEVICE=user@host ./scripts/build-android.sh
#   DEVICE=user@host PORT=8022 INSTALL_TO=~/bin ./scripts/build-android.sh
#   DEVICE=user@host SKIP_INSTALL=1 ./scripts/build-android.sh   # build only
#
# The build runs on the device and takes a few minutes: modernc.org/sqlite is a
# very large generated package and the phone has little RAM. The script limits
# parallelism and memory for that reason.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DEVICE="${DEVICE:-}"
PORT="${PORT:-8022}"
INSTALL_TO="${INSTALL_TO:-~/study-system}"
LIBC_VERSION="${LIBC_VERSION:-}"
WORK="${WORK:-/tmp/study-system-android}"

if [ -z "$DEVICE" ]; then
    echo "error: DEVICE is not set." >&2
    echo "  usage: DEVICE=user@host $0" >&2
    echo "  the target must be a Termux host with 'pkg install golang clang poppler'" >&2
    exit 1
fi

MODCACHE="$(go env GOMODCACHE)"

# Resolve the libc version from go.mod so this never drifts from the dependency
# the module actually uses.
if [ -z "$LIBC_VERSION" ]; then
    LIBC_VERSION="$(go list -m -f '{{.Version}}' modernc.org/libc 2>/dev/null || true)"
fi
[ -n "$LIBC_VERSION" ] || { echo "error: cannot resolve modernc.org/libc version from go.mod" >&2; exit 1; }

LIBC_SRC="$MODCACHE/modernc.org/libc@$LIBC_VERSION"
[ -d "$LIBC_SRC" ] || {
    echo "error: modernc.org/libc@$LIBC_VERSION is not in the module cache." >&2
    echo "  run: (cd $MODULE_DIR && go mod download)" >&2
    exit 1
}

PATCHED_LIBC="$WORK/patched-libc"
SRC="$WORK/src"

echo "==> staging a build tree in $WORK (the repository is not modified)"
rm -rf "$WORK"
mkdir -p "$SRC"
# Copy without data/ or .env: neither belongs in a build artifact, and the .env
# holds credentials.
rsync -a --exclude 'data' --exclude '.env' --exclude 'bin' "$MODULE_DIR/" "$SRC/"

echo "==> patching modernc.org/libc@$LIBC_VERSION so fstatat skips statx"
cp -R "$LIBC_SRC" "$PATCHED_LIBC"
chmod -R u+w "$PATCHED_LIBC"
python3 - "$PATCHED_LIBC/ccgo_linux_arm.go" <<'PY'
import sys

path = sys.argv[1]
src = open(path, encoding="utf-8").read()
start = src.index("func X__fstatat(tls *TLS")
end = src.index("\nfunc ", start + 10)
body = src[start:end]

marker = "if uint32(4) < uint32(8) {"
if marker not in body:
    raise SystemExit(
        "error: the fstatat statx guard was not found in libc. The generated "
        "layout changed; re-inspect ccgo_linux_arm.go before patching."
    )

patched = "if false { // patched: Android seccomp kills statx with SIGSYS instead of ENOSYS"
open(path, "w", encoding="utf-8").write(src[:start] + body.replace(marker, patched, 1) + src[end:])
print("    patched", path)
PY

echo "==> vendoring with the patched libc so the device needs no network"
cd "$SRC"
cp go.mod build.mod
cp go.sum build.sum
printf '\nreplace modernc.org/libc => %s\n' "$PATCHED_LIBC" >> build.mod
go mod vendor -modfile=./build.mod
rm -f build.mod build.sum

# `go mod vendor` records the replace as a "=> path" suffix on the module line.
# The stock go.mod must not see that, so rewrite the module line to drop only the
# suffix and delete the trailing replace block. Do NOT delete every line
# containing "=>": that removes the module declaration itself and Go then reports
# "inconsistent vendoring".
sed -i.bak \
    -e "s|^# modernc\.org/libc v[0-9.]* => .*$|# modernc.org/libc $LIBC_VERSION|" \
    -e '/^# modernc\.org\/libc => /d' \
    vendor/modules.txt
rm -f vendor/modules.txt.bak

grep -q '^# modernc.org/libc ' vendor/modules.txt || {
    echo "error: vendor/modules.txt lost the libc declaration" >&2
    exit 1
}
grep -q 'patched: Android seccomp' vendor/modernc.org/libc/ccgo_linux_arm.go || {
    echo "error: the vendored libc does not carry the patch" >&2
    exit 1
}
echo "    vendor tree ready ($(du -sh vendor | cut -f1))"

echo "==> shipping the tree to $DEVICE"
tar czf "$WORK/src.tar.gz" -C "$WORK" src
scp -q -P "$PORT" -o BatchMode=yes -o ConnectTimeout=10 \
    "$WORK/src.tar.gz" "$DEVICE:/data/data/com.termux/files/home/src.tar.gz"

echo "==> building on the device (a few minutes; limited parallelism for RAM)"
ssh -p "$PORT" -o BatchMode=yes -o ConnectTimeout=15 "$DEVICE" "
  set -e
  cd \$HOME
  rm -rf native-src
  tar xzf src.tar.gz && rm -f src.tar.gz && mv src native-src
  cd native-src
  export CGO_ENABLED=1
  export GOMEMLIMIT=1000MiB
  go build -mod=vendor -p 2 -trimpath -ldflags='-s -w' -o \$HOME/study-system-native ./cmd/cli
  chmod +x \$HOME/study-system-native
  echo '    built:' \$(du -h \$HOME/study-system-native | cut -f1)
  \$HOME/study-system-native help >/dev/null && echo '    smoke: help exits 0'
"

if [ "${SKIP_INSTALL:-0}" = "1" ]; then
    echo "==> SKIP_INSTALL=1: binary left at $DEVICE:~/study-system-native"
    exit 0
fi

echo "==> installing to $DEVICE:$INSTALL_TO"
ssh -p "$PORT" -o BatchMode=yes -o ConnectTimeout=15 "$DEVICE" "
  set -e
  install -D \$HOME/study-system-native $INSTALL_TO 2>/dev/null ||
    { mkdir -p \$(dirname $INSTALL_TO) && cp \$HOME/study-system-native $INSTALL_TO; }
  chmod +x $INSTALL_TO
  $INSTALL_TO help >/dev/null && echo '    installed and runs'
"
echo "done."
