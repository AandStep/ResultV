# Код-ревью: `internal/proxy/*` — ядро VPN-движка

Документ описывает архитектуру и качество ядрового модуля приложения **ResultV** (Wails v2 + Go).
Модуль `internal/proxy` — это сердце приложения: парсинг URI всех поддерживаемых протоколов,
сборка конфигурации sing-box, lifecycle движка, маршрутизация, ping, подписки, system-proxy на ОС.

> Все ссылки на код используют относительный путь от `C:\ResultVPC\` и формат `файл.go:строка`.

---

## 1. Обзор модуля

### Роль в архитектуре
`internal/proxy` — единственный путь от UI (frontend) и `app.go` (Wails-bridge) к реальному прокси-движку
[`shtorm-7/sing-box-extended`](https://github.com/shtorm-7/sing-box) (форк sagernet/sing-box). Всё, что
делает приложение в плане сетевой работы, проходит через `proxy.Manager`.

### Публичный API наружу (`app.go` использует)
| Точка входа | Файл | Назначение |
|---|---|---|
| `proxy.NewManager(log)` | manager.go:156 | Создание менеджера |
| `m.Init(ctx)` | manager.go:169 | Инициализация контекста, system-proxy, process-tree |
| `m.Connect(ctx, cfg, mode, …)` | manager.go:326 | Подключение к прокси |
| `m.Disconnect()` / `m.CancelConnect()` | manager.go:1140 / 315 | Отключение / отмена in-flight Connect |
| `m.SetMode(mode)` | manager.go:1208 | Переключение proxy ↔ tunnel |
| `m.ReconnectWithRoutingRules(...)` | manager.go:1258 | Переподключение с новыми правилами |
| `m.GetStatus()` / `m.GetMode()` | manager.go:1279 / 1323 | Текущий статус |
| `m.Ping(ip, port, type)` | manager.go:1331 | Универсальный ping (TCP / UDP / LAN-bind / сессия) |
| `m.ToggleKillSwitch(bool)` | manager.go:1534 | Включение kill switch |
| `m.Shutdown()` | manager.go:1554 | Финальная очистка при завершении приложения |
| `m.LoadBlockedLists(paths…)` | manager.go:298 | Загрузка локальных списков заблокированных |
| `proxy.ParseProxyURI(s)` | uriparser.go:61 | Парсер одного URI |
| `proxy.ParseSubscriptionBody(s)` | uriparser.go:90 | Парсер подписки (JSON/base64/lines) |
| `proxy.IsDeepLink` / `DecodeDeepLink` | deeplink.go:26/53 | `resultv://` deep-link |
| `proxy.IsEncryptedSubscription` / `DecryptSubscription` | subscription_decrypt.go:36/45 | RVSUB1-шифрованные подписки |
| `proxy.NewCountryClient(path)` | country.go:90 | Определение страны через project-API |
| `proxy.SplitAutoEntries` / `ExtractAutoGroupName` / `FinalizeSubscriptionEntries` | uriparser.go:1714/1648/1748 | Группировка/фильтрация результатов парсинга |

### Что НЕ делает модуль
- **Не работает с UI** напрямую: события только через `wailsRuntime.EventsEmit` (`status:update`).
- **Не управляет firewall** напрямую — есть хуки `KillSwitchFirewallEngage/Disengage` в `Manager`,
  заполняются из `app.go`.
- **Не хранит конфигурацию** — все persistent-данные в `internal/config`.

### Размеры и метрика сложности
| Файл | Строк | Заметка |
|---|---:|---|
| manager.go | 1593 | God-object — см. п. 14 |
| uriparser.go | 1794 | 11 парсеров + URI-helpers — см. п. 4 |
| engine.go | 821 | Типы конфигов sing-box + builders |
| outbound.go | 833 | Мэппинг 8 протоколов на sing-box outbounds |
| singbox.go | 540 | Конкретный SingBoxEngine: lifecycle |
| country.go | 339 | Project-controlled lookup + кеш |
| blocked_provider.go | 332 | HTTP + CSV/raw парсинг |
| sysproxy_linux.go | 313 | GNOME + KDE + env-backends |
| router.go | 320 | Router (whitelist matching + блокированные домены) |
| endpoints.go | 325 | WireGuard / AmneziaWG endpoint builder |
| sysproxy_darwin.go | 205 | networksetup CLI |
| sysproxy_windows.go | 150 | Реестр HKCU\\Internet Settings |
| Тесты (всего) | 3 800+ | См. п. 12 |

---

## 2. Движок sing-box (`engine.go` + `singbox.go`)

### Engine interface
Файл `engine.go:68-81` определяет интерфейс:
```go
type Engine interface {
    Start(ctx context.Context, cfg EngineConfig) error
    Stop() error
    IsRunning() bool
    GetTrafficStats() (up, down int64)
    ApplyAppWhitelist(paths []string) error
}
```
Единственная конкретная реализация — `SingBoxEngine` (`singbox.go:77`).

### Сборка sing-box конфига (`engine.go`)
- `BuildProxyModeConfig(cfg)` (engine.go:332) — `mixed` inbound на 127.0.0.1:N (или 0.0.0.0:N если listenLAN).
- `BuildTunnelModeConfig(cfg)` (engine.go:363) — `tun` inbound с фиксированным CIDR `172.19.0.1/30`
  плюс ULA-префиксом `fdfe:dcba:9876::1/126` для IPv6 (иначе `strict_route` молча блэкхолит IPv6),
  stack=`system` (Wintun), `auto_route`. Этот stack специально выбран для совместимости с anti-cheat (EAC, BattlEye)
  — см. комментарий engine.go:370.
- `buildDNS` (engine.go:434) — раздельная логика для proxy/tunnel. **Важно:** в proxy-режиме custom DNS идут
  напрямую UDP (без detour), иначе circular dependency (см. подробный комментарий engine.go:500-510).
- `buildRoute` (engine.go:562) — собирает sing-box rules: `sniff` → `hijack-dns` → probe-domains → self-direct →
  app-whitelist → user-whitelist.

### Защита от утечки DNS (`strict_route`)
Тестирование на whoer.net в России показывало утечку Rostelecom/MSK-IX резолверов
(`176.208.103.109`, `193.232.241.48`) даже при `sysdns_windows.go`, перенаправляющем DNS каждого
адаптера на `1.1.1.1`/`8.8.8.8`. Причина — **Smart Multi-Homed Name Resolution** в Windows:
DNS Client посылает запросы параллельно через каждый активный интерфейс. Пакет UDP/53 от LAN-адаптера
(пусть даже с dst=`1.1.1.1`) уходит на сетевой стык провайдера, который **прозрачно перехватывает** его
и отвечает от своего резолвера. whoer.net видит Ростелеком/MSK-IX как реального ответчика.

Защита — `strict_route: true` на TUN inbound (`EngineConfig.DNSLeakProtection`, engine.go:376):
sing-box добавляет правила Windows Filtering Platform (WFP), которые **блокируют любые исходящие
пакеты, не идущие через TUN**. Параллельный multi-homed-запрос через LAN-адаптер дропается на уровне
ядра, Windows фолбэчится на TUN, запрос идёт `hijack-dns → server → proxy detour → remote DNS`.
Провайдер не видит пакета, перехватить нечего.

