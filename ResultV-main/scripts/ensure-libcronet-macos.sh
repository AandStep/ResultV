#!/usr/bin/env bash
# Creates a universal libcronet.dylib (amd64 + arm64) in build/darwin/ for
# macOS app bundle packaging. Required for sing-box naive on macOS with
# with_purego (see docs/build-naive.md).
#
# Newer cronet-go releases ship libcronet.a (CGO) only in the darwin_* modules;
# upstream does not publish a prebuilt .dylib for Darwin. When .dylib is absent,
# we link one per architecture from libcronet.a using the same flags as
# libcronet_cgo.go in those modules, then merge with lipo.
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
STATIC_AMD64="$DIR_AMD64/libcronet.a"
STATIC_ARM64="$DIR_ARM64/libcronet.a"

# Matches github.com/sagernet/cronet-go/lib/darwin_{amd64,arm64}/libcronet_cgo.go
link_dylib_from_static() {
	local arch="$1"
	local mod_dir="$2"
	local out_dylib="$3"
	local static="$mod_dir/libcronet.a"
	if [ ! -f "$static" ]; then
		echo "ERROR: $static not found" >&2
		exit 1
	fi
	clang -arch "$arch" -dynamiclib \
		-Wl,-force_load,"$static" \
		-lbsm -lpmenergy -lpmsample -lresolv \
		-framework CoreFoundation -framework CoreGraphics -framework CoreText \
		-framework Foundation -framework Security -framework ApplicationServices \
		-framework AppKit -framework IOKit -framework OpenDirectory \
		-framework CFNetwork -framework CoreServices -framework Network \
		-framework SystemConfiguration -framework UniformTypeIdentifiers \
		-framework CryptoTokenKit -framework LocalAuthentication \
		-install_name "@loader_path/libcronet.dylib" \
		-o "$out_dylib"
}

if [ -f "$LIB_AMD64" ] && [ -f "$LIB_ARM64" ]; then
	lipo -create "$LIB_AMD64" "$LIB_ARM64" -output "$DEST"
elif [ -f "$STATIC_AMD64" ] && [ -f "$STATIC_ARM64" ]; then
	TMP=$(mktemp -d)
	trap 'rm -rf "$TMP"' EXIT
	link_dylib_from_static x86_64 "$DIR_AMD64" "$TMP/libcronet-amd64.dylib"
	link_dylib_from_static arm64 "$DIR_ARM64" "$TMP/libcronet-arm64.dylib"
	lipo -create "$TMP/libcronet-amd64.dylib" "$TMP/libcronet-arm64.dylib" -output "$DEST"
else
	echo "ERROR: expected libcronet.dylib or libcronet.a in both darwin_amd64 and darwin_arm64 module dirs" >&2
	echo "  amd64 dir: $DIR_AMD64" >&2
	echo "  arm64 dir: $DIR_ARM64" >&2
	exit 1
fi

echo "Created universal libcronet.dylib -> $DEST ($(wc -c < "$DEST" | tr -d ' ') bytes)"
