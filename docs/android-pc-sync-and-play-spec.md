# Синхронизация Android с ПК + подготовка к Google Play

Дата: 2026-09-04
Ветка: `android`
Статус: блок 1 специфицирован, блоки 2–4 описаны для следующих циклов

---

## 0. Как пользоваться этим документом

Документ рассчитан на то, что работу продолжит агент без контекста этой сессии.
Всё, что помечено «проверено», подтверждено запуском команды, а не выведено из
чтения кода. Всё, что помечено «решено», — решение человека, его не надо
переоткрывать.

Работа разбита на четыре независимых блока. Порядок не случаен: блоки 1 и 2
задают базу, на которую 3 и 4 ложатся без переделок.

| Блок | Что | Статус |
|---|---|---|
| 1 | Ядро с ПК + выравнивание 16 КБ | специфицирован ниже, к исполнению |
| 2 | Остальной Google Play комплаенс | описан, нужен свой спек |
| 3 | Три решения из редизайна ПК | описан, нужен свой спек |
| 4 | Профили маршрутизации | описан, нужен свой спек |

---

## 1. Карта репозиториев и веток (проверено)

| Путь | Что это | Ветка | История |
|---|---|---|---|
| `C:\ResultV` | рабочий репозиторий Android | `android` | есть |
| `C:\ResultVPC` | рабочий репозиторий ПК | `dev`, дерево чистое | есть |
| `C:\ResultV\ResultV-dev` | копия дерева `dev` для сверки файлов | — | **нет `.git`** |

`ResultV-dev` — не чекаут, а простая копия. Своего `.git` у неё нет, поэтому
git-команды, запущенные из этой папки, отвечают за родительский репозиторий
`C:\ResultV` (ветка `android`) — легко принять за ветку `dev` и ошибиться.
Содержимое сверено с `origin/dev` по SHA-1, совпадает побайтово. Для истории
использовать `C:\ResultVPC` или `origin/dev` в основном репозитории.

**Расхождение веток:** merge-base `44d1d17`. На `origin/dev` 230 коммитов,
которых нет на `android`; на `android` 173 своих.

**Ядро общее, но деревья разъехались.** `mobile/libbox.go` импортирует
`resultproxy-wails/internal/{proxy,config,filter}` — то есть парсер ссылок и
сборка конфига у Android и ПК одни и те же. При этом:

- на `android` **нет** десктопных файлов: `manager.go`, `sysproxy*`, `systun*`,
  `sysdns*`, `autogroup.go`, `sublists.go`, `routinglist.go`, `routingprofile.go`,
  `geodat.go`, `georesolve.go`, `doh.go`, `probe_udp_relay.go`, `serverconn*.go`,
  `ping_resolve.go`, `serverpin_cache.go`, `datadir.go`;
- на `dev` **нет** мобильных: `adblock_rules.go`, `blocked_cidrs.go`,
  `extra_ads.go`, `mobile_stubs.go`, `ping_wg_handshake.go`, `smart_apps.go`,
  `youtube_ads.go`;
- директории `internal/adblock` и `internal/filter` существуют только на
  `android`, `internal/system` и `internal/updater` — только на `dev`.

Слияние веток невозможно. Перенос — только пофайловый, и для каждого файла
решается отдельно: копировать целиком или править точечно.

---

## 2. Уже принятые решения

**Ad-block и Google Play.** Разведка прецедентов дала следующее:

- AdGuard в Google Play **не прошёл** — полную версию убрали из магазина
  25 ноября 2014, с тех пор она раздаётся APK с сайта. В магазине лежат
  «AdGuard: Content Blocker» (работает через Content Blocker API браузеров
  Samsung Internet и Yandex, это не VPN и рекламу в приложениях он не трогает)
  и «AdGuard VPN» (VPN без блокировки рекламы).
- Прошли и живут в магазине: **Rethink: DNS + Firewall + VPN** (позиционируется
  как DNS-резолвер, файрвол и анти-цензура; в Play-сборке **отключены локальные
  файлы блок-листов** — ограничение только Play-версии) и **Blokada 6**
  (фильтрация переехала в облако, приложение настраивает облачный DNS-профиль;
  бесплатная Blokada 5 с локальной фильтрацией в Play не пошла).
