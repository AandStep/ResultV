# Тесты — сводный анализ покрытия

> Сводка по всем тестовым файлам проекта. Подробности — в файлах ревью каждой зоны.

## Объём

| Зона                  | Тестовых файлов | LOC тестов | Соотношение к продакшен-коду |
| --------------------- | --------------: | ---------: | ---------------------------- |
| **Entry + сервисы**   | 9               | ~1 870      | ~31%                         |
| **Proxy-engine**      | ~25             | ~3 800      | ~29%                         |
| **System**            | ~14             | ~1 000      | ~18%                         |
| **Frontend**          | 0               | 0          | **0%**                       |
| **Итого**             | **~48**         | **~6 670**  | ~24% backend                 |

Покрытие бэкенда — нормальное для индустрии (~25-30%). Фронтенда нет вообще.

## Покрытие по зонам

### Entry + сервисы

| Файл                              | LOC | Покрывает                                                                                   | Критические дыры                              |
| --------------------------------- | --: | ------------------------------------------------------------------------------------------- | --------------------------------------------- |
| `app_icon_ssrf_test.go`           | 111 | `isPrivateOrLoopback` (RFC1918, CGNAT, link-local v6, multicast), `sameHostReferer`         | -                                             |
| `app_subscription_test.go`        | 221 | HWID-хэш per-провайдер, https→hwid, refuse-default, empty-body diagnostics, profile-title, icon resolver | Connect/Disconnect, ApplyMode, RefreshSubscription |
| `config/config_test.go`           | 401 | Round-trip Manager save/load, missing/corrupted, UpdateRoutingRules, ensureDefaults, legacy миграция директорий, e2e миграция ключа | `getHardwareID` fallback-цепочка (WMI fail → registry → file) |
| `config/crypto_test.go`           | 274 | Encrypt/Decrypt, неверный ключ, legacy plaintext, install-salt generate/reuse/rotate, fallback, уникальность IV | `TestNodeJSCompatibility` skipped (TODO)      |
| `config/export_test.go`           | 257 | v2 round-trip, wrong password, минимальная длина, недетерминированность, tamper-detection, отказ от слабых KDF, MergeImport | -                                             |
| `logger_test.go`                  | 165 | count, newest-first, capacity eviction, пагинация, event-emitter, clear, **конкурентный** доступ (100w + 50r) | Behavior при панике в emitter                 |
| `adblock_test.go`                 | 78  | fallback domains, suffix match, empty hostname, case-insensitive, load from cache, GetDomains | `UpdateLists` (HTTP fetch), конкурентный LoadFromCache+IsAdDomain |
| `updater_test.go`                 | 242 | `ValidateAssetURL` (https + whitelist), `ResolveAsset`, `verifySHA256` (match + mismatch с удалением), `FetchManifest` (OK/404/bad JSON), `downloadFile` (OK + size-mismatch + 500 + cancel) | Реальный `installUpdate` не тестируется ни на одной платформе |
| `installer_windows_test.go`       | 77  | PowerShell-handover скрипты содержат ожидаемые токены (Wait-Process, /ALLUSERS, /CURRENTUSER, registry-paths, copy operation) | -                                             |

**Сильные стороны**: SSRF-guard, crypto round-trip с tamper-detection, конкурентный логгер, sha256-валидация апдейтов с удалением файла.

**Дыры**: Connect/Disconnect / ApplyMode (критично, сложный rollback в `app.go:659-689`), реальная установка апдейта.

### Proxy-engine

