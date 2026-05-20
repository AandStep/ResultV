# Code Review 03 — `internal/system` + `internal/getlantern_systray`

Зона: системная интеграция ResultV (Wails v2 + sing-box). Платформы: Windows / Linux / macOS, с `_other`/`_stub`-fallback'ами.

---

## 1. Обзор

`internal/system` — это "тонкий слой ОС" между Wails-приложением и подсистемами хоста. Он отвечает за всё, что выходит за пределы proxy-движка и UI:

- **Безопасность трафика** (kill switch, app whitelist) — гарантирует, что при потере VPN-узла никакая утечка не уйдёт мимо туннеля.
- **Жизненный цикл процесса** (single-instance мьютекс через named class window, autostart, deep-link `resultv://`).
- **UI-в-фоне** (tray + click dispatcher + меню-модель, taskbar restore hook на Windows).
- **Диагностика среды** (WebView2 версия, network monitor, network traffic stats).
- **Привилегии** (`IsAdmin`, `RestartAsAdmin`, `runPrivileged`).
- **Локальное хранение** (`UserDataDir`, `WebviewUserDataPath`, миграция со старого `ResultProxy` → `ResultV`).

`internal/system/processtree` — вспомогательный пакет, который раз в 500ms сканирует таблицу процессов и эмитит подмножество "потомков" пользовательских "корней" (например, `steam.exe` → `steamwebhelper.exe`) для маршрутизатора sing-box.

`internal/getlantern_systray` — форк `github.com/getlantern/systray` v0.0.x под GPL-3.0 (с пометкой "Originally Apache-2.0, modifications GPL-3.0"). Используется только из `tray.go`.

### Поставщики функциональности

| Что | Кто отдаёт | Кто использует |
|---|---|---|
| Kill switch (firewall) | `killswitch_*.go` | `app.go:255+267+722`, `proxy.Manager.KillSwitchFirewallEngage` |
| Autostart | `autostart_*.go` | `app.go:768` (`SetAutostart`) |
| Tray UI | `tray.go`+`tray_model.go`+`tray_click_dispatcher.go` | `app.go:298` (callbacks: ShowWindow/Disconnect/Quit/SelectProxy) |
| Singleton + deep-link IPC | `instance_messenger_windows.go` (Windows-only) | `main.go`/`app.go` до Wails init |
| Deep-link реестрация | `deeplink_register_*.go` | startup hooks |
| Path resolution | `paths*.go` + `legacy_migration.go` | конфиг, webview cache |
| WebView fingerprint | `webview2_*.go` | sing-box uTLS preset |
| Net monitor | `netmon.go` | `app.go:288` (events: `network:status`) |
| App whitelist normalization | `appwhitelist*.go` | роутинг movement в `internal/proxy` |
| Process tree | `processtree/*.go` | `proxy.Manager` (auto-добавление дочерних exe в exclude-список) |
| Taskbar hook | `taskbar_restore_hook_windows.go` | восстановление окна при клике по taskbar-иконке после Explorer.exe restart |

---

## 2. Kill switch — критическая фича безопасности

### Общий контракт

`killswitch.go:24-42` — интерфейс `KillSwitch{Enable, Disable, IsEnabled}` + общие хелперы:

- `resolveProxyIPs(addr)` (`killswitch.go:57`) — резолвит host:port, возвращает **все** A/AAAA-записи (CDN-fronted прокси). Использует `net.DefaultResolver` БЕЗ контекстного таймаута (комментарий упоминает 2s, но `context.Background()` фактически передаётся). Если не нашли — возвращает `nil` → `Enable()` фейлит с `"no proxy IP to allow"`.
- `extractDNSIPs(servers)` (`killswitch.go:95`) — нормализация DNS-серверов: парсит `host:port`, отбрасывает hostnames (DNS не работает при отсутствии правил), дедуплицирует, **fallback на `1.1.1.1` + `8.8.8.8`** если ничего валидного нет.
- `fallbackDNS = []string{"1.1.1.1","8.8.8.8"}` (`killswitch.go:48`) — security trade-off: уходит к Cloudflare/Google вместо ISP, но не к ANY destination как раньше.

Инвариант: **DNS-запросы пускаются только на конкретные IP-резолверы**; "udp/53 → any" удалён во всех бэкендах — это была классическая DNS-leak (тесты в `killswitch_linux_test.go:55-76` и `killswitch_darwin_test.go:44-58` — регрессионные).

### Платформенные реализации

| Аспект | Windows | Linux (nft) | Linux (iptables) | macOS |
|---|---|---|---|---|
| Файл | `killswitch_windows.go:235` | `killswitch_linux.go:117-196` | `killswitch_linux.go:200-257` | `killswitch_darwin.go:125` |
| Технология | `netsh advfirewall firewall add rule` | `nft -f /tmp/resultv-killswitch.nft` | `iptables -N RESULTV_KILLSWITCH` | `pfctl -E -f /tmp/resultv-killswitch.conf` |
| Имя правила/таблицы | `ResultV_KillSwitch_*` | table `inet resultv_killswitch` | chain `RESULTV_KILLSWITCH` | весь активный pf-ruleset заменяется |
| Требуется root | **да** (`IsAdmin` проверяется в Enable) | да (через `runPrivileged`) | да | да |
| IPv6 | от `netsh advfirewall` (protocol=any) | да (раздельные `ip6 daddr`) | **нет** (skip v6 silently) | через `pf` (но правила содержат только v4-литералы) |
| Conntrack | нет | да (`ct state established,related accept`) | да (`-m conntrack --ctstate ESTABLISHED,RELATED`) | нет (pf по умолчанию не stateful в этом конфиге, нет `keep state`) |
| Default policy | block out all + allow exceptions | `policy drop` в `chain output` | DROP в кастомной chain, `-I OUTPUT -j chain` | `block out all` + `pass out from any to <ip>` |
| LAN allow | 127/8, 10/8, 172.16/12, 192.168/16 | + 169.254/16, IPv6: ::1, fe80::/10, fc00::/7 | + 169.254/16 (только v4) | + 169.254/16 |
| Disable cleanup | удаляет 1+`max(stored,8)` префиксированных правил | `nft delete table inet ...` | `-D OUTPUT … ; -F chain ; -X chain` | `pfctl -d` (выключает pf полностью) |

### Сценарий активации

В `proxy.Manager` (`internal/proxy/manager.go:1461-1511`) есть health-watchdog:

1. При connect callback `KillSwitchFirewallEngage` записывается из `app.go:267`.
2. Ticker раз в N секунд пингует прокси (TCP/UDP/Hysteria2). После `failuresBeforeDead` подряд провалов и при включённой опции `m.killSwitch` — engageFn вызывается, передаёт `(ProxyConfig, dnsServers[])` в `app.enableKillSwitchFirewall`.
3. Когда узел восстановился — `KillSwitchFirewallDisengage` → `killSwitch.Disable()`. Tray возвращается в "подключено".
4. На Disconnect/Shutdown — `a.killSwitch.Disable()` (`app.go:396`, `app.go:567-568`).

**Важно:** `ToggleKillSwitch(true)` (`app.go:732`) **не** активирует firewall сразу — он только сохраняет флажок `m.killSwitch=true` и ждёт, пока watchdog объявит узел мёртвым. То есть в UI "kill switch включён" ≠ "firewall активен"; правила появляются только при реальной потере прокси-узла.

Дополнительный sinkhole-механизм системного proxy: `ApplyKillSwitch()` в `sysproxy_*.go` ставит системный прокси на `127.0.0.1:65535` (`sysproxy_darwin.go:31`, `sysproxy_linux.go:32`, `sysproxy_windows.go:106`) — это работает в proxy-mode и режет браузерный/HTTP-трафик ещё до того, как доходит до firewall'a.

### Инварианты безопасности

1. **Резолв прокси-имени до арминга** (`killswitch.go:55` коммент): иначе сам firewall заблокирует DNS-запрос к hostname прокси. Реализовано.
2. **DNS scoped to specific IPs**: `udp/53 → any` нигде нет — подтверждено тестами `killswitch_linux_test.go:55`, `killswitch_darwin_test.go:44`.
3. **На Windows есть legacy-cleanup**: `disableFirewall` (`killswitch_windows.go:237-271`) удаляет старое имя `_AllowDNS` (без индекса/прото) и переменное количество индексированных правил — для обновлений со старой схемы. Sweep up to `max(stored,8)`.
4. **Best-effort при crash**: `app.go:263` при старте безусловно вызывает `killSwitch.Disable()`, чтобы убрать "наследованные" netsh-правила (если процесс был убит kill -9). Это критично — иначе пользователь увидит "нет интернета" сразу после краха.
5. **Mutex** в `WindowsKillSwitch` + `LinuxKillSwitch` + `DarwinKillSwitch`: одновременный Enable/Disable невозможен.

### Найденные проблемы (kill switch — секция 17)

См. сводный список ниже.

---

## 3. Autostart per OS

| Аспект | Windows | Linux | macOS | other |
|---|---|---|---|---|
| Файл | `autostart_windows.go:122` | `autostart_linux.go:98` | `autostart_darwin.go:99` | `autostart_other.go:29` |
| Механизм | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` value `ResultV` | `$XDG_CONFIG_HOME/autostart/resultv.desktop` | `~/Library/LaunchAgents/com.resultv.autostart.plist` + `launchctl load -w` | error |
| Команда | `"<exe>" --autostart [args...]` | `Exec=<quoted exe> --autostart` (XDG-spec quoting) | ProgramArguments массив, XML-escape через `html.EscapeString` | — |
| UAC/sudo | **не требует**, пишет в HKCU | не требует | не требует | — |
| Очистка legacy | `schtasks /delete /tn ResultVAutostart` + `ResultProxyAutostart` + старое значение реестра `ResultProxy` (`autostart_windows.go:52-57`, `:96`, `:109`) | нет (имя файла стабильно `resultv.desktop`) | нет | — |
| `--autostart` token | `autostartTrayToken` const повторён в каждом файле (см. дубликат ниже) | то же | то же | — |
| `SetRunAsAdminFlag` | пишет в `HKCU\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers` (`autostart_windows.go:124`) — флаг `~ RUNASADMIN` | no-op | no-op | no-op |

### Edge-cases

- **Windows + UAC**: `SetRunAsAdminFlag` помечает exe как "всегда запускать с правами админа" — это включает UAC-промпт при каждом старте. Используется когда пользователь хочет Tunnel-mode + autostart (без админа Tunnel mode не работает; см. `killswitch_windows.go:90`).
- **Linux + Wayland**: `.desktop` autostart работает; но если пользователь запустит из tmux/ssh — переменные `DISPLAY`/`XAUTHORITY` могут отличаться.
- **macOS + Gatekeeper**: `launchctl load -w` может молча провалиться на первый запуск из downloaded-app; код `autostart_darwin.go:79-83` обрабатывает это — файл уже на диске, на следующий логин система сама подхватит.
- Команда `RestartAsAdmin` (`system_unix.go:52`) на macOS использует `osascript`, на Linux — `pkexec` с прокидыванием `DISPLAY`/`XAUTHORITY`/`WAYLAND_DISPLAY`/`XDG_RUNTIME_DIR` (`system_unix.go:96`).

---

## 4. Tray (`tray.go:761` + `tray_model.go:275` + `tray_click_dispatcher.go:114`)

### Структура

`Tray` (`tray.go:51`) — состояние:

- меню-айтемы: `mStatus, mShow, mDisconnect, mServers, mQuit` + динамические (страны/серверы)
- maps: `proxyLookup`, `proxyPings`, `serverItems`, `countryIcons`, `serverTitleCache`
- `clickDispatcher *trayClickDispatcher` — отдельная горутина, читающая клик-каналы из меню-айтемов через `reflect.Select`
- `pendingProxies/pendingSelectedID` — буфер для UpdateProxyList до `onReady`
- HTTP-клиент с тайм-аутом 3s + concurrency limit 4 для скачивания флагов из `flagcdn.com`/`flagpedia.net`

### Поток данных

```
app.go:UpdateProxyList(...)
  └─> Tray.UpdateProxyList(proxies, selectedID)  [mu.Lock]
       ├─ если !running → буферизуется в pendingProxies; выйдет
       ├─ signature unchanged → лишь refreshServerTitlesLocked() (обновляет ping/чек)
       └─ signature changed → prefetchCountryIcons(параллельно) → rebuildServersMenuLocked
                                                                    ├─ скрывает прошлые dynamic items
                                                                    ├─ BuildTrayMenuGroups (model)
                                                                    └─ заново строит provider → country → server
                                                                       и обновляет clickDispatcher
