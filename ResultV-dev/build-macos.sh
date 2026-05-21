#!/usr/bin/env bash
# Build the macOS .app bundle and produce a .dmg in build/bin/.
#
# Optional environment:
#   SUBSCRIPTION_ENCRYPT_KEY   — pinned at link time if set.
#   APPLE_DEVELOPER_ID         — codesign identity (e.g., "Developer ID Application: …").
#                                If unset, the .app is left ad-hoc-signed and macOS
#                                will require a Gatekeeper override on first launch.
#   APPLE_NOTARY_APPLE_ID      — Apple ID for notarytool submission (optional).
#   APPLE_NOTARY_TEAM_ID       — team ID for notarytool (optional).
#   APPLE_NOTARY_PASSWORD      — app-specific password for notarytool (optional).

set -euo pipefail

OUT_DIR="build/bin"
APP_NAME="ResultV.app"
APP_PATH="${OUT_DIR}/${APP_NAME}"

# Auto-load local .env when the key is not already provided.
if [ -z "${SUBSCRIPTION_ENCRYPT_KEY:-}" ] && [ -f ".env" ]; then
  echo "==> loading env from .env"
  set -a
  # shellcheck disable=SC1091
  source ".env"
  set +a
fi

LDFLAGS=""
if [ -n "${SUBSCRIPTION_ENCRYPT_KEY:-}" ]; then
  LDFLAGS="-X resultproxy-wails/internal/proxy.subscriptionEncryptKey=${SUBSCRIPTION_ENCRYPT_KEY}"
fi
if [ -n "${MANIFEST_URL_OVERRIDE:-}" ]; then
  LDFLAGS="${LDFLAGS} -X resultproxy-wails/internal/updater.ManifestURLOverride=${MANIFEST_URL_OVERRIDE}"
fi

echo "==> wails build (darwin/universal)"
if [ -n "$LDFLAGS" ]; then
  wails build -clean -platform darwin/universal -ldflags "$LDFLAGS"
else
  wails build -clean -platform darwin/universal
fi

if [ ! -d "$APP_PATH" ]; then
  echo "ERROR: ${APP_PATH} not produced" >&2
  exit 1
fi

echo "==> stage libcronet.dylib (sing-box naive / Cronet)"
LIBCRONET_SRC="build/darwin/libcronet.dylib"
if [ ! -f "$LIBCRONET_SRC" ]; then
  echo "ERROR: $LIBCRONET_SRC not found — run scripts/ensure-libcronet-macos.sh" >&2
  exit 1
fi
cp -f "$LIBCRONET_SRC" "${APP_PATH}/Contents/MacOS/"

if [ -n "${APPLE_DEVELOPER_ID:-}" ]; then
  echo "==> codesign libcronet.dylib"
  codesign --force --options runtime --timestamp \
    --sign "$APPLE_DEVELOPER_ID" "${APP_PATH}/Contents/MacOS/libcronet.dylib"
  echo "==> codesign app"
  codesign --force --deep --options runtime --timestamp \
    --sign "$APPLE_DEVELOPER_ID" "$APP_PATH"
else
  echo "==> codesign skipped (APPLE_DEVELOPER_ID not set)"
fi

if [ -n "${APPLE_NOTARY_APPLE_ID:-}" ] && [ -n "${APPLE_NOTARY_TEAM_ID:-}" ] && [ -n "${APPLE_NOTARY_PASSWORD:-}" ]; then
  echo "==> notarize"
  ZIP_PATH="${OUT_DIR}/ResultV-notary.zip"
  ditto -c -k --keepParent "$APP_PATH" "$ZIP_PATH"
  xcrun notarytool submit "$ZIP_PATH" \
    --apple-id "$APPLE_NOTARY_APPLE_ID" \
    --team-id "$APPLE_NOTARY_TEAM_ID" \
    --password "$APPLE_NOTARY_PASSWORD" \
    --wait
  xcrun stapler staple "$APP_PATH"
  rm -f "$ZIP_PATH"
else
  echo "==> notarize skipped (Apple notary creds not set)"
fi

echo "==> create .dmg"
DMG_PATH="${OUT_DIR}/ResultV.dmg"
rm -f "$DMG_PATH"
hdiutil create -volname "ResultV" -srcfolder "$APP_PATH" -ov -format UDZO "$DMG_PATH"

echo "Done: $DMG_PATH"
