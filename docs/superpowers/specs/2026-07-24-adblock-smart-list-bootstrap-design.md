# Проактивная загрузка списков: движок стартует всегда

**Дата:** 2026-07-24
**Ветка:** android
**Статус:** дизайн одобрен, ожидает плана реализации

## Проблема

После переустановки при первом включении Smart-режима подключение падает с
фатальной ошибкой; в Global поднимается, но при возврате в Smart «трафик не идёт»;
только после нескольких переподключений Smart начинает работать. Разбор логов
(`resultv-logs-2026-07-24_*.txt`) выявил **две** связанные проблемы с общим
корнем — приложение подключается раньше, чем готовы сетевые списки, а на свежей
установке эти списки нельзя скачать «как есть».

### Проблема №1 — фатальный холодный старт ad-block

```
Сбой подключения: initialize rule-set[1]: initial rule-set: ads-ru:
  Get "https://raw.githubusercontent.com/.../geosite-category-ads-all.srs":
  ... timeout  /  CRYPTO_ERROR 0x12a: tls: certificate has expired
```

Механизм (`internal/proxy/adblock_rules.go:90` `buildAdBlockRuleSets`):

- На свежей установке локального SRS-кэша нет → функция отдаёт `remote` rule_set с
  `URL: src.urls[0]` (**только** `raw.githubusercontent.com`, без зеркала jsDelivr)
  и `DownloadDetour: "direct"`.
- Комментарий в коде утверждает, что неудачная удалённая загрузка «не фатальна».
  Это верно **только** после того, как rule_set хоть раз попал в `cache_file`. На
  реально чистой установке sing-box обязан скачать список синхронно при старте, и
  провал прерывает запуск движка (`initialize rule-set: initial rule-set`).
- Встроенный фетчер sing-box лезет по QUIC (`CRYPTO_ERROR`) и ловит cert-ошибку
  там, где наш собственный HTTP/2-загрузчик проходит.

Параллельный `DownloadAdBlockRuleSets` (`mobile/libbox.go:967` `FetchAdBlockLists`)
**имеет** fallback на зеркало jsDelivr и обычно успевает, но запускается
fire-and-forget из `MainActivity` и **не ожидается** перед подключением. Поэтому
«Global работает» — это на самом деле «вторая попытка работает»: к ней кэш уже
прогрет нашим загрузчиком.

### Проблема №2 — Smart подключился, трафика нет

```
[17:28:30] SMART ... записей: 78637   ← полный удалённый RU-список
[17:28:42] SMART ... записей: 1579    ← деградация до builtin-фолбэка
[17:31:50] SMART ... записей: 78637   ← снова полный (только тогда Smart заработал)
```

1579 — встроенный `defaultBlockedDomains()` (`internal/proxy/blocked_updater.go:92`).
Per-app allowlist (`BoxModule.kt:327` `applySmartAllowlist`) строится из
`SmartListRepository.currentDomains()`. Когда список схлопнулся до builtin, в
туннель попадает мало приложений → нужные (заблокированные в РФ) приложения не
маршрутизируются через VPN → «трафика нет».

## Цель

- Списки (ad-block SRS + smart-список) кэшируются **проактивно и независимо от
  подключения** нашим собственным загрузчиком (HTTP/2 + зеркало jsDelivr; GitHub в
  РФ доступен напрямую).
- Запуск движка **никогда** не блокируется доступностью списков и не может из-за
  них упасть.
- Переподключение руками **никогда** не требуется: списки применяются на лету.

GitHub в РФ не заблокирован — поэтому надёжнее полностью полагаться на наш
загрузчик и **убрать** встроенный `remote` rule_set sing-box (фатальный на холодном
старте, без зеркала, по QUIC с cert-ошибкой).

## Решение

### Компонент 1 — движок не падает из-за ad-block (Go)

`internal/proxy/adblock_rules.go` + `internal/proxy/engine.go`:

- `buildAdBlockRuleSets(dataDir)` отдаёт **только `local`** rule_set'ы — для файлов,
  что реально лежат на диске и проходят `validateSRS` (`localAdBlockSRSUsable`).
  Ветка `remote` **удаляется целиком**. Нет валидного файла → список не
  добавляется.
- Добавить `availableAdBlockRuleSetTags(dataDir) []string` — теги, для которых
  реально есть локальный файл.
- reject-правила, ссылающиеся на rule_set по тегам, переводятся с
  `adBlockRuleSetTags()` на `availableAdBlockRuleSetTags(dataDir)`:
  - DNS reject: `engine.go:621`.
  - route reject: `engine.go:879`.
  Если доступных тегов нет — правило с `RuleSet` **не добавляется** (статические
  reject-правила по доменам, `extraAdDeliveryDomains` и т.п., остаются).
- `adBlockRuleSetTags()` может стать приватным/удалиться, если больше нигде не
  используется.

Итог: пустой кэш → соединение поднимается, просто без ad-block в эту сессию; нет
«висящей» ссылки на несуществующий rule_set.

### Компонент 2 — короткое ожидание + фолбэк на connect (Kotlin)

`ResultVpnService.onStartCommand` (ветка Connect, `~строка 120`):