Управляется тогглом "Защита от утечки DNS" (frontend `settings.dns_leak_protection`,
`config.AppSettings.DNSLeakProtection *bool`). Pointer-тип нужен для миграции: легаси-конфиги без поля
получают `true` через `EffectiveDNSLeakProtection()` — иначе апгрейд приложения молча оставлял бы
пользователя в дырявом состоянии.

`sysdns_windows.go` (per-adapter DNS override) остаётся как defense-in-depth и обязателен для Proxy mode
(strict_route есть только в Tunnel mode).

### SingBoxEngine lifecycle (`singbox.go`)
| Шаг | Где | Что делает |
|---|---|---|
| `Start(ctx, cfg)` | singbox.go:224 | mutex.Lock, mkdir DataDir, `bootLocked(ctx, cfg, true)`, сохраняет `savedCfg`/`savedCtx`, `running=true` |
| `bootLocked` | singbox.go:262 | Build sb-config → JSON → пишет в `tmpDir/resultproxy-singbox.json` (0o600) → `box.New` → `AppendTracker` → `instance.Start()` |
| `shutdownInstanceLocked` | singbox.go:342 | `cancel()`, `inst.Close()` с **5-сек таймаутом и горутиной** (защита от зависания), удаление tmp-config |
| `ApplyAppWhitelist` | singbox.go:377 | **Hot-reload**: shutdown→build→boot с новым AppWhitelist; сохраняет traffic counters; ~200-500ms gap |
| `Stop()` | singbox.go:423 | shutdownInstanceLocked + reset savedCfg + `running=false` |

### Tracker и метрики
`trafficTracker` (singbox.go:135) реализует `adapter.ConnectionTracker`. Регистрируется через
`instance.Router().AppendTracker(tracker)`. Для каждого соединения:
- Заворачивает `net.Conn` в `bufio.NewInt64CounterConn` для подсчёта байт (атомики `uploadBytes`/`downloadBytes`).
- Логирует первое соединение каждого `host→outbound` (singbox.go:486-528).
- Жёсткий лимит **1000 уникальных доменов** (singbox.go:506) — после этого только ошибки логируются;
  это правильно (защита от log-flood в подписке с 1000+ серверов).

### Шум в логах sing-box (фильтр)
`singBoxLogWriter.WriteMessage` (singbox.go:103) отбрасывает:
- `dns: exchange failed`, `process dns packet` — DNS-шумы в tunnel-режиме.
- `outbound/direct ... i/o timeout|connectex|actively refused` — обычные таймауты direct-исключений.

### Lifecycle-инвариант (важно для безопасности)
`bootLocked` принимает `ctx` (саварживается в `savedCtx`). Engine запускается с **долгоживущим
ctx (контекст приложения)**, не с connectCtx (manager.go:454, явный комментарий). Это критично —
если передать connectCtx, sing-box умрёт сразу после Connect() (DNS context canceled).

---

## 3. Manager (`manager.go`, 1593 строк)

### Структура (логические группы)

#### 3.1. Поля состояния (manager.go:70-123)
```go
type Manager struct {
    mu sync.Mutex
    ctx context.Context
    log *logger.Logger
    engine Engine
    router *Router
    sysProxy SystemProxy

    connected bool
    mode ProxyMode
    proxy *ProxyConfig
    killSwitch bool
    adBlock bool
    routingMode RoutingMode
    whitelist []string
    appWhitelist []string
    connectedAt time.Time

    prevUp/prevDown int64 // для расчёта скорости
    lastTick time.Time
    localPort int
    listenLAN bool
    dnsServers []string
    tunIPv4 string

    connectCancelMu sync.Mutex   // отдельный mutex для отмены Connect
    connectCancel context.CancelFunc

    procTracker *processtree.Monitor   // отслеживание child-процессов
    proxyDead bool                     // флаг health-watchdog'а
    healthCancel context.CancelFunc

    KillSwitchFirewallEngage   func(ProxyConfig, []string)
    KillSwitchFirewallDisengage func()
}
```

#### 3.2. Lifecycle (Connect/Disconnect)
**Connect** (manager.go:326) делится на 3 фазы:
1. **Phase 1** (под `m.mu`): проверки (subscription-section, endpoint-protocol, admin для tunnel,
   pre-ping для TCP-протоколов), сборка `EngineConfig`, `validateEngineConfig`.
2. **Phase 2** (без lock): `engine.Start` (60-сек таймаут), `runPostStartProbe`. Сохраняется
   `connectCancel` для отмены извне.
3. **Phase 3** (под `m.mu`): re-check `engine.IsRunning()` (защита от race с Disconnect),
   `sysProxy.Set` (или `Disable` для tunnel+AMNEZIAWG), коммит состояния, `emitStatus`.

**Disconnect** (manager.go:1140) выполняется так:
1. `CancelConnect()` (без lock).
2. `engine.Stop()` (без lock) — **специально перед** `m.mu.Lock`, потому что
   `disconnectLocked` не остановит engine, если `m.connected==false`, а на середине Connect()
   движок уже запущен. См. подробное объяснение manager.go:1141-1149.
3. Под lock: stop trackers, sysProxy.Disable, очистка state.

#### 3.3. Internal `connectLocked` (manager.go:592)
Используется в `SetMode` / `ReconnectWithRoutingRules`. **Дублирует** Connect почти полностью
(только без pre-ping и без 60-сек таймаута). См. п. 14 — это серьёзное дублирование.

#### 3.4. Post-start probes (`runPostStartProbe`, manager.go:745-1049)
Большой `switch` по протоколам — для каждого свои стратегии retry с разными delay-таблицами:
- `vless/vmess` (только xhttp/splithttp в proxy mode): 3 попытки HTTP через прокси
- `hysteria2`: 3 HTTP попытки → fallback UDP/TCP-probe → возврат причины
- `wireguard/amneziawg`: UDP-probe → +sleep 2-3с → 3-4 HTTP-попытки с длинными delays (AmneziaWG медленнее)
- `trojan`/`naive`: 3 HTTP-попытки в обоих режимах
- **Финальный fallback** (manager.go:1018) — для всех протоколов в tunnel-режиме: 4 попытки HTTP до 8с
  (для AEAD ciphers Shadowsocks нужен warm-up).

#### 3.5. Health watchdog (manager.go:1414-1532)
Запускается в горутине из `startHealthWatchdogLocked`. Каждые 5с пингует proxy. После 2 подряд
неудач — `proxyDead=true`, emit события, опционально включает firewall kill switch.

Сигнатурно правильно: snapshot+release под lock, probe без lock, re-check после probe.

#### 3.6. Process-tree (manager.go:240-296)
Через `processtree.Monitor` отслеживает child-процессы (Steam → Steamwebhelper и т.п.).
Подход — **не делать hot-reload автоматически**, только логировать обнаруженных потомков
(объяснение manager.go:227-238). Это правильно: tear-down TUN ломает QUIC/TLS-сессии.