| Файл                              | LOC | Тестов | Покрывает                                                                                   |
| --------------------------------- | --: | -----: | ------------------------------------------------------------------------------------------- |
| `engine_route_test.go`            | 464 | 17     | Все builders, DNS/route правил, nested exceptions, app-whitelist + find_process             |
| `manager_mode_test.go`            | 701 | 12     | Connect-flow (success/fail/timeout/admin), все probe-сценарии, SetMode, fail-clear, kill switch, status |
| `manager_ping_test.go`            | 144 | 4      | Ping для UDP-протоколов в tunnel, fallback, активный proxy session                          |
| `uriparser_test.go`               | 778 | ~32    | VLESS/Trojan/VMESS/SS/WG/Naive: формы URI, JSON-подписки, xhttp/grpc/reality                |
| `uriparser_awg_test.go`           | 288 | ~8     | AmneziaWG URI roundtrip, AWG 1.0/2.0, case-insensitive                                      |
| `uriparser_hysteria_test.go`      | 61  | 1      | Hysteria-protocol subscription JSON (`protocol:"hysteria"` → HYSTERIA2)                     |
| `outbound_alpn_test.go`           | 31  | 1      | xhttp prefer h2                                                                             |
| `outbound_hysteria2_test.go`      | 94  | 3      | hysteria2 outbound ALPN/SNI/obfs/password fallback                                          |
| `singbox_protocols_test.go`       | 212 | 4      | WG/AWG/Hysteria2/Naive/SS config parses through **real sing-box `option.Options` decoder**  |
| `awg2_e2e_test.go`                | 113 | 1      | **End-to-end**: AWG 2.0 H-range config через `singjson.UnmarshalContext` + `*Xbadoption.Range` |
| `udp_quic_smoke_test.go`          | 122 | 3      | VLESS xudp + Hysteria2 + VMess xudp shapes survive `option.Options` strict decode           |
| `router_test.go`                  | 257 | 13     | normalizeRule, IsWhitelisted (single/double/triple), ShouldProxy (Global/Smart), GetSafeOSWhitelist |
| `blocked_provider_test.go`        | 239 | 7      | TLS-сервер mock, country cache, hostname resolution, plaintext rejection, CSV/dnsmasq/hosts |
| `blocked_updater_test.go`         | 117 | 4      | Resolve: remote → cache → local; country mismatch fallback                                  |
| `config_validation_test.go`       | 147 | 4      | WG missing key / bare IP normalization / invalid address / Hysteria2 password               |
| `deeplink_test.go`                | 58  | 3      | sanitize base64, deep-link rejection, rvsub vs import path detection                        |
| `subscription_decrypt_test.go`    | 137 | 6      | Round-trip, plain passthrough, missing-key, bad-key, URL-safe base64, whitespace            |
| `lcp_test.go`                     | 66  | 1      | ExtractAutoGroupName — 4 кейса                                                              |
| `ping_lan_bind_test.go`           | 43  | 2      | isEngineTunIPv4, looksLikeTunnelInterface                                                   |
| `sysproxy_windows_test.go`        | 30  | 1      | buildBypassList не использует broad wildcards                                               |
| `sysproxy_linux_test.go`          | 40  | 3      | gsettings list formatting + backends register                                               |
| `sysproxy_darwin_test.go`         | 29  | 2      | bypass defaults + whitelist expansion                                                       |

**Сильные стороны**:
- Connect-flow проверен через **stub Engine** — быстро, без реального sing-box.
- Real end-to-end (`udp_quic_smoke_test`, `awg2_e2e_test`) использует **реальный sing-box option-decoder** — гарантирует совместимость с upstream форком.
- Router algorithm покрыт (single/double/triple match).
- DNS-DPI mitigation (proxy-mode UDP DNS) явно проверяется.
- Subscription-decrypt cover edge-cases (whitespace, URL-safe, missing key).
- Manager-mode tests use both stubs AND real-network helpers.

**Критические дыры**:
- **`ApplyAppWhitelist`** (hot-reload приложений) — нет тестов, хотя `stub.applyCalls` есть.
- **Health watchdog** (proxyDead flag, kill-switch firewall) — критичная часть безопасности без тестов.
- **Concurrent Connect → Disconnect → Connect** — есть `TestConnect_FailedSwitchClearsCurrentProxyInStatus`, но без явных race-сценариев.
- Нет integration-теста реального запуска `SingBoxEngine.Start`.
- Нет тестов `CancelConnect()` mid-flight.
- `procTracker.OnChange` callback без тестов.
- Country API без race-теста при одновременных lookup'ах.

### System

