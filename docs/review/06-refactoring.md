# Сводный план рефакторинга и улучшений

> Агрегированный приоритизированный список всех найденных проблем по 4 зонам.
> Подробности и контекст — в файлах ревью каждой зоны.

## Легенда приоритетов

| Уровень       | Критерий                                                                                          |
| ------------- | ------------------------------------------------------------------------------------------------- |
| 🔴 **Critical** | Утечка трафика, потеря пользовательских данных, краш приложения, security-регрессия               |
| 🟠 **High**     | Архитектурный долг, риск багов divergence, плохой UX в edge-cases, скрытые security-проблемы     |
| 🟡 **Medium**   | Корректность, производительность, ergonomics, обновление зависимостей                            |
| 🟢 **Low**      | Cleanup, удаление dead-code, стилистика, незначительные улучшения                                |

---

## 🔴 CRITICAL — требует немедленного внимания

| #    | Файл / Локация                                                          | Проблема                                                                                                          | Решение                                                                                                          |
| ---- | ----------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| C-1  | `app.go` целиком (2150 строк)                                           | God-object: ~75 публичных методов, ~50% занимает subscription-логика, нарушение SRP, низкая тестируемость         | Выделить `internal/subscription/` (fetch, parse-icons, RVSUB-decrypt, HWID, AUTO-group). Выделить `internal/lifecycle/` |
| C-2  | `app.go:374-416` (`shutdown`)                                           | Goroutine + `os.Exit(0)` через 10s watchdog. Обрывает kill switch `Disable()` посреди записи firewall-правил — система без интернета | Kill switch `Disable()` ДО запуска goroutine, синхронно. Отдельный watchdog только на `sing-box.Instance.Close`   |
| C-3  | `app.go:225-229` (silent error на `NewCryptoService`)                   | При фейле всё `a.config/proxy/tray = nil`, окно показано, все RPC возвращают «not initialized» — приложение зомби | `os.Exit(1)` с понятным сообщением ИЛИ emit `app:fatal-init-error` и блок UI на splash-экране                    |
| C-4  | `killswitch_linux.go:217-220, 228-232` (iptables backend)               | **IPv6 leak**: при глобальном v6 без `nft` (Debian stable) трафик IPv6 не блокируется. Документировано как "skip v6 silently" | Добавить параллельные `ip6tables`-команды ИЛИ при отсутствии `nft`+глобальном v6 — отказ от Enable с явной ошибкой |
| C-5  | `killswitch_darwin.go:38-42`                                            | Заменяет ВЕСЬ активный pf-ruleset. Murus / LuLu / DNS-over-HTTPS правила пользователя молча перезаписываются и при Disable `pfctl -d` выключает pf полностью | Переход на named anchor: `pfctl -a resultv_killswitch -f rules` (сосуществование с пользовательским pf)          |
| C-6  | `killswitch.go:73` (resolve без таймаута)                               | `LookupIPAddr(context.Background(), ...)` — комментарий обещает 2s, фактически бесконечно. Заклинило резолвер → connect блокируется навсегда | `context.WithTimeout(ctx, 2*time.Second)`                                                                        |
| C-7  | `app.go:333` (`OnUnexpectedExit` после tray crash)                      | `os.Exit(0)` не вызывает `killSwitch.Disable()` — пользователь остаётся без интернета после краша tray            | Вызвать `Disable()` из shutdown handler ДО `Exit`                                                                |
| C-8  | `installer_darwin.go:58`                                                | `os.Exit(0)` сразу после `open /Applications/...` — kill switch не снят, proxy engine жив. Window без интернета между exit и launch | Вызвать `Updater.Install` ИЗ `app.go`, где доступны `killSwitch`/`proxy`. Graceful shutdown ПЕРЕД installUpdate (как в Windows portable) |

---

## 🟠 HIGH — следующий цикл разработки

### Архитектурный долг