- Ни у одного приложения в Play нет MITM с пользовательским корневым CA. Именно
  HTTPS-фильтрация — причина, по которой полный AdGuard вне магазина.

**Решение:** два product flavor.

- `play` — без `browserAdBlock` (MITM, `internal/filter`, `CertWizardScreen`,
  `CertInstaller`, `CertSelfTest`, `FilterProxyWatchdog`) и **без DNS-adblock**.

  Первоначально решение было мягче: DNS-фильтрацию оставить, но переупаковать —
  выключенной по умолчанию и с формулировками про трекеры. **Пересмотрено
  2026-09-04 при исполнении задачи 8.** Причина: сборка грузит
  `adblock_reject.srs` и `geosite-category-ads-all.srs`, а `extra_ads.go` и
  `youtube_ads.go` зашивают рекламные домены (в том числе YouTube) прямо в
  бинарь и подключены в `engine.go` без всякого тега. Переименование интерфейса
  поверх этого — не переупаковка, а расхождение описания с поведением, которое
  проверяется распаковкой .so за минуту. Поэтому DNS-фильтрация вырезана из
  play целиком: тег `no_adblock` + `internal/proxy/adblock_stub.go`, флаг
  `BuildConfig.DNS_ADBLOCK` в Kotlin. Формулировки про трекеры не понадобились —
  описывать в магазинной сборке стало нечего.
- `full` — для раздачи с сайта, без изменений.

Разрез ложится на существующий код: `adblock` и `browserAdBlock` уже два
независимых флага (`SettingsRepository.kt:41,46`).

---

## 3. Блок 1 — ядро с ПК + выравнивание 16 КБ

### 3.1 Зачем

На `android` отсутствует всё, что ПК получил в парсере ссылок за 230 коммитов.
Проверено грепом по обоим деревьям — ни одного из этих маркеров на `android` нет:

| Возможность | Что происходит сейчас на телефоне |
|---|---|
| mKCP (`kcpSettings`, seed, headerType) | тихий откат на голый TCP, узел не работает |
| Hysteria2 port hopping (`mport` → `server_ports` / `hop_interval`) | порт-хоппинг теряется |
| Shadowsocks SIP003 (`plugin` / `plugin_opts`) | query у `ss://` срезается |
| VLESS `Encryption` | теряется молча |
| sing-box `multiplex` | не мапится |
| xhttp: `xPaddingObfs`, session/seq/uplink, фильтрация `xmux` | обфускация не доезжает до ядра |
| Host-заголовок на `ws` / `httpupgrade` / `http` | движок не стартует либо ловит 400 |
| `?extra={...}` в vless/trojan | затирается дефолтами |
| `congestion: off/no` | распознаётся только `true/yes/on` |
| `AutoGroup` в `config.ProxyEntry` | группы AUTO угадываются по именам, а не по объявлению провайдера |

Отдельно рассматривались `probe_udp_relay.go` (релеит ли узел UDP — голос и
игры) и `doh.go` + `ping_resolve.go` (резолв через DoH, когда локальный
резолвер убит VPN-сессией). **При исполнении они были отклонены:** файлы
компилируются на `android` без единой правки, но вызывать их там некому — на
ПК их дёргает `manager.go`, которого мы не переносим. Проверка «компилируется»
не равна «подключено». Их место — в задаче, которая заодно их и подключит:
для UDP-пробы это ещё и поле `SBRouteRule.Inbound` и выделенный probe-inbound
в `engine.go`, которых на `android` нет.

Отдельно — блокер публикации: `jni/arm64-v8a/libgojni.so` в текущем
`android/libs/libbox.aar` собран с `LOAD align = 0x1000` (4 КБ). Google Play с
1 ноября 2025 отклоняет такие сборки. Правится флагом линковки, и раз при
переносе ядра AAR всё равно пересобирается — обе задачи закрываются одной
пересборкой.

### 3.2 Что проверено эмпирически

Перенос проверен сборкой в отдельном git worktree от `android`, а не по чтению
кода. Порядок был такой: собрать базу → скопировать файлы с `dev` → собирать,
пока не станет зелено, записывая каждую правку.

