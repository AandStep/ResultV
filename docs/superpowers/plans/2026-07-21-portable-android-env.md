# Переносимое окружение Android-сборки — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать возможность перенести проект на другой ПК одним `git clone` + `bootstrap.sh`, не таская тяжёлые артефакты в git, с секретами в зашифрованном бандле.

**Architecture:** Три bash-скрипта (`pack-secrets`, `unpack-secrets`, `bootstrap`) + `.example`-шаблоны. Код едет через git; секреты — через `secrets.enc` (openssl AES-256) в git; тулчейн проверяется/доустанавливается; машинно-зависимые артефакты (AAR, build, local.properties) регенерируются на месте.

**Tech Stack:** bash (git-bash на Windows + bash на macOS/Linux), `openssl enc -aes-256-cbc -pbkdf2`, `tar`, Go/gomobile, Gradle.

## Global Constraints

- Скрипты обязаны работать и в git-bash (Windows), и в bash (macOS/Linux). ОС определять через `uname`/`$OSTYPE`.
- Все пути — относительно корня репо; каждый скрипт вычисляет `REPO_ROOT` через `$(cd "$(dirname "$0")/.." && pwd)`.
- Шифрование строго `openssl enc -aes-256-cbc -pbkdf2 -salt`.
- Секретные файлы (список канонический во всех скриптах): `.env`, `android/keystore.properties`, `android/*.jks`, `android/*.keystore`.
- `.env` / `keystore.properties` / `*.jks` / `*.keystore` НИКОГДА не коммитятся; `secrets.enc`, `*.example` — коммитятся.
- gomobile pin: `github.com/sagernet/gomobile/cmd/gomobile@v0.1.12`.
- `set -euo pipefail` во всех скриптах.

---

### Task 1: Шаблоны секретов и правки .gitignore

**Files:**
- Create: `.env.example`
- Create: `android/keystore.properties.example`
- Modify: `.gitignore` (корень)

**Interfaces:**
- Produces: наличие `.env.example` в корне (bootstrap на него ссылается); гарантия, что `secrets.enc` не игнорируется.

- [ ] **Step 1: Создать `.env.example`**

```
# Hex-encoded 32-byte AES-GCM key, embedded at link time into
# internal/proxy.subscriptionEncryptKey. Без него resultv:// и RVSUB1
# импорты не расшифруются. Реальное значение держи в .env (в git не идёт).
SUBSCRIPTION_ENCRYPT_KEY=
```

- [ ] **Step 2: Создать `android/keystore.properties.example`**

```
# Копия -> android/keystore.properties (в git не идёт). Заполнить для release-подписи.
storeFile=release.jks
storePassword=
keyAlias=
keyPassword=
```

- [ ] **Step 3: Убедиться в .gitignore, что secrets.enc и шаблоны коммитятся**

Проверить/добавить в корневой `.gitignore` блок (рядом с секцией секретов):

```
# Secrets bundle is encrypted -> safe to commit
!secrets.enc
!.env.example
!android/keystore.properties.example
```

Run: `git check-ignore -v .env.example android/keystore.properties.example; echo "exit=$?"`
Expected: оба файла НЕ игнорируются (пустой вывод, `exit=1`).

- [ ] **Step 4: Commit**

```bash
git add -A .env.example android/keystore.properties.example .gitignore
git commit -m "chore(env): add secret templates and un-ignore encrypted bundle"
```

---

### Task 2: `scripts/pack-secrets.sh`

**Files:**
- Create: `scripts/pack-secrets.sh`

**Interfaces:**
- Produces: `secrets.enc` в корне репо из канонического списка секретов. Формат: `openssl enc -aes-256-cbc -pbkdf2 -salt` поверх `tar`.

- [ ] **Step 1: Написать скрипт**

```bash
#!/usr/bin/env bash
# pack-secrets.sh — собрать секреты в зашифрованный secrets.enc (коммитится в git).
# Расшифровка: scripts/unpack-secrets.sh
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

CANDIDATES=(.env android/keystore.properties)
# globs, которые могут не совпасть — собираем аккуратно
shopt -s nullglob
CANDIDATES+=(android/*.jks android/*.keystore)
shopt -u nullglob

FILES=()
for f in "${CANDIDATES[@]}"; do
    [[ -f "$f" ]] && FILES+=("$f")
done

if [[ ${#FILES[@]} -eq 0 ]]; then
    echo "Нет секретных файлов для упаковки (искал: .env, android/keystore.properties, android/*.jks, android/*.keystore)." >&2
    exit 1
fi

echo "Упаковываю в secrets.enc:"
printf '  - %s\n' "${FILES[@]}"
echo "Задай пароль (запомни — он нужен на другом ПК):"

tar -cf - "${FILES[@]}" | openssl enc -aes-256-cbc -pbkdf2 -salt -out secrets.enc

echo "Готово: secrets.enc"
echo "Теперь: git add secrets.enc && git commit -m \"chore: update secrets bundle\""
```