| #     | Файл                                | Проблема                                                                                                | Решение                                                              |
| ----- | ----------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| H-1   | `internal/proxy/manager.go` (1430)  | God-object, 10+ ответственностей. `Connect` и `connectLocked` (manager.go:326, :592) почти дубликаты — риск divergence | Разбить на `manager_connect.go` / `manager_health.go` / `manager_ping.go` / `manager_state.go` / `manager_processtree.go`. Выделить общий `prepareEngineCfg + startEngineAndProbe` |
| H-2   | `internal/proxy/uriparser.go` (1682)| 8 парсеров протоколов в одном файле                                                                     | Разбить: `uriparser_vless.go`, `uriparser_vmess.go`, `uriparser_trojan.go`, …, `uriparser_subscription.go`, `uriparser_helpers.go` |
| H-3   | `frontend/.../AddProxyView.jsx` (2168) | Гигант: импорт URI + 8 форм протоколов + edit/add дубликаты                                            | Разделить на 6-8 файлов: `PlainProxyForm`, `VlessVmessTrojanForm`, `WireGuardForm`, `AmneziaEditor`, `Hysteria2Form`, `NaiveProxyForm`, `FileClipboardImport`. Объединить add/edit через `mode` prop |
| H-4   | `frontend/.../proxyParser.js` (853) | Дублирует `internal/proxy/uriparser.go` (1682). Парсеры расходятся (Trojan, alpn, naive)               | Добавить `App.ParseProxyURI(text)` Wails-биндинг → удалить парсер из JS, оставить только клиентские helpers (`isVpnType`, `formatProxyDisplayName`, etc.) |
| H-5   | `frontend/.../useAppConfig.js` (462) | God-hook: state, бизнес-логика, UI-диалоги, side-effects, persist в одном                              | Разделить: `useAppConfigStorage` / `useAppDialog` / `useSubscriptionRefresh` / `useSettingsActions`             |

### Security / VPN-критичное

| #     | Локация                                              | Проблема                                                                                                | Решение                                                                                  |
| ----- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| H-6   | `killswitch_windows.go:140`, `:206`                  | `CombinedOutput` логируется в err, но `output` теряется (`_ = out`). Молчаливый фейл при GPO `ProxySettingsPerUser` | Логировать тело `out` явно (или Warning-log при failed sub-rule)                         |
| H-7   | `killswitch_windows.go:248` + `app.go:263`           | Sweep на старте делает `max(stored, 8)`. CDN с >8 A-записями → leftover rules → «нет интернета» после рестарта | Persist `dnsRuleCount`/`proxyRuleCount` в registry или файл. Либо enum правил по имени   |
| H-8   | `taskbar_restore_hook_windows.go:182-191`            | Race: `SetWindowSubclass` из goroutine vs main-thread пересоздание HWND                                 | Установить subclass на UI-потоке окна через SendMessage в DLL-callback                   |
| H-9   | Linux/macOS singleton (`instance_messenger_stub.go:21`) | No-op — вторая инстанция стартует параллельно. Race на конфиг, kill switch, named mutex                | Реализовать (Linux: D-Bus / abstract socket; macOS: distributed notification / Mach port) |
| H-10  | `app.go:464-467, 624-627`                            | DNS-серверы из `Extra` подписки переопределяют `cfg.Settings.DNSServers`. Hostile-provider → MITM на DNS | UI-предупреждение + `system:provider-dns-override` event + Warning-log                   |
| H-11  | `internal/proxy/outbound.go:200, 441-477`            | TLS `Insecure: getBoolField(extra, "insecure")` — нет Warning-логов. Hysteria2/VLESS/VMess/Trojan молча принимают `insecure=true` | Логировать Warning при `Insecure=true`: «TLS verification disabled for proxy X»          |
| H-12  | `internal/proxy/router.go:172-178`                   | `Router.IsBlockedDomain` через `strings.Contains` → `notdiscord.example.com` матчит `discord.com`. Тест router_test.go:184 фиксирует это как expected behaviour | `h == d \|\| strings.HasSuffix(h, "."+d)` + поправить тест                              |
| H-13  | `internal/proxy/subscription_decrypt.go:13-24`       | `subscriptionEncryptKey` — единый ключ через ldflags, документировано как obfuscation. Не работает для приватных подписок | Зафиксировать TODO/issue для RVSUB2 (asymmetric или per-provider key)                    |
| H-14  | `internal/proxy/singbox.go:31`                       | Импорт `sagernet/sing-box`, реально форк `shtorm-7/sing-box-extended` ≥ 1.13.11. Если форк сломает AWG H-range → silent break | Runtime-проверка функциональности (probe AWG H-range field, fail early с понятной ошибкой) |
| H-15  | `internal/system/privileged_unix.go`                 | pkexec/osascript prompt на каждый toggle kill switch. Нет debounce → click-storm = 10 промптов          | Rate-limit (debounce 1s) на app-уровне                                                   |
| H-16  | `frontend/.../utils/crypto.js`                       | PBKDF2 100k итераций — нижняя граница 2017. OWASP 2023: 600k. Окно для брутфорса пароля экспорта        | Поднять до 600k. ИЛИ удалить frontend-экспорт и использовать Go `ExportConfig` (`RESULTPROXY2:`) |