Результат: **`go build -tags=mobile ./internal/... ./mobile/...` даёт exit 0**
после шести правок, перечисленных в 3.4. Их список исчерпывающий — других
ошибок компиляции нет.

### 3.3 Состояние тестов до переноса (проверено)

На чистом `android` HEAD `1bf5cfc` **две красные проверки, обе существовали до
всякого переноса**:

```
--- FAIL: TestDefaultPublicSourceTemplatesRU (internal/proxy)
    blocked_provider_test.go:192: ru sources missing "citizenlab/test-lists/master/lists/global.csv"
    blocked_provider_test.go:192: ru sources missing "citizenlab/test-lists/master/lists/ru.csv"
--- FAIL: TestUserRuleOrder (mobile)
    libbox_rules_test.go:153: no rule_set rule emitted despite AdBlock: true
```

Их надо разобрать **до** переноса, иначе после него будет не отличить своё от
чужого. Ни одна из них переносом не вызвана — проверено запуском на нетронутом
дереве.

### 3.4 Список работ

**Шаг 0.** Привести базу к зелёному: разобраться с двумя провалами из 3.3.
`TestDefaultPublicSourceTemplatesRU` — тест ждёт списки citizenlab, которых в
шаблоне источников больше нет; решить, тест устарел или список урезали зря.
`TestUserRuleOrder` — правило `rule_set` не эмитится при `AdBlock: true`;
это либо реальная регрессия ad-block, либо тест опирается на кэш SRS.

**Шаг 1. Копировать целиком** (проверено, компилируется без правок):

```
internal/proxy/uriparser.go
internal/proxy/outbound.go
internal/proxy/autogroup.go        (новый)
internal/config/*.go               (все, включая новый export_v2.go)
```

Плюс тесты с `dev`, покрывающие перенесённое, — без них ~1500 строк новой
логики разбора не покрыты на этой ветке ничем:

```
autogroup_test.go  build_config_testhelpers_test.go  uriparser_mkcp_test.go
outbound_{mkcp,multiplex,ss_plugin,vless_encryption,ws_host}_test.go
outbound_{hysteria2_hop,transport_headers}_test.go
outbound_{xhttp_obfs,xhttp_padding,xmux_guard}_test.go
```

Ценность именно этого набора в `assertCoreAcceptsConfig` из
`build_config_testhelpers_test.go`: он отдаёт собранный конфиг настоящему
закреплённому ядру sing-box, которое декодирует с `DisallowUnknownFields`.
Незнакомое ядру поле — это не проигнорированная опция, а мёртвый движок.

Две правки в перенесённых тестах: обёртки `mustBuild*ModeConfig` подогнать под
мобильные сигнатуры (`BuildTunnelModeConfig` / `BuildProxyModeConfig` здесь
возвращают одно значение, без ошибки), и убрать `ResolvedIPs` из литерала в
`outbound_xhttp_padding_test.go` — это поле десктопного пининга серверов,
к предмету теста отношения не имеющее.

**Шаг 2. НЕ копировать.** `engine.go` и `singbox.go` разъехались слишком
сильно и в мобильную сторону: на `android` там Smart через локальный SRS,
ad-block, kill switch, правила `package_name`, маскировка адреса сервера в
логе, валидация AWG. На `dev` — TUN IPv6-фолбэк, app whitelist, переделанный
traffic tracker. Из них берутся **только определения структур** (шаг 3а).

**Шаг 3. Точечные правки.**

3а. `internal/proxy/engine.go`:

- добавить тип `SBMultiplex` (6 полей, взять с `dev`, `engine.go:352`);
- добавить в `SBOutbound` шесть полей: `Plugin`, `PluginOptions`, `Encryption`,
  `ServerPorts`, `HopInterval`, `Multiplex`. Своих полей `android` в этой
  структуре имеет три (`Outbounds`, `URL`, `Interval` — группа urltest для
  kill switch), их сохранить;
- заменить структуру `SBOutboundTransport` целиком версией с `dev`: у `android`
  в ней **нет ни одного своего поля**, а на `dev` их на 26 больше (`Seed`,
  `HeaderType`, `MTU`, `TTI`, `UplinkCapacity`, `DownlinkCapacity`,
  `Congestion`, `ReadBufferSize`, `WriteBufferSize`, весь блок `XPadding*`,
  `Session*`, `Seq*`, `UplinkData*`, `CongestionController`, `CWND`,
  `ScMaxBufferedPosts`, `UplinkChunkSize`, `SessionIDTable`, `SessionIDLength`).

