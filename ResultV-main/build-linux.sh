#!/usr/bin/env bash
# Build the Linux binary and bundle it as .deb/.rpm via nfpm, plus an .AppImage
# via linuxdeploy when available.
#
# Optional environment:
#   SUBSCRIPTION_ENCRYPT_KEY   — pinned at link time if set.
#   VERSION                    — overrides the version stamped into the packages
#                                (default: pulled from update.json).

set -euo pipefail

OUT_DIR="build/bin"
mkdir -p "$OUT_DIR"

if [ -z "${VERSION:-}" ]; then
  VERSION=$(node -e "process.stdout.write(require('./update.json').version)" 2>/dev/null || echo "0.0.0")
fi
export VERSION

LDFLAGS=""
if [ -n "${SUBSCRIPTION_ENCRYPT_KEY:-}" ]; then
  LDFLAGS="-X resultproxy-wails/internal/proxy.subscriptionEncryptKey=${SUBSCRIPTION_ENCRYPT_KEY}"
fi
if [ -n "${MANIFEST_URL_OVERRIDE:-}" ]; then
  LDFLAGS="${LDFLAGS} -X resultproxy-wails/internal/updater.ManifestURLOverride=${MANIFEST_URL_OVERRIDE}"
fi

echo "==> wails build (linux/amd64) version=$VERSION"
# webkit2_41 selects libwebkit2gtk-4.1 (Ubuntu 22.04+/24.04, Debian 12+).
# Drop the tag if you target distros that still ship webkit2gtk-4.0.
BUILD_TAGS="${WAILS_TAGS:-webkit2_41}"
if [ -n "$LDFLAGS" ]; then
  wails build -clean -platform linux/amd64 -tags "$BUILD_TAGS" -ldflags "$LDFLAGS"
else
  wails build -clean -platform linux/amd64 -tags "$BUILD_TAGS"
fi

BIN_PATH=$(find "$OUT_DIR" -maxdepth 2 -type f -name "ResultV" | head -n1)
if [ -z "$BIN_PATH" ]; then
  echo "ERROR: ResultV binary not produced under $OUT_DIR" >&2
  exit 1
fi

echo "==> stage libcronet.so (sing-box naive / Cronet)"
LIBCRONET_SRC="build/linux/libcronet.so"
if [ ! -f "$LIBCRONET_SRC" ]; then
  echo "ERROR: $LIBCRONET_SRC not found — run scripts/ensure-libcronet-linux.sh" >&2
  exit 1
fi
cp -f "$LIBCRONET_SRC" "$(dirname "$BIN_PATH")/"

echo "==> nfpm pkg (.deb)"
if command -v nfpm >/dev/null 2>&1; then
  nfpm pkg --packager deb --target "$OUT_DIR/" --config build/linux/nfpm.yaml
  nfpm pkg --packager rpm --target "$OUT_DIR/" --config build/linux/nfpm.yaml
else
  echo "WARN: nfpm not installed — skipping .deb/.rpm. Install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest"
fi

echo "==> AppImage"
if command -v linuxdeploy >/dev/null 2>&1; then
  APPDIR="$OUT_DIR/ResultV.AppDir"
  rm -rf "$APPDIR"
  mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/512x512/apps"
  cp "$BIN_PATH" "$APPDIR/usr/bin/resultv"
  cp -f "$LIBCRONET_SRC" "$APPDIR/usr/bin/"
  # AppImage requires Exec= to be a relative command (just "resultv"),
  # not the absolute /usr/bin/resultv used by the .deb/.rpm packages.
  sed 's|^Exec=.*|Exec=resultv %u|' build/linux/resultv.desktop \
    > "$APPDIR/usr/share/applications/resultv.desktop"
  cp public/logo.png "$APPDIR/usr/share/icons/hicolor/512x512/apps/resultv.png"
  ARCH=x86_64 linuxdeploy --appdir "$APPDIR" --output appimage --desktop-file "$APPDIR/usr/share/applications/resultv.desktop"
  mv ResultV*.AppImage "$OUT_DIR/" 2>/dev/null || true
else
  echo "WARN: linuxdeploy not installed — skipping AppImage. https://github.com/linuxdeploy/linuxdeploy/releases"
fi

echo "Done. Artifacts under $OUT_DIR/"
ls -la "$OUT_DIR/"