### Корректность / надёжность

| #     | Локация                                  | Проблема                                                                                                                  | Решение                                                                                          |
| ----- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| H-17  | `internal/logger/logger.go`              | 500 in-memory записей, нет персистентности. Пользователь не может приложить лог к багу. `fmt.Printf` не работает в GUI на Windows | Интеграция `lumberjack.v2` (уже в transitive deps) — писать `<userData>/logs/app.log` с ротацией |
| H-18  | `app.go:321-334` (`OnUnexpectedExit`)    | Только Warning-лог и `os.Exit(0)`. Пользователь видит исчезновение приложения без объяснений                              | Notification «ResultV завершён из-за ошибки трея, перезапустите» + запись в persistent-log       |
| H-19  | `app.go:462-482` (Connect race window)   | Между `config.GetConfig()` и `proxy.Connect(...)` другая goroutine может пересохранить конфиг                            | `connectMu` на уровне App ИЛИ snapshot-передача в `proxy.Manager` (уже частично)                 |
| H-20  | `internal/proxy/manager.go:1359` (Ping)  | `m.engine.IsRunning()` после `m.mu.Unlock()` — `engine` атомарен, но поле читается без mu. Race detector подсветит        | Снимок `engine` под mu, потом проверка                                                           |
| H-21  | `system_windows.go:48` (`IsAdmin`)       | Через `net session` — медленно, может фейлить в Server Core                                                               | `OpenProcessToken` + `GetTokenInformation(TokenElevation)`                                       |
| H-22  | `killswitch_windows.go:69-73`            | `WindowsKillSwitch.isAdmin` кешится навсегда                                                                              | Пересчитывать в Enable                                                                           |

### Frontend / UX

| #     | Локация                                                       | Проблема                                                                                                              | Решение                                                            |
| ----- | ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| H-23  | Захардкоженные русские строки                                 | `MainLayout.jsx:90,102,111,117`, `useAppConfig.js:223,259,275`, `addLog()` + `LogsView.translateLog` substring-match  | Миграция `addLog` на `{key, params}`. Убрать `t(key) \|\| "Русский fallback"`. Локализовать мобильный nav |
| H-24  | `frontend/.../wailsAPI.js:57` (`Connect` parameters)          | Имена параметров устаревшие (`proxyStr, options, mode, processName`), фактически `(candidate, routingRules, killswitch, adblock)` | Переименовать (5 мин)                                              |
| H-25  | Production `console.error/log`                                | `wailsAPI.js`, `useAppConfig.js`, `useDaemonStatus.js` — логи в production содержат subscription URL, proxy IP        | Обернуть в `import.meta.env.DEV` или удалить                       |
| H-26  | `ProxyListView` без виртуализации                             | 500+ карточек при больших подписках → лаги при сортировке/поиске                                                      | `react-window` или `@tanstack/react-virtual`                       |

---

## 🟡 MEDIUM — следующая итерация

### Crypto / Безопасность