#### 3.7. Ping (manager.go:1331-1404)
Развилка по протоколу/режиму:
- Активный UDP-протокол (HYSTERIA2/WG) при connected → `session_active` (без реального ping).
- HYSTERIA2 не активен → `pingHysteria2Probe` (UDP с TCP fallback).
- WireGuard → TCP probe.
- Tunnel + не UDP → `pingLANProbe` (`tcp_lan_bind`).
- По умолчанию → `pingTCPProbe`.

Глобальные переменные-пробники (`pingTCPProbe`, `pingLANProbe` и т.д.) — для удобства тестов.

### Mutex-дисциплина (важно)
- `m.mu` защищает всё state кроме `connectCancel`.
- `connectCancelMu` — отдельный, чтобы `Disconnect/CancelConnect` могли работать пока Connect под `m.mu`.
- `Router.mu` — `sync.RWMutex` (router.go:43), отдельный.
- `SingBoxEngine.mu` — отдельный, защищает `instance`/`cancel`/`configPath`/`savedCfg`.

Несколько найденных нюансов в п. 14.

### События наружу
Только `wailsRuntime.EventsEmit(m.ctx, "status:update", status)` в `emitStatus` (manager.go:1576).
Других событий нет.

---

## 4. URI-парсеры (`uriparser.go`, 1794 строк)

### Поддерживаемые схемы и функции

| URI префикс | Функция | Файл:строка | Note |
|---|---|---|---|
| `vless://` | `parseVLESSURI` | uriparser.go:821 | Поддержка extra=`{}` JSON embedded |
| `vmess://` | `parseVMessURI` | uriparser.go:883 | Base64+JSON (Xray legacy) |
| `ss://` | `parseShadowsocksURI` | uriparser.go:929 | Два формата: `base64@host` и полностью base64 |
| `trojan://` | `parseTrojanURI` | uriparser.go:995 | Куча алиасов SNI: sni/peer/serverName/server_name/host |
| `hy2://` / `hysteria2://` | `parseHysteria2URI` | uriparser.go:1236 | Алиасы взаимозаменяемы |
| `wg://` | `parseWireGuardURI` | uriparser.go:1275 | Принимает private_key/psk/address/allowed_ips через query |
| `awg://` | `parseAmneziaWGURI` | uriparser.go:1323 | + AWG 1.0 (uint) и AWG 2.0 (low-high) headers H1-H4 |
| `naive+https://` / `naive://` | `parseNaiveURI` | uriparser.go:1174 | требует username и password |

### JSON-формат подписок
`ParseSubscriptionBody` (uriparser.go:90) пробует в порядке:
1. Прямой JSON (один объект или массив объектов)
2. Построчный парсинг каждой строки через `ParseProxyURI`
3. Base64 → опять JSON или построчно

`parseJSONOutbound` (uriparser.go:228) умеет:
- Xray-style outbound с `settings.vnext[0]`, `streamSettings`, `tlsSettings`, `realitySettings`, `wsSettings`, `grpcSettings`, `xhttpSettings`
- sing-box-style outbound (с `type` вместо `protocol`)
- Naive Client JSON формат (`{listen, proxy, log}`) — `tryParseNaiveClientConfigMap`

### Группировка
- `SplitAutoEntries` (uriparser.go:1748) — разделяет "🇨🇦 impVPN Auto VLESS" + "🇩🇪 impVPN Auto HYSTERIA2"
  на auto-группу + индивидуальные.
- `ExtractAutoGroupName` (uriparser.go:1648) — две стратегии: exact-match после
  separator-strip и LCP runes ≥3.
- `FinalizeSubscriptionEntries` (uriparser.go:1714) — "0.0.0.0" / "::" placeholder → SECTION-маркер.

### Country detection в парсере
`countryFromNameAndHost` (uriparser.go:1587):
1. Regional Indicator emoji в начале имени → ISO alpha-2 (например, 🇩🇪 → "de")
2. Иначе hostname-hint: первые 2 буквы поддомена ("ru-1.example.com" → "ru")

### Качество парсеров: пробелы и хрупкие места
- `parseVLESSURI`: использует `http://` replacement для `url.Parse`. Это **работает в большинстве
  случаев**, но любое значение query-параметра, начинающееся с `?` без правильного экранирования,
  парсит криво. Тесты покрывают типичные кейсы.
- `parseVMessURI`: декодирует base64+JSON, но **не валидирует** обязательные поля (uuid, add, port).
  При невалидном JSON возвращает ошибку, при отсутствующих полях — `ProxyEntry` с пустыми значениями
  (uriparser.go:883-927).
- `parseShadowsocksURI`: тихо использует `aes-256-gcm` по умолчанию, что может скрыть ошибки сервера.
- Все парсеры **используют `_ =`** для ошибок `url.PathUnescape` и `json.Marshal` (uriparser.go:830,
  871, 1005, 1075…). Маловероятно, что они дадут ошибку, но без логирования.
- `getQueryParamCI` (uriparser.go:34) — case-insensitive lookup для AWG, потому что разные клиенты
  используют jc/Jc/JC. Хорошее качество.

### URI safety / SSRF
- Парсеры **не делают сетевые запросы**, так что прямого SSRF нет.
- `SubscriptionURL` хранится в `ProxyConfig`, но фактический fetch делается в `app.go` (вне ревью).
  Стоит проверить, что в `app.go` есть allowlist схем/портов.

---

## 5. Outbound и outbound-протоколы (`outbound.go`, 833 строк)

`buildProxyOutboundRaw` (outbound.go:180) — большой `switch` по `proxy.Type`. Каждый case
создаёт типовой `SBOutbound`, заполняя протокол-специфичные поля и затем
вызывает `applyTLSAndTransport`.

### Protocol mapping table

| Type | Outbound type | TLS apply | Transport apply | Special |
|---|---|---|---|---|
| HYSTERIA2 | `hysteria2` | Inline (всегда TLS, default ALPN `["h3","hysteria"]`) | — | obfs (salamander), up_mbps/down_mbps |
| SOCKS5/SOCKS | `socks` | — | — | Version="5" |
| SS | `shadowsocks` | — | — | method default `aes-256-gcm` |
| VMESS | `vmess` | `applyTLSAndTransport` | + | UUID + alterId + security ("scy"/encryption alias) + xudp default |
| VLESS | `vless` | `applyTLSAndTransport` | + | UUID + flow + xudp default |
| TROJAN | `trojan` | `applyTLSAndTransport` (auto-fallback TLS) | + | Сложная цепочка SNI: sni/serverName/peer/host/IP |
| NAIVE/NAIVEPROXY | `naive` | enabled + server_name only | — | uTLS не поддерживается, insecure=false enforced (см. п. 6) |
| **default** | `http` | — | — | Fallback HTTP proxy |

### TLS-логика (`applyTLSAndTransport`, outbound.go:405)

`security` field в extra определяет режим:
- `reality` — `Reality{Enabled, PublicKey, ShortID}` + uTLS обязательно (default `chrome`), без ALPN
  (Reality сам — masquerade), `normalizeRealityShortID` (outbound.go:148) нормализует hex в lowercase.