3б. `internal/proxy/outbound.go` после копирования: убрать импорт
`resultproxy-wails/internal/system` (директории нет на `android`) и заменить
единственный вызов `system.WebViewFingerprint()` на `defaultUTLSFingerprint()`.
Стаб положить в `internal/proxy/mobile_stubs.go` — файл уже существует ровно
для этого, с `//go:build mobile`:

```go
// defaultUTLSFingerprint is "chrome" on mobile: there is no Edge WebView2 to
// match against, and chrome is the safest mainstream fingerprint that still
// passes Reality's masquerade check.
func defaultUTLSFingerprint() string { return "chrome" }
```

Побочная выгода: версия с `dev` приносит `knownUTLSFingerprints` — неизвестный
отпечаток теперь сводится к `chrome`, а не роняет старт движка.

3в. `internal/proxy/endpoints.go`: `dev` называет список ключей AWG 3.0
`awg3Keys`, `android` — `awg3DeviceKnobs`. Списки идентичны, сверено построчно.
Привести к одному имени (предпочтительно переименовать на `android` в
`awg3Keys`, тогда следующие переносы не потребуют правки). Переименование
задевает шесть файлов, а не четыре: кроме `endpoints.go`,
`config_validation.go`, `uriparser.go` и `ping_wg_handshake.go` — ещё
`config_validation_awg3_test.go`, `endpoints_awg3_test.go` и
`ping_wg_awg3_uapi_test.go`. Добавить с `dev`
функцию `normalizeAWGKey` — она складывает написания провайдеров
(`HeaderProtectionKey`, `header_protection_key`, `header-protection-key`)
в одну форму. Этой функции на `android` нет, и она нужна перенесённому
`uriparser.go`.

3г. `internal/config/crypto.go`: у `android` есть свой `HashHWIDSource`
(`crypto.go:105`), на `dev` его нет — при копировании директории он теряется, и
`mobile/libbox.go:478` перестаёт собираться. Функцию вернуть. Это единственное
расхождение `internal/config`, всё остальное берётся с `dev` как есть.

3д. `mobile/libbox.go:563`: `proxy.SplitAutoEntriesMulti(entries)` →
`proxy.SplitAutoEntries(entries)`. У версии с `dev` три возвращаемых значения
вместо двух: `(groups []AutoGroup, individual []config.ProxyEntry, ok bool)`.
Структура `AutoGroup` у обеих идентична (`Name string`, `Members []ProxyEntry`),
переделывать вызывающий код не нужно.

3е. `internal/proxy/lcp_test.go:135`: то же переименование в тесте.

**Шаг 4. Регрессия, которую надо закрыть.** После переноса падают три
проверки из двух корней (первоначально в спеке была названа одна — оценка была
занижена):

```
--- FAIL: TestSplitAutoEntriesMulti/two_flags_two_auto-groups   (корень: AUTO, шаг 5)
--- FAIL: TestSplitAutoEntriesMulti/auto_groups_plus_individuals (корень: AUTO, шаг 5)
--- FAIL: TestAmneziaWGURIRoundTripAWG2Fields                    (корень: AWG)
--- FAIL: TestUnsupportedAWGKnobsFromParsedURI                   (корень: AWG)
```

Про AWG:

```
knobs = "", want "j1,itime"
```

Причина: парсер с `dev` выбрасывает неподдерживаемые джанк-ключи (`j1`, `itime`)
прямо при разборе URI, а `android` их сохраняет в `extra.amnezia`, чтобы
`UnsupportedAWGKnobs` мог о них предупредить пользователя. Поведение `android`
здесь правильнее — оно и было целью коммита `7b4b1c5`. При переносе сохранить
подход «сохранить и сообщить». Соседний тест `TestUnsupportedAWGKnobs`
(вариант с готовым entry-JSON) проходит — расходится только путь через URI.

**Шаг 5. Решение по поведению AUTO-групп.** Версии расходятся не только именем:

- `android`, `SplitAutoEntriesMulti` — делит AUTO-записи по ведущему
  флаг-эмодзи, каждая группа от двух участников становится своим пулом.
  Несколько пулов возможны.
- `dev`, `SplitAutoEntries` — сначала структурная стратегия (поле `AutoGroup`,
  проставленное парсером из объявленного провайдером xray-балансировщика),
  и только при её неудаче — эвристика по именам, **ограниченная одним пулом**.

Структурная стратегия строго лучше: она не ломается при переименовании группы
и переживает кириллическое «Авто».

А вот про откат первоначальная рекомендация («взять флаг-разбиение `android`»)
**оказалась неверной, и это выяснилось только на тестах**. Провайдеры раздают
авто-секции строками в двух разных формах, и обе настоящие:

1. **Один пул на несколько стран** — `🇨🇦 impVPN Auto | VLESS`,
   `🇩🇪 impVPN Auto | HYSTERIA2`. Флаг здесь свойство узла, а не граница
   группы. Флаг-разбиение рвёт такой пул на одиночек.
2. **Пул на страну** — `🇷🇺 RU Auto VLESS`, `🇺🇸 US Auto VLESS`. Разбиение на
   один пул схлопывает секции и отбирает у пользователя выбор страны.

Различает их `ExtractAutoGroupName`: у формы 1 общее имя набора есть
(«impVPN Auto»), у формы 2 его нет (замерено). Отсюда принятое решение —
**двухступенчатый откат**: сначала общее имя на всём наборе, и только когда его
нет — раскладка по флагу. Реализовано в `splitAutoEntriesByName` +
`splitAutoEntriesByFlag`, обе формы покрыты тестами.

**Шаг 6. Выравнивание 16 КБ.** В `scripts/build-android-aar.sh` в строку
`LDFLAGS` добавить `-extldflags=-Wl,-z,max-page-size=16384`, пересобрать AAR
и проверить результат замером (3.5). Нужен NDK r27+; скрипт сам берёт
новейший NDK из `$ANDROID_HOME/ndk`, версию стоит зафиксировать в логе сборки.

**Шаг 7. Пересборка AAR обязательна.** `gradlew assembleDebug` упаковывает
уже лежащий `android/libs/libbox.aar` и не пересобирает Go. Без запуска
`scripts/build-android-aar.sh` все изменения этого блока не доедут до
устройства и будут выглядеть как «ничего не поменялось».

**Шаг 8. Проверка на стороне Kotlin.** Появление `AutoGroup` в `ProxyEntry`
меняет разбиение AUTO-групп, которое видит UI. Проверить `AutoSelection.kt`,
`ProfileSort.kt` и группировку в `ProxiesScreen.kt` на реальной подписке
с балансировщиком.

### 3.5 Как проверять

```bash
# сборка и тесты (без -tags=mobile internal/proxy и mobile не собираются)
go build -tags=mobile ./internal/... ./mobile/...
go test  -tags=mobile -count=1 ./internal/... ./mobile/...

# пересборка AAR — иначе .so останется старым
bash scripts/build-android-aar.sh

# APK на реальное устройство (никогда не uninstall — сотрёт профили)
cd android && ./gradlew assembleDebug -Pdebug.abi=arm64-v8a
adb install -r -d app/build/outputs/apk/debug/app-arm64-v8a-debug.apk
```

Замер выравнивания — критерий приёмки шага 6. Должно стать `0x4000`:

```bash
unzip -o -j android/libs/libbox.aar 'jni/arm64-v8a/libgojni.so' -d "$TMP"
python - <<'PY'
import struct
f = open(r'<TMP>\libgojni.so', 'rb').read()
off  = struct.unpack_from('<Q', f, 0x20)[0]
size = struct.unpack_from('<H', f, 0x36)[0]
num  = struct.unpack_from('<H', f, 0x38)[0]
for i in range(num):
    o = off + i * size
    if struct.unpack_from('<I', f, o)[0] == 1:   # PT_LOAD
        print('LOAD align =', hex(struct.unpack_from('<Q', f, o + 48)[0]))
PY
```

### 3.6 Критерии готовности блока 1