```

### Модель меню (`tray_model.go:51`)

`BuildTrayMenuGroups`:

1. Группирует прокси по `Provider` (пустой → `"Мои прокси"`).
2. Внутри провайдера группирует по `Country` (через `normalizeCountry`, пустой/`unknown` → `Unknown`).
3. Сортировка: алфавитно, но `defaultProviderName` всегда внизу, `defaultCountryName` (`Unknown`) тоже последний.
4. `perCountryLimit` (по умолчанию 20) — ограничивает число серверов в стране; остаток считается через `HiddenCount` и показывается как `"... еще N серверов"`.

Локализация: `countryNamesRU` — табличка ISO2 → русское название (24 страны: `US, GB, DE, FR, NL, FI, SE, NO, CH, IT, ES, PL, CZ, AT, CA, JP, SG, HK, KR, TR, AE, RU, UA, KZ`). Без России это самое непредставительное место в проекте: 24 страны хардкодом, новые добавляются вручную.

Флаг (emoji `🇩🇪`): `countryToFlag` собирает из Regional Indicator Symbols (`0x1F1E6` + (буква − 'A')). PNG-иконка качается отдельно (см. ниже).

`formatServerTitle`: `"✅ Berlin [73ms]"` или `"  Berlin [...]"` (отступ 2 пробела если не подключён). `sanitizeServerDisplayName` обрезает префикс типа `"kz Kazakhstan"` если имя начинается с ISO-кода страны (тест `tray_model_test.go:106`).

### Click dispatcher (`tray_click_dispatcher.go`)

Проблема: getlantern_systray делает каждому `MenuItem` свой `ClickedCh`. Если меню перестраивается (новый список серверов), старые айтемы остаются в памяти с открытыми каналами. Если просто запустить `go func(){<-ch; cb()}()` для каждого — на каждом rebuild количество "висящих" горутин растёт.

Решение: один диспетчер с `reflect.Select` — собирает `[]serverClickBinding{proxyID, ch}` и реселектится. На `update(newBindings)`:

1. Дренирует `updates` chan (`update()` сначала пытается non-blocking `select case d.updates <- bindings`, если не получилось — пытается стащить старое значение и положить новое).
2. Главная горутина в `start()` ловит сигнал через `chosen == 1` и перестраивает `cases`.

Регрессионный тест `tray_click_dispatcher_test.go:24` — отправляет на 63 "старых" канала и убеждается, что счётчик кликов = 0; затем кликает на текущий — счётчик = 1.

### Иконки в трее

- Основная (`SetIcon`): принимает `[]byte` (ICO), хранится в `t.icon` и применяется в `onReady`.
- Иконки стран: `prefetchCountryIcons` параллельно (sem из 4) скачивает PNG с `flagcdn.com`, конвертит в 16×16 ICO через `pngToICO` (`tray.go:639`). 64KB limit на ответ. Хранится в `t.countryIcons[iso]`. Fallback — `buildFallbackIcon` (квадратик 6x6 в сером 16x16).
- `pngToICO` руками собирает ICONDIR + ICONDIRENTRY + BITMAPINFOHEADER + 32-bit BGRA pixel array + AND mask. Тест `tray_icon_test.go:26` — что заголовок начинается с `00 00 01 00`.

### Состояние и потоки

- `Tray.mu` (sync.Mutex) защищает почти всё кроме `clickDispatcher`/HTTP.
- `onReady` идёт в горутине через `systray.Run`. Между `Start()` и `onReady` — окно "running == false" + pendingProxies буфер.
- `Stop()` тайм-аут 2s на `systray.Quit` (`tray.go:133`).
- `OnUnexpectedExit` — если `onExit` сработал без `stopRequested` (т.е. systray умер сам — обычно от сообщения сессии или ошибки message loop) → колбэк делает `os.Exit(0)` при скрытом окне (`app.go:321`). Без этого — zombie process.

---

## 5. Внешний systray (`internal/getlantern_systray`)

### Зачем форк

Upstream `github.com/getlantern/systray` v0.0.0 (имеет несколько багов и не принимает PR'ы регулярно). Локальный форк под GPL-3.0 (`go.mod` всё ещё указывает `module github.com/getlantern/systray` — но в `internal/getlantern_systray/go.mod` это означает только идентификатор импорта, реально код локальный).

### Что в форке кастомного

`systray_windows.go` 970 строк (упомянуто 813 — фактически 970):

1. **`SetWindowsTrayLeftClick`** (`systray.go:39`) — глобальная переменная-колбэк. Upstream поддерживает только правый клик (TrackPopupMenu), а ResultV нужен ЛКМ → show window (`tray.go:150`). Реализация в `systray_windows.go:298-305` в `wndProc/WM_LBUTTONUP`.
2. **Обработка `WM_TASKBARCREATED`** (`systray_windows.go:309-312`) — переинициализация иконки после рестарта `explorer.exe`. (Это есть в upstream, но в форке точно сохранено.)
3. **`menuOf` map** (`systray_windows.go:208-209`) — родительская меню-handle для каждого item, нужно для `convertToSubMenu` (вкл подменю динамически когда первый child добавлен).
4. **`menuItemIcons`** (`systray_windows.go:213-214`) — поддержка иконок на пунктах меню (`MIIM_BITMAP`); upstream Windows не поддерживал, но для флагов стран это критично.
5. **`iconToBitmap`** (`systray_windows.go:764`) — конвертация HICON → HBITMAP (DrawIconEx на compatible DC) для отображения иконки в пункте меню.
6. **`hideMenuItem`** через `RemoveMenu` + `delFromVisibleItems` (`systray_windows.go:645-664`) — на следующий `showMenuItem` пункт переинсёртится. Используется при rebuild списка серверов.

### Можно ли upstream'нуть

Технически да — `SetWindowsTrayLeftClick` и иконки на пунктах меню это публичные фичи. Но требуется большой PR + maintainer activity (низкая). Прагматично оставить форк.

### Линуксовая часть форка

- `systray_linux.c` (275 строк) — GTK/AppIndicator реализация (cgo). Не модифицировался.
- `systray_linux_appindicator.go` (build tag `legacy_appindicator`) — старый AppIndicator (Ubuntu).
- `systray_linux_ayatana.go` (default) — ayatana-appindicator3-0.1 (современный Fedora/Debian).
- `systray_linux.go` (24 строки) — stub без поддержки `SetIcon` на пункте меню. Это значит на **Linux флаги стран в меню не покажутся** (`tray.go:430-444` это не учитывает: вызовы `SetIcon` будут no-op).

### macOS часть

`systray_darwin.go` (110 строк) + `systray_darwin.m` (293 строки cgo Cocoa). Не модифицировано относительно upstream.

---

## 6. Netmon (`netmon.go:132`)

### Что делает

Раз в `interval` (default 5s) пытается `net.DialTimeout("tcp", host, 3s)` к одному из трёх хостов:
- `dns.google:443`
- `one.one.one.one:443`
- `208.67.222.222:443` (OpenDNS)

Первый успешный → online; все провалились → offline. Эмитит `NetworkStatus{Online, Latency, CheckedAt, Error}` через `handler` колбэк **только при изменении** `Online` (`netmon.go:134`).

### Реакция на смену интерфейса

**Прямого хука "интерфейс сменился" нет**. Если Wi-Fi выключился и Ethernet включился — реакция через **косвенное измерение connectivity-теста**:

1. Ticker сработал.
2. Все 3 хоста провалились (Wi-Fi умирает) → `online=false` → event.
3. Через ≤5s следующий tick после восстановления → `online=true` → event.

То есть **задержка детекции ~10 секунд в худшем случае**. На Windows есть `IPHelper.NotifyAddrChange` (event-driven), на Linux netlink `RTNLGRP_LINK`, на macOS SystemConfigurationFramework — но они **не используются**.

### Взаимодействие с kill switch

`netmon` не знает про kill switch и не зовёт его. Связь только через UI: фронт получает `network:status` events и обновляет индикатор. Kill switch активируется **только** через health-watchdog в `proxy.Manager` (по проба прокси, а не общей connectivity). Это разделение намеренное:

- Network down → kill switch НЕ активируется (всё равно нечего блокировать).
- Network up, proxy down → kill switch активируется (это leak-сценарий).

---

## 7. AppWhitelist per OS

### Назначение

В Tunnel/SplitVPN-режимах sing-box может **исключить** перечисленные приложения из туннеля (`bypass` rule). Routing engine матчит по regex `(^|[\/])<entry>$` против полного пути процесса (`appwhitelist.go:30-34`).

`NormalizeAppEntry(input)` (`appwhitelist.go:37`) приводит ввод к канонической базе:

| OS | Что делает | Файл |
|---|---|---|
| Windows | `C:\Foo\bar.exe`, `bar.exe`, `bar` → `bar.exe` (добавляет `.exe` если нет) | `appwhitelist_windows.go:20` |
| Linux/BSD | `/usr/bin/firefox` → `firefox`; `.desktop` файл → парсит `Exec=` строку (учитывает кавычки) | `appwhitelist_other.go:26` |
| macOS | `/Applications/VSCode.app` → читает `Contents/Info.plist`, берёт `CFBundleExecutable` (для VSCode это `Electron`); fallback на имя бандла без `.app` | `appwhitelist_darwin.go:32` |

`NormalizeAppList` (`appwhitelist.go:47`) — case-insensitive дедуп, сохраняет порядок.

### Per-process exclusion работает только на Windows/Linux

В sing-box `process_name` rule — это сравнение basename. Поэтому:
- Электрон-приложения на Linux нужно нормализовать к `electron`, не к названию пакета.
- На Linux пользователь может выбрать `firefox.desktop` через файловый диалог — `execFromDesktopFile` его развернёт.
- На macOS специально читается `Info.plist` (через regex `bundleExecKeyRe`), т.к. имя бандла ≠ имя процесса.

---

## 8. Privileged operations (`privileged_unix.go:94`)

`runPrivileged(argv)` — пытается:

1. Если `Geteuid()==0` → exec напрямую.
2. `sudo -n argv...` — non-interactive sudo (только если timestamp валидный или NOPASSWD).
3. GUI fallback:
   - macOS: `osascript "do shell script ... with administrator privileges"` (системный диалог пароля).
   - Linux: `pkexec --disable-internal-agent argv...` (PolicyKit GUI prompt).
4. Если ничего нет → ошибка `"privilege escalation requires pkexec or a passwordless sudo"`.

`shellQuote(s)` (`privileged_unix.go:95`) — POSIX-shell single-quoted form с правильным escape `'\''`. Тесты в `privileged_unix_test.go:7`.

**Кто использует:** kill switch на Linux/macOS (`enableNftables`, `enableIptables`, `pfctl`). Это значит: при каждом arm/disarm на Linux без NOPASSWD пользователь увидит pkexec/osascript prompt. UX-стоимость, но без альтернативы (firewall API требует root).

`RestartAsAdmin` (`system_unix.go:52`) — отдельный механизм для перезапуска **самого приложения** под root (для Tunnel-mode). На Windows аналог `Start-Process -Verb RunAs` через PowerShell (`system_windows.go:52`).

---

## 9. Instance messenger (Windows-only)

### Архитектура (`instance_messenger_windows.go:213`)

1. Создаётся именованный `Global\ResultVAppSingletonMutex_v2`.
2. Если `ERROR_ALREADY_EXISTS` или `ERROR_ACCESS_DENIED` → вторая инстанция → передаёт deep-link через `WM_COPYDATA`:
   - `FindWindowW("ResultVSingletonBridgeCls", "ResultVSingletonBridgeWnd")`.
   - `SendMessageW(hwnd, WM_COPYDATA, …)` с payload = первый `resultv:*` аргумент.
   - `os.Exit(0)`.
3. Первая инстанция:
   - Запускает горутину, locked OS thread (`runtime.LockOSThread()`).
   - Регистрирует класс окна `ResultVSingletonBridgeCls`, создаёт невидимое popup-окно.
   - `wndProc` обрабатывает `WM_COPYDATA` с magic `0x52505349` (`"ISPR"` little-endian) → `onActivate(payload)`.
   - Стандартный message loop `GetMessageW/Translate/Dispatch`.
   - Cleanup через `PostThreadMessageW(WM_QUIT)`.

### Параметры безопасности

- **Global mutex** — гарантирует уникальность на всей машине (даже между разными сессиями RDP).
- **Magic 0x52505349** — отсеивает рандомные WM_COPYDATA от других приложений.
- **Hidden + WS_EX_NOACTIVATE** — окно не светится в Alt-Tab и не получает фокус.

### Не-Windows

`instance_messenger_stub.go:21` — `InitSingletonMessenger` — no-op. Single-instance на Linux/macOS не реализован (sic!).

### Передача deep-link

`ExtractDeepLinkArg(args)` ищет первый аргумент с префиксом `resultv:` (`instance_messenger_windows.go:230`). Используется обоими ветками — и при первой инстанции (передаст в обработчик после Wails init), и при вторичной (положит в `WM_COPYDATA`). Stub-версия `instance_messenger_stub.go:27` повторяет логику без зависимости от windows.

---

## 10. Deep-link registration (`resultv://`)

