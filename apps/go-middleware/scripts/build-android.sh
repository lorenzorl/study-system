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
# It installs to ~/study-system/bin/study-system by default, which is the layout a
# consolidated setup uses: STUDY_HOME=~/study-system holds .env and data/ while
# bin/ and skill/ sit beside them and are never read by the CLI.
#
# The build runs on the device and takes a few minutes: modernc.org/sqlite is a
# very large generated package and the phone has little RAM. The script limits
# parallelism and memory for that reason.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DEVICE="${DEVICE:-}"
PORT="${PORT:-8022}"
INSTALL_TO="${INSTALL_TO:-~/study-system/bin/study-system}"
LIBC_VERSION="${LIBC_VERSION:-}"
WORK="${WORK:-/tmp/study-system-android}"

if [ -z "$DEVICE" ]; then
    echo "error: DEVICE is not set." >&2
    echo "  usage: DEVICE=user@host $0" >&2
    echo "  the target must be a Termux host with 'pkg install golang clang poppler'" >&2
    exit 1
fi

# The libc patch is applied with python3 and the tree is staged with rsync. Both
# must exist before the script starts mutating anything.
for required in python3 rsync; do
    command -v "$required" >/dev/null || {
        echo "error: $required is required on this machine to run the build" >&2
        exit 1
    }
done

# WORK is removed recursively, so only a scoped directory under /tmp is accepted:
# a mistyped value must not be able to delete a broader tree.
case "$WORK" in
    /tmp/?*) ;;
    *)
        echo "error: WORK must be a directory under /tmp (got: $WORK)" >&2
        exit 1
        ;;
esac

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

# The patch rewrites one per-architecture generated file, so the device's own
# toolchain decides which one. Refuse early rather than shipping a binary whose
# SQLite path still calls statx.
DEVICE_GOARCH="$(ssh -p "$PORT" -o BatchMode=yes -o ConnectTimeout=10 "$DEVICE" 'go env GOARCH' 2>/dev/null || true)"
[ -n "$DEVICE_GOARCH" ] || {
    echo "error: could not read 'go env GOARCH' on $DEVICE (is golang installed there?)" >&2
    exit 1
}
if [ "$DEVICE_GOARCH" != "arm" ]; then
    echo "error: only android/arm (32-bit) is supported today." >&2
    echo "  $DEVICE reports GOARCH=$DEVICE_GOARCH, which would instead need" >&2
    echo "  ccgo_linux_${DEVICE_GOARCH}.go patched and reviewed on its own." >&2
    exit 1
fi
LIBCCGO="ccgo_linux_${DEVICE_GOARCH}.go"

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
python3 - "$PATCHED_LIBC/$LIBCCGO" <<'PY'
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
grep -q 'patched: Android seccomp' "vendor/modernc.org/libc/$LIBCCGO" || {
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
  # A binary whose SQLite path still calls statx answers help perfectly well, so
  # exercise the database too. The expected failure is 'not found' (a fresh home),
  # never SIGSYS.
  DBHOME=\$(mktemp -d)
  dbout=\$(STUDY_HOME=\$DBHOME \$HOME/study-system-native review smoke-probe -grade 3 2>&1 || true)
  rm -rf \$DBHOME
  case \$dbout in
    *SIGSYS*)
      echo 'error: the SQLite path crashes with SIGSYS; the libc patch did not take effect' >&2
      exit 1
      ;;
  esac
  echo '    smoke: SQLite path opens without SIGSYS'
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