| #     | Локация                                  | Проблема                                                                                                | Решение                                                              |
| ----- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| M-1   | `internal/config/crypto.go:228-237`      | Нестандартный nonce-size 16 в AES-GCM (NewGCMWithNonceSize). Корректно но не совместимо со стандартным `cipher.NewGCM`. Наследие Node-совместимости (`TestNodeJSCompatibility` skipped) | Документировать или мигрировать на 12-байтный nonce с поддержкой обоих на чтение |
| M-2   | `internal/updater/download.go:34`        | `http.DefaultTransport` следует за redirect. `ValidateAssetURL` проверяет только исходный URL — redirect может увести на любой https-хост | `client.CheckRedirect = func(req, via) error { return ValidateAssetURL(req.URL.String(), ...) }` |
| M-3   | `internal/updater/` (download verify)    | SHA-256 проверка ловит подмену, но нет проверки magic-bytes (PE/Mach-O/ELF)                            | Sanity-check: первые байты соответствуют формату платформы           |
| M-4   | `frontend/.../utils/network.js:21-41`    | `detectCountry` не покрывает 172.16-31, fd::/8, fe80::/10 IPv6                                          | Расширить классификацию приватных диапазонов                         |

### Производительность

| #     | Локация                                  | Проблема                                                                                                | Решение                                                              |
| ----- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| M-5   | `logger.go:207`                          | O(n) prepend: `append([]LogEntry{entry}, l.entries...)` копирует весь буфер на каждый лог              | Круговой буфер (`container/ring` или собственный с head-индексом)    |
| M-6   | `adblock.go:179-186`                     | Linear suffix-match: `for d := range domains { HasSuffix(...) }` — O(n) на каждый DNS-запрос           | Trie / map по последнему лейблу: `m["net"] = ["doubleclick.net", ...]` |
| M-7   | `internal/system/tray.go:367`            | UpdateProxyList перестраивает всё меню при любой смене signature. Flicker для >100 серверов            | Diff-based update (но сложно с подменю)                              |
| M-8   | `tray.go:553-595` (flagcdn.com)          | Иконки стран синхронно в `UpdateProxyList`. Без интернета — 3s × batches задержки                      | Полностью fire-and-forget, fallback icon, push-update через events   |
| M-9   | `internal/proxy/singbox.go:355`          | 5-sec timeout в `shutdownInstanceLocked` → goroutine leak при зависании sing-box                       | Forced kill goroutine + log warning                                  |
| M-10  | `frontend/.../useDaemonStatus.js:233-249` | `daemonStatus`, `settings`, `activeProxy` в deps → setInterval пересоздаётся каждую секунду            | `useRef` для актуальных значений + один setInterval на mount         |
| M-11  | `frontend/.../HoverMarquee.jsx:33-37`    | window.resize listener на каждом экземпляре. 100 карточек → 100 listeners                              | `ResizeObserver` или один глобальный listener в провайдере           |
| M-12  | `frontend/.../ConfigContext.jsx:73-83`, `ConnectionContext.jsx:114-137` | Inline `value` объект на каждом рендере провайдера → invalidate для всех потребителей | `useMemo` с явными deps                                              |

### Корректность