- [ ] База зелёная: две проверки из 3.3 разобраны
- [ ] `go build -tags=mobile ./internal/... ./mobile/...` — exit 0
- [ ] `go test -tags=mobile -count=1 ./internal/... ./mobile/...` — без провалов
- [ ] `TestUnsupportedAWGKnobsFromParsedURI` зелёный (шаг 4)
- [ ] Решение по AUTO-группам (шаг 5) зафиксировано и покрыто тестом
- [ ] `LOAD align = 0x4000` в свежем AAR
- [ ] На устройстве проверены: ссылка с mKCP, Hysteria2 с `mport`,
      `ss://` с `plugin=`, подписка с xray-балансировщиком

### 3.7 Что сознательно не переносим

`manager.go`, `sysproxy*`, `systun*`, `sysdns*`, `serverconn*`, `tray*`,
`webview2*`, `processtree`, `killswitch_*`, `priority_*`, `deviceinfo*`,
`instance_messenger*`, весь `internal/updater` — десктопное. `internal/updater`
не нужен принципиально: самообновление APK в Google Play запрещено, и на
Android его сейчас нет.

`geodat.go`, `georesolve.go`, `routinglist.go`, `routingprofile.go`,
`sublists.go` — относятся к блоку 4.

Из Smart-режима ПК стоит рассмотреть отдельно (не входит в блок 1, требует
своего разбора): пин голосовой сети Discord `66.22.192.0/18` (`dev/router.go`),
Discord по процессу, path-qualified записи в app-списках, домены аккаунт-слоя и
записи со знаком `^`. Часть Smart-фиксов на `android` уже своя (`2f48f61`,
`534ed1c`) — дублировать не надо.

---

## 4. Блок 2 — остальной Google Play комплаенс

Требует своего спека. Проверенные факты и объём:

### 4.1 Target API 36

Сейчас `compileSdk = 34`, `targetSdk = 34` (`android/app/build.gradle.kts`).
С 31 августа 2026 новые приложения и обновления должны таргетить Android 16
(API 36); существующие с API ≤ 34 перестают быть доступны новым пользователям
на более новых ОС. Продление возможно до 1 ноября 2026.

Тянет за собой: принудительный edge-to-edge (введён в API 35) — придётся
пройтись по всем `Scaffold` и вставкам в `HomeScreen`, `ProxiesScreen`,
`RulesScreen`, `SettingsScreen`, `AddScreen`, `LogsScreen`, `CertWizardScreen`;
изменения правил foreground-сервисов.

AGP уже 8.13.2, Gradle 9.0.0, Kotlin 2.0.21 — поднимать не нужно.

### 4.2 Видимость приложений

Сейчас в манифесте объявлен `QUERY_ALL_PACKAGES` с `tools:ignore`. В списке
разрешённых применений Google (поиск по устройству, антивирусы, файловые
менеджеры, браузеры) per-app VPN routing прямо не назван, и требуется
обосновать, почему менее интрузивный способ не подходит. Риск отказа высокий.

Замена: убрать разрешение, добавить `<queries>`:

```xml
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

Затрагивает: `AppInventory.installedApps` (`AppInventory.kt:112`),
`AppInventory.browserPackages` (второй intent нужен именно ей),
`RulesScreen.loadInstalledApps` (`RulesScreen.kt:740`). Проверить отдельно:
`SmartAppMembership` ключуется на reverse-DNS имени пакета, и приложения без
launcher-активити после этого станут невидимы — надо убедиться, что членство в
туннеле не деградирует.

### 4.3 Формат сборки

Сейчас `splits { abi }` + universal APK. Для Play нужен AAB и Play App Signing.
Ключ лежит в `android/release.keystore` (в зашифрованном `secrets.enc`).

### 4.4 Флейворы play/full

По решению из раздела 2. Вырезать из `play`: `internal/filter`,
`internal/adblock` (проверить, что не используется DNS-веткой),
`CertWizardScreen.kt`, `CertInstaller.kt`, `CertSelfTest.kt`, `CertStore.kt`,
`CertExporter.kt`, `FilterProxyWatchdog.kt`, `browserAdBlock` из
`SettingsRepository` и `BuildOptions`, `BrowserAdBlockRow` из
`SettingsScreen.kt:305`, а также `network_security_config.xml` (его единственная
причина существования — self-test доверия к MITM CA) и `BrowserAdBlockSocksPort`
из `mobile/libbox.go`. DNS-adblock вырезан из play целиком (см. раздел 2):
тег `no_adblock` убирает списки и домены из .so, `BuildConfig.DNS_ADBLOCK`
убирает раздел из настроек и глушит сохранённый флаг, приехавший из
full-сборки.

### 4.5 Декларации в Play Console

Форма VpnService; обоснование `FOREGROUND_SERVICE_SPECIAL_USE` (свойство
`PROPERTY_SPECIAL_USE_FGS_SUBTYPE` в манифесте уже есть); политика
конфиденциальности; Data safety — приложение отправляет панели подписки HWID,
модель устройства и версию ОС (`BuildOptionsBuilder.currentSubscriptionFetchOptionsJson`),
это подлежит декларированию; возрастной рейтинг.

---

## 5. Блок 3 — три решения из редизайна ПК

### 5.1 Smart по умолчанию

На ПК: `config.go:348` — `Mode: "smart"` в `DefaultConfig`, плюс миграция на
строке 593. На Android: `RoutingRules.kt:31` — `RoutingRulesState.mode =
RoutingMode.Global`.

Подвох: дефолтные исключения `["localhost", "127.0.0.1", "*.ru", "*.рф"]`
(`RoutingRules.kt`, поле `outOfVpn`) осмысленны только в Global; в Smart активна
вкладка «в туннель» (`intoVpn`). Смена дефолта без пересмотра этих списков даст
свежему пользователю набор правил, который ни на что не влияет.

Открытый вопрос: менять только для новых установок или мигрировать
существующих (как сделано на ПК)?

### 5.2 Поле-теги

На ПК: `frontend/src/components/kit/TagField.jsx` — Enter коммитит,
`onBlur` тоже коммитит (чтобы не терять набранное), плейсхолдер только у пустого
поля, клик по пустому месту ставит фокус в ввод.

На Android сейчас: `OutlinedTextField` + отдельная кнопка «Добавить» + `FlowRow`
из `DomainChip` (`RulesScreen.kt:204-250`, `DomainChip` на `:466`).

Требование: одно поле-контейнер; пробел коммитит и оставляет клавиатуру; Enter
коммитит и скрывает клавиатуру. Реализация — `BasicTextField` внутри `FlowRow`
с `InputChip` (Material 3), `ImeAction.Done` для Enter, перехват пробела в
`onValueChange`. Дополнительно стоит предусмотреть: backspace на пустом
черновике удаляет последний чип; коммит при потере фокуса (как на ПК).

То же поле нужно и в per-app секции, и в блоке 4 (списки правил профиля).

### 5.3 Вставка конфигов одним textarea

Сейчас: `ManualPane.kt` — сетка из 8 протоколов (`ProtocolGrid`, `:92`) и форма
по полям (`ProtocolForm`, `:153`). Половина работы уже есть в другом месте:
`WireGuardConfParser.kt` и `AddScreen.importWireGuardConf` (`AddScreen.kt:584`)
разбирают `.conf`, а `LinkPane` (`:313`) принимает share-ссылки, `resultv://` и
подписки.

Требование: одно многострочное поле, принимающее `.conf` (WG/AWG) и JSON
(sing-box outbound, xray outbound) наравне со ссылками; определение формата по
содержимому, а не по выбору протокола. Форма по полям остаётся как
запасной путь для ручного ввода и для редактирования
(`ProfileFullEditSheet`, `ManualPane.kt:659`).

Открытый вопрос: JSON какого вида принимаем — только outbound-объект, или целый
конфиг с массивом `outbounds`, из которого предлагается выбрать?

---

## 6. Блок 4 — профили маршрутизации

Самая крупная подсистема, примерно равна трём остальным вместе. Нужен свой спек.

### 6.1 Что есть на ПК

- `internal/proxy/routingprofile.go` — разбор диплинка, модель профиля, лимиты.
  Экспортирует `IsRoutingDeepLink`, `DeepLinkKind`, `DecodeRoutingDeepLink`,
  `ParseRoutingProfileJSON`, `RoutingProfileTokens`.
- `internal/proxy/routinglist.go` — удалённые списки правил: нормализация URL,
  разбор payload, компиляция в SRS, кэш.
