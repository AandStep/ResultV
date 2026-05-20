# Код-ревью 01: Точка входа и базовые сервисы

Зона ответственности: корневые Go-файлы (`main.go`, `app.go`, `version.go`, `icon_*.go`), внутренние модули `internal/config`, `internal/logger`, `internal/adblock`, `internal/updater`, скрипты сборки, инструменты, тесты в этих модулях.

Не входит в обзор: `internal/proxy`, `internal/system`, фронтенд, sing-box / sing — это покрывается отдельными ревью.

---

## 1. Обзор зоны

| Подсистема | Назначение | Ключевые файлы |
|---|---|---|
| Точка входа | Bootstrap Wails, single-instance, deeplink, флаги CLI | `main.go`, `version.go`, `icon_*.go` |
| App "контроллер" | Связующий слой между React-фронтом и доменом; все методы экспортируются через Wails `Bind` | `app.go` (2150 строк) |
| Конфигурация | JSON-конфиг (профили, маршрутизация, настройки), AES-GCM шифрование на основе machineID + install-salt, экспорт/импорт через `RESULTPROXY2:` | `internal/config/*.go` |
| Логгер | In-memory ring-buffer на 500 записей, пагинация, эмит в Wails-runtime | `internal/logger/logger.go` |
| AdBlock | Список доменов (fallback + кэш), suffix-match | `internal/adblock/adblock.go` |
| Updater | Check → Download (SHA-256) → Verify → Install с per-OS стратегиями | `internal/updater/*.go` |
| Сборка | Wails build + sing-box build-tags + libcronet (Cronet) + nfpm/linuxdeploy/hdiutil | `build-*.sh`, `scripts/ensure-libcronet-*` |

---

## 2. `main.go` и точка входа

`main.go:35-109` — 75 строк bootstrap-логики. Поток:

1. **AUMID для Windows** (`main.go:37`) — `system.SetProcessAppUserModelID()` для корректного pinning в taskbar и нотификаций.
2. **Создание `App`** (`main.go:39`) — структура только инициализируется; реальная работа в `App.startup` под Wails-callback.
3. **CLI-флаги**:
   - `--start-in-tray` через `system.ArgsStartInTray` → `app.startInTray`.
   - Deeplink `resultv://...` через `system.ExtractDeepLinkArg` → `app.QueueDeepLink` (хранится в `pendingDeepLink` до момента, когда `ctx` доступен).
4. **Иконка трея** загружается из embed-варианта по OS (`icon_*.go` через build-теги).
5. **Single-instance**:
   - Не-Windows платформы: встроенный Wails `SingleInstanceLock{UniqueId: "resultv-desktop"}`, у которого `OnSecondInstanceLaunch` принимает `data.Args`, обрабатывает deeplink и восстанавливает окно (`main.go:92-102`).
   - Windows: используется собственный `system.InitSingletonMessenger` (`main.go:49-55`), а Wails-lock НЕ подключается. Причина не комментирована в коде — вероятно, Wails-lock на Windows конфликтует с WebView2 / именованным mutex. Cleanup-функция возвращается и вызывается defer'ом.
6. **`options.App`**:
   - Hooks: `OnStartup → app.startup`, `OnBeforeClose → app.BeforeClose`, `OnShutdown → app.shutdown`.
   - Размер 1080×720, MinWidth 800×600, фон `#18181B` (dark).
   - Windows-секция содержит крайне специфичные цвета title-bar (все равны `RGB(24,24,27)`) — кастомная dark-тема, заголовок сливается с фоном; `WebviewUserDataPath` управляется через `system.WebviewUserDataPath()`.
   - `Bind: []interface{}{app}` — весь публичный API передаётся фронтенду одной структурой.

### Найденные нюансы
- `main.go:97-100` — на Linux/macOS `system.ArgsStartInTray(data.Args)` вообще не проверяется; повторный запуск с `--start-in-tray` восстановит окно (т.к. идёт безусловный `restoreMainWindow`). Это не критично, но симметрия с Windows нарушена.
- `log.Fatalf` (`main.go:107`) при ошибке `wails.Run` — никаких финальных cleanup-операций (например, `cleanupMessenger` defer уже отработает, но `app.shutdown` мог не вызваться, если сбой произошёл до wails-loop).

---

## 3. `app.go` — структура и группы методов

`App` (`app.go:79-108`) — 25 полей, типичный god-object. Подчёркнуты ключевые инварианты:

| Поле | Тип | Защита | Назначение |
|---|---|---|---|
| `ctx`, `cancel` | `context.Context`, `CancelFunc` | устанавливается в `startup`, проверяется почти всеми методами на `nil` | базовый контекст приложения |
| `log` | `*logger.Logger` | внутренний `sync.RWMutex` | общий логгер |
| `crypto`, `config` | `*config.CryptoService`, `*config.Manager` | внутренний mutex `Manager.mu` | конфигурация |
| `proxy` | `*proxy.Manager` | свой mutex | sing-box engine |
| `adblock`, `tray`, `killSwitch`, `netmon` | сервисы | свои примитивы | реестр платформенных сервисов |
| `stateMu` | `sync.Mutex` | защищает `quitRequested` | синхронизация trayHide/quit |
| `trayHidden` | `atomic.Uint32` | без локов | флаг "приложение в трее" |
| `deepLinkMu` + `pendingDeepLink` | mutex + string | пара | очередь deeplink до готовности `ctx` |
| `updateMu` + `updateCancel` | mutex + cancel | пара | используется в методах CheckForUpdate*/Cancel*Update (не показаны в зачитанной части, объявлены тут) |
| `taskbarUnhook` | `func()` | без локов, пишется только в `startup` | unhook WinAPI taskbar-восстановления |
| `smartProvider` | `proxy.BlockedListProvider` | назначается единожды в `initSmartBlockedDomains` | для GeoLite и smart-routing |

`stableHWIDProvider = config.StableHardwareID` (`app.go:54`) — глобальная переменная-указатель функции; единственная цель — позволить тестам подменить HWID без хака внутри `config`. Это нормально, но это глобальное состояние.

### Жизненный цикл