| #     | Локация                                  | Проблема                                                                                                | Решение                                                              |
| ----- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| M-13  | `internal/adblock/adblock.go:147-160`    | `os.Create + Fprintln` — не атомарная запись cache. Crash посередине = битый cache                     | Писать в `<cachePath>.tmp` → `os.Rename` + header-magic               |
| M-14  | `tools/genico/main.go:30,44,50,57`       | Hardcoded `c:/ResultProxyPC/...` — старое имя. Запуск сейчас = no such file or directory               | `os.Getwd` + `flag.String` или `runtime.Caller` для корня репо       |
| M-15  | `app.go:2059-2065` (`IsMainWindowVisible`) | Hard-coded sleep 400мс + `os.Exit(0)`. Медленный WebView2 → ложно-положительный убийство              | Backoff до 2-3s, event-driven через Wails-runtime "WindowShown"      |
| M-16  | `app.go:438-453` (`SaveConfig`)          | «cfg.Subscriptions nil ИЛИ len==0 → взять существующие» — удалить все подписки через SaveConfig нельзя | Отдельный `ClearSubscriptions()` или явный флаг в DTO                |
| M-17  | `app.go:1683, 1715` (subscription ID)    | `crc32.ChecksumIEEE` — 32 бита, склонен к коллизиям                                                     | `sha256[:8]` или `xxhash`                                            |
| M-18  | `internal/system/processtree/processtree_other.go:28-32` | macOS truncates `comm` до 15 символов. `WhatsApp Helper` → `WhatsApp Helpe` — не матчит regex     | `proc_pidpath()` через cgo (требует `<libproc.h>`)                   |
| M-19  | `internal/system/killswitch.go:48`       | Hardcoded `1.1.1.1` + `8.8.8.8` fallback. В России Google DNS заблокирован, в Китае оба                | Использовать прокси-собственные DNS или конфигурируемый fallback     |
| M-20  | `internal/system/netmon.go`              | Polling 5s, не event-driven. До 10s задержки при смене интерфейса                                       | (опционально) Windows: `IPHelper.NotifyAddrChange`; Linux: netlink; macOS: SystemConfiguration |
| M-21  | `frontend/.../useDaemonControl.js:110-371` | `toggleConnection`/`selectAndConnect` — 90% общий код                                                  | Выделить `attemptConnect(candidates, isAuto, addLog, ...)`           |
| M-22  | `frontend/.../AppDialogModal.jsx`        | Не закрывается по Esc                                                                                   | `useEffect` на `keydown` Esc                                         |
| M-23  | Frontend: дубликат `ALLOWED_DOWNLOAD_HOSTS` | В `UpdaterModal.jsx:25` и `UpdateNotificationModal.jsx:28`                                            | Вынести в `utils/updateSafety.js`                                    |
| M-24  | Frontend: два пути экспорта              | Web Crypto AES-GCM (`SettingsView`) vs Go `ExportConfig` (`wailsAPI.js`, нигде не вызывается)         | Решить: один путь. Если Go — поддержка HWID-binding без user-пароля   |
| M-25  | `SettingsView.jsx:91-102` (download)     | `document.createElement('a')` вместо native Wails `SaveFileDialog`                                      | Wails native dialog (имя файла, расширение)                          |
| M-26  | `frontend/.../useDaemonPing.js:98-100`   | `flushSync` — anti-pattern в современном React                                                          | Разделить state на «инициирован» и «выполнен»                        |
| M-27  | `internal/system/autostart_*.go`         | Дубликат `autostartTrayToken` const в 3 файлах                                                          | Вынести в `autostart.go` без build tag                               |

---

## 🟢 LOW — фоновая работа