| Файл                              | LOC | Build         | Покрывает                                                                                   |
| --------------------------------- | --: | ------------- | ------------------------------------------------------------------------------------------- |
| `system_test.go`                  | 37  | all           | Smoke: GetNetworkTraffic, IsAdmin, IsAutostartEnabled (зависят от состояния тестового бокса) |
| `killswitch_test.go`              | 67  | all           | `TestExtractDNSIPs` (8 cases): fallback, whitespace, hostname→fallback, IPv4/IPv6, dedup     |
| `killswitch_linux_test.go`        | 98  | linux         | nft policy drop, IPv4/v6 proxy rules, LAN allow, DNS-scoping (регрессия), IP-семейства, ExtractValidIP |
| `killswitch_darwin_test.go`       | 74  | darwin        | pf rules: blocks-by-default, proxy-pass, LAN, DNS-scoping, ExtractValidIP                   |
| `autostart_windows_test.go`       | 57  | windows       | BuildAutostartRunCommand (forward → backslash, exe quoting, extra args), ArgsStartInTray   |
| `autostart_linux_test.go`         | 39  | linux         | RenderAutostartDesktopEntry (Desktop Entry keys, Exec, X-GNOME-Autostart-enabled), quoting paths with spaces |
| `autostart_darwin_test.go`        | 32  | darwin        | RenderLaunchAgentPlist (Label, ProgramArguments, RunAtLoad), escape `<` `&`                |
| `appwhitelist_test.go`            | 39  | all           | Common dedup + nil-safety                                                                   |
| `appwhitelist_windows_test.go`    | 17  | windows       | basename + force .exe, case retention                                                       |
| `appwhitelist_other_test.go`      | 46  | linux         | .desktop parsing, firstExecToken с quoting                                                  |
| `appwhitelist_darwin_test.go`     | 54  | darwin        | bundleRoot, Info.plist parsing, fallback                                                    |
| `tray_model_test.go`              | 118 | all           | BuildTrayMenuGroups (sort + fallback на «Мои прокси»), per-country limit, countryToFlag, formatCountryTitle, formatServerTitle, countryISOCode |
| `tray_click_dispatcher_test.go`   | 64  | all           | RebuildDoesNotAccumulateHandlers (63 stale channels)                                        |
| `tray_icon_test.go`               | 43  | all           | pngToICO (2x2 PNG → ICO, header check)                                                      |
| `legacy_migration_test.go`        | 101 | all           | OldOnly, KeepsNewConfig, TreeMovesWhenTargetMissing, ErrorWhenNewPathIsFile                 |
| `privileged_unix_test.go`         | 24  | darwin\|linux | ShellQuote (12 кейсов)                                                                      |
| `processtree/processtree_test.go` | 131 | all           | NormalizeRootsDedupes, SnapshotEqualIgnoresRoots, MonitorEmitsOnChangeAndDedupes, double-start no-op, empty-roots edge, scanInterval in (0, 2s] |

**Сильные стороны**:
- Все builders kill switch покрыты (nft, pf).
- Tray model и dispatcher покрыты unit-тестами.
- Legacy migration с 4 кейсами.
- Process tree monitor с dedup-логикой.

**Критические дыры**:
- **iptables kill switch backend нет тестов** (`enableIptables`/`disableIptables`) — это fallback для Debian < 11 / Ubuntu 18.04, и именно у него leak'ает IPv6 (см. рефакторинг H-1).
- **`EnableAutostart` на Windows** требует моковать registry — нет тестов.
- **`legacyScheduledTaskPresent` на Windows** требует моковать `schtasks.exe` — нет.
- **`tray_click_dispatcher_test.go` полагается на `time.Sleep`** (40ms + 80ms + 500ms) — flaky в CI под нагрузкой.
- **`pngToICO` для 256x256** не покрыт (special-case `size==256 → w=0` в `tray.go:730`).
- **Singleton на Linux/macOS** — не реализован (`instance_messenger_stub.go:21` — no-op), тестов нет.
- **Deep-link runtime на macOS** — не реализован, тестов нет.
- **Taskbar restore hook** — нет тестов на race с пересозданием HWND (см. H-6 в 03).
- **Реальная интеграция с `/proc` или `Toolhelp32`** — нет (понятно почему — flaky).

### Frontend

**Нет тестов вообще.** Кандидаты на unit-тесты, упомянутые в ревью:

| Функция / Файл                          | Почему важно                                                                |
| --------------------------------------- | --------------------------------------------------------------------------- |
| `proxyParser.js / parseProxies`         | Парсит URI 8 протоколов; критично для импорта                               |
| `utils/versionCheck.js / compareVersions` | Используется для решения «показать update notification»                   |
| `utils/pingSort.js / getPingSortMetric` | Логика сортировки по пингу                                                  |
| `utils/crypto.js`                       | AES-GCM экспорт/импорт (PBKDF2 100k — низковато)                           |
| `useAppConfig.js / mergeSubscriptionRefreshCountries` | Merge state после refresh                                      |
| `utils/network.js / detectCountry`      | IP-классификация (не покрывает все приватные диапазоны)                     |

Рекомендуется поднять `vitest` + `@testing-library/react`, начать с pure-функций в `utils/`.

## Шаблоны / Антипаттерны в тестах

### Хорошие практики (надо сохранить)
- **stub-объекты** для Engine, SysProxy, KillSwitch — позволяют тестировать Manager без реального sing-box.
- **httptest server** для blocked_provider, blocked_updater, updater — изоляция от сети.
- **Real-decoder smoke tests** (`udp_quic_smoke_test`, `awg2_e2e_test`) — гарантия совместимости с форком sing-box.
- **Конкурентные тесты** для logger (100w + 50r).
- **Tamper-detection** в crypto (bit-flip → `ErrWrongPassword`).

### Антипаттерны (надо исправить)
- **`time.Sleep` в `tray_click_dispatcher_test.go`** — заменить на `chan` с `select { case <-ch: case <-time.After(...) }` для детерминированности.
- **`TestNodeJSCompatibility` skipped с TODO** — либо удалить, либо реализовать.
- **`smoke-tests`** в `system_test.go` (`TestGetNetworkTraffic`, `TestIsAdmin`, `TestIsAutostartEnabled`) — только логируют, не assert'ят. Превратить в полноценные unit-тесты или удалить.
- **Substring-match закреплён тестом** (`router_test.go:178` для `TestIsBlockedDomain`) — этот тест фиксирует семантический баг (см. H-3 в proxy-engine).

## Метрики, которые стоит добавить

1. **Coverage по пакетам** — `go test -coverprofile=cover.out ./internal/...` + HTML-отчёт.
2. **Race-detector в CI** — `go test -race ./...` (если ещё не включён).
3. **Mutation testing** — `go-mutesting` для критических security-инвариантов (config crypto, updater verify).
4. **Frontend coverage** — vitest с `--coverage`, целевая планка 30-40% для критичных utils.
5. **Long-running stress tests** — Connect→Disconnect 1000 циклов, Ping 100 прокси параллельно (chase down race conditions).

## Приоритизированный план улучшения тестов

| # | Приоритет | Что                                                                                          |
| - | --------- | -------------------------------------------------------------------------------------------- |
| 1 | High      | Тесты на iptables kill switch backend + явный IPv6-leak test (для H-1)                       |
| 2 | High      | Тесты на `health watchdog` (proxyDead + kill switch arm/disarm flow)                         |
| 3 | High      | Race-тест `Connect → Disconnect → Connect` (для подтверждения отсутствия goroutine leak)     |
| 4 | High      | Frontend: vitest + unit-тесты для `parseProxies`, `compareVersions`, `getPingSortMetric`     |
| 5 | Medium    | Тесты на `ApplyMode` (rollback flow `app.go:659-689`)                                        |
| 6 | Medium    | Тесты на `RefreshSubscription` + `AddSubscription` (mock subscription server)                |
| 7 | Medium    | Тесты на `ApplyAppWhitelist` hot-reload (engine stub уже имеет applyCalls)                   |
| 8 | Medium    | Заменить `time.Sleep` в `tray_click_dispatcher_test.go` на детерминированную синхронизацию    |
| 9 | Low       | Mutation testing для config/crypto                                                            |
| 10| Low       | Long-running fuzz для `uriparser` (corpus из реальных подписок)                              |

---

**Дальше**: [06-refactoring.md](./06-refactoring.md) для агрегированного списка улучшений.