- `internal/proxy/sublists.go` — извлечение списков маршрутизации из ответа
  подписки (заголовок или JSON-тело), включая правила xray.
- `internal/proxy/geodat.go`, `georesolve.go` — чтение `geosite.dat` / `geoip.dat`
  и разворачивание токенов `geosite:` / `geoip:`.
- `config.RoutingProfile` и `config.RoutingList` в `internal/config/config.go`
  (приедут в блоке 1 вместе с директорией `internal/config`).
- Уровень приложения: `app_routingprofile.go`, `app_routinglist.go`,
  `app_sublists.go`.
- Документация формата: **`C:\ResultVPC\docs\ROUTING-DEEPLINK.md`**.

### 6.2 Ключевые свойства формата

`https://panel.example/routing/resultv/whitelist` → 302 →
`resultv://routing/onadd/<base64url(JSON)>`. Принимаются также префиксы
`routing/add/` и `routing/`. Payload **не шифруется** (в отличие от
`resultv://` подписок): профиль маршрутизации публичен, и требование ключа
RVSUB1 означало бы, что сторонняя панель не может его опубликовать.
Поля читаются как приходят, включая строковые `"true"` и `"1788322632"`.

Лимиты: `MaxRoutingProfileTokens = 20000`, `MaxRoutingProfileNameLen = 200`,
`MaxRoutingDeepLinkPayload = 4 MiB`. Диплинк доступен атакующему — любой может
прислать ссылку — поэтому превышение отклоняется, а не обрезается молча.

Профиль действует **только в Global**: в Smart клиент решает маршрутизацию сам,
и профиль воевал бы с тем, ради чего Smart существует (`config.go:45`).
Активен ровно один профиль (`ActiveProfileID`).

Один и тот же объект приходит тремя путями — введён руками, доставлен внутри
подписки, открыт по диплинку — и модель у всех трёх одна; `Source` различает
(`manual` / `deeplink` / `subscription`), `OriginName` хранит имя издателя,
чтобы переименованный пользователем профиль не форкнулся при повторном
открытии ссылки.

### 6.3 Чего потребует Android

- Перенос пяти Go-файлов и проверка, что они не тянут десктопные зависимости
  (для `probe_udp_relay.go` / `doh.go` / `ping_resolve.go` это уже проверено —
  тянутся чисто; для этих пяти проверку надо повторить тем же способом).
- Новые биндинги в `mobile/libbox.go`: превью и импорт диплинка, CRUD профилей,
  обновление списков, компиляция в SRS. Ограничение gomobile: только базовые
  типы и JSON-строки.
- Kotlin: репозиторий по образцу `RoutingRulesRepository` / `SmartListRepository`,
  фоновое обновление списков по образцу `SubscriptionRefresher`.
- Обработка `resultv://routing/...` в `MainActivity` и `DeepLinkImporter`
  (сейчас `DeepLinkImporter.kt` знает только подписки; intent-filter на схему
  `resultv` в манифесте уже есть и переиспользуется).
- Извлечение списков из ответа подписки — расширение `fetchSubscriptionV3`.
- UI под Material Design: экран списка профилей, экран редактирования с шестью
  списками правил (direct/proxy/block × sites/IPs) — там переиспользуется
  поле-теги из блока 3, порядок применения (`RouteOrder`), `DomainStrategy`,
  URL geo-баз. На ПК это отдельные окна; на телефоне логичнее bottom sheet
  для превью диплинка и полноэкранный редактор.

---

## 7. Приложение: как воспроизвести проверку переноса

```bash
SC="<scratch>"
git worktree add --detach "$SC/portcheck" android
cd "$SC/portcheck"
go build -tags=mobile ./internal/... ./mobile/...        # база: exit 0

cp /c/ResultVPC/internal/proxy/{uriparser,outbound,autogroup}.go internal/proxy/
cp /c/ResultVPC/internal/proxy/{probe_udp_relay,doh,ping_resolve}.go internal/proxy/
cp /c/ResultVPC/internal/config/*.go internal/config/
go build -tags=mobile ./internal/... ./mobile/...        # далее правки из 3.4

git worktree remove "$SC/portcheck"
```
