#!/usr/bin/env bash
# Creates a universal libcronet.dylib (amd64 + arm64) in build/darwin/ for
# macOS app bundle packaging. Required for sing-box naive on macOS with
# with_purego (see docs/build-naive.md).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$REPO_ROOT/build/darwin/libcronet.dylib"
mkdir -p "$(dirname "$DEST")"

go mod download "github.com/sagernet/cronet-go/lib/darwin_amd64"
go mod download "github.com/sagernet/cronet-go/lib/darwin_arm64"

DIR_AMD64=$(go list -m -json "github.com/sagernet/cronet-go/lib/darwin_amd64" | grep '"Dir"' | head -1 | awk -F'"' '{print $4}')
DIR_ARM64=$(go list -m -json "github.com/sagernet/cronet-go/lib/darwin_arm64" | grep '"Dir"' | head -1 | awk -F'"' '{print $4}')

LIB_AMD64="$DIR_AMD64/libcronet.dylib"
LIB_ARM64="$DIR_ARM64/libcronet.dylib"

if [ ! -f "$LIB_AMD64" ]; then echo "ERROR: $LIB_AMD64 not found" >&2; exit 1; fi
if [ ! -f "$LIB_ARM64" ]; then echo "ERROR: $LIB_ARM64 not found" >&2; exit 1; fi

lipo -create "$LIB_AMD64" "$LIB_ARM64" -output "$DEST"
echo "Created universal libcronet.dylib -> $DEST ($(wc -c < "$DEST" | tr -d ' ') bytes)"
