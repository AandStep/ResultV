#!/usr/bin/env bash
# Copies libcronet.so from the Go module cache into build/linux/ for Linux
# binary packaging. Required for sing-box naive on Linux with with_purego
# (see docs/build-naive.md).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$REPO_ROOT/build/linux/libcronet.so"
mkdir -p "$(dirname "$DEST")"

go mod download "github.com/sagernet/cronet-go/lib/linux_amd64"

DIR=$(go list -m -json "github.com/sagernet/cronet-go/lib/linux_amd64" | grep '"Dir"' | head -1 | awk -F'"' '{print $4}')
SRC="$DIR/libcronet.so"

if [ ! -f "$SRC" ]; then echo "ERROR: $SRC not found" >&2; exit 1; fi

cp -f "$SRC" "$DEST"
echo "Copied libcronet.so -> $DEST ($(wc -c < "$DEST" | tr -d ' ') bytes)"
