# Google Play: флейворы, разрез MITM и базовые требования — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Получить сборку, которую Google Play примет: без MITM в бинаре, с targetSdk 36, без `QUERY_ALL_PACKAGES`, в формате AAB — и сохранить полную сборку для раздачи с сайта.

**Architecture:** Разрез идёт через оба слоя. В Go — build-тег `no_mitm`, подменяющий MITM-биндинги заглушками, отчего `internal/filter` перестаёт линковаться; отсюда две сборки AAR. В Kotlin — product flavors `play`/`full`: реализации сертификатов и MITM-прокси переезжают в `src/full/java`, в `src/play/java` кладутся заглушки с теми же сигнатурами, а UI прячется за `BuildConfig`-флагом. Общий код остаётся общим и компилируется в обоих флейворах без изменений.

**Tech Stack:** Go 1.26.4, gomobile (форк sagernet), AGP 8.13.2, Gradle 9.0, Kotlin 2.0.21, Compose BOM 2024.10.01, Android SDK 36 (установлен).

**Spec:** `docs/android-pc-sync-and-play-spec.md`, раздел 4. Решение о разрезе MITM — раздел 2.

**Предшествующий блок:** `docs/superpowers/plans/2026-09-04-android-core-sync-and-16kb.md` (закрыт; выравнивание 16 КБ уже сделано там).

## Global Constraints

- Пакеты `internal/proxy` и `mobile` **не собираются без `-tags=mobile`**. Play-вариант собирается с `-tags=mobile,no_mitm`.
- Экспортируемые сигнатуры биндингов **обязаны совпадать** в обеих сборках: общий Kotlin компилируется против любой из них. Заглушка возвращает ошибку, а не отсутствует.
- `docs/superpowers/` закрыт `.gitignore` (строка 34). Файлы оттуда добавлять только `git add -f`.
- `android/libs` тоже в `.gitignore` — оба AAR остаются локальными артефактами.
- AAR пересобирается только через `scripts/build-android-aar.sh`. `gradlew` упаковывает уже лежащий файл.
- Установка на устройство: `adb install -r -d`. **Никогда не `adb uninstall`** — стирает профили.
- `gofmt -l` на этой машине ругается на все файлы из-за CRLF в рабочей копии (git хранит LF). Это шум, а не сигнал; проверять форматирование только у своих файлов через `gofmt -d <file>` и смотреть на содержательные различия.
- Сообщения коммитов заканчиваются строкой:
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`

---

### Task 1: Build-тег `no_mitm` в Go-слое

Сейчас `mobile/libbox.go:33` импортирует `internal/filter` безусловно, build-тегов у пакета нет. Значит генерация корневого CA, перехват TLS и форк gomitmproxy компилируются в `libgojni.so` при любой сборке — и разделение только на уровне Kotlin их оттуда не уберёт. Ревью Google смотрит на содержимое бинаря, а не на то, что вызывается из Kotlin.

Проверено спайком: после выноса MITM-биндингов в отдельный файл под тегом `internal/filter` уходит из `go list -deps` с 4 пакетов до 0.

**Files:**
- Create: `mobile/libbox_filter.go` (`//go:build !no_mitm`)
- Create: `mobile/libbox_filter_stub.go` (`//go:build no_mitm`)
- Modify: `mobile/libbox.go` — убрать перенесённое, поправить импорты

**Interfaces:**
- Produces (одинаково в обеих сборках): `FetchFilterLists(dataDir string) (string, error)`, `FilterCARootPath(dataDir string) (string, error)`, `SetFilterCASeed(dataDir, seed string) error`, `StartFilterProxy(dataDir string, listenPort int) (string, error)`, `StopFilterProxy()`, `FilterStatus(dataDir string) (string, error)`
- Только в `!no_mitm`: `getFilterManager(dataDir string) *filter.Manager`, переменные `filterMu`/`filterManager`

- [ ] **Step 1: Зафиксировать исходное состояние зависимостей**

Run:
```bash
go list -tags=mobile -deps ./mobile/ | grep -c 'internal/filter'
```
Expected: `4`

- [ ] **Step 2: Перенести MITM-биндинги в `mobile/libbox_filter.go`**

Из `mobile/libbox.go` вырезать и перенести без изменений тела:

- блок `var ( filterMu sync.Mutex; filterManager *filter.Manager )` и функцию `getFilterManager` (строки ~37–57);
- функции `FetchFilterLists`, `FilterCARootPath`, `SetFilterCASeed`, `StartFilterProxy`, `StopFilterProxy`, `FilterStatus` вместе с их комментариями (от комментария `// FetchFilterLists downloads` до закрывающей скобки `FilterStatus`, строки ~1549–1668).

**`BrowserAdBlockSocksPort` НЕ переносить** — константу использует сборка конфига движка в `libbox.go:1391`, она нужна в обеих сборках. Спайк на этом споткнулся.

Шапка нового файла:

```go
//go:build !no_mitm

// Browser ad-block (MITM) bindings. Built into the `full` distribution only:
// the `play` build is compiled with -tags=no_mitm, which swaps this file for
// libbox_filter_stub.go and drops internal/filter — and with it the CA
// generation and TLS-interception code — out of the linked .so entirely.

package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	socksproxy "golang.org/x/net/proxy"
	"resultproxy-wails/internal/filter"
)
```

Набор импортов проверен спайком: без `context`, `time`, `net` и `socksproxy` файл не собирается.

- [ ] **Step 3: Написать заглушку `mobile/libbox_filter_stub.go`**

```go
//go:build no_mitm

// Stub browser ad-block bindings for the `play` distribution.
//
// Google Play's VpnService policy forbids VPN apps from interfering with ads,
// and no app in the store ships TLS interception with a user-installed root CA
// — that is exactly why the full AdGuard is distributed outside Play. The Play
// build therefore must not merely leave this feature unreachable: the code must
// not be in the binary at all, because review looks at what the binary
// contains, not at what Kotlin calls.
//
// Signatures are kept identical to the real ones so the shared Kotlin code
// compiles against either build.

package mobile

import "errors"

// ErrMITMUnavailable is returned by every browser ad-block binding here.
var ErrMITMUnavailable = errors.New("browser ad-block is not available in this build")

func FetchFilterLists(dataDir string) (string, error) { return "", ErrMITMUnavailable }

func FilterCARootPath(dataDir string) (string, error) { return "", ErrMITMUnavailable }

func SetFilterCASeed(dataDir, seed string) error { return ErrMITMUnavailable }

func StartFilterProxy(dataDir string, listenPort int) (string, error) {
	return "", ErrMITMUnavailable
}

func StopFilterProxy() {}

func FilterStatus(dataDir string) (string, error) { return "", ErrMITMUnavailable }
```

- [ ] **Step 4: Поправить импорты в `mobile/libbox.go`**

После выноса в `libbox.go` остаются неиспользуемыми `sync`, `golang.org/x/net/proxy` (алиас `socksproxy`) и импорт `resultproxy-wails/internal/filter`. Убрать их. Остальные импорты проверить компилятором, а не глазами.

- [ ] **Step 5: Собрать обе конфигурации**

Run:
```bash
go build -tags=mobile ./internal/... ./mobile/...
go build -tags=mobile,no_mitm ./internal/... ./mobile/...
```
Expected: обе exit 0.

- [ ] **Step 6: Доказать, что MITM ушёл из сборки — это критерий приёмки задачи**

Run:
```bash
go list -tags=mobile        -deps ./mobile/ | grep -c 'internal/filter'   # ждём 4
go list -tags=mobile,no_mitm -deps ./mobile/ | grep -c 'internal/filter'   # ждём 0
go list -tags=mobile,no_mitm -deps ./mobile/ | grep -ci 'gomitmproxy'      # ждём 0
```

- [ ] **Step 7: Тесты в обеих конфигурациях**

Run:
```bash
go test -tags=mobile -count=1 ./internal/... ./mobile/...
go test -tags=mobile,no_mitm -count=1 ./mobile/...
```

Ожидаемо: часть тестов пакета `mobile` относится к MITM (`browser_adblock_inbound_test.go`). Если они не собираются под `no_mitm` — пометить их тем же тегом `//go:build !no_mitm`, а не удалять: в `full` они должны продолжать работать.

- [ ] **Step 8: Коммит**