| OS | Файл | Где регистрируется | Идемпотентность |
|---|---|---|---|
| Windows | `deeplink_register_windows.go:62` | `HKCU\Software\Classes\resultv` (shell\open\command) | Перезаписывает только при отличии (`existing != command`) |
| Linux | `deeplink_register_linux.go:81` | `$XDG_DATA_HOME/applications/resultv.desktop` + `xdg-mime default` + `update-desktop-database` | сравнивает body файла, пишет только при изменении |
| macOS | `deeplink_register_stub.go:13` | **NO-OP** | — |

### Linux desktop entry

```
[Desktop Entry]
Type=Application
Name=ResultV
Exec=<quoted exe> %u
MimeType=x-scheme-handler/resultv;
StartupWMClass=resultv
```

### macOS — где?

В macOS deep-link регистрация делается через `Info.plist` приложения (`CFBundleURLSchemes`) на этапе сборки `.app`-бандла, а не runtime. Wails build добавляет это через `wails.json` / `Info.plist` template. Здесь нет runtime hooks — это правильно (rt-register на macOS не работает без `lsregister`).

---

## 11. WebView2 per OS

| OS | Файл | Что определяет | Fingerprint |
|---|---|---|---|
| Windows | `webview2_windows.go:90` | Версия Evergreen WebView2 из реестра (HKLM/HKCU `EdgeUpdate\Clients\{F3017226-...}` value `pv`) | `"edge"` (если установлен) или `"chrome"` (fallback) |
| Linux | `webview2_linux.go:78` | `pkg-config --modversion` для webkitgtk-6.0, webkit2gtk-4.1, webkit2gtk-4.0 | `"safari"` (WebKit = Safari engine) |
| macOS | `webview2_darwin.go:65` | `sw_vers -productVersion` (macOS version stand-in для WKWebView) | `"safari"` |

