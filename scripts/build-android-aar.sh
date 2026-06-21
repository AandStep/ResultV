#!/usr/bin/env bash
# build-android-aar.sh — Rebuild the gomobile AAR for the Android app.
#
# Usage:
#   ./scripts/build-android-aar.sh              # default tags
#   ./scripts/build-android-aar.sh --with-naive  # include NaiveProxy outbound
#
# Env:
#   SUBSCRIPTION_ENCRYPT_KEY  Hex-encoded 32-byte AES-GCM key embedded into
#                             internal/proxy.subscriptionEncryptKey at link
#                             time. Required to decode resultv:// deep-links
#                             and RVSUB1 subscription bodies. Without it the
#                             import path returns ErrSubscriptionKeyMissing
#                             and the user sees "could not decode the link".
#                             Mirrors the desktop release.yml ldflag.
#
# Prerequisites:
#   - Go toolchain (go.mod-compatible version)
#   - gomobile (sagernet fork): go install github.com/sagernet/gomobile/cmd/gomobile@v0.1.12
#   - Android SDK with NDK installed (ANDROID_HOME set, or android/local.properties)
#   - JDK 17+ (JAVA_HOME, or Android Studio's bundled JBR on macOS)
#   - gomobile init (run once per machine)
#
# Output: android/libs/libbox.aar

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT="${REPO_ROOT}/android/libs/libbox.aar"

# --- Auto-detect toolchain paths when not already exported -------------------
# macOS ships /usr/bin/javac as a stub that prompts to install Java; gomobile
# needs a real JDK. Android Studio's bundled JBR is the most common source on
# dev Macs, so we probe it before falling back to /usr/libexec/java_home.
if [[ -z "${JAVA_HOME:-}" ]]; then
    as_jbr="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
    if [[ -x "${as_jbr}/bin/javac" ]]; then
        JAVA_HOME="${as_jbr}"
    elif command -v /usr/libexec/java_home >/dev/null 2>&1; then
        JAVA_HOME="$(/usr/libexec/java_home 2>/dev/null || true)"
    fi
fi
if [[ -z "${JAVA_HOME:-}" || ! -x "${JAVA_HOME}/bin/javac" ]]; then
    echo "ERROR: Java (JDK) not found. Install JDK 17+ or set JAVA_HOME." >&2
    echo "       Android Studio's bundled JBR works:" >&2
    echo "         export JAVA_HOME=\"/Applications/Android Studio.app/Contents/jbr/Contents/Home\"" >&2
    exit 1
fi

# Gradle writes sdk.dir into android/local.properties; reuse it for gomobile.
if [[ -z "${ANDROID_HOME:-}" && -f "${REPO_ROOT}/android/local.properties" ]]; then
    sdk_dir="$(grep -E '^sdk\.dir=' "${REPO_ROOT}/android/local.properties" | cut -d= -f2- | tr -d '\r')"
    if [[ -n "${sdk_dir}" ]]; then
        ANDROID_HOME="${sdk_dir}"
    fi
fi
if [[ -z "${ANDROID_HOME:-}" ]]; then
    for candidate in "${HOME}/Library/Android/sdk" "${HOME}/Android/Sdk"; do
        if [[ -d "${candidate}" ]]; then
            ANDROID_HOME="${candidate}"
            break
        fi
    done
fi
if [[ -z "${ANDROID_HOME:-}" || ! -d "${ANDROID_HOME}" ]]; then
    echo "ERROR: Android SDK not found. Set ANDROID_HOME or sdk.dir in android/local.properties." >&2
    exit 1
fi

# Pick the newest NDK under the SDK when the caller didn't pin one.
if [[ -z "${ANDROID_NDK_HOME:-}" && -d "${ANDROID_HOME}/ndk" ]]; then
    ndk_ver="$(ls -1 "${ANDROID_HOME}/ndk" 2>/dev/null | sort -V | tail -n1)"
    if [[ -n "${ndk_ver}" ]]; then
        ANDROID_NDK_HOME="${ANDROID_HOME}/ndk/${ndk_ver}"
    fi
fi
if [[ -z "${ANDROID_NDK_HOME:-}" || ! -d "${ANDROID_NDK_HOME}" ]]; then
    echo "ERROR: Android NDK not found under ${ANDROID_HOME}/ndk. Install via SDK Manager." >&2
    exit 1
fi

# gomobile shells out to javac; make sure the resolved JDK is on PATH.
PATH="${JAVA_HOME}/bin:${PATH:-}"

# Base tags — always included
TAGS="mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard"

# Optional: NaiveProxy support (blocked by NDK 27.x ld.lld, see Known issues)
if [[ "${1:-}" == "--with-naive" ]]; then
    TAGS="${TAGS},with_naive_outbound"
    echo "⚠️  Including with_naive_outbound — requires compatible NDK/cronet-go"
fi

