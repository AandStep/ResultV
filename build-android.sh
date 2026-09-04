#!/usr/bin/env bash
# build-android.sh — One-shot Android build mirroring the desktop's
# build-linux.sh / build-macos.sh pattern: source `.env`, rebuild the
# gomobile AAR with the subscription decryption key pinned, then assemble
# the APK.
#
# Usage:
#   На новой машине сперва: bash scripts/bootstrap.sh  (см. docs/PORTING.md)
#   bash build-android.sh                       # debug APK, full dist (arm64-v8a)
#   bash build-android.sh release               # release APKs per ABI, full dist
#   DIST=play bash build-android.sh release     # release AAB for Google Play
#   bash build-android.sh debug --install       # build + adb install on the connected device
#   bash build-android.sh release --install     # same for release (APK dists only)
#   ABI=armeabi-v7a bash build-android.sh       # override debug ABI
#
# DIST picks the distribution *and* the release format: `full` builds the
# per-ABI APKs handed out from the site, `play` builds the App Bundle the
# store requires. Both rebuild their own AAR first (see step 1 below).
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
# Distribution: DIST=full (default) or DIST=play — see scripts/build-android-aar.sh.
# Only the requested one is rebuilt: a gomobile bind takes minutes, and building
# the other variant nobody asked for costs that on every iteration.
DIST="${DIST:-full}"
echo "==> Step 1/2: rebuild gomobile AAR (dist=${DIST})"
DIST="${DIST}" bash "${REPO_ROOT}/scripts/build-android-aar.sh"

echo
echo "==> Step 2/2: gradle assemble (${VARIANT})"

# Gradle 9 auto-provisions JDK 17 via foojay when JAVA_HOME is unset; on a
# fresh Mac that download often fails and the build dies here. Reuse Android
# Studio's bundled JBR (same probe as the AAR script) so ./gradlew just runs.
if [[ -z "${JAVA_HOME:-}" ]]; then
    as_jbr="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
    if [[ -x "${as_jbr}/bin/java" ]]; then
        JAVA_HOME="${as_jbr}"
    elif command -v /usr/libexec/java_home >/dev/null 2>&1; then
        JAVA_HOME="$(/usr/libexec/java_home 2>/dev/null || true)"
    fi
fi
if [[ -n "${JAVA_HOME:-}" ]]; then
    export JAVA_HOME
    export PATH="${JAVA_HOME}/bin:${PATH:-}"
fi

cd "${REPO_ROOT}/android"

# Gradle task fragment for the flavour. Spelled out rather than built with
# ${DIST^} so this keeps running on macOS's bash 3.2. Without the flavour in
# the task name `assembleDebug` builds *both* dists and writes them under
# apk/<dist>/debug — while this script used to look in apk/debug, which still
# exists from before the flavours and holds a stale APK. Silently installing
# that is worse than failing, hence the explicit task and path.
case "${DIST}" in
    full) FLAVOR="Full" ;;
    play) FLAVOR="Play" ;;
    *)
        echo "ERROR: DIST must be 'full' or 'play', got '${DIST}'" >&2
        exit 1
        ;;
esac

case "${VARIANT}" in
    debug)
        ./gradlew "assemble${FLAVOR}Debug" "-Pdebug.abi=${ABI}"
        OUT_DIR="app/build/outputs/apk/${DIST}/debug"
        ;;
    release)
        # Play accepts only an App Bundle and re-signs it with Play App
        # Signing; the site distribution stays per-ABI APKs. The ABI splits in
        # app/build.gradle.kts stand down for bundle* tasks, because AGP
        # refuses to build an AAB while multi-APK is on.
        if [[ "${DIST}" == "play" ]]; then
            ./gradlew "bundle${FLAVOR}Release"
            OUT_DIR="app/build/outputs/bundle/${DIST}Release"
        else
            ./gradlew "assemble${FLAVOR}Release"
            OUT_DIR="app/build/outputs/apk/${DIST}/release"
        fi
        ;;
    *)
        echo "ERROR: unknown variant '${VARIANT}' (expected: debug | release)" >&2
        exit 1
        ;;
esac

echo
echo "✅ Build complete."
ls -lh "${OUT_DIR}"

# Optional: push to the connected device. Uses `adb install -r` so an
# existing install of the same package is upgraded in place (preserves the
# user's profiles / subscriptions / settings under filesDir).
if [[ "${INSTALL_FLAG}" == "--install" ]]; then
    if [[ "${VARIANT}" == "release" && "${DIST}" == "play" ]]; then
        echo "ERROR: --install has nothing to push — a play release is an .aab and adb installs APKs." >&2
        echo "       Use DIST=full for an installable release build." >&2
        exit 1
    fi
    APK_FILE=$(ls -1 "${OUT_DIR}"/*.apk | head -n1)
    if [[ -z "${APK_FILE}" ]]; then
        echo "ERROR: no APK found under ${OUT_DIR}" >&2
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