### Зачем

sing-box использует uTLS fingerprint для трафика, который генерирует сам WebView2 (например, обновление favicon, OCSP-проверки). Используя `edge` на Windows / `safari` на macOS — мимикрия идёт под нативный браузер ОС.

Результат кешируется через `sync.Once` (`webViewVersionOnce`) — runtime не может смениться без рестарта процесса.

---

## 12. Taskbar restore hook (Windows, `taskbar_restore_hook_windows.go:209`)

### Что это

Хук восстановления окна **из taskbar** когда окно скрыто в tray, но иконка приложения осталась в taskbar (например, кликнули по иконке Windows-задачи после "Hide to tray").

### Как работает

1. `StartTaskbarRestoreHook(ctx, cfg)` — стартует горутину с ticker 120ms.
2. Тикер ищет верхнеуровневое окно процесса (`findTopLevelMainHWND`) по класс-name `"ResultVMainWClass"` через `EnumWindows`.
3. Когда нашли — устанавливает subclass через `comctl32.SetWindowSubclass` (subclass ID `0x52504741` = `"REPA"`).
4. В `subclassProc`:
   - `WM_ACTIVATE` с `wParam!=0` (active) → `cfg.OnRestore()` (если окно "скрыто в tray").
   - `WM_SYSCOMMAND` со `wParam == SC_RESTORE (0xF120)` → `cfg.OnRestore()`.
5. Cleanup — `RemoveWindowSubclass`.

### Зачем

Wails v2 Hide() прячет окно через `ShowWindow(SW_HIDE)`, но не сразу убирает иконку из taskbar. Если пользователь нажмёт на эту иконку — Windows пошлёт `WM_ACTIVATE`/`WM_SYSCOMMAND SC_RESTORE` к hidden HWND, а Wails не реагирует. Хук перехватывает это и зовёт `OnRestore` → `app.restoreMainWindow()`.

### Не-Windows

`taskbar_restore_hook_stub.go:36` — no-op. На macOS/Linux Wails корректно интегрирован с менеджером окон, проблемы нет.

---

## 13. Process tree (`processtree/`)

### Назначение

Раз в 500ms сканирует таблицу процессов, фильтрует подмножество, где предок (по PPID-цепочке) принадлежит "roots" set'у. Эмитит callback `OnChange(Snapshot)` только при изменении desc-set'a.

Пример: пользователь добавил `steam.exe` в exclusion list. Steam запустил `steamwebhelper.exe`, `gameoverlayui.exe`, `Half-Life 2.exe`. Все три попадают в `Snapshot.Descendants` → routing engine добавляет их в bypass-rule.

### Реализация per OS

| OS | Файл | Технология | Особенность |
|---|---|---|---|
| Linux | `processtree_linux.go:179` | `/proc/<pid>/status` для PPid + `/proc/<pid>/exe` symlink для имени | избегает TASK_COMM_LEN truncation; для `/proc/<pid>/exe` чужого пользователя нужен `CAP_SYS_PTRACE` или root |
| Windows | `processtree_windows.go:142` | `CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS)` + Process32First/Next | `ParentProcessID` + `ExeFile[15]` (basename only) |
| macOS/BSD/Solaris | `processtree_other.go:101` | `mitchellh/go-ps` | **TASK_COMM_LEN truncation** — длинные имена обрезаны (≤15-16 символов) |

### Контракт

`Monitor` — `New() → SetRoots(list) → Start(ctx) → OnChange(cb) → Trigger()`. `Trigger()` форсирует немедленный скан без ожидания тика. `Stop()` — отменяет контекст.

`Snapshot.Equal(other)` сравнивает только `Descendants` (roots контролируются пользователем и сравнение не важно).

`normalizeRoots(roots)` — lowercase + basename + дедуп (`processtree.go:186-211`).

### Безопасность

В Linux без root некоторые `/proc/<pid>/exe` будут unreadable → "coverage gap" (некоторые процессы не обнаружатся), но не security risk — они всё равно идут через прокси по умолчанию. Комментарий в коде это явно обосновывает (`processtree_linux.go:34-37`).

---

## 14. Paths

| OS | Файл | new path | legacy path |
|---|---|---|---|
| Windows | `paths_windows.go:53` | `%APPDATA%\ResultV` (FOLDERID_RoamingAppData) | `%APPDATA%\ResultProxy` |
| Linux/macOS | `paths.go:62` | `$XDG_CONFIG_HOME/ResultV` или `~/Library/Application Support/ResultV` | …`/ResultProxy` |
| WebView cache Windows | `paths_windows.go:57` | `%LOCALAPPDATA%\ResultV\webview` | `%LOCALAPPDATA%\ResultProxy\webview` |
| WebView cache Linux/macOS | `paths.go:67` | `$XDG_CACHE_HOME/ResultV/webview` или `~/Library/Caches/ResultV/webview` | …`/ResultProxy/webview` |

`knownFolderPath` на Windows (`paths_windows.go:27`): SHGetKnownFolderPath → env var → `~/<fallback>`. Безопасен относительно localized paths.

---

## 15. Legacy migration (`legacy_migration.go:130`)

### Что мигрирует

Список файлов из старой папки `ResultProxy` в новую `ResultV`:

```
proxy_config.json
.machine-fallback-id
blocked_cache.json
sing-box-cache.db
```

+ всё дерево webview-кеша как директория (`migrateLegacyTree`).

### Правила

1. Если legacy-папки нет → no-op (`migrateLegacyFiles:32`).
2. Если в новой папке уже есть файл с тем же именем → не перезаписывает (`legacy_migration.go:43`).
3. После всех файлов — пытается `os.Remove(legacyDir)` (`legacy_migration.go:53`) — успешно только если папка пуста (т.е. ничего пользовательского не потеряли).
4. Webview-tree: если target-папки нет — пытается `os.Rename` (атомарно). Если есть — `copyDirMissing` (копирует только отсутствующие файлы).

