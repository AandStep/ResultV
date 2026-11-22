#!/usr/bin/env bash
# Build and sign the nested WidgetKit extension and its host reload library.
set -euo pipefail

APP_PATH="$1"
: "${APPLE_DEVELOPER_ID:?Developer ID Application identity required}"
SOURCE="build/darwin/ControlCenter"
PRODUCTS="$2"
INFO="$APP_PATH/Contents/Info.plist"

TEAM_ID="$(codesign -dvv "$APP_PATH/Contents/MacOS/libcronet.dylib" 2>&1 | sed -n 's/^TeamIdentifier=//p')"
if [[ ! "$TEAM_ID" =~ ^[A-Z0-9]{10}$ ]]; then
  echo "ERROR: Developer ID signing did not produce a valid Team ID" >&2
  exit 1
fi
GROUP="$TEAM_ID.rvctl"
APP_ID="$(plutil -extract CFBundleIdentifier raw "$INFO")"
VERSION="$(plutil -extract CFBundleShortVersionString raw "$INFO")"
BUILD="$(plutil -extract CFBundleVersion raw "$INFO")"
mkdir -p "$PRODUCTS" "$APP_PATH/Contents/PlugIns" "$APP_PATH/Contents/Frameworks"

echo "==> build ResultV Control Center extension"
xcodebuild -project "$SOURCE/ResultVControl.xcodeproj" -target ResultVControl \
  -configuration Release -quiet SYMROOT="$PRODUCTS" OBJROOT="$PRODUCTS/obj" \
  'ARCHS=arm64 x86_64' PRODUCT_BUNDLE_IDENTIFIER="$APP_ID.Control" \
  MARKETING_VERSION="$VERSION" CURRENT_PROJECT_VERSION="$BUILD" RESULTV_CONTROL_GROUP="$GROUP" build
EXTENSION="$APP_PATH/Contents/PlugIns/ResultVControl.appex"
ditto "$PRODUCTS/Release/ResultVControl.appex" "$EXTENSION"

echo "==> build WidgetKit reload library"
for arch in arm64 x86_64; do
  xcrun swiftc -target "$arch-apple-macos26.0" -emit-library -O \
    -module-cache-path "$PRODUCTS/module-cache" \
    -module-name ResultVControlReload "$SOURCE/ReloadControl.swift" \
    -o "$PRODUCTS/ResultVControlReload-$arch.dylib"
done
LIBRARY="$APP_PATH/Contents/Frameworks/ResultVControlReload.dylib"
xcrun lipo -create "$PRODUCTS/ResultVControlReload-arm64.dylib" \
  "$PRODUCTS/ResultVControlReload-x86_64.dylib" -output "$LIBRARY"

plutil -insert ResultVControlGroup -string "$GROUP" "$INFO"
cat > "$PRODUCTS/host.entitlements" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.application-groups</key>
    <array><string>$GROUP</string></array>
</dict>
</plist>
PLIST
cp "$PRODUCTS/host.entitlements" "$PRODUCTS/extension.entitlements"
/usr/libexec/PlistBuddy -c 'Add :com.apple.security.app-sandbox bool true' "$PRODUCTS/extension.entitlements"

echo "==> codesign Control Center extension and library"
codesign --force --options runtime --timestamp --sign "$APPLE_DEVELOPER_ID" "$LIBRARY"
codesign --force --options runtime --timestamp --sign "$APPLE_DEVELOPER_ID" \
  --entitlements "$PRODUCTS/extension.entitlements" "$EXTENSION"
