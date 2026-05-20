# ResultV — Архитектурное ревью кода

> Версия приложения: **3.2.0**
> Дата ревью: **2026-05-14**
> Лицензия: **GPL v3.0**

## Что это

**ResultV** (ранее ResultProxy) — кросс-платформенный десктопный клиент для VPN/прокси на базе **sing-box**. Цель — пользовательский GUI на Windows (приоритет), macOS и Linux (бета) с поддержкой широкого набора протоколов, маршрутизацией по правилам, kill switch, autostart, системным треем, in-app апдейтером и зашифрованным экспортом конфигурации.

## Стек

| Слой           | Технология                                                                                             |
| -------------- | ------------------------------------------------------------------------------------------------------ |
| Оболочка       | [Wails v2](https://wails.io/)                                                                          |
| UI             | React 18, Vite, Tailwind CSS, i18next (ru/en), lucide-react                                            |
| Бэкенд         | Go 1.26.1                                                                                              |
| Прокси-движок  | sing-box (форк `shtorm-7/sing-box-extended` v1.13.11) с тегами `with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_naive_outbound,with_purego` |
| Трей           | Форк `getlantern/systray` в `internal/getlantern_systray/`                                             |
| Cronet/QUIC    | `sagernet/cronet-go` для transport-обёрток                                                             |
| Webview        | WebView2 (Windows), WebKitGTK (Linux), WKWebView (macOS)                                               |

## Архитектурная схема

```
                       ┌───────────────────────────────────────────────┐
                       │           main.go  (Wails entry)               │
                       │  • Single-instance lock (Windows: named mutex) │
                       │  • Deep-link parsing (resultv://)              │
                       │  • Theme/window opts • startup → shutdown      │
                       └───────────────────────┬───────────────────────┘
                                               │ Bind: app
                                               ▼
                       ┌───────────────────────────────────────────────┐
                       │             app.go  (Wails RPC face)           │
                       │  ~75 публичных методов; god-object (Critical)  │
                       │  Events: status:update, log:append, proxy:*    │
                       └─┬───────────┬───────────┬───────────┬─────────┘
                         │           │           │           │
            ┌────────────▼──┐  ┌─────▼─────┐ ┌──▼────────┐ ┌▼────────────┐
            │ internal/     │  │ internal/ │ │ internal/ │ │ internal/   │
            │ config        │  │ proxy     │ │ system    │ │ updater     │
            │ (AES-GCM,     │  │ (sing-box │ │ (kill sw, │ │ (Check→DL→  │
            │  PBKDF2,      │  │  engine,  │ │  autost., │ │  Verify→    │
            │  v1→v2 mig,   │  │  manager, │ │  tray,    │ │  Install)   │
            │  HWID-bind)   │  │  uri-     │ │  netmon,  │ │             │
            │               │  │  parser,  │ │  paths,   │ │ per-OS:     │
            │               │  │  country, │ │  single-  │ │ Win NSIS/   │
            │               │  │  router)  │ │  inst.,   │ │ portable    │
            │               │  │           │ │  proctree)│ │ macOS DMG   │
            │               │  │           │ │           │ │ Linux       │
            │               │  │           │ │           │ │ AppImage    │
            └───────────────┘  └─────┬─────┘ └─────┬─────┘ └─────────────┘
                                     │             │
                                     ▼             ▼
                       ┌───────────────────────────────────────────────┐
                       │   internal/logger  •  internal/adblock        │
                       │   internal/getlantern_systray (форк трея)     │
                       └───────────────────────────────────────────────┘

                                  ── Wails IPC ──
                                  • RPC: Bind методы
                                  • Events: EmitEvent/On
                                  ▲
                                  │
                       ┌──────────┴────────────────────────────────────┐
                       │            frontend/ (React 18)               │
                       │                                                │
                       │  main.jsx → App.jsx (router, providers)        │
                       │                                                │
                       │  Views:                                        │
                       │    • HomeView          (Connect/Disconnect)    │
                       │    • ProxyListView     (список, пинг, edit)    │
                       │    • AddProxyView      (2168 LOC, импорт/edit) │
                       │    • SettingsView      (настройки + export)    │
                       │    • RulesView         (умные правила)         │
                       │    • LogsView                                  │
                       │    • BuyProxyView                              │
                       │                                                │
                       │  Hooks: useAppConfig, useDaemonControl/Ping/   │
                       │         Status, useLogs, useCheckUpdate        │
                       │                                                │
                       │  Contexts: App, Config, Connection, Log        │
                       │                                                │
                       │  Modals: Updater, UpdateNotification,          │
                       │          DeepLinkImport, ProtocolSelection,    │
                       │          ProtocolWarning, AppDialog            │
                       └────────────────────────────────────────────────┘
```

## Модули и навигация

| Зона                    | Файлы (LOC)            | Ревью                                         |
| ----------------------- | ---------------------- | --------------------------------------------- |
| **Entry + сервисы**     | ~6 000 LOC             | [01-entry-and-services.md](./01-entry-and-services.md) |
| `main.go`               | 98                     |                                               |
| `app.go` (god-object)   | 2 150                  |                                               |
| `internal/config/*`     | ~1 940                 |                                               |
| `internal/logger`       | 335                    |                                               |
| `internal/adblock`      | 244                    |                                               |
| `internal/updater`      | ~1 075                 |                                               |
| `tools/`, `scripts/`    | ~430                   |                                               |
| **Proxy-engine**        | ~13 000 LOC            | [02-proxy-engine.md](./02-proxy-engine.md)    |
| `manager.go`            | 1 430                  |                                               |
| `uriparser.go`          | 1 682                  |                                               |
| `outbound.go`           | 791                    |                                               |
| `engine.go`             | 736                    |                                               |
| `singbox.go`            | 464                    |                                               |
| `country.go`            | 313                    |                                               |
| `blocked_provider.go`   | 312                    |                                               |
| `endpoints.go`          | 306                    |                                               |
| `router.go`             | 255                    |                                               |
| `sysproxy_*.go`         | 4 файла, ~624          |                                               |
| тесты                   | ~3 800                 |                                               |
| **System (cross-OS)**   | ~5 500 LOC             | [03-system.md](./03-system.md)                |
| `tray.go`               | 679                    |                                               |
| `killswitch_*.go`       | 720 + 235+248+125 per-OS                                |
| `autostart_*.go`        | 122+98+99 per-OS       |                                               |
| `netmon.go`             | 132                    |                                               |
| `processtree/*`         | 727                    |                                               |
| `instance_messenger_*`  | 260                    |                                               |
| `taskbar_restore_hook_*`| 198                    |                                               |
| `webview2_*.go`         | 206                    |                                               |
| `legacy_migration.go`   | 130                    |                                               |
| `getlantern_systray/*`  | ~1 200                 |                                               |
| **Frontend (React)**    | ~12 000 LOC            | [04-frontend.md](./04-frontend.md)            |
| `AddProxyView.jsx`      | 2 168                  |                                               |
| `proxyParser.js`        | 853                    |                                               |
| `HomeView.jsx`          | 755                    |                                               |
| `ProxyListView.jsx`     | 752                    |                                               |
| `SettingsView.jsx`      | 502                    |                                               |
| `useAppConfig.js`       | 462                    |                                               |
| `useDaemonControl.js`   | 406                    |                                               |
| `wailsAPI.js`           | 354                    |                                               |
| views/components/hooks/contexts/utils/locales |                                               |

## Сквозные документы

- 📋 [05-tests.md](./05-tests.md) — детальный разбор тестового покрытия по всем зонам
- 🛠 [06-refactoring.md](./06-refactoring.md) — приоритизированный список улучшений (Critical / High / Medium / Low)

## Ключевые потоки данных (high-level)

### A. Запуск приложения
```
main.go → NewApp → Wails.Run
        → app.startup(ctx)
          → CryptoService (install-salt, HWID-bind)
          → ConfigManager.Init (load + decrypt + migrate v1→v2)
          → ProxyManager.Init
          → AdBlock.LoadFromCache
          → KillSwitch.Disable (cleanup leftover rules)
          → NetMonitor.Start
          → Tray.Start
          → applyQueuedDeepLink (если из --link)
```

### B. Connect (пользовательский путь)
```
UI: click Connect → Wails RPC → app.Connect(dto, rules, killSwitch, adBlock)
  → manager.Connect (Phase 1: pre-checks, TCP-ping, validate)
  → manager (Phase 2, 60s timeout): engine.Start → sing-box.Instance.Start → probe
  → manager (Phase 3): sysProxy.Set / sysProxy.Disable, commit state, healthWatchdog
  → tray.SetConnectedProxy → wailsRuntime emits "status:update"
```

### C. Импорт подписки
```
UI: paste URL → app.AddSubscription
  → fetchSubscriptionFromURL (with x-hwid, custom UA, https-only by default)
    → resolveEncryptedSubscriptionURL (RVSUB1 unwrap)
    → normalize URL → doFetch
    → ParseSubscriptionBody (uriparser): JSON xray / sing-box / base64 / raw-lines
  → FinalizeSubscriptionEntries → SplitAutoEntries (auto-group)
  → ConfigManager.SaveConfig
```

### D. In-app обновление
```
UI: Check update → app.GetUpdateManifest → updater.Check (FetchManifest https-only)
UI: Update now → updater.Download (https + host whitelist + size limit + atomic rename)
  → updater.Verify (sha256, mismatch → delete)
  → updater.Install (per-OS strategy):
       Windows portable → batch handover (copy from %TEMP%)
       Windows NSIS     → silent installer
       macOS            → mount DMG + ditto + open
       Linux            → AppImage replace + syscall.Exec
```

## Поддерживаемые протоколы

| Категория            | Протоколы                                                                            |
| -------------------- | ------------------------------------------------------------------------------------ |
| Классические         | HTTP, HTTPS, SOCKS5                                                                  |
| VPN-стек (sing-box)  | VLESS, VMESS, Trojan, ShadowSocks, WireGuard, **AmneziaWG 2.0** (полный набор), Hysteria2, NaiveProxy (beta) |
| Транспорт-обёртки    | TLS, Reality, WS, gRPC, xHTTP, mKCP, QUIC                                            |
| URI-схемы            | `vless://`, `vmess://`, `trojan://`, `ss://`, `wg://`, `awg://`, `hysteria2://`, `naive+https://` |
| Deep-link            | `resultv://import/<base64>`, `resultv://rvsub/<base64>`, `resultv://crypt4/...`      |

## Глоссарий

| Термин                | Значение                                                                                       |
| --------------------- | ---------------------------------------------------------------------------------------------- |
| **Proxy mode**        | Системный HTTP/SOCKS-прокси, sing-box слушает локальный порт                                   |
| **Tunnel/TUN mode**   | sing-box создаёт TUN-интерфейс, маршрутизирует весь трафик через него (требует admin)          |
| **Global / Smart**    | Режимы Router: Global = всё через VPN; Smart = по правилам (домены, приложения, geosite/geoip) |
| **Kill Switch**       | Firewall-правила, блокирующие весь трафик кроме прокси-узла при разрыве VPN                    |
| **RVSUB**             | Собственный формат шифрования подписочного URL (subscription_decrypt.go, ключ через ldflags)   |
| **HWID**              | Стабильный идентификатор машины (для install-salt и анти-обхода device-лимита провайдера)      |
| **AUTO group**        | Автоматическая группа прокси из подписки, выбираемая по тегу `[auto]`                          |
| **AmneziaWG (AWG)**   | WireGuard с обфускацией Jc/Jmin/Jmax/S1-S4/H1-H4/I1-I5/J1-J3 (форк sing-box)                   |
| **Deep-link**         | Открытие приложения через URL `resultv://...` (Windows registry, Linux .desktop, macOS plist)  |

## Сводка состояния качества

| Аспект                  | Оценка | Комментарий                                                                                          |
| ----------------------- | ------ | ---------------------------------------------------------------------------------------------------- |
| Тестовое покрытие       | 🟢      | ~6 600 LOC тестов; proxy-engine и system хорошо покрыты, фронт — без тестов                          |
| Безопасность (crypto)   | 🟢      | AES-GCM, PBKDF2, sha256-валидация апдейтов, HTTPS-only, SSRF-guard                                   |
| Безопасность (KS)       | 🟡      | Linux iptables не покрывает IPv6, macOS clobbers global pf-ruleset, resolve без таймаута             |
| Безопасность (URL)      | 🟢      | host-whitelist для апдейтов, https-only, валидация после redirect (см. M-6)                          |
| Архитектура             | 🟡      | `app.go` 2150 LOC, `manager.go` 1430 LOC, `uriparser.go` 1682 LOC, `AddProxyView.jsx` 2168 — god-объекты |
| Кросс-платформенность   | 🟡      | Windows полный, macOS/Linux рабочие но Linux singleton отсутствует, macOS deep-link runtime отсутствует |
| i18n                    | 🟡      | en/ru есть, но есть хардкоды в `MainLayout`, `useAppConfig`, `addLog`                                |
| Логирование             | 🔴      | Не персистентно (500 in-memory), пользователь не может приложить лог к багу                          |
| Shutdown discipline     | 🔴      | `os.Exit(0)` после 10s watchdog обрывает kill switch Disable; OnUnexpectedExit вообще не зовёт KS    |

🟢 хорошо • 🟡 нужны улучшения • 🔴 критично

---

**Дальше**: [01-entry-and-services.md](./01-entry-and-services.md) для разбора корневого кода и базовых сервисов.