| Метод | Что делает |
|---|---|
| `NewApp()` | Только `log` и `adblock` инициализируются (`app.go:110-115`). Всё остальное — в `startup` |
| `startup(ctx)` | 165 строк — длинная процедура; см. ниже |
| `shutdown(ctx)` | Через goroutine + 10-секундный watchdog + `os.Exit(0)` на таймауте (`app.go:371-417`) |
| `BeforeClose(ctx)` | Возвращает `true` (не дать закрыть), вместо этого скрывает окно в трей; `false` только если `quitRequested` |

#### Поток `startup`

1. `context.WithCancel(ctx)` — формирует `a.ctx` (`app.go:206`).
2. Logger получает `EventEmitter`, который проксирует через Wails-runtime (`app.go:208-210`).
3. Регистрация протокола `resultv://` (`system.RegisterResultVProtocol`, `app.go:214`).
4. Миграция legacy user-data (`system.MigrateLegacyUserData`).
5. Инициализация `CryptoService` (`app.go:225`) — может вернуть ошибку, тогда `a.crypto` остаётся nil и **`startup` возвращается без поднятия остальных подсистем** — критическая точка отказа, ниже см. Issue C-3.
6. `config.Manager.Init(userDataPath)` (`app.go:236`). При `ErrDecryptFailed` логирует Warning и продолжает с default-конфигом.
7. `proxy.NewManager(log)` + `Init(ctx)`.
8. `initSmartBlockedDomains` (для GeoLite/smart-routing — не показано детально).
9. `adblock.LoadFromCache(userDataPath)`.
10. **Kill-switch cleanup** (`app.go:255-265`) — `Disable()` вызывается всегда, чтобы снять остаточные правила фаервола после crash'а предыдущего запуска. Комментарий объясняет.
11. Установка `KillSwitchFirewallEngage / Disengage` колбэков на `proxy.Manager` (`app.go:267-286`) — watchdog внутри proxy эскалирует firewall, когда upstream становится недоступным.
12. `NetMonitor.Start(ctx)` (`app.go:288-296`) — пушит события `network:status` во фронт.
13. `Tray.Start()` + callbacks (`OnShowWindow`, `OnSelectProxy`, `OnConnectSelected`, `OnDisconnect`, `OnQuit`, `OnUnexpectedExit`) (`app.go:298-336`). `OnUnexpectedExit` интересный: если systray умер, а окно скрыто, делает `time.Sleep(300ms) + os.Exit(0)` — единственный способ восстановления.
14. `DetectGPOConflict` (Windows GPO override warning).
15. `StartTaskbarRestoreHook` — Windows hack для возврата из tray по клику на taskbar.
16. Если `startInTray` — `WindowHide`, ставит флаг trayHidden=1.
17. Применение отложенного deeplink (`a.pendingDeepLink`).

#### Группы публичных методов (примерно 70+ методов)

| Группа | Методы | Что делают |
|---|---|---|
| Версия/мета | `GetVersion`, `GetPlatform`, `GetUpdateManifest`, `DebugFrontendLog` | базовая инфа фронту |
| Deep-link | `QueueDeepLink`, `HandleDeepLink` | хранение/обработка `resultv://` |
| Подключение | `Connect`, `Disconnect`, `CancelConnect`, `GetStatus`, `GetMode`, `SetMode`, `ApplyMode`, `PingProxy` | core proxy flow |
| Kill-switch | `enableKillSwitchFirewall` (internal), `ToggleKillSwitch` | firewall control |
| AdBlock | `ToggleAdBlock` | отметка в конфиге |
| Конфиг | `GetConfig`, `SaveConfig`, `UpdateRules`, `ExportConfig`, `ImportConfig` | CRUD над AppConfig |
| Автозапуск | `SetAutostart`, `IsAutostartEnabled` | OS-specific |
| Сетевые помощники | `GetNetworkTraffic`, `GetNetworkStatus`, `GetLANIPs`, `DetectCountry` | net info |
| Подписки | `FetchSubscription`, `ParseSubscriptionText`, `RefreshSubscription`, `AddSubscription`, `DeleteSubscription`, `SyncProxies` | sub-flow |
| App-whitelist | `PickAppForWhitelist` | OpenFileDialog → нормализованный entry |
| Окно | `restoreMainWindow`, `verifyWindowOrExit`, `BeforeClose` | tray ↔ window |
| Tray | `connectFromTray`, `setLastSelectedProxyId`, `refreshTrayProxyList`, `resolveProxyID`, `startTrayPingLoop` (не показан) | синхронизация трея и состояния |
| Логи | `GetLogs(page, size)` | пагинированный fetch |
| Прав. | `IsAdmin`, `RestartAsAdmin` | UAC-перезапуск |
| Внутренние | `getUserDataPath`, `getAppRootDir`, `markQuitRequested` | хелперы |

#### Подсистема подписок (`app.go:1044-1972` — почти половина файла!)

Сложная многоступенчатая логика:

- **`fetchSubscriptionFromURL`** (`app.go:1533-1740`) — обёртка с retry через два User-Agent:
  - Сначала `ResultV/<version>`.
  - Если нет hysteria-entries и тело не JSON — повтор с `Happ/1.0`.
- **HWID-схема**: per-host hashed HWID (`subscriptionHWID`, `app.go:1137-1153`) — `sha256(hwid + "|" + host + "|resultv-sub-hwid-v2")`. Если host пустой (paste flow) — отдаётся "сырой" hwid. Хорошее cross-correlation prevention.
- **Insecure URL gating**: `ErrInsecureSubscription` (`app.go:1518`), `isInsecureSubURL` блокирует http://. `AllowInsecure` запоминается в `Subscription`.
- **SSRF guard для иконок**: `safeImageDialer` (`app.go:1284-1302`) использует `Dialer.Control` для проверки IP после DNS-resolve, что закрывает DNS-rebinding. Очень аккуратная реализация.
- **Same-host Referer** (`app.go:59-75`) — Referer крепится только при совпадении хоста (защита от утечки subscription URL с токеном третьим сторонам).
- **Auto-group построение** (`app.go:1665-1736`) — если у подписки много серверов одного провайдера, формируется виртуальный `AUTO` proxy и собранные участники прячутся под ним. Используется `crc32.ChecksumIEEE(subURL+"auto")` для стабильного ID.
- **HTML-парсинг иконок** (`app.go:1396-1452`) — 7 разных regex'ей для og:image, twitter:image, apple-touch-icon, обычного `<link rel=icon>`, `<img logo>`. Все ограничены 256 КБ HTML.