- [ ] **Step 2: Сделать исполняемым и прогнать**

Run:
```bash
chmod +x scripts/pack-secrets.sh
printf 'testpass\ntestpass\n' | bash scripts/pack-secrets.sh
```
Expected: создан `secrets.enc`, в выводе перечислен `.env`. (openssl читает пароль из stdin-промпта.)

- [ ] **Step 3: Проверить, что файл зашифрован (не plaintext)**

Run: `head -c 16 secrets.enc | xxd | head -1`
Expected: начинается с `Salted__` (openssl salt-заголовок), не с `SUBSCRIPTION`.

- [ ] **Step 4: Commit (скрипт, без secrets.enc пока)**

```bash
git add scripts/pack-secrets.sh
git commit -m "feat(scripts): pack-secrets.sh encrypts secrets into secrets.enc"
```

---

### Task 3: `scripts/unpack-secrets.sh` + round-trip тест

**Files:**
- Create: `scripts/unpack-secrets.sh`

**Interfaces:**
- Consumes: `secrets.enc`, созданный `pack-secrets.sh`.
- Produces: восстановленные секретные файлы в дереве репо. Флаг `--force` перезаписывает существующие.

- [ ] **Step 1: Написать failing round-trip тест**

Создать временный тест-скрипт `/tmp/rt.sh`:

```bash
set -euo pipefail
cd /c/ResultV
printf 'KEY=abc123\n' > /tmp/env.orig
cp /tmp/env.orig .env.rttest
# упаковка одного файла напрямую
tar -cf - .env.rttest | openssl enc -aes-256-cbc -pbkdf2 -salt -pass pass:tp -out /tmp/rt.enc
rm .env.rttest
# распаковка через будущий скрипт нельзя — файла ещё нет; здесь проверяем сам механизм
openssl enc -d -aes-256-cbc -pbkdf2 -pass pass:tp -in /tmp/rt.enc | tar -xf -
diff /tmp/env.orig .env.rttest && echo ROUNDTRIP_OK
rm -f .env.rttest /tmp/rt.enc /tmp/env.orig
```

Run: `bash /tmp/rt.sh`
Expected: `ROUNDTRIP_OK` (подтверждает, что openssl+tar round-trip корректен до написания скрипта).

- [ ] **Step 2: Написать `unpack-secrets.sh`**

```bash
#!/usr/bin/env bash
# unpack-secrets.sh — восстановить секреты из secrets.enc. Пароль тот же, что в pack-secrets.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

FORCE=0
[[ "${1:-}" == "--force" ]] && FORCE=1

if [[ ! -f secrets.enc ]]; then
    echo "secrets.enc не найден в ${REPO_ROOT}. Нечего распаковывать." >&2
    exit 1
fi

# Без --force не затираем уже существующие секреты.
if [[ $FORCE -eq 0 && -f .env ]]; then
    echo ".env уже существует — пропускаю распаковку. Запусти с --force, чтобы перезаписать." >&2
    exit 0
fi

echo "Введи пароль от secrets.enc:"
openssl enc -d -aes-256-cbc -pbkdf2 -in secrets.enc | tar -xf -
echo "Секреты восстановлены."
```

- [ ] **Step 3: Полный round-trip через оба реальных скрипта**

Run:
```bash
chmod +x scripts/unpack-secrets.sh
cp .env /tmp/env.backup 2>/dev/null || true
printf 'testpass\ntestpass\n' | bash scripts/pack-secrets.sh
mv .env /tmp/env.moved
printf 'testpass\n' | bash scripts/unpack-secrets.sh
diff /tmp/env.moved .env && echo REAL_ROUNDTRIP_OK
```
Expected: `REAL_ROUNDTRIP_OK` — `.env` восстановлен байт-в-байт.

- [ ] **Step 4: Проверить защиту от перезаписи**

Run: `printf 'testpass\n' | bash scripts/unpack-secrets.sh; echo "exit=$?"`
Expected: сообщение «.env уже существует — пропускаю», `exit=0` (не затёрло).

- [ ] **Step 5: Commit**

```bash
git add scripts/unpack-secrets.sh
git commit -m "feat(scripts): unpack-secrets.sh restores secrets from secrets.enc"
```