| #     | Локация                                  | Проблема                                                                                                |
| ----- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| L-1   | `internal/system/killswitch_*.go`        | `extractValidIP` дублируется в 3 файлах. Вынести в `killswitch.go`                                       |
| L-2   | `internal/system/tray.go:639` (`pngToICO`) | 122 строки ручной бинарной упаковки. Можно `golang.org/x/image/ico` или сторонний пакет                |
| L-3   | `internal/system/tray.go` (761 LOC)      | Расщепить: `tray.go` (API + lifecycle), `tray_icons.go` (pngToICO + flag download), `tray_render.go`    |
| L-4   | `internal/getlantern_systray/go.mod`     | Отдельный модуль, форк не апдейтится с основным. Слить через replace directive                          |
| L-5   | `tools/licenseheaders/main.go`           | Одноразовый rename-helper ResultProxy → ResultV. Удалить или переименовать в `tools/applyheaders/`      |
| L-6   | Лицензионные шапки 14 строк × ~60 файлов | ~840 строк boilerplate. Сократить до 2-3 строк со ссылкой на LICENSE                                    |
| L-7   | `app.go:1022`                            | Type assertion без `ok`: `a.smartProvider.(*proxy.HTTPBlockedListProvider)` — panic при unexpected type |
| L-8   | `app.go:2002-2020` (`extractProviderName`) | Не учитывает eTLD+1 (`foo.co.uk` → "Co"). Использовать `golang.org/x/net/publicsuffix`                |
| L-9   | `app.go:1044-1072` (`parseSubscriptionUserInfoHeader`) | Не валидирует диапазоны → отрицательный трафик в UI. Clamp в [0, max]                       |
| L-10  | `internal/adblock/adblock.go:46-47`      | Единственный URL списка — `pgl.yoyo.org`. Если пропадёт → 11 fallback доменов навсегда                  |
| L-11  | `instance_messenger_windows.go:47` + `getlantern_systray/systray_windows.go:88` | Дубликат `wndClassEx` struct. Shared package                          |
| L-12  | `killswitch_windows.go:275` (`DetectGPOConflict`) | Возвращает true просто при существовании ключа. Не отличает enabled vs locked-out              |
| L-13  | `internal/system/tray.go:614-637`        | Fallback icon — серый квадрат 6x6. Заменить на embedded ICO resource                                    |
| L-14  | `internal/getlantern_systray/systray.go:30` | `SetWindowsTrayLeftClick` — глобальная переменная. Один listener на процесс                          |
| L-15  | Linux installer .bak accumulation        | `installer_linux.go:44-50` — `os.Rename` перезаписывает (фактически 1 .bak). Комментарий бы помог       |
| L-16  | Frontend: глобальный `outline: none !important` | `App.css:27-38`, `MainLayout.jsx:56-71` — убивает фокус-стили (a11y)                              |
| L-17  | Frontend: a11y                           | Нет `aria-modal="true"` / `role="dialog"` на модалках                                                   |
| L-18  | Frontend: формат даты                    | DD.MM.YY hardcoded в `ProxyListView.jsx:638-646`. Использовать `Intl.DateTimeFormat(i18n.language)`     |
| L-19  | Frontend: Cloudflare-CDN зависимость флагов | `FlagIcon.jsx:51` — офлайн без флагов. Бандлить SVG локально                                         |
| L-20  | Нет CSP                                  | `wails.json` и `index.html` без CSP                                                                     |

---

## Стратегия: что делать в каком порядке

### Спринт 1 — VPN-критичные баги (Critical, 1-2 недели)

**Цель**: устранить утечки трафика и риск «нет интернета после краха».

1. **C-4 + C-5 + C-6** — kill switch IPv6 leak (Linux), pf clobbering (macOS), resolve timeout. Один MR.
2. **C-2 + C-7 + C-8** — корректный shutdown: kill switch Disable синхронно ДО watchdog; OnUnexpectedExit вызывает Disable; macOS updater не Exit'ит. Один MR.
3. **C-3** — fatal-init error UI (splash + блок).

**После спринта 1**: пользователи без интернета после краха — устранено; IPv6 утечки на Linux — устранены; macOS пользователи с собственным pf — спасены.

### Спринт 2 — Архитектурный split (High, 2-3 недели)

**Цель**: декомпозировать god-объекты для тестируемости.

1. **H-1**: разбить `manager.go` (5 файлов). Выделить общий `prepareEngineCfg + startEngineAndProbe` — устранить дубликат `Connect/connectLocked`.
2. **C-1**: выделить `internal/subscription/` из `app.go` (приоритетно — это половина `app.go`).
3. **H-3**: разбить `AddProxyView.jsx` (6-8 компонентов).
4. **H-5**: разделить `useAppConfig.js` (4 хука).
5. **H-2**: разбить `uriparser.go` по протоколам (можно в одном MR с H-1 и тестами).

**После спринта 2**: god-объекты устранены, тестируемость повышена, можно добавлять unit-тесты на новые модули.

### Спринт 3 — Security audit / прозрачность (High, 1-2 недели)

**Цель**: устранить тихие security-регрессии.