---

## 4. `internal/config`

### Файлы и формат

| Файл | Что внутри |
|---|---|
| `config.go` | Структуры `AppConfig`, `ProxyEntry`, `Subscription`, `AppSettings`, `RoutingRules`; `Manager` с RWMutex, методы Init/Save/Update; миграции legacy директорий `ResultProxy → ResultV` |
| `crypto.go` | `CryptoService`; PBKDF2-derived AES-256 ключ + legacy SHA-256 fallback; install-salt management |
| `export.go` | `ExportConfig`, `ImportConfig`, `MergeImport`; обработка legacy prefix `RESULTPROXY:` |
| `export_v2.go` | `EncryptExportV2`/`DecryptExportV2`: новый `RESULTPROXY2:` формат — PBKDF2(600k) + AES-GCM + AAD |
| `machineguid_windows.go` | `windowsMachineGUID` (HKLM Crypto + WMI UUID fallback) |
| `machineguid_other.go` | stub'ы для non-Windows |

### Шифрование (config-at-rest)

Поток `NewCryptoService` (`crypto.go:105-120`):
1. `getHardwareID(userDataPath)` — Windows: Registry `SOFTWARE\Microsoft\Cryptography\MachineGuid` или PowerShell-WMI UUID; Darwin: `ioreg IOPlatformUUID`; Linux: `/etc/machine-id`. Fallback: UUID v4 в `<userData>/.machine-fallback-id`.
2. `loadOrCreateInstallSalt` — 32 байта в `<userData>/.install-salt` (mode 0o600).
3. Производный ключ: `PBKDF2(machineID, installSalt, 200000, sha256, 32)`.
4. Legacy ключ: `sha256(machineID + "_ResultProxy_SafeVault_v1")` — для миграции.

`Encrypt` (`crypto.go:214-258`): AES-GCM с **16-байтным nonce** через `cipher.NewGCMWithNonceSize(block, 16)`. Это нестандартный размер (обычно 12). См. Issue M-1.

`Decrypt` (`crypto.go:263-309`): пробует current key → legacy key. При успехе legacy-key выставляется флаг `needsReencrypt`. `Manager.Init` после `loadLocked` смотрит этот флаг и пересохраняет (`config.go:159-169`).

`secureEnvelope` (`crypto.go:82-87`): `_isSecure, iv, data, authTag` (Node-style); если `_isSecure=false` → возвращается raw как есть → совместимость со старыми plaintext-конфигами.

### Экспорт/импорт (cross-machine)

Формат v2 (`exportPrefixV2 = "RESULTPROXY2:"`):
```
RESULTPROXY2:<base64(json{v, kdf, iter, salt, nonce, ct})>
```
- KDF: PBKDF2-SHA256 600k iter (OWASP 2023+).
- AAD = `"RESULTPROXY2"` (без двоеточия) — препятствует prefix-rewrap-атакам.
- Iter < 100 000 — отказ (`export_v2.go:155-159`), защита от downgrade.

Legacy v1 (`RESULTPROXY:`) парсится для совместимости, но возвращает `ErrLegacyPlaintext`, чтобы UI мог предупредить пользователя.

`MergeImport` (`export.go:104-117`) — overwrite routing rules, **append** proxies со скипом по ID. Settings из импорта игнорируются (per-device).

### Миграции

- `migrateLegacyConfigFile` (`config.go:173-195`) — переносит `ResultProxy/proxy_config.json` → `ResultV/proxy_config.json`, если у нового места ничего нет. Сначала `os.Rename`, при неудаче — `ReadFile`+`WriteFile`+`Remove`.
- `promoteLegacyConfigIfNeeded` (`config.go:197-233`) — если новый пустой, а legacy непустой, заменяет содержимое.
- `legacyUserDataDir` (`config.go:235-240`) — простая склейка `<dirname>/ResultProxy` от пути с `ResultV`.

### Замечания
- `Manager.UpdateRoutingRules` (`config.go:281-288`) кратко берёт snapshot под Lock, тут же отпускает, и вызывает `SaveConfig` — корректный pattern, но race window между unlock и SaveConfig теоретически даёт другому writer обогнать (хотя SaveConfig тоже под Lock — последнего записавшего сохранит, первого перетрёт).
- `legacyUserDataDir` возвращает `""` для произвольного `userDataPath` (если базовое имя не `ResultV`), что приводит к "осмысленному" `os.Stat("")` → ENOENT и тихому скипу миграции — в тестах работает, в проде тоже, но это сильное допущение.

---

## 5. `internal/logger`

`Logger` (`logger.go:58-63`):
- Ring buffer на **500 записей** (`defaultCapacity`), новые в **начало** (prepend → `[]LogEntry{entry}, l.entries...`).
- `sync.RWMutex`, RLock на чтения, Lock на запись.
- `EventEmitter` (любая `func(name string, data any)`) — фронт получает событие `"log"` на каждый `add` (`logger.go:215-217`).
- Не пишет на диск! `fmt.Printf` в stdout (`logger.go:220`), но это идёт в "никуда" в GUI-приложении на Windows (нет консоли) — реально логи только в памяти.

### Замечания
- **Нет ротации, нет персистентности.** При рестарте приложения вся история логов теряется. См. Issue H-2.
- **`emit` дёргается вне локов** (`logger.go:215`) — корректно, но возможна реентерантность, если emit куда-то опять вызывает `log.Info` → дедлок, поскольку `add` уже отдал мьютекс к этому моменту. На практике безопасно.
- **Prepend `O(n)`** через `append([]{entry}, old...)` — на каждое сообщение копируется весь буфер. 500 элементов мало, но при горячих логах (например, ping каждую секунду) это десятки тыс. аллокаций/мин. См. Issue M-2.

---

## 6. `internal/adblock`

`Blocker` (`adblock.go:52-67`): `map[string]struct{}` + RWMutex. На старте — fallback 11 доменов (`fallbackAdDomains`).