---

### Task 4: `scripts/bootstrap.sh`

**Files:**
- Create: `scripts/bootstrap.sh`

**Interfaces:**
- Consumes: `unpack-secrets.sh`, `build-android.sh` (существует), `secrets.enc` (опц.).
- Produces: проверенный тулчейн, сгенерированный `android/local.properties`, восстановленные секреты; печатает следующий шаг.

- [ ] **Step 1: Написать скрипт**

```bash
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
echo "==> java: $(java -version 2>&1 | head -1)"

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
```

- [ ] **Step 2: Прогнать на текущей машине (она уже настроена — должно пройти все проверки)**

Run: `chmod +x scripts/bootstrap.sh && bash scripts/bootstrap.sh`
Expected: все `==>` строки без ERROR, финальное `✅ Bootstrap завершён`. `.env` уже есть → секреты не трогаются.

- [ ] **Step 3: Проверить генерацию local.properties**

Run:
```bash
mv android/local.properties /tmp/lp.bak
bash scripts/bootstrap.sh >/dev/null
cat android/local.properties
```
Expected: строка `sdk.dir=...` с корректно экранированным путём к SDK текущей машины.

Затем восстановить оригинал: `mv /tmp/lp.bak android/local.properties`.

- [ ] **Step 4: Commit**

```bash
git add scripts/bootstrap.sh
git commit -m "feat(scripts): bootstrap.sh verifies toolchain and restores env"
```

---

### Task 5: Документация переноса

**Files:**
- Create: `docs/PORTING.md`
- Modify: `build-android.sh:1-20` (добавить ссылку на bootstrap в шапку-usage — только комментарий)

**Interfaces:**
- Produces: понятная инструкция «как перенести на новый ПК».

- [ ] **Step 1: Создать `docs/PORTING.md`**

```markdown
# Перенос проекта на другой ПК

## На текущей машине (один раз / после смены ключа)
```
bash scripts/pack-secrets.sh          # -> secrets.enc (по паролю)
git add secrets.enc && git commit -m "chore: update secrets bundle"
git push
```

## На новом ПК
Требуется предустановленное: Go, JDK 17+, Android Studio (SDK + NDK).
```
git clone <repo> && cd ResultV
bash scripts/bootstrap.sh             # проверит тулчейн, поставит gomobile,
                                      # создаст local.properties, спросит пароль
                                      # и распакует секреты
bash build-android.sh                 # пересоберёт AAR + APK
```

## Что НЕ переносится (регенерируется)
- `android/libs/libbox.aar` — собирается `scripts/build-android-aar.sh`
- `android/app/build/`, `node_modules` — билд-вывод
- `android/local.properties` — генерится bootstrap под путь SDK машины

## Секреты
`.env` и keystore едут только внутри зашифрованного `secrets.enc`.
Пароль передавай отдельным каналом (не в git).
```

- [ ] **Step 2: Добавить ссылку в шапку build-android.sh**

В блок `# Usage:` файла `build-android.sh` добавить строку-комментарий:

```bash
#   На новой машине сперва: bash scripts/bootstrap.sh  (см. docs/PORTING.md)
```

- [ ] **Step 3: Commit**

```bash
git add docs/PORTING.md build-android.sh
git commit -m "docs: porting guide for moving env to a new machine"
```

---

## Self-Review

**Spec coverage:**
- pack/unpack секретов (openssl) → Task 2, 3 ✓
- bootstrap: детект ОС, проверки Go/JDK/SDK/NDK, gomobile install+init, генерация local.properties, распаковка секретов → Task 4 ✓
- `.env.example` / `keystore.properties.example` → Task 1 ✓
- .gitignore: secrets.enc коммитится, секреты — нет → Task 1 ✓
- порядок использования / границы → Task 5 (PORTING.md) ✓
- round-trip `.env` байт-в-байт → Task 3 Step 3 ✓

**Placeholder scan:** нет TBD/TODO; весь код приведён целиком. ✓

**Type/имя consistency:** канонический список секретов идентичен в pack (Task 2) и в дизайне; `--force` в unpack (Task 3) согласован; `secrets.enc`, `android/local.properties`, `SUBSCRIPTION_ENCRYPT_KEY` пишутся одинаково везде. ✓

**Известное ограничение:** `pack-secrets` в тестах кормится паролем через stdin (`printf 'testpass\ntestpass\n'`), т.к. openssl читает пароль из tty/stdin-промпта; при ручном запуске пароль вводится интерактивно. Это осознанно.