1. **H-10** — DNS-override из подписки → UI badge + Warning-log + opt-in.
2. **H-11** — TLS insecure → Warning-log на каждый proxy с insecure=true.
3. **H-12** — `Router.IsBlockedDomain` substring → suffix-match (с поправкой теста).
4. **H-16** — PBKDF2 100k → 600k ИЛИ удаление frontend-экспорта.
5. **H-25** — production `console.*` → DEV-guard или удалить.
6. **M-2** — updater redirect через `CheckRedirect` validate.

### Спринт 4 — Frontend де-дупликация и unification (High, 1-2 недели)

1. **H-4** — `proxyParser.js` удалить, использовать Go-биндинг `ParseProxyURI`. Гарантирует один source-of-truth для парсинга URI.
2. **M-12** — мемоизация context value (5 мин).
3. **H-23** — фикс i18n хардкодов.
4. **H-26** — виртуализация ProxyListView.

### Спринт 5 — Тесты, наблюдаемость, надёжность (Medium, 1-2 недели)

1. **H-17** — персистентные логи (lumberjack). Без этого пользователи не могут репортить баги.
2. Тесты на iptables KS backend (для C-4).
3. Тесты на health watchdog.
4. Vitest + frontend unit-тесты на `parseProxies`, `compareVersions`, `getPingSortMetric`.
5. **H-9** — Linux/macOS singleton.

### Continuous — Low priority cleanup

- `L-1` — деdup `extractValidIP`.
- `L-3` — split `tray.go`.
- `L-5` — удалить `tools/licenseheaders`.
- `L-6` — лицензионные шапки.
- `M-27` — дедуп `autostartTrayToken`.
- `M-14` — `tools/genico` фикс.

---

## Сводная статистика

| Уровень       | Количество | По зонам (Entry/Proxy/System/Frontend)         |
| ------------- | ---------: | ---------------------------------------------- |
| 🔴 Critical    | 8          | 3 / 0 / 4 / 1                                  |
| 🟠 High        | 26         | 6 / 6 / 8 / 6                                  |
| 🟡 Medium      | 27         | 6 / 1 / 8 / 12                                 |
| 🟢 Low         | 20         | 5 / 0 / 8 / 7                                  |
| **Итого**     | **81**     |                                                |

## Что НЕ требует рефакторинга (сильные стороны проекта)

Чтобы план не выглядел как «всё плохо», стоит зафиксировать **что хорошо**:

| Зона                              | Что хорошо                                                                                       |
| --------------------------------- | ------------------------------------------------------------------------------------------------ |
| **Crypto-слой** (`internal/config`) | Грамотная миграция ключей, install-salt, tamper-detection, IV-уникальность, тесты                |
| **Updater**                       | https-only, host-whitelist, sha256 с удалением файла, size limit, атомарный rename               |
| **Subscription security**         | SSRF-guard, per-host HWID hash, same-host referer, insecure-URL gating, all heavy-test cover     |
| **Proxy DNS-leak fix**            | `engine.go:500-510` — DNS-DPI mitigation в proxy mode                                            |
| **Country API privacy**           | `country.go:34-40` — приватный fallback при недоступности                                        |
| **Real sing-box decoder тесты**   | `udp_quic_smoke_test`, `awg2_e2e_test` — гарантия совместимости с форком                         |
| **Phase 1/2/3 Connect lifecycle** | Чёткое разделение и cancel-control через `connectCancelMu`                                       |
| **Tun stack = system** для anti-cheat | Корректный выбор для совместимости с играми                                                 |
| **No XSS surface на фронте**      | Нет `dangerouslySetInnerHTML`, нет `eval`/`new Function`, `window.open` с `noopener,noreferrer` |
| **Update URL validation**         | `isSafeDownloadURL` host whitelist на фронте                                                     |
| **Crypto round-trip тесты**       | Bit-flip → `ErrWrongPassword`, неверный ключ, install-salt rotate                                |
| **Конкурентный logger тест**      | 100 writers + 50 readers одновременно                                                            |

---

**Конец сводного плана.** Подробности по конкретным проблемам — в файлах ревью каждой зоны (`01-04`).