```bash
git add mobile/
git commit -m "$(cat <<'EOF'
feat(mobile): build-тег no_mitm убирает MITM из бинаря

Google Play смотрит на то, что в бинаре есть, а не на то, что вызывается,
поэтому разделения на уровне Kotlin недостаточно: генерация корневого CA и
перехват TLS компилировались в libgojni.so при любой сборке. Под тегом
no_mitm биндинги подменяются заглушками и internal/filter перестаёт
линковаться — проверено go list -deps: было 4 пакета, стало 0.

Сигнатуры заглушек совпадают с настоящими, чтобы общий Kotlin собирался
против любой из сборок.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Две сборки AAR

`scripts/build-android-aar.sh` берёт теги из `scripts/android-build-tags.txt` и кладёт результат в `android/libs/libbox.aar`. Нужен параметр дистрибутива: `full` — как сейчас, `play` — с добавленным `no_mitm` и своим именем выходного файла.

**Files:**
- Modify: `scripts/build-android-aar.sh`
- Modify: `build-android.sh`

**Interfaces:**
- Consumes: тег `no_mitm` из задачи 1
- Produces: `android/libs/libbox-full.aar`, `android/libs/libbox-play.aar`

- [ ] **Step 1: Добавить параметр дистрибутива в `scripts/build-android-aar.sh`**

Рядом с разбором существующего флага `--with-naive` (строка ~94) добавить `--dist=play|full` со значением по умолчанию `full`. Ниже строки `TAGS="$(tr -d ...)"`:

```bash
# Дистрибутив: full — всё, включая браузерный ad-block (MITM); play — без него.
# Тег no_mitm выкидывает internal/filter из линковки целиком, см. Task 1 плана
# docs/superpowers/plans/2026-09-04-android-play-flavors-and-baseline.md.
DIST="${DIST:-full}"
if [[ "${DIST}" == "play" ]]; then
    TAGS="${TAGS},no_mitm"
    OUTPUT="${REPO_ROOT}/android/libs/libbox-play.aar"
else
    OUTPUT="${REPO_ROOT}/android/libs/libbox-full.aar"
fi
```

Существующее присвоение `OUTPUT="${REPO_ROOT}/android/libs/libbox.aar"` (строка ~30) убрать, чтобы имя задавалось в одном месте.

- [ ] **Step 2: Прокинуть параметр в `build-android.sh`**

`build-android.sh:51` вызывает скрипт AAR. Дать ему тот же выбор дистрибутива и собирать нужный вариант, а не оба: сборка одного AAR долгая, а собирать второй без нужды — терять минуты на каждой итерации.

- [ ] **Step 3: Собрать оба AAR**

Ключ расшифровки подписок скрипт из `.env` не читает — экспортировать его самому, иначе сборка потеряет импорт `resultv://`:

```bash
set -a; . ./.env; set +a
export ANDROID_HOME="$HOME/AppData/Local/Android/Sdk"   # или свой путь
DIST=full bash scripts/build-android-aar.sh
DIST=play bash scripts/build-android-aar.sh
```

Обе сборки должны напечатать `🔐 Embedding subscription decryption key`.

- [ ] **Step 4: Проверить, что play-сборка действительно меньше и без MITM**

```bash
ls -la android/libs/libbox-full.aar android/libs/libbox-play.aar
for d in full play; do
  unzip -o -j "android/libs/libbox-$d.aar" 'jni/arm64-v8a/libgojni.so' -d "/tmp/$d" >/dev/null
  echo -n "$d: "; ls -la "/tmp/$d/libgojni.so" | awk '{print $5}'
  echo -n "  строк gomitmproxy в бинаре: "; strings "/tmp/$d/libgojni.so" | grep -ci gomitmproxy
done
```

Expected: у `play` символов `gomitmproxy` **0**, у `full` — ненулевое число; play-библиотека заметно меньше.

- [ ] **Step 5: Перепроверить выравнивание 16 КБ в обоих AAR**

Требование Play действует для каждой сборки, а не для той, на которой его однажды проверили. Замер из предыдущего блока, для `arm64-v8a` в обоих файлах:

```bash
python - <<'PY'
import struct, os
for d in ('full', 'play'):
    p = os.path.join('/tmp', d, 'libgojni.so')
    f = open(p, 'rb').read()
    off = struct.unpack_from('<Q', f, 0x20)[0]
    size = struct.unpack_from('<H', f, 0x36)[0]
    num = struct.unpack_from('<H', f, 0x38)[0]
    a = [hex(struct.unpack_from('<Q', f, off+i*size+48)[0])
         for i in range(num) if struct.unpack_from('<I', f, off+i*size)[0] == 1]
    print(d, 'LOAD align:', ', '.join(a))
PY
```
Expected: везде `0x4000`.