- `tls` — простой TLS + опционально uTLS (default через `system.WebViewFingerprint()` = Edge на Windows).
- default — если ни `security=tls` ни extra-field `tls`, без TLS (если security не явный `none`).

**Авто-детект**: если `pbk` (public_key для Reality) присутствует — security принудительно `reality`
(outbound.go:409).

### ALPN-логика (важно для безопасности)
`applyTLSAndTransport` после применения TLS:
- xhttp/splithttp **требуют h2 ALPN** (outbound.go:480) — иначе протокол не работает.
- `xhttpPreferH2ALPN(alpn, false)` (outbound.go:782) — если ALPN начинается с h3, переставляет h2 в начало.
- Reality — `len(ALPN) == 0` (Reality сам управляет fingerprint'ом).
- HTTP-транспорты (grpc/h2) — `["h2","http/1.1"]`.
- WebSocket — `["h2","http/1.1"]`.
- **plain TCP (Trojan/VLESS/VMESS без transport)** — ALPN оставляется пустым (см. комментарий
  outbound.go:493). Раньше выставлялся `["h2","http/1.1"]`, что в Trojan приводило к
  `ERR_SSL_PROTOCOL_ERROR` (сервер выбирал HTTP/2, а sing-box продолжал слать Trojan binary).

### Transport-mapping (`applyTransportOnly`, outbound.go:501)

| network | transport type | Особое |
|---|---|---|
| ws/websocket | `ws` | path + host + ed= early-data parsing (outbound.go:526) |
| httpupgrade | `httpupgrade` | path + host |
| grpc | `grpc` | serviceName/authority (но **authority выставляется пустым** в финальном SBOutboundTransport — см. uriparser_test.go:478) |
| http/h2 | `http` | host + path + method |
| tcp | `http` (если header.type=http) | Xray TCP+headerType=http obfuscation |
| xhttp/splithttp | `xhttp` | path/host/mode + UplinkHTTPMethod + XPadding (default `"100-1000"`) + Xmux + Sc* range |

### uTLS / WebView fingerprint
`resolvedFingerprint` (outbound.go:98) — приоритет:
1. Явный `fp`/`client-fingerprint` в extra
2. Иначе `system.WebViewFingerprint()` — на Windows возвращает текущую версию Edge WebView2, на
   macOS/Linux — Safari.

Это **очень хорошее решение для bypass**: fingerprint совпадает с тем, что видит сервер от
реального браузера на той же машине.

---

## 6. Router (`router.go`, 320 строк)

Простой in-memory router для:
- Списка заблокированных доменов (Smart-mode).
- Whitelist matching (правила `.ru`, `2ip.ru` и т.п.).

### Ключевой алгоритм
`IsWhitelisted` (router.go:87) использует **четность правил-совпадений**:
- 1 совпадение → whitelisted (direct)
- 2 совпадения → NOT whitelisted (proxy) — позволяет inversion
- 3 совпадения → whitelisted

То есть `[".ru", "avito.ru"]` для `avito.ru`: совпадает с `.ru` И с `avito.ru` → 2 совпадения → **через
прокси** (нужный override). Для `m.avito.ru` + третий entry `m.avito.ru` → снова direct.

Это очень элегантный механизм nested-exception, который покрывается тестами router_test.go:50-83.

### Где используется
- `engine.go:679-689` — в `buildRoute` для генерации `DomainSuffix` правил sing-box.
- `sysproxy_windows.go:127-144` (`buildBypassList`) — для ProxyOverride в реестре.
- `sysproxy_linux.go:75-78`, `sysproxy_darwin.go:106-116` — для bypass на других ОС.

`GetSafeOSWhitelist` (router.go:184) — берёт **только terminal-листы whitelist'а** (где нет
вложенных детей-правил). Это правильно: OS-уровень не поддерживает nested-exception, и
если в whitelist'е есть `.ru` и `avito.ru`, то на OS-уровне ничего не выставляется (так как
правила конфликтуют), а это разруливается sing-box rules-движком.

### blockedDomains
- Default — захардкоженные `instagram/facebook/twitter/x/t.me/discord/netflix` (router.go:271).
- `LoadBlockedLists` загружает из файлов (комментарии `#`/`;`/`!`/`//`).
- `SetBlockedDomains` заменяет полностью (используется `blocked_updater`).
- `IsBlockedDomain` (router.go:164) — **substring match** (`strings.Contains`), не суффикс! Это даёт
  false positives. Например, `notdiscord.example.com` сматчит `discord.com`. См. п. 14.

---

## 7. Endpoints (`endpoints.go`, 325 строк)

Endpoint = sing-box endpoint protocol для WireGuard / AmneziaWG (вместо outbound). Это особый
тип в sing-box, который работает на L3 уровне и идёт в `route.final="proxy"`.

### `buildEndpoints` (endpoints.go:66)
Возвращает `[]SBEndpoint{ep}` где `ep`:
- `Type: "wireguard"`, `Tag: "proxy"`, `Detour: "direct"` (через локальный direct outbound)
- `Address` — local CIDR (`10.0.0.2/32` по умолчанию)
- `PrivateKey`, `Peers[0]` (server endpoint + PublicKey + PSK + AllowedIPs)
- `Amnezia` (опционально, только для AMNEZIAWG)

### AWG 2.0 (AmneziaWG)
H1-H4 могут быть **string range** `"low-high"` (AWG 2.0) либо просто uint (AWG 1.0). См. подробный
комментарий engine.go:217-225. Это требует форк sing-box-extended ≥ v1.13.11.

`normalizeAmnezia` (endpoints.go:279) — клиппинг junk-полей до 4096 символов, нормализация
порядка (jmin ≤ jmax), защита от отрицательных.

Хорошо покрыто `awg2_e2e_test.go` (end-to-end через upstream parser).

---

## 8. Country (`country.go`, 339 строк)

### Архитектура
`CountryClient` — клиент к **проекту-контролируемому** API (`https://result-proxy.ru/countryAPI/api.php`).
До недавнего рефакторинга было 3 third-party провайдера (ip-api.com через plain HTTP, ipapi.co, geojs.io)
— это утекало реальный IP. Сейчас:
- Один endpoint
- **Принудительный HTTPS** (country.go:215) — даже env-override не принимается без https.
- TTL **24 часа** (country.go:44).
- Per-IP кеш + self-кеш (отдельная "self" entry, country.go:82-86).
- Лимит **512 entries** в кеше (country.go:50) с эвикцией старейших (`evictLocked`).
- Disk-persistence в `country.cache.json` (0o600, country.go:310).

### Resolution
`resolveToIP` (country.go:155) разрешает hostname в IP (MaxMind на сервере требует IP-литерал),
**IPv4-first**. Таймаут 3с для DNS (country.go:159).

### Безопасность
- Generic UA "ResultV" (country.go:229) — не fingerprint'ит билд.
- Response limit 32KB (country.go:239).
- Жёсткая валидация: код должен быть ровно 2 буквы, не "??" (country.go:257-263).

Хорошо покрыто `blocked_provider_test.go:30-179` (TLS-server, caching, plaintext-rejection, hostname-resolution).

---

## 9. Подписки (`blocked_*.go`, `subscription_decrypt.go`)