# Build the ldflags string. -checklinkname=0 is the gomobile baseline we've
# always needed; the -X injection is what teaches the binary to decode
# RVSUB1 / resultv:// payloads. When the env var is missing we emit a warning
# and continue — the build still works, the user just can't import encrypted
# subscriptions (same behaviour the desktop workflow falls back to).
LDFLAGS="-checklinkname=0"
if [[ -n "${SUBSCRIPTION_ENCRYPT_KEY:-}" ]]; then
    LDFLAGS="${LDFLAGS} -X resultproxy-wails/internal/proxy.subscriptionEncryptKey=${SUBSCRIPTION_ENCRYPT_KEY}"
    echo "🔐 Embedding subscription decryption key from SUBSCRIPTION_ENCRYPT_KEY"
else
    echo "⚠️  SUBSCRIPTION_ENCRYPT_KEY not set — resultv:// deep-links and RVSUB1 subscriptions will fail to decode in this build"
fi

echo "📦 Building AAR with tags: ${TAGS}"
echo "📁 Output: ${OUTPUT}"

mkdir -p "$(dirname "${OUTPUT}")"

cd "${REPO_ROOT}"

# Clean gomobile's per-ABI scratch dirs from any prior aborted run. gomobile
# emits go_libboxmain.go etc. into ./build/{386,amd64,arm,arm64}/libgojni/
# and refuses to overwrite ("The file exists") if a previous bind crashed
# mid-write. These are pure scratch — wails uses build/bin, build/windows,
# build/darwin, so the per-ABI subdirs are safe to delete.
for arch in 386 amd64 arm arm64; do
    if [[ -d "${REPO_ROOT}/build/${arch}" ]]; then
        rm -rf "${REPO_ROOT}/build/${arch}"
    fi
done

# Windows / Git Bash workaround: MSYS forwards cmd.exe pseudo env vars
# whose NAME starts with `=` (`=C:`, `=D:`, …, `=::`, `=ExitCode`). They
# encode cmd.exe's "current dir per drive". gomobile@v0.1.12 parses
# os.Environ() by splitting on `=` and panics on the empty-key entries
# ("panic: malformed env var \"=::=::\\\""). coreutils `env -u` refuses
# to unset names beginning with `=` ("Invalid argument"), so we go via
# `env -i` (clear everything) and re-pass an explicit whitelist of vars
# gomobile/go/NDK actually need. On Linux/macOS the unset names are simply
# empty and the whitelist still works.
env -i \
    PATH="${PATH:-}" \
    HOME="${HOME:-}" \
    TMPDIR="${TMPDIR:-}" \
    TEMP="${TEMP:-}" \
    TMP="${TMP:-}" \
    LANG="${LANG:-}" \
    LC_ALL="${LC_ALL:-}" \
    USER="${USER:-}" \
    USERNAME="${USERNAME:-}" \
    USERPROFILE="${USERPROFILE:-}" \
    LOCALAPPDATA="${LOCALAPPDATA:-}" \
    APPDATA="${APPDATA:-}" \
    PROGRAMFILES="${PROGRAMFILES:-}" \
    PROGRAMDATA="${PROGRAMDATA:-}" \
    SYSTEMROOT="${SYSTEMROOT:-}" \
    SYSTEMDRIVE="${SYSTEMDRIVE:-}" \
    WINDIR="${WINDIR:-}" \
    PATHEXT="${PATHEXT:-}" \
    COMSPEC="${COMSPEC:-}" \
    MSYSTEM="${MSYSTEM:-}" \
    MSYS="${MSYS:-}" \
    MINGW_PREFIX="${MINGW_PREFIX:-}" \
    GOPATH="${GOPATH:-${HOME}/go}" \
    GOROOT="${GOROOT:-}" \
    GOMODCACHE="${GOMODCACHE:-}" \
    GOCACHE="${GOCACHE:-}" \
    GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" \
    GOFLAGS="${GOFLAGS:-}" \
    GOPROXY="${GOPROXY:-}" \
    GOSUMDB="${GOSUMDB:-}" \
    GOPRIVATE="${GOPRIVATE:-}" \
    GO111MODULE="${GO111MODULE:-}" \
    CGO_ENABLED="${CGO_ENABLED:-}" \
    ANDROID_HOME="${ANDROID_HOME:-}" \
    ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}" \
    ANDROID_NDK_HOME="${ANDROID_NDK_HOME:-}" \
    ANDROID_NDK_ROOT="${ANDROID_NDK_ROOT:-${ANDROID_NDK_HOME:-}}" \
    NDK_HOME="${NDK_HOME:-${ANDROID_NDK_HOME:-}}" \
    JAVA_HOME="${JAVA_HOME:-}" \
    gomobile bind \
        -target=android \
        -androidapi=26 \
        -tags="${TAGS}" \
        -ldflags="${LDFLAGS}" \
        -o "${OUTPUT}" \
        ./mobile github.com/sagernet/sing-box/experimental/libbox

echo "✅ AAR built successfully: ${OUTPUT}"
ls -lh "${OUTPUT}"