`copyFile` (`legacy_migration.go:98`) — `O_CREATE|O_EXCL|O_WRONLY`, mode 0o600 (приватный).

### Тесты

`legacy_migration_test.go:101` — 4 кейса:
- OldOnly — миграция работает.
- KeepsNewConfig — старый не перезаписывает новый.
- TreeMovesWhenTargetMissing — rename работает.
- ErrorWhenNewPathIsFile — корректная ошибка если new path занят файлом (не папкой).

---

## 16. Тесты — детальный разбор

### `system_test.go` (42 строки)

| Test | Coverage | Build constraint | Gap |
|---|---|---|---|
| `TestGetNetworkTraffic` | logs result, проверяет non-negative | all OS | не assert'ит конкретное значение (что нормально для system-level теста) |
| `TestIsAdmin` | smoke, только logs | all OS | не проверяет UAC-elevation сценарии |
| `TestIsAutostartEnabled` | smoke, только logs | all OS | не вызывает Enable/Disable |

Это **smoke-tests**, не unit-tests — они зависят от состояния системы тестового бокса.

### `killswitch_test.go` (67 строк)

`TestExtractDNSIPs` (8 cases) — таблица:
- nil → fallback `1.1.1.1+8.8.8.8`
- whitespace-only → fallback
- hostnames (`dns.google`) → fallback (резолва нет в kill-switch time)
- pure IPv4 retained
- `host:port` стрипается
- bracketed IPv6 with port `[2606:...]:53`
- dedup with order preservation
- mix valid+invalid