### Subscription decrypt (`subscription_decrypt.go`, 124 строк)
- Магия `RVSUB1:` (subscription_decrypt.go:27).
- AES-256-GCM, ключ инъецируется через `-ldflags` (см. subscription_decrypt.go:13-24).
- **Честно задокументировано**: "это не cryptographic privacy boundary, это обфускация" —
  ключ один на все билды, любой может извлечь его через `strings`. Хорошо, что это документировано.
- Гибкий base64 (`decodeBase64Flexible`, subscription_decrypt.go:105) — пробует все 4 варианта.
- Whitespace-tolerant: вырезает spaces/CR/LF/TAB перед декодом.

### Blocked-domain provider (`blocked_provider.go`, 332 строк)
- `HTTPBlockedListProvider.FetchBlockedDomains`:
  - 8 MB лимит на body (blocked_provider.go:99) — защита от malicious endpoint.
  - Default-источники для country=`ru`: citizenlab + itdoginfo (blocked_provider.go:316-326).
  - Env-override: `RESULTPROXY_BLOCKED_LIST_SOURCES` (CSV) или `RESULTPROXY_BLOCKED_LIST_URL_TEMPLATE`.
- `parseDomainPayload` (blocked_provider.go:181) — поддерживает 4 формата:
  - CSV (citizenlab `url,category`)
  - JSON-массив `["a.com", …]`
  - Plain text per-line с комментариями `#`/`;`/`!`/`//`
  - hosts-формат (`0.0.0.0 facebook.com`)
  - dnsmasq (`server=/youtube.com/127.0.0.1`)
  - Adblock-style (`||discord.com^`)

### Cache (`blocked_provider.go:129-168`, `blocked_updater.go`)
- `BlockedDomainsCache` структура: country, timestamp, source ("remote"/"cache"/"local"/"builtin"), domains
- `ResolveBlockedDomains` (`blocked_updater.go:32`) — каскад:
  1. Через provider → если ok, сохранить кеш
  2. Иначе из disk-cache (если country совпадает с resolved)
  3. Иначе из local files
  4. Иначе built-in defaults
- `RefreshRemoteBlockedDomains` — только remote, без fallback.

### config_validation.go (133 строк)
`validateEngineConfig` (config_validation.go:30) — финальная проверка перед connect:
1. `validateRouteFinalTarget` — `route.final` должен матчиться tag'ом в outbounds или endpoints.
2. `validateDNSConfig` — для tunnel: dns + hijack-dns rule обязательны.
3. `validateProtocolRequiredFields` — для WG/AWG: private_key + public_key + address + allowed_ips;
   для HYSTERIA2: password; для Naive: host + port + user + pass, **insecure запрещён**.

---

## 10. System-proxy per-OS

| ОС | Файл | Подход | Snapshot | Edge-cases |
|---|---|---|---|---|
| Windows | sysproxy_windows.go | HKCU\\…\\Internet Settings (`ProxyEnable`, `ProxyServer`, `ProxyOverride`) + `ipconfig /flushdns` + `netsh winhttp reset` на disable | Нет, прямая работа с реестром | GPO-блокировки настроек проксии (нужно проверять; в коде нет) |
| Linux | sysproxy_linux.go | Fan-out на 3 backend'а: GNOME (gsettings), KDE (kwriteconfig5/6), env-файл (~/.config/environment.d) | Нет; считает sufficient если applied ≥ 1 | Все 3 могут отсутствовать → ошибка |
| Darwin | sysproxy_darwin.go | `networksetup` для каждого enabled сервиса (Wi-Fi, Ethernet) + bypass list | Запоминает `appliedServices` для disable | На disable: если процесс перезапустился — best-effort на все сервисы |
| Stub | sysproxy_other.go | Возвращает error | — | — |

### Kill switch (`ApplyKillSwitch`)
Везде одинаково: ставит **проксю в 127.0.0.1:65535** (заведомо unreachable). На Windows
это вызывает поведение "no internet" для всех HTTP-приложений. **На Linux/macOS** — то же самое.

Это **простой и эффективный** подход для proxy-mode, но НЕ для tunnel-mode: TUN-уровневый
kill switch требует firewall-правил (см. `KillSwitchFirewallEngage` hook в Manager).

### Замечания
- На Windows нет **синхронизации с групповой политикой**: если есть GPO, выставляющая
  настройки HKLM, регистрация HKCU не сработает. Тест на это не предусмотрен.
- На Linux `envBackend` пишет файл, который читается **только при логин-сессии**: уже запущенные процессы
  не подхватят его. Это документировано в комментарии (sysproxy_linux.go:259-265).
- На Darwin `runNetworksetup` блокирующий: на машинах с 10+ network services может быть медленным.

### Дублирование (см. п. 14)
- Структурно 3 файла очень похожи по форме (SystemProxy interface + Set/Disable/DisableSync/ApplyKillSwitch).
- bypassList логика очень разная между Win/Lin/Mac — sing-box bypass и OS bypass семантически
  отличаются, что верно, но решение не унифицировано.

---

## 11. Ping LAN-bind (`ping_lan_bind.go`, 137 строк)

### Зачем?
Когда туннель уже подключен через WireGuard/sing-box TUN, обычный `net.Dial` идёт **через туннель**
(потому что system route table). Это:
- Дает неправильный ping (через VPN-сервер, а не напрямую).
- Может циклить (proxy → proxy через тот же proxy).

`PingProxyLANBind` (ping_lan_bind.go:119) выбирает локальный LAN IPv4 интерфейс (не loopback,
не tunnel) через `preferLANBindIPv4` и явно делает `LocalAddr: &net.TCPAddr{IP: local}` в Dialer.

### `preferLANBindIPv4` (ping_lan_bind.go:66)
- Перебирает все interfaces, исключает:
  - down/loopback (`net.FlagUp == 0 || net.FlagLoopback != 0`)
  - tunnel-like (`tun/tap/wintun/tailscale/wireguard/nordlynx/zerotier/sing-tun` — `looksLikeTunnelInterface`)
- IPv4-only, не link-local
- **Исключает sing-box's own TUN** (`172.19.0.0/30`, `isEngineTunIPv4`)
- Приоритет: private (RFC1918) → public

### Fallback
Если LAN-bind не получилось — fallback на `PingProxy` (обычный TCP dial). Это лучше, чем
ничего, но даст неточный ping в tunnel-режиме.

### Качество
- Простая логика, понятная
- Тесты только для helper'ов (`ping_lan_bind_test.go`) — нет integration-теста с реальным TCP listener
- IPv6 не поддерживается (только IPv4)

---

## 12. Тесты — детальный разбор

Всего ~25 тест-файлов, ~3 800 строк тестов. Покрытие — высокое, особенно для парсинга URI и
маршрутизации.

### По файлам