- `LoadFromCache(dir)` — читает `<dir>/adblock_domains.txt`, пропускает `#`-комментарии, добавляет в lowercase.
- `UpdateLists(dir)` — качает с `https://pgl.yoyo.org/adservers/serverlist.php?...`, парсит hosts-формат, мерджит. Кэш пересохраняется (`os.Create` → перезапись).
- `IsAdDomain(host)` — точный hit, потом **итерация по всей карте** и проверка `strings.HasSuffix(h, "."+d)`. См. Issue M-3.

### Замечания
- Линейный suffix-поиск (`adblock.go:180-184`) — при 30к доменов и тысячах вызовов в секунду (на каждый DNS-запрос sing-box) это узкое место. Должен быть `domainTrie` или индекс по последнему лейблу.
- `UpdateLists` (`adblock.go:99-161`) — НЕ имеет таймаута на `client.Get` сверх дефолта 30s; ошибка одного из URL'ов — пропуск (`continue`), без логирования. Также `defer resp.Body.Close()` в цикле — будет накапливаться, если URL'ов много (тут один, но pattern опасный).
- Кэш-файл пишется без atomic-rename (`os.Create` → запись по байту) — при крэше посередине файл окажется частично записан, и `LoadFromCache` затащит мусор. См. Issue M-4.
- Нет TTL — `UpdateLists` нужно явно вызывать; неясно, откуда (в просмотренной части `app.go` не нашлось).

---

## 7. `internal/updater`

Цепочка `Check → Download → Verify → Install` с разделённой ответственностью.

### `manifest.go`
- `FetchManifest(ctx, url)` — HTTP GET с таймаутом 15s, `LimitReader 64 КБ` (защита от bomb).
- `Manifest{Version, DownloadURL, Platforms map[string]*PlatformAsset}` — пер-OS асеты.
- `Manifest.ResolveAsset()` — выбирает asset по `currentPlatformKey()` (build-tag dispatched).
- `ValidateAssetURL` — **HTTPS only** + **whitelist хостов**.

### `download.go`
- `downloadFile(ctx, url, dest, expectedSize, fn)`:
  - Hard limit **200 МБ** (`maxDownloadBytes`).
  - Сверяет `Content-Length` ↔ `expectedSize` из манифеста.
  - `os.CreateTemp(dir, "resultv-update-*.tmp")` + `chmod 0o600` (`download.go:65-75`).
  - Чанки 32 КБ, progress callback (downloaded, total, bps).
  - **Cancel checked between chunks** (`download.go:111-115`).
  - На успехе — атомарный `os.Rename(tmp, dest)`.
- `defer os.Remove(tmpPath)` — temp удаляется в любом исходе. На успехе rename уже сделан, Remove получит ENOENT — корректно.

### `verify.go`
- `verifySHA256(path, expectedHex)` — SHA-256 по содержимому, hex-compare (case-insensitive).
- **При несовпадении удаляет файл** перед возвратом ошибки — защита от использования.

### `updater.go` (orchestrator)
- `Updater{AllowedHosts, ManifestURL, DownloadDir}`.
- Manifest URL: `ManifestURLOverride` (ldflag) → `RESULTV_MANIFEST_URL` env → `productionManifestURL` (GitHub raw на `main`).
- `AllowedHosts` хардкодит `github.com`, `objects.githubusercontent.com`, `result-proxy.ru`, `www.result-proxy.ru`.
- `Download(ctx, asset, fn)` — собирает имя `ResultV-update-<sha8>.<ext>` где `<sha8>` — первые 8 hex SHA-256.

### Per-OS installer

| OS | Файл | Стратегия |
|---|---|---|
| Windows | `installer_windows.go:62-67` | Если exe в `Program Files*` → NSIS silent (`installer /S /ALLUSERS|/CURRENTUSER`); иначе portable copy через PowerShell handover script |
| macOS | `installer_darwin.go:33-60` | `hdiutil attach -nobrowse -mountpoint /tmp/resultv-update` → `ditto` копия → `open /Applications/ResultV.app` → `os.Exit(0)` |
| Linux | `installer_linux.go:33-55` | chmod +x → backup `<exe>.bak` → rename → `syscall.Exec(currentExe, ...)` (in-place exec) |
| Прочие | `installer_stub.go` | возврат ошибки |

#### Windows: hand-over PowerShell

Скрипт пишется в `%TEMP%\resultv-updater-<ns>.ps1` (mode 0o600), запускается detached. **Логи** — `%TEMP%\resultv-updater.log` (append).

`buildInstallerHandoverScript` (`installer_windows.go:140-194`) формирует PS-скрипт:
- `Wait-Process -Id <pid> -Timeout 45` — ждёт выход родителя.
- `Start-Process installer.exe -ArgumentList '/S', '/ALLUSERS' -Wait -PassThru` → ловит `ExitCode`.
- При ExitCode=0 — ищет `InstallLocation` в реестре `HKLM\...\Uninstall\ResultVResultV` и `HKCU\...` (порядок зависит от scope) и запускает обновлённый exe.

`buildPortableHandoverScript` (`installer_windows.go:105-138`): Sleep 8s → 20 попыток `[IO.File]::Copy(src, dst, $true)` по 1s → Remove src → Start-Process dst.

`powershellEscapeSingleQuoted` (`installer_windows.go:219-221`) экранирует одинарную кавычку удвоением — стандартная техника для `'...'`-строк PS. **Однако** все пути идут в PowerShell как одинарно-цитированные литералы, и инъекция через специальные символы вне `'` теоретически возможна, если бы они попали в шаблон **не через переменную**. Здесь все пользовательские пути идут через переменные → защита достаточна.

#### macOS особенности
- `defer exec.Command("hdiutil", "detach", mountPoint, "-force").Run()` — best-effort detach, не проверяет ошибку.
- **`os.Exit(0)` сразу после launch'а нового бандла** — никакого graceful shutdown, kill switch правила и proxy-engine могут остаться поднятыми. См. Issue H-3.

#### Linux особенности
- Бэкап `.bak` НЕ удаляется при успехе → накапливается мусор на каждом обновлении. См. Issue L-4.
- `syscall.Exec(currentExe, os.Args, os.Environ())` — при сбое syscall.Exec возвращает ошибку и текущий процесс продолжает выполнение (хотя файл уже подменён) — потенциально опасное состояние.