**Coverage gap**: нет теста для `resolveProxyIPs` (DNS resolver mock'ать сложно, но тестировать SplitHostPort + ParseIP можно).

### `killswitch_linux_test.go` (109 строк, `//go:build linux`)

- `TestBuildNftablesRulesetDefaultDrop` — есть `policy drop;`
- `TestBuildNftablesRulesetAddsIPv4Proxy` — `ip daddr 1.2.3.4 accept`, **не** `ip6`
- `TestBuildNftablesRulesetAddsIPv6Proxy` — `ip6 daddr ::1 accept`
- `TestBuildNftablesRulesetAllowsLAN` — все 4 RFC1918 + loopback
- `TestBuildNftablesRulesetScopesDNSToResolvers` — **регрессия**: запрещает `udp dport 53 accept` к `any`
- `TestBuildNftablesRulesetSeparatesIPv4AndIPv6DNS` — разделение по IP-семействам
- `TestBuildNftablesRulesetNoDNSRulesWhenEmpty`
- `TestExtractValidIP` — `host:port`, IPv6 в скобках, edge cases

**Gap**: нет теста для iptables-бэкенда (`enableIptables`/`disableIptables`). Это критично, т.к. iptables — fallback для систем без nft.

### `killswitch_darwin_test.go` (83 строки, `//go:build darwin`)

Аналогично linux, но для pf:
- `TestBuildPFRulesAlwaysBlocksByDefault`
- `TestBuildPFRulesAddsProxyPass`
- `TestBuildPFRulesAllowsLAN`
- `TestBuildPFRulesScopesDNSToResolvers` — регрессия
- `TestBuildPFRulesNoDNSRuleWhenEmpty`
- `TestExtractValidIP`

**Gap**: нет теста на восстановление prev pf-ruleset (потому что pf полностью отключается на Disable — это документированная потеря).

### `autostart_windows_test.go` (62 строки, `//go:build windows`)

- `TestBuildAutostartRunCommand` — 3 кейса; форвард-слеши конвертятся в обратные; кавычки вокруг exe; extra args добавляются после `--autostart`.
- `TestArgsStartInTray` — проверка `--autostart` и `--tray` token detection.

**Gap**: нет теста для `EnableAutostart` (требует моковать registry). Нет теста для `legacyScheduledTaskPresent` (требует моковать schtasks.exe).

### `autostart_linux_test.go` (45 строк, `//go:build linux`)

- `TestRenderAutostartDesktopEntry` — проверка ключей `[Desktop Entry]`, `Type=Application`, `Exec=...--autostart`, `X-GNOME-Autostart-enabled=true`.
- `TestRenderAutostartDesktopEntryQuotesPathWithSpace` — путь с пробелом → quotted Exec.
- `TestQuoteDesktopExec` — 4 кейса.

### `autostart_darwin_test.go` (37 строк, `//go:build darwin`)

- `TestRenderLaunchAgentPlistContainsRequiredKeys` — `Label`, `ProgramArguments`, `RunAtLoad`.
- `TestRenderLaunchAgentPlistEscapesSpecial` — `<` → `&lt;`, `&` → `&amp;`.

### `appwhitelist_*_test.go`

`appwhitelist_test.go` (44) — общая dedup + nil-safety.
`appwhitelist_windows_test.go` (20) — basename + force `.exe`; case retention.
`appwhitelist_other_test.go` (51) — `.desktop` parsing; firstExecToken с quoting.
`appwhitelist_darwin_test.go` (60) — bundleRoot, Info.plist parsing, fallback.

Покрытие хорошее.

### `tray_model_test.go` (132 строки)

- `TestBuildTrayMenuGroups_SortsAndFallbacks` — multi-provider, sort с fallback на `Мои прокси` last.
- `TestBuildTrayMenuGroups_RespectsPerCountryLimit` — `HiddenCount` корректный.
- `TestCountryToFlag` — DE → 🇩🇪; Unknown → 🌐.
- `TestFormatCountryTitle` — `fi` → "Финляндия".
- `TestFormatServerTitle` — `✅` prefix for connected.
- `TestFormatServerTitle_StripsCountryCodePrefixFromServerName` — sanitize.
- `TestCountryISOCode` — `kz` valid; `Kazakhstan` invalid (не ISO2).

### `tray_click_dispatcher_test.go` (72 строки)

`TestTrayClickDispatcher_RebuildDoesNotAccumulateHandlers` — 63 stale channel, верификация что они не считаются.

**Gap**: тест полагается на `time.Sleep` (40ms, 80ms, 500ms total) — flaky risk в CI под нагрузкой.

### `tray_icon_test.go` (46 строк)

`TestPngToICO` — 2x2 PNG → ICO; проверяет header `00 00 01 00`.

**Gap**: не тестирует 256x256 case (special-case `size == 256` → `w = 0` в `pngToICO:730`).

### `legacy_migration_test.go` (113 строк)

См. секцию 15 выше.

### `privileged_unix_test.go` (28 строк, `//go:build darwin || linux`)

`TestShellQuote` — 12 кейсов. Покрытие отличное.

### `processtree/processtree_test.go` (149 строк)

- `TestNormalizeRootsDedupesAndStrips` — normalization работает.
- `TestSnapshotEqualIgnoresRoots` — равенство по descendants.
- `TestMonitorEmitsOnChangeAndDedupes` — injection `m.scan` mock, проверка что одинаковый snap не эмитится.
- `TestMonitorStartStopGoroutine` — double-start no-op.
- `TestMonitorEmptyRootsEmitsEmptyDescendants` — initial zero-vs-zero не эмитит.
- `TestScanIntervalIsReasonable` — `0 < scanInterval ≤ 2s`.

**Gap**: нет интеграционного теста с реальным `/proc` или `Toolhelp32` (понятно почему — flaky).

---

## 17. Найденные проблемы и предложения по рефакторингу

### High priority (security / VPN-критичное)

**H1. Kill switch на Linux iptables не покрывает IPv6.**
Файл: `killswitch_linux.go:217-220`, `:228-232`. Если у пользователя на машине есть глобальный IPv6 и iptables-бэкенд (нет nft) — после `disableIptables` (нет ip6tables правил вообще) трафик IPv6 при потере прокси-узла **не блокируется**. Тест прямо документирует это (`comment: skip v6 silently`), но это leak. Решение: либо добавить параллельные ip6tables-команды, либо при отсутствии nft + наличии глобального v6 — отказываться от Enable с явной ошибкой.

**H2. Kill switch на Windows не имеет `enable=yes profile=any` privacy mode правила и работает на уровне `netsh advfirewall`. Это значит при включённом GPO `ProxySettingsPerUser` (detected в `DetectGPOConflict`) добавление правил может не сработать молча.** Файл: `killswitch_windows.go:140`. CombinedOutput логируется в `err` но `output` теряется (`_ = out` в `killswitch_windows.go:206`) — пользователь видит только generic ошибку. Нужен явный лог тела `out` (или anti-error log при failed sub-rule).

**H3. Kill switch на macOS заменяет ВЕСЬ активный pf-ruleset.** `killswitch_darwin.go:38-42` явно об этом предупреждает в комментарии, но: если у пользователя есть другой инструмент управления pf (Murus, LuLu, IceFloor, или DIY-ruleset для DNS-over-HTTPS), он будет **молча перезаписан** на время kill switch и **полностью отключён** на Disable (`pfctl -d`). Решение: использовать `pfctl -a resultv_killswitch -f rules` (named anchor) вместо top-level — это позволяет сосуществовать.

**H4. Resolve таймаут не настроен.** `killswitch.go:73`: `net.DefaultResolver.LookupIPAddr(context.Background(), host)`. Комментарий обещает 2s, но `context.Background()` означает "никогда не таймаут". Если резолвер заклинило → connect блокируется навсегда. Решение: `context.WithTimeout(ctx, 2*time.Second)`.

**H5. `Disable()` при старте на Windows может оставить мусор.** `app.go:263` вызывает `killSwitch.Disable()`, но `dnsRuleCount`/`proxyRuleCount` обнулены (новый объект). Sweep делает `max(stored, 8)` — это покроет наиболее частые случаи (≤8 IP-адресов CDN или DNS), но **если в предыдущем запуске у прокси было >8 A-записей** (Cloudflare часто отдаёт 4-8, но возможны кейсы) — leftover правила останутся. Решение: либо persist'ить counts на диск, либо использовать `netsh advfirewall firewall show rule name=...` для перечисления.

**H6. Race condition в taskbar restore hook.** `taskbar_restore_hook_windows.go:182-191`: `SetWindowSubclass` вызывается из горутины, в это время main thread может изменять HWND (например, Wails переcоздаёт окно). Между `findTopLevelMainHWND` и `SetWindowSubclass` HWND может стать invalid. Subclass проc будет вызван из `wndProc` chain — если HWND уже разрушен, поведение undefined. Решение: SetWindowSubclass нужно вызывать на UI-потоке окна (через SendMessage в DLL-callback) или принять что failure корректно обрабатывается (но сейчас `ok==0 → continue` — тикер продолжает попытки, что ок).

**H7. Privilege escalation prompt не throttled.** Если на Linux/macOS пользователь нажимает Enable/Disable kill switch несколько раз подряд, каждый раз появляется pkexec/osascript prompt (или silent sudo если cache valid). Нет защиты от accidental click-storm. Решение: rate-limit на app-уровне или в `runPrivileged` (например, debounce 1s).

### Medium priority (correctness / UX)

**M1. Дубликат `autostartTrayToken` константы.**
Объявлена в 3 местах: `autostart_windows.go:34`, `autostart_linux.go:29`, `autostart_darwin.go:33`. Все одинаковые `"--autostart"`. Решение: вынести в общий файл (например, в `autostart.go` без build tag).

**M2. NetMonitor не event-driven.** `netmon.go` polling-based (5s). Реакция на смену интерфейса — до 10s задержка. Это нормально для backup-индикатора, но не для kill switch (хорошо что kill switch не зависит от netmon). Решение опционально: на Windows `IPHelper.NotifyAddrChange`, на Linux netlink, на macOS SystemConfiguration framework.

**M3. Tray UpdateProxyList перестраивает всё меню при любой смене signature.** `tray.go:367` — даже добавление одного сервера приведёт к Hide() всех старых dynamic items и пересозданию. Для больших списков (>100 серверов) это может вызвать flicker меню. Решение: diff-based update (но это сложно с учётом подменю).

**M4. `flagcdn.com` blocking при первом запуске.** `tray.go:553-595` пытается скачать иконки стран синхронно в `UpdateProxyList`. Запросы идут в горутинах (`wg.Wait`), но при первом старте у пользователя без интернета — каждый запрос ждёт 3s (HTTP timeout). Если 10 стран — UpdateProxyList займёт до 3s (4 параллельных potoка, 10/4 = 3 batches). Сейчас вызов из `onReady` и `UpdateProxyList` — UI потенциально подвисает. Решение: сделать prefetch fully fire-and-forget, использовать fallback icon до загрузки, по завершении — push update.

**M5. Hardcoded `1.1.1.1` + `8.8.8.8` fallback DNS.** `killswitch.go:48`. Эти провайдеры доступны в большинстве стран, но в России Google DNS заблокирован, в Китае оба. Решение: использовать прокси-собственные DNS (если sing-box умеет), или конфигурируемый fallback.

**M6. Process tree на macOS использует truncated comm.** `processtree_other.go:28-32` явно документирует это, но: `WhatsApp Helper` → `WhatsApp Helpe` (truncated), что не матчит regex. Это **silent gap** в split-VPN coverage. Решение: на macOS использовать `proc_pidpath()` через cgo (требует include `<libproc.h>`).

**M7. `IsAdmin` на Windows полагается на `net session`.** `system_windows.go:48`. Это работает, но: 1) Может быть медленным (UNC service check), 2) Может фейлить в Server Core или Nano Server. Альтернатива: `OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY)` + `GetTokenInformation(TokenElevation)`.