| Файл | Строк | Тестов | Покрывает |
|---|---:|---:|---|
| `engine_route_test.go` | 464 | 17 | Все builders, DNS/route правил, nested exceptions, app-whitelist + find_process |
| `manager_mode_test.go` | 701 | 12 | Connect-flow (success/fail/timeout/admin), все probe-сценарии, SetMode, fail-clear, kill switch, status |
| `manager_ping_test.go` | 144 | 4 | Ping для UDP-протоколов в tunnel, fallback, активный proxy session |
| `uriparser_test.go` | 778 | ~32 | VLESS/Trojan/VMESS/SS/WG/Naive: формы URI, JSON-подписки, xhttp/grpc/reality |
| `uriparser_awg_test.go` | 288 | ~8 | AmneziaWG URI roundtrip, AWG 1.0/2.0, case-insensitive |
| `uriparser_hysteria_test.go` | 61 | 1 | Hysteria-protocol subscription JSON (`protocol:"hysteria"` → HYSTERIA2) |
| `outbound_alpn_test.go` | 31 | 1 | xhttp prefer h2 |
| `outbound_hysteria2_test.go` | 94 | 3 | hysteria2 outbound ALPN/SNI/obfs/password fallback |
| `singbox_protocols_test.go` | 212 | 4 | WG/AWG/Hysteria2/Naive/SS config parses through real sing-box `option.Options` decoder |
| `awg2_e2e_test.go` | 113 | 1 | **End-to-end**: real-world AWG 2.0 H-range config through `singjson.UnmarshalContext` + `*Xbadoption.Range` |
| `udp_quic_smoke_test.go` | 122 | 3 | VLESS xudp + Hysteria2 + VMess xudp shapes survive `option.Options` strict decode |
| `router_test.go` | 257 | 13 | normalizeRule, IsWhitelisted (single/double/triple match), ShouldProxy (Global/Smart), GetSafeOSWhitelist, blocklist load/set |
| `country_test` | — | — | (нет отдельного country_test.go, тесты в blocked_provider_test) |
| `blocked_provider_test.go` | 239 | 7 | TLS-сервер mock, country cache, hostname resolution, plaintext rejection, CSV/dnsmasq/hosts парсинг |
| `blocked_updater_test.go` | 117 | 4 | Resolve: remote → cache → local; country mismatch fallback |
| `config_validation_test.go` | 147 | 4 | WG missing key / bare IP normalization / invalid address / Hysteria2 password |
| `deeplink_test.go` | 58 | 3 | sanitize base64, deep-link rejection of non-scheme, rvsub vs import path detection |
| `subscription_decrypt_test.go` | 137 | 6 | Round-trip, plain passthrough, missing-key, bad-key, URL-safe base64, whitespace tolerance |
| `lcp_test.go` | 66 | 1 | ExtractAutoGroupName — 4 кейса |
| `ping_lan_bind_test.go` | 43 | 2 | isEngineTunIPv4, looksLikeTunnelInterface |
| `sysproxy_windows_test.go` | 30 | 1 | buildBypassList не использует broad wildcards |
| `sysproxy_linux_test.go` | 40 | 3 | gsettings list formatting + backends register |
| `sysproxy_darwin_test.go` | 29 | 2 | bypass defaults + whitelist expansion |
| `build_config_testhelpers_test.go` | 36 | — | helper'ы `mustBuildTunnelModeConfig`/`mustBuildProxyModeConfig` |

### Сильные стороны coverage
- Connect-flow проверен через **stub Engine**, не реальный sing-box.
- Real end-to-end (`udp_quic_smoke_test`, `awg2_e2e_test`) использует **реальный sing-box option-decoder**.
  Это гарантирует, что сгенерированные конфиги совместимы с upstream форком.
- Router algorithm покрыт (single/double/triple match).
- DNS-DPI mitigation (proxy-mode UDP DNS) явно проверяется (`TestBuildProxyModeConfig_CustomDNSDirectUDP`).
- Subscription-decrypt cover edge-cases (whitespace, URL-safe, missing key).
- Manager-mode tests use both stubs (engine/sysProxy) AND real-network helpers (`startReachableTCP`).

### Пробелы / TODO для покрытия
- **Нет тестов для `ApplyAppWhitelist`** (hot-reload приложений). Stub engine содержит `applyCalls`,
  но не используется ни в одном тесте.
- **Нет тестов для health watchdog'а** (proxyDead flag, kill-switch firewall). Это критичная часть
  безопасности.
- **Нет тестов для concurrent Connect → Disconnect → Connect** — есть `TestConnect_FailedSwitchClearsCurrentProxyInStatus`,
  но без явных race-сценариев.
- Нет integration-теста реального запуска `SingBoxEngine.Start` (только через builders).
- Нет тестов для `CancelConnect()` mid-flight.
- Нет тестов для `procTracker.OnChange` callback.
- Country API нет race-теста при одновременных lookup'ах.

---

## 13. Потоки данных (user-flows)

### 13.1. Импорт подписки
```
UI: paste URL/text
   ↓ Wails: a.ImportSubscription(text)
   ↓ app.go:1486 IsEncryptedSubscription → DecryptSubscription (subscription_decrypt.go)
   ↓ app.go:1596 ParseSubscriptionBody (uriparser.go:90)
   ↓   tryDecryptSubscription → normalizeSubscriptionBody → parseSubscriptionJSON OR parseSubscriptionLines
   ↓   base64Decode fallback
   ↓ app.go:1663 FinalizeSubscriptionEntries (uriparser.go:1714) — placeholder → SECTION
   ↓ app.go:1667 SplitAutoEntries — выделяет auto-группу
   ↓ Wails: event "subscription:imported" + persist в config
   ↓ UI: render list
```

### 13.2. Connect
```
UI: click "Connect"
   ↓ Wails: a.Connect(proxyDTO, rules, killSwitch, adBlock)
   ↓ app.go:455 → m.Connect(...) (manager.go:326)
   ↓ ── Phase 1 (m.mu held) ──
   ↓ checks (SECTION? WG+proxy? Admin?)
   ↓ TCP pre-ping (не для UDP/Hysteria2)
   ↓ effectiveAppWhitelist = pre-scan process tree
   ↓ validateEngineConfig
   ↓ ── m.mu released ──
   ↓ ── Phase 2 (60-sec timeout) ──
   ↓ engine.Start (app ctx) → singbox.go:224
   ↓   bootLocked → BuildProxyMode|TunnelModeConfig → marshal → write tmp → box.New → instance.Start
   ↓ runPostStartProbe (per-protocol retry strategy)
   ↓ ── m.mu held again ──
   ↓ ── Phase 3 ──
   ↓ re-check engine.IsRunning()
   ↓ sysProxy.Set (для proxy-mode) ИЛИ sysProxy.Disable (для AMNEZIAWG tunnel)
   ↓ commit state, startProcessTracker, startHealthWatchdog
   ↓ emitStatus → wailsRuntime event "status:update"
   ↓ return ConnectResultDTO
```

### 13.3. Ping all (для UI листа)
```
UI: click "Ping All" / автоматический
   ↓ Wails: app.go:2171 цикл a.proxy.Ping(p.IP, p.Port, p.Type) (manager.go:1331)
   ↓ per proxy:
     ↓ if connected && active && UDP-протокол → "session_active", latency=-1
     ↓ if Hysteria2 → pingHysteria2Probe (UDP, TCP fallback)
     ↓ if WireGuard → pingTCPProbe (TCP)
     ↓ if connected && tunnel → pingLANProbe (LAN-bind)
     ↓ else → pingTCPProbe
   ↓ result: PingResultDTO{Reachable, LatencyMs, CheckType}
```

