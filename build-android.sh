#!/usr/bin/env bash
# build-android.sh — One-shot Android build mirroring the desktop's
# build-linux.sh / build-macos.sh pattern: source `.env`, rebuild the
# gomobile AAR with the subscription decryption key pinned, then assemble
# the APK.
#
# Usage:
#   bash build-android.sh                       # debug APK (arm64-v8a)
#   bash build-android.sh release               # release APK (requires keystore.properties)
#   bash build-android.sh debug --install       # build + adb install on the connected device
#   bash build-android.sh release --install     # same for release
#   ABI=armeabi-v7a bash build-android.sh       # override debug ABI
#
# Env (read from .env if present, then overridden by anything already
# exported in the current shell):
#   SUBSCRIPTION_ENCRYPT_KEY  Hex AES-GCM key. Without it resultv:// and
#                             RVSUB1 subscriptions fail to decrypt.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "${REPO_ROOT}"

# Load .env if present. `set -a` auto-exports every assignment, so the
# downstream AAR script and gradle pick the key up via the environment.
# Anything already exported in the user's shell wins (mirrors the desktop
# behaviour where CI env beats committed defaults).
if [[ -f "${REPO_ROOT}/.env" ]]; then
    echo "📄 Loading ${REPO_ROOT}/.env"
    set -a
    # shellcheck disable=SC1091
    source "${REPO_ROOT}/.env"
    set +a
fi

if [[ -z "${SUBSCRIPTION_ENCRYPT_KEY:-}" ]]; then
    echo "⚠️  SUBSCRIPTION_ENCRYPT_KEY not set (no .env, or .env missing the line) — resultv:// imports will fail in this build"
else
    # Mask all but the last 4 hex chars so we don't log the full secret.
    masked="****${SUBSCRIPTION_ENCRYPT_KEY: -4}"
    echo "🔐 SUBSCRIPTION_ENCRYPT_KEY = ${masked}"
fi

VARIANT="${1:-debug}"
INSTALL_FLAG="${2:-}"
ABI="${ABI:-arm64-v8a}"

echo
echo "==> Step 1/2: rebuild gomobile AAR"
bash "${REPO_ROOT}/scripts/build-android-aar.sh"

echo
echo "==> Step 2/2: gradle assemble (${VARIANT})"
cd "${REPO_ROOT}/android"

case "${VARIANT}" in
    debug)
        ./gradlew assembleDebug "-Pdebug.abi=${ABI}"
        APK_DIR="app/build/outputs/apk/debug"
        ;;
    release)
        ./gradlew assembleRelease
        APK_DIR="app/build/outputs/apk/release"
        ;;
    *)
        echo "ERROR: unknown variant '${VARIANT}' (expected: debug | release)" >&2
        exit 1
        ;;
esac

echo
echo "✅ Build complete."
ls -lh "${APK_DIR}"

# Optional: push to the connected device. Uses `adb install -r` so an
# existing install of the same package is upgraded in place (preserves the
# user's profiles / subscriptions / settings under filesDir).
if [[ "${INSTALL_FLAG}" == "--install" ]]; then
    APK_FILE=$(ls -1 "${APK_DIR}"/*.apk | head -n1)
    if [[ -z "${APK_FILE}" ]]; then
        echo "ERROR: no APK found under ${APK_DIR}" >&2
        exit 1
    fi
    echo
    echo "==> adb install -r ${APK_FILE}"
    if ! command -v adb >/dev/null 2>&1; then
        echo "ERROR: adb not on PATH. Add \"\$ANDROID_HOME/platform-tools\" or install Android Platform Tools." >&2
        exit 1
    fi
    adb install -r "${APK_FILE}"
    echo
    echo "📱 Installed. Launch via:"
    echo "   adb shell am start -n com.resultv.android/.MainActivity"
fi