- Сборку конфига и старт выполнять на worker'е. **Перед** `buildConfig` —
  ограниченное по времени ожидание готовности кэша:
  `withTimeoutOrNull(CONNECT_LIST_WAIT_MS) { AdBlockRepository.ensureLoaded();
  SmartListRepository.ensureLoaded() }`. Состояние UI — `Connecting` (подпись
  «Подготовка списков…»).
- Успели → конфиг с локальными SRS и полным smart-списком; ad-block/smart активны
  с первого пакета.
- Таймаут → стартуем всё равно (Компонент 1 делает это безопасным); списки
  применяются позже (Компонент 3).
- `CONNECT_LIST_WAIT_MS = 4000` (GitHub напрямую отвечает ~1–2с; 4с — запас, но не
  раздражает). Т.к. префетч обычно уже отработал на старте приложения (Компонент
  4), ожидание почти всегда близко к нулю.

`ensureLoaded()` уже коалесцирует параллельные вызовы через `Mutex`, поэтому
ожидание на connect и фоновый префетч не дублируют загрузку.

### Компонент 3 — авто-применение без переподключения (Kotlin)

Если на момент `BoxModule.start` списки не были готовы: подписаться на
`AdBlockRepository.state` / `SmartListRepository.state`; как только становятся
`ready` (есть локальные SRS / получен «настоящий» smart-список), пересобрать конфиг
и вызвать `BoxModule.reload(cfg)` — по образцу `attachBrowserAdBlockAsync`
(`ResultVpnService.kt:231`). Туннель не рвётся (существующий безопасный in-place
handover TUN). Подписка одноразовая: снимаемся после применения или при
disconnect.

### Компонент 4 — префетч + защита smart-списка от схлопывания в builtin (Kotlin)

- Проактивный префетч на старте приложения уже есть (`MainActivity` →
  `AdBlockRepository.ensureLoadedAsync()` / `SmartListRepository.ensureLoadedAsync()`).
  Оставляем — именно он делает ожидание Компонента 2 почти нулевым.
- Защита от Проблемы №2: в `SmartListRepository.refresh()` **не перезаписывать**
  уже загруженный «настоящий» снимок (source `remote`/`cache`, непустой) снимком с
  source `builtin`. Builtin трактуется как «ещё не готово» → сохраняется прошлый
  хороший снимок. Это напрямую убирает симптом «подключился, но трафика нет».
  `parseSnapshot` уже читает `source`.

## Обработка ошибок / крайние случаи

- Оба зеркала GitHub недоступны → ad-block не включается в эту сессию (не
  фатально); TTL-рефреш (24ч) попробует снова, при следующем connect подхватится.
- Antizapret недоступен и кэша нет → smart-список пуст → `buildRoute` уже сохраняет
  глобальное поведение (`engine.go:781`: `SmartMode && len(domains)>0` иначе
  `final="proxy"`), туннель не «no-op».
- Reload на живом соединении — существующий безопасный in-place handover TUN
  (`BoxModule.openTun`: `establish()` до закрытия старого PFD).
- Пользователь отключается во время ожидания на connect — таймаут/отписка не должны
  оставлять висящих корутин; ожидание отменяется вместе со стартовой задачей.

## Тестирование

**Go** (расширяем `internal/proxy/adblock_rules_test.go`, `engine_adblock_test.go`):

- `buildAdBlockRuleSets`: нет файла → пустой результат, ни одного `remote`;
  частичный кэш (один валидный, один отсутствует/битый) → только валидный `local`.
- `availableAdBlockRuleSetTags`: возвращает только присутствующие теги.
- Сборка route/DNS: reject-правила ссылаются только на присутствующие теги; при
  пустом кэше правило с `RuleSet` отсутствует, статические reject-правила на месте.

**Kotlin**:

- `SmartListRepository`: builtin-снимок не затирает непустой `remote`/`cache`
  снимок; `remote`/`cache` затирает как обычно.
- Ожидание-с-таймаутом на connect: путь «успели» (конфиг видит локальные списки) и
  путь «таймаут» (старт без списков, затем reload по готовности).

## Затрагиваемые файлы

- `internal/proxy/adblock_rules.go` — local-only, `availableAdBlockRuleSetTags`.
- `internal/proxy/engine.go` — reject-правила по доступным тегам (`:621`, `:879`).
- `internal/proxy/adblock_rules_test.go`, `internal/proxy/engine_adblock_test.go`.
- `android/.../vpn/ResultVpnService.kt` — ожидание+фолбэк на connect, авто-reload.
- `android/.../vpn/SmartListRepository.kt` — защита от builtin-схлопывания.
- (тест) `android/.../vpn/SmartListRepositoryTest.kt` (новый, если тестовая
  инфраструктура позволяет).

## Не входит в объём (YAGNI)

- Изменение источников списков или формата SRS.
- Изменение per-app matcher'а (`SmartAppMatcher`) — allowlist чинится тем, что на
  вход подаётся полный список.
- Незакоммиченные правки про *битый* кэш (`zlib invalid checksum`,
  `EngineErrors`/`startOrRecover`) — это отдельная тема, не пересекается.