### 13.4. Deep-link `resultv://`
```
OS: открывает URL resultv://import/<base64>
   ↓ Single-instance bridge передаёт URL
   ↓ app.go:177 → IsDeepLink → DecodeDeepLink (deeplink.go:53)
   ↓   trim/sanitize → strip prefix (import/, rvsub/, crypt4/, …)
   ↓   sanitizeBase64 → tryDecryptSubscription
   ↓ Возвращает plaintext (URL or per-line URIs)
   ↓ app.go: тот же pipeline что и ImportSubscription
   ↓ Если DeepLinkUsesRvsubPath → метка "из RVSUB"
```

---

## 14. Найденные проблемы и предложения

### High priority

#### H1. `manager.go` — God-object на 1593 строки
`Manager` совмещает 10+ ответственностей: state, lifecycle, ping, health watchdog, process-tree,
status-events, validation, system-proxy орхестрация. Это:
- Тяжело тестировать (нужны 4-5 stub-объектов на каждый тест).
- Высокий риск coupling-багов: например, `connectLocked` (manager.go:592) — почти полностью копия
  `Connect`, что неминуемо приведёт к divergence (один путь починили, другой — нет).

**Предложение**: вынести в подмодули
- `manager_connect.go` (Connect, connectLocked, runPostStartProbe)
- `manager_health.go` (health watchdog, proxyDead)
- `manager_ping.go`
- `manager_state.go` (state struct + lock-mediation helpers)
- `manager_processtree.go`

Из `Connect`/`connectLocked` выделить **общий** `prepareEngineCfg + startEngineAndProbe`, чтобы
не дублировать сборку EngineConfig (~50 строк в каждой функции).

#### H2. `uriparser.go` — 1794 строки, 8 парсеров в одном файле
Каждый протокол имеет свой парсер, JSON-парсер, helpers. **Очень удобно** иметь всё в одном файле
для grep, но за 2000 строк трудно ориентироваться.

**Предложение**: разбить по протоколам:
- `uriparser_vless.go`, `uriparser_vmess.go`, `uriparser_trojan.go`, …
- `uriparser_subscription.go` (parseSubscriptionBody + JSON-парсеры)
- `uriparser_naming.go` (StripLeadingFlagEmoji, ExtractAutoGroupName, …)
- `uriparser_helpers.go` (base64, splitCSV, asMap, asSlice, …)

#### H3. `Router.IsBlockedDomain` использует substring match
router.go:172-178:
```go
h := strings.ToLower(hostname)
for _, d := range r.blockedDomains {
    if strings.Contains(h, d) {
        return true
    }
}
```

Это даёт **false positives**:
- `r.IsBlockedDomain("notdiscord.example.com")` → true (содержит "discord.com")
- `r.IsBlockedDomain("instagram.evil.com")` → true (содержит "instagram.com")

Тест `TestIsBlockedDomain` (router_test.go:178) проверяет именно substring-match как
**ожидаемое поведение**, но это семантически неверно: должен быть exact/suffix match.

**Предложение**: исправить на:
```go
if h == d || strings.HasSuffix(h, "."+d) {
    return true
}
```
И поправить тест.

#### H4. Race в `m.engine.IsRunning()` без mu (Manager.Ping)
`manager.go:1359`:
```go
if m.engine != nil && m.engine.IsRunning() {
```
`m.engine.IsRunning()` атомарен (atomic.Bool), это ОК, но **перед этим** `m.mu.Unlock()` отпускает
mutex, а потом мы читаем `m.engine` (само поле). Гипотетически, если кто-то заменит `m.engine` в
рантайме — race. Но `engine` пишется только в `NewManager` (manager.go:160) и в тестах. На
практике не проблема, но `go race detector` подсветит.

#### H5. `subscriptionEncryptKey` инъектируется через `-ldflags`, документация честная, но
любое расширение должно учитывать, что **ключ один на всех пользователей**. Если когда-либо
появится **per-user secret** (личные подписки конкретного пользователя у конкретного провайдера),
текущая схема **не подходит**.

Стоит в коде явно держать TODO/issue для RVSUB2 (asymmetric or per-provider key).

#### H6. TLS-defaults: `Insecure: getBoolField(extra, "insecure")` без аудита
В `applyTLSAndTransport` (outbound.go:441-477), значение `insecure` берётся из user-supplied URI/JSON.
Это правильно (если юзер хочет тест-сервер с self-signed, ему надо), но:
- В **HYSTERIA2** (outbound.go:200) `insecure` берётся из extra напрямую, без любого предупреждения
  или валидации.
- **NaiveProxy** — единственный, который **запрещает** insecure (config_validation.go:130).
- Нет logging/warning при insecure=true, что **должно быть** для прозрачности.

**Предложение**: при `Insecure=true` логировать `Warning` в app.log: "TLS verification disabled
for proxy X.Y.Z — connection is unauthenticated".

#### H7. sing-box версия — захардкожен форк, нет ясной политики апдейтов
Импорт `github.com/sagernet/sing-box` (`singbox.go:31`) на самом деле резолвится в форк
`shtorm-7/sing-box-extended` (через `replace` в go.mod, проверь). Если upstream sing-box добавит
API-breaking changes (например, `box.New(box.Options)` сигнатуру), форк должен будет mergerить;
это технический долг.

В коде есть комментарии вроде "upstream sing-box-extended >= v1.13.11-extended-2.0.0" (engine.go:220),
но нет автоматической проверки версии. Если форк обновится без AWG 2.0 — приложение сломается без
явной диагностики.

**Предложение**: при старте приложения логировать sing-box version от форка.

#### H8. `tunnelProbeDomains` форсятся через прокси (нужны для probe), но также эмитятся
в DNS (engine.go:613-619). Если эти domains добавлены пользователем в whitelist (`.gstatic.com`),
они **всё равно идут через прокси** (правило выше в списке). Это правильно для health-check, но
плохо документировано в UI — пользователь может не понять, почему его `*.gstatic.com` whitelist не
работает в первые 2 секунды.

### Medium priority

#### M1. `proxy.go` 5-секундный timeout в shutdownInstance
`singbox.go:355-358`:
```go
case <-time.After(5 * time.Second):
    e.log.Warning("[SING-BOX] Close() timeout — принудительное завершение")
```
В случае таймаута мы **только warning logging**, но горутина `go inst.Close()` остаётся жить и
держит память. Это goroutine leak.

**Предложение**: либо использовать `runtime.Goexit` (нельзя — горутина не управляется), либо
добавить explicit `cancel` на этой goroutine, либо хотя бы документировать "accepted leak".

#### M2. `Connect` фаза 1 → 2 → 3: re-check `IsRunning` есть, но **нет re-check `m.connected`**
`manager.go:529`:
```go
if !m.engine.IsRunning() {
    return ConnectResultDTO{...cancelled}
}
```
Между Phase 2 и Phase 3 другой Connect мог уже подключиться к **другому** прокси. Технически это
невозможно (sequencing через `m.mu`), но в теории есть гонка с `SetMode`/`ReconnectWithRoutingRules`,
которые тоже могут запускать engine.