---

## 8. Скрипты сборки

| Скрипт | Что делает | Зависимости |
|---|---|---|
| `build-release.sh` | Чтение `SUBSCRIPTION_ENCRYPT_KEY` из `.env`, вызов `wails build -nsis` с `-ldflags -X` | `.env` (с ключом), `scripts/ensure-libcronet-windows.ps1`, NSIS |
| `build-linux.sh` | `wails build -tags webkit2_41`, stages `libcronet.so` к выходному бинарю, делает `nfpm pkg` (deb/rpm), AppImage через `linuxdeploy` | nfpm, linuxdeploy, libwebkit2gtk-4.1 |
| `build-macos.sh` | `wails build darwin/universal`, копирует `libcronet.dylib` в .app/Contents/MacOS/, codesign + notarize, упаковывает в .dmg | Apple Developer ID, hdiutil, notarytool |
| `scripts/ensure-libcronet-windows.ps1` | `go mod download` модуля `cronet-go/lib/windows_amd64`, копирует `libcronet.dll` в `build/windows/` | go, доступ к модулю |
| `scripts/ensure-libcronet-linux.sh` | `cronet-go/lib/linux_amd64` → `build/linux/libcronet.so` | go |
| `scripts/ensure-libcronet-macos.sh` | amd64 + arm64 → `lipo -create` → `build/darwin/libcronet.dylib` | go, lipo (Xcode) |

### Build tags (wails.json)
```
with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_naive_outbound,with_purego
```
`with_purego` критичен для динамической загрузки libcronet через purego (без CGo).

### `-ldflags -X` для секретов
- `SUBSCRIPTION_ENCRYPT_KEY` → `internal/proxy.subscriptionEncryptKey` — ключ для расшифровки RVSUB1 deeplink-payload'ов.
- `MANIFEST_URL_OVERRIDE` → `internal/updater.ManifestURLOverride` — для dev-веток.

### Замечания по скриптам
- `build-release.sh` использует `grep '^SUBSCRIPTION_ENCRYPT_KEY=' .env | cut -d= -f2` — наивный парсер, не учитывает кавычки/escape, но для одного ключа сойдёт.
- `build-linux.sh`/`build-macos.sh` обрабатывают пустой LDFLAGS через **дублирование** вызова `wails build` (`if [ -n "$LDFLAGS" ]; then ... else ...`). Можно упростить: `wails build $LDFLAGS_FLAG` с условной переменной. Не критично.
- macOS-скрипт **загружает .env вручную через `set -a; source .env`** — потенциальный security risk, если `.env` имеет shell-инъекцию (на машине разработчика — терпимо).

---

## 9. Тесты

### Корневой пакет `main`

| Файл | Что тестирует | Покрытие |
|---|---|---|
| `app_icon_ssrf_test.go` (111 строк) | `isPrivateOrLoopback` на широкий диапазон IP (RFC1918, CGNAT, link-local v6, multicast); `sameHostReferer` на match/mismatch/case/empty/malformed | Полный набор edge-cases для SSRF guard |
| `app_subscription_test.go` (221 строк) | HWID-хэш per-провайдер, https→hwid, http→no-hwid, refuse-default, empty-body diagnostics с base64-декодом, Profile-Title override, apple-touch-icon resolver | Хорошее покрытие основных security-инвариантов |

**Дыры**:
- `Connect/Disconnect`, `ApplyMode` — нет тестов. Учитывая сложный rollback-флоу в `ApplyMode` (`app.go:659-689`), это критическая дыра.
- `RefreshSubscription`, `AddSubscription` — не тестируются вообще.
- `DetectCountry` — нет.
- Tray-callbacks, deeplink end-to-end — нет.

### `internal/config`

| Файл | Покрытие |
|---|---|
| `config_test.go` (≈460 строк) | Round-trip Manager save/load, missing/corrupted file (`ErrDecryptFailed`), `UpdateRoutingRules`, `ensureDefaults`, легаси-миграция директорий, e2e миграция ключа (legacy → PBKDF2) |
| `crypto_test.go` (≈313 строк) | Encrypt/Decrypt round-trip, неверный ключ, legacy plaintext, install-salt generate/reuse/rotate, fallback на legacy key, отказ при обоих неверных ключах, уникальность IV |
| `export_test.go` (≈282 строк) | v2 round-trip, wrong password (`ErrWrongPassword`), требование пароля при импорте, минимальная длина, недетерминированность (salt/nonce), tamper-detection (bit-flip → `ErrWrongPassword`), отказ от слабых KDF-параметров (`iter=1`), `MergeImport` |

**Слабые места**:
- `TestNodeJSCompatibility` (`crypto_test.go:143-158`) — `t.Skip("TODO")`, оставлен как заметка о кросс-совместимости с Node-вариантом шифрования. Если такой кросс используется (например, веб-вариант) — это дыра.
- Нет тестов на `getHardwareID` fallback-цепочку (windows-WMI fail → registry → fallback file).

### `internal/logger`

`logger_test.go` (165 строк): count, newest-first, capacity eviction, пагинация, event-emitter, clear, **конкурентный** доступ (100 writers + 50 readers).

Дыры: нет тестов на behavior при панике в emitter (что произойдёт?).

### `internal/adblock`

`adblock_test.go` (78 строк): fallback domains, suffix match, empty hostname, case-insensitive, load from cache, `GetDomains` count.

Дыры: нет тестов на `UpdateLists` (HTTP-fetch — можно через httptest), на конкурентный LoadFromCache+IsAdDomain.

### `internal/updater`

| Файл | Покрытие |
|---|---|
| `updater_test.go` (≈277 строк) | `ValidateAssetURL` (https-only, host whitelist, case-insensitive), `Manifest.ResolveAsset` для текущей платформы / nil platforms / empty URL, `verifySHA256` match + mismatch (с удалением файла), `FetchManifest` OK / 404 / bad JSON, `downloadFile` OK + size-mismatch + 500 + context cancel |
| `installer_windows_test.go` (77 строк, build-windows) | PowerShell-handover скрипты содержат ожидаемые токены (Wait-Process, /ALLUSERS vs /CURRENTUSER, registry-paths, copy operation) |

