# Kill Switch — устранение ложных срабатываний (пин IP сервера)

## Симптомы (от пользователя)

- Kill switch «случайным образом» выскакивает при рабочем сервере.
- Никакие правила фаервола при этом не применяются — просто на секунду режет
  соединение.
- После ложного срабатывания **замирают логи**: новые `[CONN]` не появляются,
  хотя ресурсы грузятся.
- Настоящее срабатывание (реальное падение сервера) работает корректно.

## Первопричина — ре-резолв домена сервера в рантайме

Предыдущие правки меняли **механизм пробы** watchdog'а, но не первопричину. Лог
(`resultv-logs-2026-05-30`) показал её однозначно. Оба «срабатывания» начинались
с серии ошибок sing-box:

```
ERROR connection: open connection to ... using outbound/hysteria2[proxy]:
      lookup k.sunsetglow.today: context deadline exceeded
ERROR connection: ... lookup k.sunsetglow.today: dial udp [::1]:53: i/o timeout
[KILL SWITCH] VPN-сервер k.sunsetglow.today:3443 недоступен
[KILL SWITCH] Не удалось включить фаервол: kill switch: no proxy IP to allow
```

### Цепочка отказа

1. Адрес сервера — **домен**. В tunnel-режиме (`buildDNS`) его резолвинг прибит
   к DNS-серверу `local` (системный резолвер ОС).
2. Приложение само же перенаправляет системный DNS для защиты от утечек
   (`applySystemDNSOverride`). В TUN + strict_route этот `local`-резолвер
   становится хрупким и периодически отваливается (`dial udp [::1]:53: i/o
   timeout`).
3. sing-box ре-резолвит домен сервера для **каждого нового** коннекта.
   Транзиентный сбой резолва → все новые коннекты падают. Существующая
   QUIC-сессия ещё жива (поэтому «сервер рабочий»), но `probeTunnelHealth` ходит
   тем же путём и тоже падает → 2 промаха → kill switch.
4. `resolveProxyIPs` в kill switch резолвит домен **в тот же момент, когда DNS
   уже лёг** → пусто → `no proxy IP to allow` → фаервол не встаёт. «Выскакивает,
   но ничего не блокирует».
5. «Замирание логов» — симптом: новые коннекты не доходят до роутинга sing-box,
   поэтому `[CONN]` не пишется, идёт только сырой stderr. Восстанавливается само,
   когда резолв оживает.

## Исправление — пин IP сервера один раз при подключении

`ProxyConfig.ResolvedIP` резолвится один раз в `Connect`/`connectLocked` (пока
DNS ОС ещё работает, **до** `applySystemDNSOverride`) и используется дальше:

- **`buildProxyOutbound`** — `Server` = pinned IP, `server_name`/SNI остаётся
  доменом → sing-box больше не ре-резолвит сервер → нет storm'а
  `lookup ... context deadline exceeded` → нет ложных трипов и замирания логов.
- **`buildRoute` (`routeExclude`)** — CIDR сервера теперь добавляется и для
  доменных адресов (раньше — только для литеральных IP).
- **`KillSwitchFirewallEngage`** — фаервол получает готовый IP → реально встаёт
  при настоящем падении даже с доменным сервером.

Fallback: если резолв при подключении не удался, `ResolvedIP` пуст и поведение
остаётся прежним (домен в `Server`, правило `local`) — без регрессии.

> Ограничение: пин применяется к протоколам через `buildProxyOutbound`
> (hysteria2 / VLESS / VMess / SS / Trojan / SOCKS). Эндпоинты
> WireGuard/AmneziaWG (`buildEndpoints`) пока резолвятся как раньше; для них kill
> switch всё равно получает pinned IP.

## Скрытие адреса сервера в логах для подписок

Для серверов из подписки (`SubscriptionURL != ""`) адрес backend'а больше не
светится в логах:

- `newSingBoxLogWriter` редактирует домен и resolved IP в сырых ошибках sing-box
  → `<сервер>`.
- Сообщения watchdog'а (`[KILL SWITCH] VPN-сервер … недоступен`) для подписок
  пишутся без `host:port`.
- Фронтенд (`useDaemonStatus.js`) скрывает IP в «Узел … перестал отвечать» для
  подписочных серверов.

Ручные серверы (без подписки) сохраняют полную детализацию — пользователь сам их
вводил и видит адрес в UI.

## Поведение до и после

| Сценарий | До | После |
|---|---|---|
| Туннель, домен-сервер, транзиентный сбой резолва | Ложный kill switch, фаервол не встаёт, логи замирают | Нет ре-резолва → нет трипа, логи живут |
| Туннель, домен-сервер реально упал | Попап, но `no proxy IP to allow` — фаервол не блокирует | Фаервол реально блокирует (pinned IP) |
| Подписочный сервер в логах | Виден домен/IP backend'а | `<сервер>` / без адреса |
| Ручной сервер | Полная детализация | Полная детализация (без изменений) |

## Тесты

`internal/proxy/server_pin_test.go`:

- `TestBuildProxyOutbound_PinnedResolvedIPKeepsDomainSNI` — Server=IP, SNI=домен
- `TestBuildProxyOutbound_NoPinKeepsRawServer` — без пина поведение прежнее
- `TestResolvePinnedServerIP_LiteralIPReturnsEmpty` — литеральный IP не пинуется
- `TestSingBoxLogWriter_RedactsSubscriptionServer` — редакция для подписок
- `TestSingBoxLogWriter_ManualServerNotRedacted` — ручной сервер не редактируется