- [ ] **Step 6: Коммит**

```bash
git add scripts/build-android-aar.sh build-android.sh
git commit -m "$(cat <<'EOF'
build(android): собирать AAR в двух дистрибутивах

DIST=full — как раньше; DIST=play добавляет тег no_mitm и пишет
libbox-play.aar. В play-библиотеке символов gomitmproxy нет.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Product flavors в Gradle

**Files:**
- Modify: `android/app/build.gradle.kts`

**Interfaces:**
- Consumes: `android/libs/libbox-{full,play}.aar` из задачи 2
- Produces: флейворы `play` и `full`; `BuildConfig.BROWSER_ADBLOCK` (Boolean)

- [ ] **Step 1: Объявить флейворы и флаг**

В блок `android { }` после `defaultConfig`:

```kotlin
    // Дистрибутивы: play — сборка для Google Play, без браузерного ad-block
    // (MITM); full — для раздачи с сайта, со всем. Разрез идёт и по Go
    // (тег no_mitm, свой AAR), и по Kotlin (src/play vs src/full).
    flavorDimensions += "dist"
    productFlavors {
        create("full") {
            dimension = "dist"
            buildConfigField("boolean", "BROWSER_ADBLOCK", "true")
        }
        create("play") {
            dimension = "dist"
            buildConfigField("boolean", "BROWSER_ADBLOCK", "false")
        }
    }
```

- [ ] **Step 2: Развести зависимость на AAR по флейворам**

Строку `implementation(files("$rootDir/libs/libbox.aar"))` в блоке `dependencies` заменить на:

```kotlin
    "fullImplementation"(files("$rootDir/libs/libbox-full.aar"))
    "playImplementation"(files("$rootDir/libs/libbox-play.aar"))
```

- [ ] **Step 3: Проверить, что оба варианта конфигурируются**

Run: `cd android && ./gradlew tasks --all | grep -iE 'assemble(Full|Play)(Debug|Release)'`

Expected: перечислены `assembleFullDebug`, `assemblePlayDebug`, `assembleFullRelease`, `assemblePlayRelease`.

- [ ] **Step 4: Собрать оба debug-варианта**

Run:
```bash
cd android
./gradlew assembleFullDebug -Pdebug.abi=x86_64
./gradlew assemblePlayDebug -Pdebug.abi=x86_64
```

Ожидается провал `assemblePlayDebug` на неразрешённых ссылках из общего кода на классы сертификатов — это и есть работа задачи 4. Записать список ошибок: он и будет её списком.

- [ ] **Step 5: Коммит** (после того как задача 4 сделает сборку зелёной — до неё коммитить нечего работающего)

Коммит объединяется с задачей 4.

---

### Task 4: Разрез Kotlin по флейворам

Двенадцать файлов ссылаются на MITM. Шесть из них — чистые реализации, они переезжают в `src/full/java`, и в `src/play/java` кладутся заглушки с теми же именами и сигнатурами. Тогда общий код (`MainActivity`, `SettingsScreen`, `ResultVpnService`, `BoxModule`, `BuildOptions`, `SettingsRepository`) компилируется в обоих флейворах **без единой правки логики** — меняется только показ UI, спрятанный за `BuildConfig.BROWSER_ADBLOCK`.

**Files:**
- Move: `vpn/CertStore.kt`, `vpn/CertInstaller.kt`, `vpn/CertSelfTest.kt`, `vpn/CertExporter.kt`, `vpn/FilterProxyWatchdog.kt`, `ui/screens/CertWizardScreen.kt` → `android/app/src/full/java/com/resultv/android/...`
- Create: те же имена в `android/app/src/play/java/com/resultv/android/...` — заглушки
- Modify: `ui/screens/SettingsScreen.kt:300`, `MainActivity.kt:411`

**Interfaces:**
- Consumes: `BuildConfig.BROWSER_ADBLOCK` из задачи 3
- Produces: одинаковый публичный API шести классов в обоих флейворах

- [ ] **Step 1: Переместить реализации в `src/full/java`**

```bash
cd android/app/src
mkdir -p full/java/com/resultv/android/vpn full/java/com/resultv/android/ui/screens
git mv main/java/com/resultv/android/vpn/CertStore.kt full/java/com/resultv/android/vpn/
git mv main/java/com/resultv/android/vpn/CertInstaller.kt full/java/com/resultv/android/vpn/
git mv main/java/com/resultv/android/vpn/CertSelfTest.kt full/java/com/resultv/android/vpn/
git mv main/java/com/resultv/android/vpn/CertExporter.kt full/java/com/resultv/android/vpn/
git mv main/java/com/resultv/android/vpn/FilterProxyWatchdog.kt full/java/com/resultv/android/vpn/
git mv main/java/com/resultv/android/ui/screens/CertWizardScreen.kt full/java/com/resultv/android/ui/screens/
```

Пакеты в файлах не меняются: source set другой, пакет тот же.

- [ ] **Step 2: Написать заглушки в `src/play/java`**

Публичный API, который заглушки обязаны повторить (выписан из оригиналов; при расхождении верить файлам в `src/full`, а не этому списку):

```kotlin
object CertStore {
    fun isInstalled(dataDir: String): Boolean          // → false
    fun applySeed(dataDir: String, seed: String)       // → no-op
    fun staleEntryCount(dataDir: String): Int          // → 0
    fun isResultVCommonName(subjectDn: String?): Boolean // → false
}