Дыры:
- Нет тестов реального `installUpdate` ни на одной платформе — handover, ditto, syscall.Exec не верифицируются.
- Нет тестов `RESULTV_MANIFEST_URL` env-fallback.

---

## 10. Карта потоков данных

### Поток A: Connect (нажатие кнопки на фронте)

```
frontend "Connect" click
  → wails RPC → app.Connect(proxyDTO, rules, killSwitch, adBlock)  [app.go:455]
    → a.config.GetConfig()                                          [config/config.go:243]
    → dnsServersFromProxyExtra(proxyDTO)                            [app.go:495]
    → a.proxy.Connect(ctx, ...)                                     [proxy.Manager — другая зона]
    → если успешно: a.tray.SetConnectedProxy(...)
                    wailsRuntime.EventsEmit("proxy:connected", proxyDTO)
```

### Поток B: Запуск приложения

```
main()                                                              [main.go:35]
  → system.SetProcessAppUserModelID() (win)
  → NewApp() [app.go:110]
  → parse os.Args → SetStartInTray / QueueDeepLink
  → InitSingletonMessenger (Windows)
  → wails.Run(opts) — wails вызывает app.startup(ctx)               [app.go:205]
    → context.WithCancel
    → logger.SetEmitter(wails)
    → system.RegisterResultVProtocol
    → system.MigrateLegacyUserData
    → config.NewCryptoService(userDataPath) — генерит install-salt   [config/crypto.go:105]
    → config.NewManager + Manager.Init                               [config/config.go:137]
      → migrateLegacyConfigFile, promoteLegacyConfigIfNeeded
      → loadLocked → cs.DecryptInto
      → если needsReencrypt → SaveConfig + ClearReencryptFlag
    → proxy.NewManager + Init
    → initSmartBlockedDomains
    → adblock.LoadFromCache
    → killSwitch cleanup (Disable())
    → netmon.Start
    → tray.Start (+ callbacks)
    → StartTaskbarRestoreHook (win)
    → applyQueuedDeepLink (если есть)
```

### Поток C: Импорт подписки

```
frontend "Add subscription" → AddSubscription(name, url, allowInsecure, source)  [app.go:1855]
  → a.config.GetConfig() — поиск stale-записи по URL
  → fetchSubscriptionFromURL(url, allowInsecure)                                  [app.go:1533]
    → resolveEncryptedSubscriptionURL — RVSUB1 unwrap                             [app.go:1485]
    → normalizeSubscriptionURL — особые правила для my.impio.space                [app.go:1504]
    → проверка https/insecure → ErrInsecureSubscription если http без consent
    → doFetch(primaryUA: "ResultV/x.x.x") с x-hwid (per-host hash)
      → resolveSubscriptionIcon → headers / page / origin-fallback / safe-image
      → proxy.ParseSubscriptionBody (другая зона)
    → если pin не hysteria + не JSON → retry с UA="Happ/1.0"
    → providerName из URL или Profile-Title
    → FinalizeSubscriptionEntries
    → SplitAutoEntries / ExtractAutoGroupName → формирование AUTO-group
  → config.Manager.SaveConfig(cfg with new Subscription)
```

### Поток D: Обновление приложения

```
frontend "Check update" → app.GetUpdateManifest()                  [app.go:131]
  → updater.New().Check(ctx)
    → FetchManifest(ctx, manifestURL)                              [updater/manifest.go:44]
  → возвращает Manifest

frontend "Update now" → (другой метод не показан, но логика):
  → Updater.Download(ctx, asset, fn)                               [updater/updater.go:75]
    → ValidateAssetURL (https + host whitelist)
    → downloadFile(ctx, url, dest, expectedSize, fn)               [updater/download.go:33]
      → temp file + chmod 0600
      → chunked read + progress + cancel-check
      → atomic rename
  → Updater.Verify(path, sha256)                                   [updater/verify.go:29]
    → SHA-256 compare; mismatch → delete + error
  → Updater.Install(path)                                          [updater/updater.go:107]
    → платформенный installUpdate
    → Windows: PowerShell handover (portable/NSIS) — возврат nil
    → Darwin: hdiutil + ditto + open + os.Exit(0)
    → Linux: backup + rename + syscall.Exec
  → frontend получает успех → может предложить graceful quit (для Win)
```

---

## 11. Найденные проблемы и предложения по рефакторингу

### Critical

#### C-1 — `app.go` god-object (2150 строк, ~75 публичных методов)
**Файл**: `app.go` целиком  
**Проблема**: один тип агрегирует все сервисы и весь бизнес-API. Подсистема подписок занимает почти половину файла (`app.go:1044-1972`). Нарушение SRP, тестируемость низкая, изменения в одной подсистеме ломают компиляцию всего файла.  
**Предложение**:
- Выделить `internal/subscription/` (fetch, parse-icons, RVSUB-decrypt, HWID, AUTO-group build) — это совершенно самостоятельный модуль, и в `app.go` останутся только тонкие RPC-обёртки.
- Выделить `internal/lifecycle/` (startup/shutdown coordination) или `internal/appservice/` для группировки kill-switch firewall callbacks и netmon-hooks.
- В `App` оставить только маршрутизацию + Wails-Bind. Текущий App нарушает 5+ принципов сразу.

#### C-2 — `shutdown` через goroutine + `os.Exit(0)` (`app.go:374-416`)
**Проблема**: при таймауте 10s основной shutdown-goroutine может ещё работать — `cancel()`, `proxy.Shutdown()`, `killSwitch.Disable()` — и `os.Exit(0)` обрывает её посреди операции. Если в этот момент идёт `killSwitch.Disable()` (запись правил фаервола), это оставит систему в полузаблокированном состоянии без интернета.  
**Предложение**: либо ужесточить порядок (kill-switch Disable выполнять первым и СИНХРОННО до запуска goroutine), либо добавить отдельный watchdog timer на конкретно ту операцию, что может зависнуть (sing-box `instance.Close`).