**M8. `WindowsKillSwitch.NewKillSwitch()` кешит `isAdmin` навсегда.** `killswitch_windows.go:69-73`. Если пользователь перезапустит приложение через UAC (RestartAsAdmin), новый процесс создаст новый `WindowsKillSwitch` с правильным `isAdmin=true`. Но если в одном процессе как-то получили elevation на лету (нет такой API) — флаг останется false. Это edge case, но решение: вычислять `IsAdmin()` каждый раз в Enable.

### Low priority (cleanup / refactor)

**L1. `extractValidIP` дублируется три раза.** В `killswitch_windows.go:218`, `killswitch_linux.go:259`, `killswitch_darwin.go:101` — одинаковая функция. Решение: вынести в `killswitch.go` (cross-platform).

**L2. `pngToICO` (`tray.go:639`) — 122 строки ручной бинарной упаковки.** Можно заменить на `golang.org/x/image/ico` или сторонний пакет. Сейчас работает, но если изменится формат — болезненно поддерживать. Тест покрывает только смоук.

**L3. Дубликат `wndClassEx` структуры.** `instance_messenger_windows.go:47` + `getlantern_systray/systray_windows.go:88`. Поля одинаковые. Можно вынести в shared internal package.

**L4. `DetectGPOConflict` на Windows возвращает true просто при существовании ключа.** `killswitch_windows.go:275`. Это не учитывает значение ключа (ProxyEnable=0 значит "GPO явно отключает прокси" — это другая ситуация, чем "GPO форсит свой прокси"). Решение: парсить значение, отличать enabled vs locked-out.

**L5. Tray fallback icon — серый квадрат 6x6.** `tray.go:614-637`. Это работает, но визуально некрасиво. Решение: embed-ить статический ICO в бинарь как resource.

**L6. `getlantern_systray/go.mod` отдельный модуль.** Это значит у форка свои зависимости (`getlantern/golog`, `lxn/walk` хотя не используется в реальности). При обновлении основного `go.mod` подмодуль не обновляется. Решение: слить в основной модуль через replace directive или удалить `go.mod`.

**L7. `tray.go` — 761 строка, монолит.** Делает много: модель меню, иконки стран, HTTP-загрузка флагов, ICO-encoder, dispatcher init. Можно расщепить:
- `tray.go` — только публичный API + жизненный цикл
- `tray_icons.go` — pngToICO + buildFallbackIcon + flag download
- `tray_render.go` — rebuildServersMenuLocked + refreshServerTitlesLocked

**L8. `SetWindowsTrayLeftClick` — глобальная переменная.** `getlantern_systray/systray.go:30`. Один listener на весь процесс. Если будет нужно несколько tray-инстансов — придётся переделать. Сейчас один tray — ок, но `getlantern_systray` форк теряет переносимость.

**L9. Logging при критических операциях недостаточный.** Kill switch fail в `Disable()` на Linux — `firstErr` возвращается, но если backend (iptables) фейлит на отдельных шагах (`disableIptables:253-255` — три `_, _ = runPrivileged(...)`), детали теряются. Решение: хотя бы Warning-log с output.

**L10. Shutdown cleanup.** При `os.Exit(0)` в `OnUnexpectedExit` (`app.go:333`) **не вызывается** `killSwitch.Disable()`. Если в этот момент firewall активен — пользователь останется без интернета после рестарта. Решение: вызвать Disable из shutdown handler ДО Exit.

---

## Сводная таблица: что покрыто, что нет

| Аспект | Реализовано | Тестировано | Проблема |
|---|---|---|---|
| KS Windows | ✓ | unit (extract*) | H2, H5, L1, L4 |
| KS Linux nft | ✓ | unit (build*) | — |
| KS Linux iptables | ✓ | нет | H1 (IPv6 leak), нет тестов |
| KS macOS | ✓ | unit (build*) | H3 (clobbers pf), L1 |
| KS resolve timeout | частично | нет | H4 |
| Autostart Win | ✓ | unit (command build) | M1 dup const |
| Autostart Linux | ✓ | unit (desktop entry) | M1 dup const |
| Autostart macOS | ✓ | unit (plist) | M1 dup const |
| Tray menu | ✓ | unit (model), integration (dispatcher) | M3 full rebuild, M4 HTTP block |
| Tray icons | ✓ | smoke (pngToICO) | L2, L5 |
| Singleton (Windows) | ✓ | нет | — |
| Singleton (Linux/macOS) | **нет** | — | Linux/macOS multi-instance not handled |
| Deep-link Win | ✓ | нет | — |
| Deep-link Linux | ✓ | нет | — |
| Deep-link macOS | **нет** (runtime) | — | Только через `Info.plist` (build-time) |
| Netmon | ✓ | нет | M2 polling-only |
| AppWhitelist | ✓ | unit (norm*) | — |
| Process tree | ✓ | unit (monitor) | M6 macOS truncation |
| Privilege esc | ✓ | unit (shellQuote) | H7 no rate limit |
| WebView2 detect | ✓ | нет | — |
| Taskbar hook Win | ✓ | нет | H6 race |
| Legacy migration | ✓ | unit (full) | — |
| Paths | ✓ | косвенно через migration | — |

---

## Приоритизированный план рефакторинга

1. **H1 + H4** — за один MR: добавить ip6tables в Linux iptables-бэкенд + сделать resolve timeout 2s через `context.WithTimeout`. Критично для безопасности.
2. **H5 + L1** — за один MR: вынести `extractValidIP` в общий файл + persist'ить `dnsRuleCount`/`proxyRuleCount` (на Windows, в registry или файле) для надёжного cleanup после crash.
3. **H3** — переход на pf anchor: `pfctl -a resultv_killswitch -f rules`. Может сломать существующих пользователей; релизить под флагом.
4. **L10** — добавить shutdown handler, который гарантированно зовёт `killSwitch.Disable()` перед `os.Exit`. Защита от "интернет не работает после краха".
5. **M1** — мерж дубликатов const.
6. **H6** — рефакторинг taskbar hook на установку subclass через SendMessage (но это нудно — текущий polling практически работает).
7. **M4** — асинхронные иконки стран (fire-and-forget с push-update через events).
8. **M2** — event-driven netmon (опционально, после остального).
9. **L7** — расщепить `tray.go` на 3 файла.

---

**Конец review зоны 03.**