object CertInstaller {
    fun canInstallDirectly(): Boolean                  // → false
    fun buildInstallIntent(context: Context, dataDir: String): Intent
        // → throw UnsupportedOperationException: вызывать это в play-сборке
        //   нельзя, и молчаливый пустой Intent спрятал бы такую ошибку
}

object CertSelfTest {
    enum class Result { PASS, CERT_UNTRUSTED, INCONCLUSIVE }
    fun run(port: Int): Result                         // → Result.INCONCLUSIVE
}

object CertExporter {
    fun buildSaveIntent(): Intent                      // → throw UnsupportedOperationException
    fun writeTo(context: Context, destination: Uri, dataDir: String): String
                                                        // → throw UnsupportedOperationException
}

class FilterProxyWatchdog(
    private val dataDir: String,
    private val onUnhealthy: () -> Unit,
) {
    fun start()                                        // → no-op
    fun stop()                                         // → no-op
}

@Composable
fun CertWizardScreen(dataDir: String, onClose: () -> Unit)  // → LaunchedEffect { onClose() }
```

Возвращать «безопасное ничто» там, где вызов возможен по общему коду (`isInstalled`, `run`, `start`/`stop`), и бросать `UnsupportedOperationException` там, где вызов означал бы ошибку в разводке флейворов (`buildInstallIntent`, `buildSaveIntent`, `writeTo`) — эти три достижимы только из UI, которого в play-сборке нет.

Каждой заглушке — комментарий, объясняющий, почему она пуста, чтобы её не приняли за недоделку:

```kotlin
/**
 * Заглушка для дистрибутива Play.
 *
 * Браузерный ad-block перехватывает TLS пользовательским корневым
 * сертификатом. Политика VpnService в Google Play запрещает VPN-приложениям
 * вмешиваться в рекламу, и ни одно приложение в магазине не поставляет
 * перехват TLS — именно поэтому полный AdGuard раздаётся мимо Play.
 *
 * Реализация живёт в src/full. Go-слой в этой сборке тоже без неё: тег
 * no_mitm выкидывает internal/filter из линковки.
 */
```

`CertWizardScreen` в play-варианте — composable с той же сигнатурой, немедленно вызывающий `onClose()`.

- [ ] **Step 3: Спрятать UI за флагом**

`ui/screens/SettingsScreen.kt`, строка ~300 — вызов `BrowserAdBlockRow(settings, onOpenCertWizard)` обернуть:

```kotlin
        if (BuildConfig.BROWSER_ADBLOCK) {
            BrowserAdBlockRow(settings, onOpenCertWizard)
        }