#### C-3 — Silent error на инициализации `CryptoService` (`app.go:225-229`)
**Проблема**: если `NewCryptoService` упал — `startup` логирует и `return`'ит, оставляя `a.config = nil`, `a.proxy = nil`, `a.tray = nil`. Дальше любой RPC-вызов вернёт "X manager not initialized", UI в неопределённом состоянии, окно показано пустое. Приложение фактически зомби.  
**Предложение**: либо `os.Exit(1)` с понятным сообщением, либо emit'нуть фронту `app:fatal-init-error` и заблокировать UI на splash-экране с инструкцией.

### High

#### H-1 — Inconsistent shutdown в macOS updater (`installer_darwin.go:58`)
**Проблема**: `os.Exit(0)` сразу после `open /Applications/...` — kill-switch правила не сняты, proxy engine жив, новые правила не применены. После запуска нового бандла он попытается снять/поднять правила, но между exit и launch может быть окно без интернета или с лежащим кэшем правил.  
**Предложение**: вызвать `Updater.Install` ИЗ `app.go`, где доступны `killSwitch`/`proxy`, и сделать graceful shutdown ПЕРЕД installUpdate (как уже сделано на Windows portable: `return nil` + wails quit).

#### H-2 — Логи не персистентны (`internal/logger/logger.go`)
**Проблема**: 500 in-memory записей, нет ротации на диск. При краше пользователь не может приложить лог к багу. Большой undocumented недостаток.  
**Предложение**: интегрировать `gopkg.in/natefinch/lumberjack.v2` (уже в зависимостях через transitive) — писать `<userData>/logs/app.log` с ротацией по размеру/дням. `fmt.Printf` на `logger.go:220` — рудимент. Также внутри updater'а есть hint на `resultv-updater.log` в `%TEMP%` — стоит унифицировать.

#### H-3 — `lastFatal` от systray не передаётся пользователю (`app.go:321-334`)
**Проблема**: `OnUnexpectedExit` логирует Warning и `os.Exit(0)` через 300мс. Пользователь не понимает, что произошло — окно скрыто в трей и приложение просто исчезает.  
**Предложение**: показать notification "ResultV завершён из-за ошибки трея, перезапустите приложение" перед exit. Запись в persistent-log (см. H-2).

#### H-4 — DNS-серверы из `Extra` подписки могут переопределить пользовательские настройки (`app.go:464-467, 624-627`)
**Проблема**: `dnsServersFromProxyExtra(proxyDTO)` побеждает над `cfg.Settings.DNSServers`. Hostile-провайдер может задать собственные DNS, маршрутизировать всё через свой контрольный DNS — privacy/security регрессия. Нет UI-сигнала об этом.  
**Предложение**: либо явное согласие пользователя на DNS-override, либо приоритет в обратную сторону. Минимум — лог "Используются DNS из подписки: ..." Warning-уровня и `system:provider-dns-override` event для UI-badge.

#### H-5 — Race window в Connect между config.GetConfig и proxy.Connect (`app.go:462-482`)
**Проблема**: между `cfg := a.config.GetConfig()` и `a.proxy.Connect(...)` другой goroutine может сохранить новую конфигурацию. Сейчас не критично, но если UI асинхронно зовёт SaveConfig+Connect — порядок неопределён.  
**Предложение**: явная блокировка connect-flow (`connectMu`) на уровне App, либо передача snapshot'а в proxy.Manager как value (что уже частично делается).

### Medium

#### M-1 — Нестандартный nonce-size 16 в config-шифровании (`crypto.go:228-237`)
**Проблема**: AES-GCM спецификация рекомендует 12-байтный nonce. Использование 16-байтного через `NewGCMWithNonceSize` математически корректно (NIST SP 800-38D позволяет 12-16 для определения IV), но:
- Чуть медленнее (no inline-optimization Go runtime).
- Производит нестандартный envelope, который не откроется стандартным `cipher.NewGCM`.
- Не совместим с типичными библиотеками других ЯП без явной настройки.
Это решение — наследие Node-совместимости (см. `TestNodeJSCompatibility` skipped test) — но если кросс-совместимость уже не нужна, лучше мигрировать на 12-байт.  
**Предложение**: либо документировать это решение явным комментарием, либо мигрировать (с поддержкой обоих размеров на чтение). Сейчас комментарий есть `(crypto.go:225-227)` но он пустой — это просто пустые строки.

#### M-2 — Logger.add — O(n) prepend (`logger.go:207`)
**Проблема**: `l.entries = append([]LogEntry{entry}, l.entries...)` — каждое добавление копирует весь буфер. 500 элементов — 500 копий за лог. При ping-loop с тысячами сообщений в минуту это заметная нагрузка.  
**Предложение**: использовать круговой буфер (`container/ring` или собственный с `head` индексом). Или просто append в конец и reverse на запросе `GetLogs` (если важен LIFO-порядок UI).

#### M-3 — Adblock linear suffix-match (`adblock.go:179-186`)
**Проблема**: `for d := range b.domains { if strings.HasSuffix(...) }` — O(n) на каждый запрос. На 50k доменах и сотнях DNS-запросов в секунду это тормоз.  
**Предложение**: построить domain trie / map по последнему лейблу. Например, `m["net"] = ["doubleclick.net", ...]` → сначала найти последний лейбл, потом проверить.

#### M-4 — Adblock cache pis non-atomic (`adblock.go:147-160`)
**Проблема**: `os.Create + Fprintln` — при крэше посередине файл частично перезаписан. `LoadFromCache` подхватит обрезанный список.  
**Предложение**: писать в `<cachePath>.tmp` → `os.Rename`. Также добавить header-magic в файл для валидации.