Решение: добавить generation counter в Manager, который инкрементируется на каждом Connect/Disconnect.
После Phase 2 сравнить generation: если поменялся — bail.

#### M3. `engine.go` `BuildTunnelModeConfig`: `tunStack="system"` захардкожен
engine.go:374. Раньше для WG/AWG использовался system, для других — gvisor. Сейчас всё унифицировано
на system из-за anti-cheat. Но это **снижает гибкость** для тестов / debug.

**Предложение**: добавить env-override `RESULTPROXY_TUN_STACK=gvisor` для debug-сценариев.

#### M4. Logger в `Manager` может быть nil
manager.go:156 — `NewManager` принимает `*logger.Logger`. Если передан `nil`, многие методы упадут
(например, `m.log.Info(...)`). Стоит сделать `NewManager` принимать `Logger interface` или
проверять `nil`.

#### M5. Ошибки `json.Marshal` в URI парсерах игнорируются
В uriparser.go много мест с `extraJSON, _ := json.Marshal(extra)`. На практике ошибка маловероятна
(`map[string]interface{}` с базовыми типами), но если в `extra` попадёт `func` или цикличный
объект — silent corruption. Стоит хотя бы `if err != nil { ... }`.

#### M6. `defaultBlockedDomains` — captured at startup, не reload-aware
router.go:271 захардкожен список из 7 доменов. Если в будущем нужно их обновить — нужен ребилд.
Можно вынести в файл `internal/assets/default-blocked.txt` и читать на старте.

#### M7. `PingProxyUDP` (engine.go:756) использует `latency=-1` для "reachable но timeout"
Это работает как сигнал в UI ("серый ping"), но не отражено в типе `PingResultDTO.LatencyMs int64`
(нет nullability). Если позже добавят логику "latency > 0", то -1 сломает её.

**Предложение**: добавить отдельное поле `LatencyKnown bool` или вернуть `int64 + bool, ok`.

#### M8. `manager.go:1576 emitStatus` — без локального копирования slice
`emitStatus` берёт ссылки на `m.proxy` (`*ProxyConfig`), `m.connected`, `m.killSwitch`. Если frontend
получит status и потом начнёт читать `m.proxy` (через JSON), это будет копия по value, ОК. Но в
`StatusDTO.CurrentProxy: m.proxy` (manager.go:1587) **передаётся указатель**, и Wails сериализует
его, что safe (он сделает копию).

Однако если кто-то добавит slice-поле в `ProxyConfig` (например, `Tags []string`), то будет race.

### Low priority

#### L1. Дублирование `sysproxy_*.go` структурно
Каждый файл реализует один SystemProxy. Можно вынести `interface` + base struct, но это будет
overhead — текущая реализация ОК.

#### L2. `appWhitelistEqual` (singbox.go:406) — O(n) с map
Маленькое количество элементов (10-20), OK.

#### L3. `pingReasonFromError` (engine.go:786) — большой `switch` по substring
Хрупкое решение: зависит от error-strings из stdlib (которые могут поменяться). Лучше использовать
`errors.Is` для специфичных типов (DNS, ECONNREFUSED, etc.).

#### L4. `parseExtra` (outbound.go:26) игнорирует ошибки `json.Unmarshal`
Если `proxy.Extra` — невалидный JSON, возвращается пустая map. Хорошо для робастности, плохо для
диагностики.

#### L5. `engine.go:tunIPv4 default "172.19.0.1/30"` захардкожен
Прерывает разработку, если кто-то использует эту сеть локально. Но это редкий edge-case.

#### L6. Отсутствует rate-limit на Ping
`Ping` (manager.go:1331) можно вызвать тысячи раз в секунду из frontend. На реальном TCP/UDP dial
это даст DOS на proxy-сервер. UI должен throttle, но **defensive coding** в `Manager` тоже не помешает.

#### L7. `internal/proxy/blocked_provider.go:80` — нет timeout в `client().Do(req)` помимо http.Client.Timeout
Это OK на 10с (default), но если есть `ctx.Done()`, надо учесть. Реально учитывается через
`http.NewRequestWithContext`.

#### L8. `tryDecryptSubscription` — `keyBytes != 32`: ошибка "must be 32 bytes". При build-error
ключа не хватит. Хорошо бы checksum/test-vector в init().

---

## Сводная таблица проблем (приоритизация)

| ID | Уровень | Файл | Проблема | Effort |
|---|---|---|---|---|
| H1 | High | manager.go | God-object 1593 строк | High |
| H2 | High | uriparser.go | 1794 строки, 8 парсеров в одном файле | Medium |
| H3 | High | router.go:172 | substring match в IsBlockedDomain → false positives | Low |
| H4 | High | manager.go:1359 | Race на чтении m.engine без mu (formal only) | Low |
| H5 | High | subscription_decrypt.go | shared key, нужен RVSUB2 для приватных подписок | High |
| H6 | High | outbound.go | TLS Insecure без warning-логирования | Low |
| H7 | High | go.mod | sing-box форк-версия без runtime-проверки | Low |
| H8 | High | engine.go:613 | tunnelProbeDomains форсят proxy выше whitelist — UX issue | Low |
| M1 | Med | singbox.go:355 | 5-sec close timeout → goroutine leak | Low |
| M2 | Med | manager.go:529 | re-check IsRunning, но не generation | Med |
| M3 | Med | engine.go:374 | tunStack hardcoded "system" | Low |
| M4 | Med | manager.go:156 | nil-logger crashes | Low |
| M5 | Med | uriparser.go | json.Marshal errors silenced | Low |
| M6 | Med | router.go:271 | defaultBlockedDomains hardcoded | Low |
| M7 | Med | engine.go:756 | latency=-1 magic value | Low |
| M8 | Med | manager.go:1576 | emitStatus с указателями (defensive) | Low |
| L1-L8 | Low | различные | мелкие | Low |

---

## Что хорошо

- **Архитектура lifecycle** в Manager (Phase 1/2/3 + раздельный mutex для cancel) — корректная,
  с осмысленным re-check после blocking операций.
- **DNS-leak fix** в proxy mode (engine.go:500-510) — явно прокомментирован и протестирован.
- **DPI-bypass конфигурации** (uTLS fingerprint, Reality auto-detect, xhttp h2 ALPN forcing, AWG 2.0 H-range) —
  серьёзная инженерия с учётом реальной телеметрии.
- **Country-API privacy fix** (country.go:34-40) — конкретный приоритет приватности, ясно
  документирован "почему сейчас единственный endpoint, а не fallback chain".
- **Тестовое покрытие** — особенно E2E через реальный sing-box option-decoder
  (`udp_quic_smoke_test`, `awg2_e2e_test`, `singbox_protocols_test`).
- **process-tree без auto-reload** (manager.go:227-238) — правильное архитектурное решение.
- **Subscription decrypt** — честно задокументированные ограничения.
- **Per-protocol post-start probe** — детальная и протестированная.
- **Anti-cheat compatibility** (engine.go:370 system stack vs gvisor) — реальная проблема,
  документированное решение.

---
