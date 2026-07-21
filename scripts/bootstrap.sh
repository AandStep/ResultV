#!/usr/bin/env bash
# bootstrap.sh — подготовить свежую машину к сборке Android APK.
# Проверяет тулчейн, ставит gomobile, генерит local.properties, распаковывает секреты.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) OS=windows ;;
    Darwin) OS=macos ;;
    Linux) OS=linux ;;
    *) OS=unknown ;;
esac
echo "==> OS: ${OS}"

fail() { echo "ERROR: $*" >&2; exit 1; }

# --- Go ---
command -v go >/dev/null || fail "Go не найден. Установи: https://go.dev/dl/ и добавь в PATH."
echo "==> go: $(go version)"

# --- JDK 17+ ---
if [[ -z "${JAVA_HOME:-}" ]] && ! command -v javac >/dev/null; then
    fail "JDK не найден. Установи JDK 17+ (Temurin/Android Studio JBR) и задай JAVA_HOME."
fi
JAVA_BIN="java"; [[ -n "${JAVA_HOME:-}" && -x "${JAVA_HOME}/bin/java" ]] && JAVA_BIN="${JAVA_HOME}/bin/java"
echo "==> java: $("${JAVA_BIN}" -version 2>&1 | head -1 || echo "unknown")"

# --- Android SDK ---
if [[ -z "${ANDROID_HOME:-}" ]]; then
    if [[ -f android/local.properties ]]; then
        ANDROID_HOME="$(grep -E '^sdk\.dir=' android/local.properties | cut -d= -f2- | tr -d '\r' | sed 's/\\\\/\//g;s/\\/\//g')"
    fi
fi
if [[ -z "${ANDROID_HOME:-}" || ! -d "${ANDROID_HOME}" ]]; then
    for c in "${LOCALAPPDATA:-}/Android/Sdk" "$HOME/AppData/Local/Android/Sdk" "$HOME/Library/Android/sdk" "$HOME/Android/Sdk"; do
        [[ -n "$c" && -d "$c" ]] && ANDROID_HOME="$c" && break
    done
fi
[[ -n "${ANDROID_HOME:-}" && -d "${ANDROID_HOME}" ]] || fail "Android SDK не найден. Установи через Android Studio и задай ANDROID_HOME."
echo "==> Android SDK: ${ANDROID_HOME}"

# --- NDK ---
[[ -d "${ANDROID_HOME}/ndk" ]] || fail "NDK не найден в ${ANDROID_HOME}/ndk. Установи через SDK Manager."
echo "==> NDK: $(ls -1 "${ANDROID_HOME}/ndk" | sort -V | tail -1)"

# --- gomobile ---
if ! command -v gomobile >/dev/null && [[ ! -x "$(go env GOPATH)/bin/gomobile" ]]; then
    echo "==> gomobile не найден, ставлю..."
    go install github.com/sagernet/gomobile/cmd/gomobile@v0.1.12
    "$(go env GOPATH)/bin/gomobile" init
    echo "==> gomobile установлен"
else
    echo "==> gomobile: ok"
fi

# --- local.properties (генерим, путь под текущую машину) ---
if [[ ! -f android/local.properties ]]; then
    if [[ "$OS" == "windows" ]]; then
        win_path="$(cygpath -w "${ANDROID_HOME}" 2>/dev/null || echo "${ANDROID_HOME}")"
        esc="${win_path//\\/\\\\}"; esc="${esc//:/\\:}"
        echo "sdk.dir=${esc}" > android/local.properties
    else
        echo "sdk.dir=${ANDROID_HOME}" > android/local.properties
    fi
    echo "==> создан android/local.properties"
else
    echo "==> android/local.properties уже есть, не трогаю"
fi

# --- секреты ---
if [[ -f secrets.enc && ! -f .env ]]; then
    echo "==> распаковываю секреты (введи пароль):"
    bash scripts/unpack-secrets.sh || echo "  (пропущено/ошибка — заполни .env вручную из .env.example)"
elif [[ ! -f .env ]]; then
    echo "==> .env отсутствует и secrets.enc нет. Скопируй .env.example -> .env и заполни ключ."
fi

echo
echo "✅ Bootstrap завершён. Следующий шаг:"
echo "   bash build-android.sh"