#### M-5 — Updater downloads не проверяют MIME-type/exe-magic
**Проблема**: SHA-256 проверка ловит подмену контента, но манифест с правильным sha указывает на правильный exe — нет проверки, что это действительно exe, а не, скажем, текстовый файл с тем же sha.  
Это **не** уязвимость (sha коммитит фактический контент), но дополнительная защита от ошибок (типа сборки manifest'а с правильным sha для неправильного файла).  
**Предложение**: лёгкая sanity-check: первые байты — PE/Mach-O/ELF magic в зависимости от платформы.

#### M-6 — Updater не handles HTTP redirect explicit (`download.go:34`)
**Проблема**: `http.DefaultTransport` следует за redirect'ами автоматически. `ValidateAssetURL` проверяет ИСХОДНЫЙ URL, но redirect может увести на любой хост (всё ещё https, но не whitelist).  
**Предложение**: `client.CheckRedirect = func(req, via) error { return ValidateAssetURL(req.URL.String(), allowedHosts) }`.

#### M-7 — Иконки в `tools/genico/main.go` — hardcoded path `c:/ResultProxyPC/`
**Проблема**: `tools/genico/main.go:30,44,50,57` — абсолютные пути к `c:/ResultProxyPC/...` — старое имя проекта (ResultProxy), не соответствует текущему `ResultVPC`. Если кто-то запустит этот tool сейчас — ошибка `no such file or directory`.  
**Предложение**: использовать `os.Getwd` или `flag.String` для входной картинки/выходного каталога, либо `runtime.Caller` для нахождения корня репо.

#### M-8 — `system.IsMainWindowVisible` polling 400мс (`app.go:2059-2065`)
**Проблема**: hard-coded sleep как условие восстановления окна, после которого `os.Exit(0)`. Если WebView2 медленный, окно появится через 500мс и приложение убьёт себя ложно-положительно.  
**Предложение**: цикл с экспоненциальным backoff'ом до 2-3 секунд, и только потом выход. Или event-driven через Wails-runtime "WindowShown".

#### M-9 — `SaveConfig` пересохраняет subscriptions из существующей конфигурации (`app.go:438-453`)
**Проблема**: логика "если cfg.Subscriptions nil ИЛИ len==0 при существующих — взять из существующих" слишком хитрая. Это нужно для случая, когда фронт прислал AppConfig без subscriptions (т.е. не зная их), но если фронт ХОЧЕТ очистить subscriptions, отправит `[]` — и оно тоже сольётся в `len==0`. Получается, удалить все подписки через SaveConfig нельзя.  
**Предложение**: добавить отдельный метод `ClearSubscriptions()` или флаг в DTO. Сейчас семантика непрозрачна.

#### M-10 — Подписки: `crc32.ChecksumIEEE(subURL+"auto")` для ID (`app.go:1683, 1715`)
**Проблема**: 32-битный CRC, не криптографический, склонен к коллизиям при многих subscription'ах. Используется как primary key для AUTO-entry.  
**Предложение**: использовать `sha256[:8]` или `xxhash` для большей энтропии.

### Low

#### L-1 — Лицензионные комментарии везде дублируются (~14 строк × 60+ файлов)
**Файлы**: все *.go в проекте  
**Проблема**: 14-строчная GPL-шапка в начале каждого файла — ~840 строк только заголовков. На code-review раздражает.  
**Предложение**: `LICENSE` в корне уже хватает (если он есть). Шапку можно сократить до 2-3 строк с ссылкой. Не критично — стандартная практика для GPL.

#### L-2 — `app.go:31` импорт `path/filepath` без явного использования (фактически использован в `getAppRootDir`, проверено)
Не проблема — false alarm после внимательного чтения.

#### L-3 — `tools/licenseheaders/main.go` — `oldName="ResultProxy"`, `newName="ResultV"` (`tools/licenseheaders/main.go:13-14`)
**Проблема**: этот инструмент — одноразовый rename-helper. Он переименовывает "ResultProxy" → "ResultV" в Copyright-строках. Если запустить второй раз — ничего не сделает (но и не сломает). Но это dead-code, который занимает место.  
**Предложение**: после успешного перехода удалить tool, либо переименовать в `tools/applyheaders/` и убрать rename-логику.

#### L-4 — Linux installer: `.bak` накапливается (`installer_linux.go:44-50`)
**Проблема**: при каждом обновлении создаётся `<exe>.bak`, который не удаляется. После 10 обновлений будет 10 файлов `.bak` (хотя `os.Rename` уже перезапишет существующий — фактически останется один последний).  
Уточнение: `os.Rename(currentExe, backupPath)` со старым `<exe>.bak` существующим — на Linux это перезапись, так что накопления НЕ происходит. Замечание снимается, но стоит добавить комментарий.

#### L-5 — `app.go:1022` приведение `a.smartProvider.(*proxy.HTTPBlockedListProvider)` без проверки `ok`
**Проблема**: если `smartProvider` инициализирован не как `*HTTPBlockedListProvider`, будет panic. В коде нет других реализаций, но как защита — нет.  
**Предложение**: либо явный `ok` (and graceful fallback), либо тип в интерфейсе с методом `Country() *CountryClient`.

#### L-6 — `extractProviderName` грубая эвристика (`app.go:2002-2020`)
**Проблема**: берёт второй-с-конца лейбл хоста, делает первую букву заглавной. Для `sub.foo.bar.example.com` вернёт `Example`. Корректно, но для `co.uk`-доменов (`foo.co.uk` → `Co`) сломается.  
**Предложение**: использовать `golang.org/x/net/publicsuffix` для корректного парсинга eTLD+1.

#### L-7 — `parseSubscriptionUserInfoHeader` не валидирует диапазоны (`app.go:1044-1072`)
**Проблема**: принимает любые int64, в т.ч. отрицательные. UI потом покажет минус-трафик.  
**Предложение**: clamp в `[0, max]`.

#### L-8 — Hardcoded list URL в adblock (`adblock.go:46-47`)
**Проблема**: единственный URL списка — pgl.yoyo.org. Если этот сайт пропадёт, AdBlock полагается на fallback из 11 доменов навсегда.  
**Предложение**: 2-3 источника + fallback chain. Возможно — github.com/StevenBlack/hosts.

---

## Сводка

`main.go` и updater — чистые, хорошо инкапсулированные модули. Crypto-слой имеет грамотную миграцию ключей и тесты на security-инварианты. Подсистема подписок (внутри `app.go`) хорошо защищена (SSRF-guard, per-host HWID hash, same-host referer, insecure-URL gating) и хорошо протестирована.

**Главная архитектурная боль** — `app.go` как god-object: 2150 строк, ~75 публичных методов, ~50% занимает subscription-логика, которая должна жить в собственном пакете. Вторая боль — отсутствие персистентных логов: пользователь не может приложить трейс к багу.

Compatibility-слой (legacy crypto, legacy export, ResultProxy→ResultV миграция) хорошо проработан и покрыт тестами.