```

`MainActivity.kt`, строка ~411:

```kotlin
    if (BuildConfig.BROWSER_ADBLOCK && showCertWizard) {
```

Добавить импорт `com.resultv.android.BuildConfig` в оба файла, если его там нет.

- [ ] **Step 4: Погасить настройку в play-сборке**

`vpn/SettingsRepository.kt:118` читает сохранённое значение:

```kotlin
            browserAdBlock = prefs.getBoolean(K_BROWSER_ADBLOCK, false),
```

Заменить на:

```kotlin
            // В Play-сборке функции нет ни в Kotlin, ни в .so. Сохранённое
            // значение может прийти из full-сборки при переустановке поверх,
            // и включённый флаг заставил бы ResultVpnService дёргать биндинг,
            // который здесь возвращает ошибку.
            browserAdBlock = BuildConfig.BROWSER_ADBLOCK &&
                prefs.getBoolean(K_BROWSER_ADBLOCK, false),
```

- [ ] **Step 5: Собрать оба флейвора**

Run:
```bash
cd android
./gradlew assembleFullDebug -Pdebug.abi=x86_64
./gradlew assemblePlayDebug -Pdebug.abi=x86_64
```
Expected: обе `BUILD SUCCESSFUL`.

- [ ] **Step 6: Юнит-тесты обоих флейворов**

Run: `cd android && ./gradlew testFullDebugUnitTest testPlayDebugUnitTest`

Тесты, относящиеся к сертификатам (`app/src/test/.../CertStoreTest.kt`), под play не соберутся — перенести их в `src/testFull/java`, а не удалять.

- [ ] **Step 7: Проверить APK — критерий приёмки**

```bash
cd android
unzip -p app/build/outputs/apk/play/debug/app-play-debug.apk classes*.dex > /tmp/play.dex 2>/dev/null || true
for v in full play; do
  echo -n "$v: упоминаний CertWizard/gomitmproxy в APK: "
  unzip -p "app/build/outputs/apk/$v/debug/app-$v-debug.apk" '*' 2>/dev/null | strings | grep -ciE 'gomitmproxy|CertWizardScreen'
done
```
Expected: у `play` — 0, у `full` — ненулевое.

- [ ] **Step 8: Коммит**

```bash
git add -A android/
git commit -m "$(cat <<'EOF'
feat(android): флейворы play и full, разрез браузерного ad-block

Реализации сертификатов и MITM-прокси переехали в src/full, в src/play
лежат заглушки с теми же сигнатурами — общий код компилируется в обоих
флейворах без правок логики. UI спрятан за BuildConfig.BROWSER_ADBLOCK,
сохранённая настройка в play-сборке принудительно выключена.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: `<queries>` вместо `QUERY_ALL_PACKAGES`

`QUERY_ALL_PACKAGES` разрешён Google только приложениям, чья основная функция требует видеть все установленные приложения (поиск по устройству, антивирусы, файловые менеджеры, браузеры), и требует обоснования, почему менее интрузивный способ не годится. Маршрутизация VPN по приложениям в этот перечень прямо не входит.

**Files:**
- Modify: `android/app/src/main/AndroidManifest.xml:7-10`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/AppInventory.kt:112`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RulesScreen.kt:740`

**Interfaces:**
- Produces: список приложений, видимый через `<queries>` вместо разрешения

- [ ] **Step 1: Заменить разрешение на `<queries>`**

В `AndroidManifest.xml` убрать блок `uses-permission android:name="android.permission.QUERY_ALL_PACKAGES"` вместе с комментарием и `tools:ignore`, а сразу после закрывающего `</uses-permission>`-блока разрешений добавить:

```xml
    <!-- Видимость приложений для пикера per-app маршрутизации. Раньше здесь
         стоял QUERY_ALL_PACKAGES: Google разрешает его только приложениям,
         чья основная функция — видеть все установленные приложения (поиск по
         устройству, антивирусы, файловые менеджеры, браузеры), и требует
         обосновать, почему менее интрузивный способ не подходит. Пикеру
         исключений хватает запускаемых приложений и обработчиков http —
         последнее нужно AppInventory.browserPackages. -->
    <queries>
        <intent>
            <action android:name="android.intent.action.MAIN" />
            <category android:name="android.intent.category.LAUNCHER" />
        </intent>
        <intent>
            <action android:name="android.intent.action.VIEW" />
            <data android:scheme="http" />
        </intent>
    </queries>
```

- [ ] **Step 2: Написать падающую проверку видимости**

`getInstalledApplications` под `<queries>` вернёт только видимые пакеты, поэтому список станет короче — это ожидаемо. Не ожидаемо другое: `SmartAppMembership` ключуется на reverse-DNS имени пакета, и если приложение перестало быть видимым, его трафик молча пойдёт мимо туннеля. Проверку писать на этот инвариант, а не на длину списка.

Добавить в `android/app/src/androidTest` (нужен инструментальный тест — `PackageManager` на JVM не поднять) проверку: браузер по умолчанию и хотя бы одно обычное запускаемое приложение присутствуют в `AppInventory.installedApps(ctx)`, и `AppInventory.browserPackages(ctx)` непуст.

- [ ] **Step 3: Прогнать на устройстве**

Run: `cd android && ./gradlew connectedPlayDebugAndroidTest`

- [ ] **Step 4: Сверить списки до и после вручную**

Собрать APK до и после изменения, на одном устройстве открыть «Правила» → per-app и сравнить количество и состав. Ожидаемо пропадут пакеты без launcher-активити (сервисные компоненты). Если пропало что-то, что пользователь стал бы исключать, — вернуться к шагу 1 и добавить нужный `<intent>`.

- [ ] **Step 5: Коммит**

```bash
git add android/app/src/main/AndroidManifest.xml android/app/src/androidTest
git commit -m "$(cat <<'EOF'
feat(android): заменить QUERY_ALL_PACKAGES на <queries>

Разрешение выдаётся Google только приложениям, чья основная функция —
видеть все установленные приложения; per-app маршрутизация VPN в этот
перечень не входит. Пикеру исключений достаточно запускаемых приложений
и обработчиков http.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: targetSdk 36 и edge-to-edge

С 31 августа 2026 новые приложения и обновления должны таргетить Android 16 (API 36). SDK 36 и 36.1 на машине установлены. Главное следствие — принудительный edge-to-edge, введённый в API 35: `android:statusBarColor` и `android:navigationBarColor` из `themes.xml` перестают действовать, и контент уезжает под системные панели.

**Files:**
- Modify: `android/app/build.gradle.kts` (`compileSdk`, `targetSdk`)
- Modify: `android/app/src/main/res/values/themes.xml`
- Modify: `MainActivity.kt`, `ui/screens/SettingsScreen.kt`, `ui/screens/LogsScreen.kt`, `ui/components/SubscriptionEditSheet.kt`, `android/app/src/full/java/.../CertWizardScreen.kt`

- [ ] **Step 1: Поднять SDK**

`compileSdk = 34` → `36`, `targetSdk = 34` → `36`.

- [ ] **Step 2: Собрать и вычитать предупреждения**

Run: `cd android && ./gradlew assemblePlayDebug -Pdebug.abi=x86_64 2>&1 | grep -iE 'deprecat|warning'`

Список предупреждений — это и есть список работ по шагу 3. Прочитать его целиком, а не по диагонали.

- [ ] **Step 3: Перевести окно на edge-to-edge**

В `MainActivity.onCreate` перед `super.onCreate` уже вызывается `setTheme`; рядом добавить `enableEdgeToEdge()` (`androidx.activity:activity-compose`, зависимость уже есть). Из `themes.xml` убрать `android:statusBarColor` и `android:navigationBarColor` у `Theme.ResultV.Splash` — в API 35+ они игнорируются, и оставлять их значит вводить в заблуждение следующего читателя.

Каждому экрану верхнего уровня дать отступы под системные панели. Экраны перечислены в разделе Files; проверять каждый на устройстве, а не полагаться на то, что «Compose сам».

- [ ] **Step 4: Проверить на устройстве все экраны**

Установить и пройти: Главная, Серверы, Добавить, Настройки, Логи, лист подписки, мастер сертификата (в full-сборке). Ни один элемент управления не должен уходить под статус-бар или под жест-навигацию.

- [ ] **Step 5: Коммит**

```bash
git add android/
git commit -m "$(cat <<'EOF'
feat(android): targetSdk 36 и edge-to-edge

С 31 августа 2026 Google Play требует API 36 от новых приложений и
обновлений. API 35 сделал edge-to-edge принудительным: statusBarColor и
navigationBarColor больше не действуют, поэтому убраны из темы, а экраны
получили отступы под системные панели.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: AAB для Play

Новые приложения Play принимает только в формате Android App Bundle. Сейчас релиз собирается как APK со сплитами по ABI.

**Files:**
- Modify: `android/app/build.gradle.kts` (блок `splits`)
- Modify: `build-android.sh`

- [ ] **Step 1: Оставить сплиты только для full**

Блок `splits { abi { ... } }` нужен APK-раздаче с сайта; для AAB он не нужен — bundle сам режет по ABI. Сплиты включать только когда собирается `full`, иначе `bundlePlayRelease` и сплиты будут спорить за `versionCode`.

- [ ] **Step 2: Собрать бандл**

Run: `cd android && ./gradlew bundlePlayRelease`

Expected: `app/build/outputs/bundle/playRelease/app-play-release.aab`

- [ ] **Step 3: Проверить содержимое бандла**

```bash
unzip -l android/app/build/outputs/bundle/playRelease/app-play-release.aab | grep -E '\.so|BundleConfig'
```
Expected: библиотеки для `arm64-v8a` и `armeabi-v7a`, `x86*` отсутствуют.

- [ ] **Step 4: Замерить выравнивание в бандле**

Тот же замер, что в задаче 2, но по `.so` из AAB: Play проверяет именно загружаемый артефакт.

- [ ] **Step 5: Коммит**

---

### Task 8: Переупаковать DNS-adblock

По решению из раздела 2 спека DNS-фильтрация остаётся, но в форме, которая соответствует прецеденту Rethink: выключена по умолчанию, без предзагруженного включённого рекламного списка, формулировки про трекеры и вредоносные домены, а не про рекламу.

**Files:**
- Modify: `android/app/src/main/res/values/strings.xml`, `values-ru/strings.xml`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt:37`

- [ ] **Step 1: Проверить дефолт**

`SettingsRepository.kt:37` — `val adblock: Boolean = false`. Дефолт уже выключен; убедиться, что миграции его не включают, и зафиксировать это тестом.

- [ ] **Step 2: Переписать строки**

Найти пользовательские строки про ad-block и переформулировать в терминах трекеров и вредоносных доменов. Формулировки согласовать с человеком — это то, что читает ревьюер Play, и угадывать тут нечего.

- [ ] **Step 3: Коммит**

---

### Task 9: Приёмка

- [ ] **Step 1: Обе Go-конфигурации зелёные**

```bash
go build -tags=mobile ./internal/... ./mobile/...
go build -tags=mobile,no_mitm ./internal/... ./mobile/...
go test -tags=mobile -count=1 ./internal/... ./mobile/...
go test -tags=mobile,no_mitm -count=1 ./mobile/...
```

- [ ] **Step 2: В play-артефакте нет MITM**

`go list -deps` под `no_mitm` не содержит `internal/filter`; `strings` по `libgojni.so` из `libbox-play.aar` не содержит `gomitmproxy`; APK флейвора `play` не содержит `CertWizardScreen`.

- [ ] **Step 3: Оба флейвора ставятся и работают**

Установить `app-full-debug.apk` и `app-play-debug.apk` (на разных устройствах или по очереди, **через `adb install -r -d`**), в обоих: подключиться к рабочему серверу, проверить Smart-режим и per-app исключения. В `play` — убедиться, что переключателя браузерного ad-block нет, а DNS-adblock на месте и выключен.

- [ ] **Step 4: Выравнивание 16 КБ во всех артефактах**

`libbox-full.aar`, `libbox-play.aar`, оба APK, AAB — везде `LOAD align = 0x4000`.

- [ ] **Step 5: Список для Play Console**

Собрать в отдельный документ то, что заполняется руками и кодом не проверяется: форма VpnService, обоснование `FOREGROUND_SERVICE_SPECIAL_USE`, политика конфиденциальности, Data safety (приложение отправляет панели подписки HWID, модель устройства и версию ОС — см. `BuildOptionsBuilder.currentSubscriptionFetchOptionsJson`), возрастной рейтинг.

---

## Готовность блока

- [ ] Обе Go-конфигурации собираются и проходят тесты
- [ ] `internal/filter` отсутствует в зависимостях play-сборки
- [ ] Оба флейвора собираются, ставятся и работают
- [ ] `QUERY_ALL_PACKAGES` убран, пикер исключений не потерял нужных приложений
- [ ] `targetSdk = 36`, ни один экран не уезжает под системные панели
- [ ] `bundlePlayRelease` даёт AAB с выравниванием 16 КБ
- [ ] Список ручных заполнений для Play Console составлен

Дальше — блок 3 (Smart по умолчанию, поле-теги, вставка конфигов одним textarea) по разделу 5 спека.
