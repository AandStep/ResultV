# ResultV — Code Review: Frontend

Файлы: `C:\ResultVPC\frontend\src\` (React 18 + Vite 3 + Tailwind 3 + i18next).
Подключение к Go-бэку: автогенерируемые биндинги `wailsjs/go/main/App.*` + рантайм-события `wailsjs/runtime`.

---

## 1. Обзор фронтенда

### Стек
| Компонент | Версия | Назначение |
|---|---|---|
| React | 18.2 | UI |
| Vite | 3.0.7 | dev/build (`vite.config.js`) — внедряет `__APP_VERSION__` из package.json |
| TailwindCSS | 3.4 | стили (`tailwind.config.js`) |
| i18next + react-i18next | 23.10 / 14.1 | i18n (en/ru, `LanguageDetector` через localStorage→navigator) |
| lucide-react | 0.363 | иконки |
| PostCSS + autoprefixer | 8.4 / 10.4 | препроцессинг |

### Версия приложения
`vite.config.js:27` — `__APP_VERSION__` инжектится из `package.json.version` (сейчас `3.2.0`), доступно как глобал.

### Связь с Go
Через автогенерируемые wrapper-функции `../../wailsjs/go/main/App` (импортируются в `utils/wailsAPI.js`).
События — через `EventsOn`/`EventsOff` из `wailsjs/runtime/runtime`. Используются события: `log`, `deeplink:received`, `deeplink:error`, `config:updated`, `update:progress`, `update:verifying`, `update:verified`, `update:installing`, `update:failed`, `proxy:connected` (последнее эмитится Go, но не подписано на фронте).

### Структура каталога `frontend/src/`
```
App.jsx, main.jsx, index.css, App.css
assets/                — статика
components/
  layout/              — MainLayout, Sidebar, MobileHeader
  ui/                  — модалки, селект, чарт, флаг, toggle
context/               — AppContext (root), ConfigContext, ConnectionContext, LogContext
hooks/                 — useAppConfig, useDaemonControl, useDaemonStatus, useDaemonPing, useLogs, useCheckUpdate
lib/i18n.js            — конфиг i18next
locales/en.json, ru.json
utils/                 — wailsAPI, proxyParser, crypto, pingSort, formatters, network, subscriptionSecurity, versionCheck
views/                 — HomeView, ProxyListView, AddProxyView, SettingsView, RulesView, LogsView, BuyProxyView
```

---

## 2. `main.jsx` и `App.jsx`

### `main.jsx` (52 строк)
- Bootstrap: `createRoot` → `<StrictMode><ErrorBoundary><App/>`.
- **Хорошо**: есть `ErrorBoundary` (`main.jsx:24-47`) — отлавливает падения и рендерит сообщение в полноэкранном fallback.
- **Проблема (Low)**: ErrorBoundary один, обёрнут на корень. Нет более точечных boundaries вокруг views (например, ошибка в `AddProxyView` уронит всё приложение).

### `App.jsx` (131 строк)
- **Роутинга нет** — нет `react-router`. Switch по `activeTab` из контекста (`App.jsx:86-95`): `'home' | 'list' | 'rules' | 'add' | 'buy' | 'logs' | 'settings'`.
- `ProxyListView` рендерится постоянно (через `<div className={activeTab==='list' ? '' : 'hidden'}>`), остальные размонтируются. Это сохраняет состояние `searchQuery`/`sortBy`, но удваивает виртуальную DOM-нагрузку.
- Хранит state `dismissedUpdateVersion` через `sessionStorage` (`App.jsx:53-55`) — «Позже» работает до перезапуска.
- Глобальные модалки: `ProtocolWarningModal`, `AppDialogModal`, `DeepLinkImportModal`, `UpdaterModal`/`UpdateNotificationModal`. Выбор между updater (in-app) и notification (внешний браузер) делается по `hasPlatformAsset` из `useCheckUpdate`.
- **Loader** при `!isConfigLoaded` — отдельный полноэкранный spinner (`App.jsx:67-83`).

---

## 3. `MainLayout` + `Sidebar` + `MobileHeader`

### `MainLayout.jsx` (115 строк)
- Двух-колоночный (desktop): `Sidebar` слева + основной контент справа.
- Mobile: `<MobileHeader>` сверху, нижняя tab-bar с 5 пунктами (`MainLayout.jsx:87-121`).
- Скролл-логика: `mainScrollRef` сбрасывает скролл на 0 при переходе `add → list` (`MainLayout.jsx:43-52`).
- **High — захардкоженные русские строки** (`MainLayout.jsx:90,102,111,117`): `label="Главная"`, `"Добавить"`, `"Прокси"`, `"Настройки"`. Все остальные сайдбар-пункты идут через `t("sidebar.*")`. Это явный bug при английском интерфейсе.
- **Глобальный inline-`<style>`** (`MainLayout.jsx:56-71`) принудительно убирает outline у всех элементов через `* { outline: none !important; }` — нарушает a11y (см. раздел 12).

### `Sidebar.jsx` (124 строк)
- Скрывается на mobile (`hidden md:flex`).
- Показывает баннер «Служба не отвечает» когда `daemonStatus === "offline"` (`Sidebar.jsx:64-69`).
- Версия и `LanguageSwitcher` снизу.

### `MobileHeader.jsx` (41 строк)
- Логотип + индикатор состояния (зелёная/розовая точка) на мобильной шапке.

---

## 4. Views — постранично

### 4.1 `AddProxyView.jsx` (**2168 строк** — гигант)

Файл объединяет четыре существенно разных режима. **Кандидат №1 на split** (см. раздел 12).

**Подсекции:**

#### 4.1.1 Импорт URI (`AddProxyView.jsx:457-558`)
- `handleVpnUriSubmit` → ветка для подписочной URL/RVSUB1/обычного URI.
- Если 1 VPN-результат — сразу сохраняет; иначе открывает `ProtocolSelectionModal`.

#### 4.1.2 Импорт подписки (`AddProxyView.jsx:259-441`)
- `processImport` (~180 строк) — ветвление между `isSubscriptionURL`, `isEncryptedSubscription`, и обычным `parseProxies`.
- Обрабатывает `isInsecureSubscriptionError` (plaintext HTTP) — спрашивает пользователя и повторяет с `allowInsecure=true` (`AddProxyView.jsx:266-284`, `391-419`).
- `handleConfirmImport` — после выбора протокола: если все из одной подписки — `addSubscription(label, subURL)`, иначе bulk-save. При ошибке «уже добавлена» — fallback на `refreshSubscription` существующей (`AddProxyView.jsx:410-416`).

#### 4.1.3 Импорт файла (`AddProxyView.jsx:445-455`)
- `FileReader` → `processImport(text, fileName)`. Для `.conf` (WireGuard) дополнительно подставляет имя файла как имя профиля (`AddProxyView.jsx:336-347`).

#### 4.1.4 Ручное добавление по протоколам (`AddProxyView.jsx:560-931` и далее JSX до 2179)
- 4 разных формы:
  - **Plain (HTTP/HTTPS/SOCKS5)** — `name/ip/port/username/password`.
  - **VLESS/VMESS/TROJAN/SS** — selector `vpnNetwork` × `vpnSecurity` + поля транспорта (ws/grpc/h2/xhttp).
  - **WIREGUARD/AMNEZIAWG** — приватный/публичный ключ, allowed_ips, optional AmneziaWG fields (`AmneziaEditor`).
  - **HYSTERIA2** — SNI/ALPN/insecure/upMbps/downMbps/obfs.
  - **NAIVEPROXY** — username/password/sni/insecure/listen/log.
- Все формы дублируются дважды для режима «новый» vs «редактирование» (`isVpnEditMode`) — большой copy-paste (см. строки 1055-1457 vs 1668-2178).
- `AmneziaEditor` (`AddProxyView.jsx:84-183`) — вложенный компонент для редактирования AWG-обфускации (jc/jmin/jmax/s1..s4/h1..h4/i1..i5/itime/j1..j3), с переключателем «поля ↔ JSON».

#### 4.1.5 Deep-link — обрабатывается отдельным `DeepLinkImportModal`, но `AddProxyView` тоже умеет в URI/clipboard.

### 4.2 `HomeView.jsx` (755 строк)

- Connect/Disconnect через большую круглую кнопку `Power` (`HomeView.jsx:269-306`).
- Статус: 6 состояний — `disconnecting`, `connecting`, `connected+healthy`, `connected+proxyDead`, `error`, `unprotected` (`HomeView.jsx:228-251`).
- Выбор сервера: dropdown с группировкой по `provider` + раздел «Избранное» (через `sortProxiesByOption` из `pingSort.js`).
- `renderDropdownItem` — внутренняя функция (1 экземпляр на render) → ре-создание ссылки каждый раз (`HomeView.jsx:663-783`).
- При `WIREGUARD/AMNEZIAWG` форсит mode→`tunnel` (`HomeView.jsx:125-130`).
- `displayProxy` — мемо-цепочка: `failedProxy → activeProxy → lastSelectedProxyId → proxies[0]` (`HomeView.jsx:99-110`).
- Speed-чарты (`SpeedChart`) — `speedHistory.down/up`, последние 20 точек.
- **Проблема (Medium)**: `displayProxyTitle` (`HomeView.jsx:112-124`) — три каскадных useMemo, но JSX в `renderDropdownItem` всё равно делает `formatProxyDisplayName(proxy.name, proxy.country)` для каждого элемента списка — не мемоизировано.

### 4.3 `ProxyListView.jsx` (752 строк)

- Поиск + сортировка (sortBy: `default | newest | oldest | country | type | provider | ping`).
- Группировка по `provider`, иконка-фавикон подгружается из `subscription.url` (`ProxyListView.jsx:58-126`), fallback — `impLogo` для `subscriptionUsesImpLogo`.
- Свёртывание групп (`collapsedGroups`), кнопки: refresh подписки, ping группы, delete подписки/группы.
- `renderProxyCard` (`ProxyListView.jsx:365-543`) — карточка прокси с кнопками `[fav][edit][delete][connect]`, статусом ping, для AUTO — расчёт лучшего пинга по членам.
- **Проблема (High)**: при 500+ прокси из подписки рендерится 500+ карточек без виртуализации. См. раздел 12.
- **Проблема (Low)**: формат даты подписки — хардкод DD.MM.YY (`ProxyListView.jsx:638-646`), не использует `Intl.DateTimeFormat`.

### 4.4 `SettingsView.jsx` (502 строк)

- Секции:
  1. Toggle: autostart / killswitch / adblock (через `SettingToggle`).
  2. DNS — пресеты (auto/google/cloudflare/quad9) + custom (`SettingsView.jsx:328-389`).
  3. LAN listen — `listenLan` toggle + порт + список `lanIPs`.
  4. Export/Import — модалка с паролем.
- **High — клиентское шифрование (`SettingsView.jsx:76-107`)**: экспорт идёт через `encryptWithPassword` (web crypto API, AES-GCM PBKDF2 100k SHA-256). Однако `wailsAPI.js:131` имеет `exportConfig(password)` который вызывает Go-side `ExportConfig` — **никем не используется в SettingsView**. То есть имеется два независимых пути шифрования (см. раздел 7).
- Экспорт делается через `document.createElement('a')` + data-URL — не использует Wails native dialog.
- Сохранение через `updateSetting()` (для killswitch/autostart/adblock в `useAppConfig`) и через `setSettings(...) + wailsAPI.saveConfig(...)` (DNS, localPort).
- **Medium — потеря синхронизации**: `setSettings(imported.settings)` (`SettingsView.jsx:146`) после import не вызовет `useAppConfig.useEffect → wailsAPI.syncProxies` для autostart/killswitch (они применяются только через `updateSetting`).

### 4.5 `LogsView.jsx` (111 строк)
- Объединяет `logs` (frontend) + `backendLogs` (Go), сортирует по `timestamp desc`, обрезает до 150.
- **Medium — анти-паттерн перевода**: функция `translateLog` (`LogsView.jsx:22-86`) делает `.startsWith("Сбой подключения:")` и заменяет на `t("logs.msg.conn_failed")` — strings-matching by Russian substring. Все `addLog()`-вызовы хардкодят русскую строку, поэтому английский интерфейс с не-русским логом не сработает.

### 4.6 `RulesView.jsx` (289 строк)
- Mode: `global` ↔ `smart`.
- Whitelist доменов (с поддержкой `*.ru`, `*.рф`).
- App-whitelist (Win: `.exe`, mac: `.app`) с поддержкой native picker (`PickAppForWhitelist` через Wails) + HTML fallback.

### 4.7 `BuyProxyView.jsx` (143 строк)
- Партнёрская витрина (один партнёр `imp_vpn` сейчас). Кнопки «Перейти» + «Скопировать промокод».
- Использует `BrowserOpenURL` из Wails-runtime — открывает в системном браузере.

---

## 5. UI-компоненты

| Компонент | LOC | Назначение | Пропсы | Reuse |
|---|---|---|---|---|
| `AppDialogModal` | 106 | Универсальный alert/confirm с вариантами info/warning/danger | `isOpen, title, message, variant, showCancel, confirmText, cancelText, onConfirm, onClose` | Используется через `appDialog` state + `showAlertDialog/showConfirmDialog` |
| `AppSelect` | 208 | Кастомный `<select>` с keyboard a11y (ArrowUp/Down/Enter/Esc) | `value, options, onChange, placeholder, disabled, className, buttonClassName, listClassName, ariaLabel` | В `AddProxyView` (VPN network/security) |
| `DeepLinkImportModal` | 264 | Обработка `resultv://` импорта | пропсов нет (читает из `ConfigContext.pendingDeepLink`) | Только один экземпляр в `App.jsx` |
| `FlagIcon` | 58 | Иконка флага — `flag-icons` CDN, fallback на текст/Globe/Server | `code, className` | В HomeView, ProxyListView, LanguageSwitcher |
| `HoverMarquee` | 56 | Бегущая строка при overflow при hover | `text, className` | ProxyListView для длинных имён |
| `LanguageSwitcher` | 40 | Переключатель ru ↔ en через `i18n.changeLanguage` | — | Sidebar |
| `ProtocolSelectionModal` | 154 | Выбор HTTP/HTTPS/SOCKS5 для plain proxies в импорте | `isOpen, onClose, onConfirm, count, proxies` | AddProxyView, DeepLinkImportModal |
| `ProtocolWarningModal` | 51 | Предупреждение об уточнении протокола | `isOpen, onClose` | App.jsx (через `showProtocolModal`) |
| `SettingToggle` | 37 | Toggle карточка | `title, description, isOn, onToggle` | SettingsView |
| `SpeedChart` | 68 | SVG polyline-чарт | `data, color, fillHeight` | HomeView |
| `UpdateNotificationModal` | 111 | Уведомление о новой версии (без in-app-обновления) | `currentVersion, latestVersion, downloadUrl, onClose` | App.jsx fallback (когда `!hasPlatformAsset`) |
| `UpdaterModal` | 239 | In-app updater (idle→downloading→verifying→installing→failed) | `currentVersion, latestVersion, downloadUrl, onClose` | App.jsx primary path |

### Особенности
- **`AppSelect`** — самый качественный компонент: ARIA-разметка (`role=listbox`/`role=option`, `aria-activedescendant`), keyboard nav, дискриминация по `useId`.
- **`UpdateNotificationModal` и `UpdaterModal`** дублируют `ALLOWED_DOWNLOAD_HOSTS` и `isSafeDownloadURL` — DRY-violation (`UpdaterModal.jsx:25-37`, `UpdateNotificationModal.jsx:28-44`).
- **`AppDialogModal`** — единственная точка для alert/confirm-диалогов, хорошо абстрагирована.

---

## 6. Контексты (Context API)

```
LogProvider
  └── ConfigProvider
        └── ConnectionProvider
              └── <AppContent>
```

| Контекст | Что хранит | Источник |
|---|---|---|
| `LogContext` | `logs` (frontend), `backendLogs` (Go), `addLog` | `useLogs` |
| `ConfigContext` | Всё из `useAppConfig` + `activeTab/setActiveTab`, `editingProxy/setEditingProxy`, `pendingDeepLink`, `pendingDeepLinkSource` | `useAppConfig` + локальное |
| `ConnectionContext` | `isConnected`, `isConnecting`, `isDisconnecting`, `failedProxy`, `activeProxy`, `pings`, `daemonStatus`, `stats`, `speedHistory`, `disconnectOnly`, `toggleConnection`, `selectAndConnect`, `deleteProxy`, `cancelConnect` | `useDaemonStatus` + `useDaemonControl` + `useDaemonPing` |

### Анти-паттерны / замечания

- **`ConfigContext` = god-context**: тянет в `value` объект с ~25 полями (см. `useAppConfig` return + `ConfigContext.jsx:73-83`). Любое изменение пересчитывает каждый потребитель. Каждое изменение proxies/settings → ре-рендер всего дерева компонентов, использующих `useConfigContext`.
- **`value` не мемоизирован** в провайдере (`ConfigContext.jsx:73-83` создаётся inline на каждом рендере). Технически — каждый рендер `<ConfigProvider>` инвалидирует контекст для потребителей.
- Аналогично `ConnectionContext.jsx:114-137` — value тоже не мемо.
- **Дублирование с `useAppConfig`**: `useAppConfig` уже возвращает state, а провайдер сверху ещё добавляет `activeTab`, `editingProxy`. Можно было держать в едином хуке.

---

## 7. Хуки

### 7.1 `useAppConfig.js` (462 строк) — **God-hook**

Сохраняет почти весь app-state:
- `isConfigLoaded`, `proxies`, `routingRules`, `settings`, `subscriptions`, `platform`, `isApplyingMode`.
- Диалоговая система: `appDialog`, `showAlertDialog`, `showConfirmDialog`, `closeAppDialog`, `handleAppDialogConfirm` (`useAppConfig.js:50-137`) — `confirmResolverRef` для promise-based confirm.
- `updateSetting(key, value)` (`useAppConfig.js:193-312`) — switch по key c rollback при ошибках. Особые ветки для `mode` (через `ApplyMode`), `autostart`, `killswitch`, `adblock`, `dnsServers`. Иначе — generic `setSettings + persistSettings`.
- `handleSaveProxy` (`useAppConfig.js:359-413`) — сохранение одного, с авто-детектом страны через `network.detectCountry`.
- `handleBulkSaveProxies` (`useAppConfig.js:415-466`) — мерж с дедупликацией по `ip:port:type` (для VPN-типов).
- Refresh подписок раз в 6 часов (`useAppConfig.js:328-357`).
- Авто-`syncProxies`/`updateRules` через useEffect на изменение `proxies/routingRules` (`useAppConfig.js:182-191`).

**Проблема (High)** — `useAppConfig` смешивает:
1. Хранение state.
2. Бизнес-логику (rollback, валидация).
3. UI-диалоги (`showAlertDialog` живёт в этом же хуке).
4. Side-effects (timer 6h refresh, auto-sync).
5. Persist через `wailsAPI.saveConfig`.

Это нарушает SRP. Рекомендую разнести: `useConfigStorage`, `useDialogStack`, `useSubscriptionRefresh`.

### 7.2 `useDaemonControl.js` (406 строк)

- `toggleConnection`, `selectAndConnect`, `disconnectOnly`, `deleteProxy`, `cancelConnect`.
- Поддержка AUTO-серверов: `getConnectCandidates` сортирует членов по ping и пробует до 5 (`useDaemonControl.js:50-83`).
- Гонки: `isSwitchingRef` + `statusGenerationRef.current++` (`bumpGen`) защищают от race с поллером `useDaemonStatus`. Поллер сбросит ответ, если поколение изменилось.
- Особо обрабатывает `errorCode === "tun_privileges"` — показывает диалог с предложением рестарта от админа.
- `selectAndConnect` для SECTION-типа сразу выкидывает алёрт «это секция, не сервер».
- **Замечание**: код очень длинный (`useDaemonControl.js:110-246` — `toggleConnection`, 248-371 — `selectAndConnect`). Эти два метода почти полностью одинаковые — отличаются только в branch для активного соединения. **Кандидат на DRY** (выделить общий `attemptConnect(candidates, isAuto)`).

### 7.3 `useDaemonStatus.js` (237 строк)

- **Polling**, не events: `setInterval(fetchStatus, 1000)` (`useDaemonStatus.js:231`).
- Использует генерационный счётчик: захватывает `genAtStart` до `await wailsAPI.getStatus()`, после ответа сравнивает с текущим — если поколение менялось, дропает ответ (`useDaemonStatus.js:54-61`).
- 3 страйка ошибок подряд → `daemonStatus="offline"` (`useDaemonStatus.js:222-226`).
- Kill switch popup: ловит «(а) был connected, стал disconnected + KS on» или «(б) connected, но proxyDead + KS on» и показывает диалог `«Отключить и снять блокировку»` (`useDaemonStatus.js:87-132`). При confirm дёргает `disconnect()` + `toggleKillSwitch(false)`.
- **Medium**: dependency array (`useDaemonStatus.js:233-249`) — слишком много значений, эффект пересоздаётся часто и интервал переинициализируется. Каждый сетевой запрос → новый колбек → новый интервал. (`daemonStatus`, `isConnected`, `settings` в зависимостях.)

### 7.4 `useDaemonPing.js` (155 строк)

- Polling каждые 60 секунд + manual через `refreshPings(ids)`.
- `inFlightRef` — defensive lock против повторного запуска.
- `pendingPingIds` — Set, обновляется через `flushSync` для синхронной видимости pending-состояний.
- Для AUTO-типа — мапит `extra.members` → их пинги.

### 7.5 `useLogs.js` (66 строк)

- `logs` — клиентские, обрезается до 50.
- `backendLogs` — подгружаются через `wailsAPI.getLogs(1, 100)` + слушаются через event `log` (Wails), обрезается до 500.
- **Замечание**: фронтенд-логи и бэкенд-логи смешиваются в LogsView (см. 4.5).

### 7.6 `useCheckUpdate.js` (91 строк)

- Resolves локальной версии: сначала `wailsAPI.getVersion()` (Go-side), fallback на `__APP_VERSION__` (vite-define).
- Тянет манифест: сначала через `window.go.main.App.GetUpdateManifest()` (Go-кешированный), fallback на `fetch(UPDATE_URL)` с кэш-бастером.
- `hasPlatformAsset` — true если в manifest есть хоть один платформенный asset с `url` и `sha256`.
- `compareVersions` (см. `utils/versionCheck.js`) — semver-aware.

---

## 8. Utils

### 8.1 `proxyParser.js` (853 строк) — **дублирует Go-парсер**

Парсит **на клиенте**:
- `ss://` — base64 auth + host (`parseShadowsocks`)
- `vmess://` — base64 JSON (`parseVMess`)
- `vless://` — query params + URI hash (`parseVLESS`)
- `trojan://` — query params (`parseTrojan`)
- `hy2://`, `hysteria2://` (`parseHysteria2`)
- `naive+https://`, `naive://`, NaiveProxy JSON-конфиг (`parseNaiveURI`, `parseNaiveClientJSON`)
- WireGuard `.conf` файлы (`parseWireGuardConf`) — включая Amnezia extensions (jc/jmin/jmax/s1-s4/h1-h4/i1-i5/itime/j1-j3)
- CSV (header: ip/port/login/password/type/name)
- Plain `ip:port:user:pass`

**High — Source of Truth violation**: в Go-бэке есть `internal/proxy/uriparser.go` (1794 строк) — те же протоколы. На фронте используется в:
- `AddProxyView.jsx:336, 538` — `parseProxies(text)` для local file/clipboard/manual paste
- `DeepLinkImportModal.jsx:139` — `parseProxies(text)` для deep-link с raw URI

При этом для подписок (HTTP-URL) идёт `wailsAPI.fetchSubscription` через Go — там парсит Go. Получается:
- raw URI (clipboard/file) → парсит JS.
- URL подписки → парсит Go.
- RVSUB1: → парсит Go (`parseSubscriptionText`).

Два парсера расходятся (например, в Trojan, в spx/flow, в trim'е alpn). Рекомендация: убрать JS-парсер, всё гнать через `parseSubscriptionText` (надо добавить Go-биндинг `ParseURI(text)` для одиночного URI).

Дополнительно `proxyParser.js` содержит чистые helpers, которые точно должны остаться на клиенте:
- `VPN_TYPES`, `isVpnType`, `isSubscriptionURL`, `isEncryptedSubscription` (`proxyParser.js:604-770`)
- `formatProxyDisplayName`, `subscriptionLabelFromURL`, `getProtocolLabel`
- `mergeSubscriptionRefreshCountries` — merge старых и новых записей подписки (с сохранением страны и ID для непрерывности пинга) — **корректная клиентская логика**.
- `readVpnTransportFieldsFromExtra`, `applyVpnTransportFieldsToExtra`, `sanitizeVpnExtraForEdit`, `parseProxyExtra` — для формы редактирования VPN.
- `rebuildSubscriptionsFromProxies` — реконструкция списка подписок по экспорту/импорту (`proxyParser.js:911-933`).

### 8.2 `crypto.js` (88 строк)

- **Не симметрично с `internal/config/crypto.go`**. У бэка ключ выводится из `machineID + installSalt` (приложенческий), у фронта — из user-supplied пароля.
- Web Crypto API: PBKDF2(SHA-256, 100k iter) + AES-GCM-256, salt 16B + IV 12B.
- Используется только в `SettingsView.executeExport`/`executeImport`.

**Medium — две системы экспорта параллельно**:
- Frontend: `_isSecure:true, _version:2, data: <base64(salt|iv|ct)>` — кастомный формат, AES-GCM PBKDF2 100k.
- Backend (`wailsAPI.exportConfig` → `App.ExportConfig`, минимум 8 символов): RESULTPROXY2-формат, который тоже шифрует, но другим способом — см. `internal/config/export_v2.go`.

В `SettingsView` используется **только** frontend-шифрование. `wailsAPI.exportConfig` объявлен, но ни одного callsite в `src/` нет. Если есть планы на унификацию — стоит решить, какой формат основной, и удалить второй.

**Medium — PBKDF2 100k iterations** — нижняя граница современных рекомендаций (OWASP советует 600k для SHA-256). На дешёвом железе атакующего перебор пароля заметно ускоряется по сравнению с 600k.

### 8.3 `wailsAPI.js` (354 строк)

- Тонкая обёртка над `wailsjs/go/main/App` со стандартной try/catch + console.error.
- Все API-методы Go-бэка (32 функции): connect/disconnect/ping, config CRUD, status, traffic/network, logs, mode/autostart/killswitch/adblock, rules, subscription CRUD, update.
- **Low — устаревшие имена параметров**: `connect: async (proxyStr, options, mode, processName)` (`wailsAPI.js:57`), но фактически передаётся `(candidate, routingRules, killswitch, adblock)`. Не баг (имена параметров в JS — лишь документация), но вводит в заблуждение.
- **Low — `syncProxies` параметр** называется `(url)` (`wailsAPI.js:199`), но передаётся массив прокси.
- **Low** — `addSubscription` теперь принимает `subscriptionSource` (для deep-link `rvsub` flag), но `wailsAPI.js:354` его помечает в комментариях. ОК.

### 8.4 `pingSort.js` (95 строк)

- `parseExtra(raw)` — универсальный decoder для `extra` (Uint8Array | string | object).
- `getPingSortMetric(proxy, pings)`: возвращает числовую метрику:
  - SECTION → `1e12` (всегда в конце).
  - AUTO → min ping членов или `1e11`.
  - Прокси с числовым ping → значение в мс.
  - `"Online"` → 500_000.
  - Ошибочные (`Timeout/Refused/Unreachable/Closed/Error/Unavailable`) → `700_000 + rank*100`.
  - Unknown/empty → `1e11`.
- `sortProxiesByOption(list, sortBy, pings)` — wrapper для default/country/type/newest/provider/ping.

### 8.5 `versionCheck.js` (65 строк)

- `compareVersions(v1, v2)` — semver-aware: ядро (`1.2.3`) сравнивает по числам, поддерживает prerelease (`1.2.3-rc.1`) с rule «no-prerelease > with-prerelease». Numeric и string prerelease различаются: numeric < string.
- Возвращает -1/0/1.

### 8.6 `network.js` (37 строк)

- `detectCountry(ip)` — для локальных IP (127.x, 192.168.x, 10.x, localhost) возвращает "local", иначе зовёт `wailsAPI.detectCountry`.
- **Low — неполный CIDR-чек**: пропускает 172.16-31, fd::/8, fe80::/10 IPv6.

### 8.7 `subscriptionSecurity.js` (9 строк)

- `INSECURE_SUBSCRIPTION_ERROR_MARKER` + `isInsecureSubscriptionError(err)` — детектор ошибки от Go-бэка для plaintext HTTP.

### 8.8 `formatters.js` (28 строк)

- `formatBytes` — MB/GB, нелокализованные единицы.
- `formatSpeed` — KB/s, MB/s.

---

## 9. i18n

### Конфиг (`lib/i18n.js`)
- Detector: `localStorage → navigator`, кэширует в `localStorage`.
- `fallbackLng: "en"`, `escapeValue: false` (React сам экранирует).
- 2 языка: `en`, `ru`.

### Локали
- `en.json` — 386 строк, `ru.json` — 386 строк. Покрытие практически идентичное.
- Структура:
  - `sidebar.*`, `home.*`, `add.*`, `proxyList.*`, `rules.*`, `settings.*`, `logs.*`, `buy.*`, `update.*`, `common.*`, `killswitchPopup.*`, `tunnel.*`, `deeplink.*`, `app.*`.

### Проблемы

- **High — захардкоженные русские строки в `MainLayout.jsx`** (мобильная nav, `MainLayout.jsx:90,102,111,117`): `"Главная"`, `"Добавить"`, `"Прокси"`, `"Настройки"` — без `t()`.
- **High — захардкоженные русские строки в `LogsView.jsx`/`useDaemonControl`/etc.**: все `addLog(...)` пишут русские строки прямо (`addLog("Отключение...", "info")`). Затем `LogsView.translateLog` пытается заменить эти подстроки через `startsWith` (`LogsView.jsx:22-86`). При смене формата строки в одной точке (например, в `useDaemonControl`) перевод отвалится молча. **Правильное решение**: писать ключи перевода в лог (`addLog({key: "logs.msg.disconnecting", level: "info"})`), а в `LogsView` переводить через `t()`.
- **Medium — fallback-строки прямо в JSX**: `t("home.modeProxy") || "ПРОКСИ"` (`HomeView.jsx:334`), `t("add.protocolWarningTitle") || "Важное уточнение"` (`ProtocolWarningModal.jsx:39`) — нарушает принцип «единственного места правды». При смене EN locale `||` оставит русский fallback.
- **Medium — Russian-only сообщения в alert-диалогах**: `useAppConfig.js:223`, `259`, `275` пишут `«Не удалось применить режим»`, `«Ошибка автостарта»`, `«Ошибка Kill Switch»` без `t()`.

---

## 10. Стили (Tailwind + кастом)

### Tailwind config (`tailwind.config.js`)
- Простейший — только расширение для анимации `marquee` (бегущая строка).
- Темы нет (`theme.extend` пустой кроме keyframes).

### Глобальные стили
- **`index.css`** — Tailwind directives, базовые типографика/скроллбары, тёмная палитра по умолчанию (`color: rgba(255,255,255,0.87)`, `background-color: #242424`).
- **`App.css`** — корневые правила `#root`, **тотальное `outline: none !important` для всех `*`** (`App.css:27-38`). Не дружит с a11y (см. раздел 12).
- **`MainLayout.jsx`** дублирует это inline-`<style>` (`MainLayout.jsx:56-71`).

### Цветовая палитра (повторяется в Tailwind-классах)
- Основной зелёный: `#007E3A` (action), `#00A819` (hover/highlight).
- Тёмный фон: `zinc-950`, `zinc-900`, `zinc-800` (карточки, фон, бордеры).
- Ошибки: `rose-400`/`rose-500`/`rose-600`.
- Предупреждения: `amber-400`/`amber-500`.
- Sub-section: `violet-300`/`violet-400` (для SECTION-типа).

**Замечание**: `#007E3A`/`#00A819` фигурируют в десятках классов — стоит вынести в `tailwind.config.theme.colors.brand` для семантики (`bg-brand-primary` вместо `bg-[#007E3A]`).

### Тема
- Только тёмная. `settings.theme` = `"dark"` хардкод. Светлой темы нет.

---

## 11. Потоки данных

### 11.1 Открытие приложения
```
main.jsx → <App/> → <AppProvider/>
  LogProvider → useLogs (subscribe to "log" event + fetch GetLogs(1,100))
  ConfigProvider → useAppConfig (GetPlatform + GetConfig + subscribe deeplink/config events)
    setIsConfigLoaded(true) → 
  ConnectionProvider → useDaemonStatus (start 1s polling) + useDaemonPing (start 60s polling) + useDaemonControl
<AppContent/> renders MainLayout + HomeView/ProxyListView/...
useCheckUpdate fires after mount → fetches manifest → may show UpdaterModal
```

### 11.2 Connect
```
HomeView → toggleConnection() (useDaemonControl)
  ↓
  bumpGen() → isSwitchingRef = true
  setIsConnecting(true) → UI shows "amber spin"
  wailsAPI.connect(candidate, routingRules, killswitch, adblock)
  ↓
  Go: App.Connect → proxy.Engine.Connect → sing-box config build → start engine
  ↓
  Returns ConnectResultDTO { success, errorCode, tunnelFailed, fallbackUsed, ... }
  ↓
  res.success → setIsConnected(true); res.errorCode==='tun_privileges' → show admin dialog
  bumpGen() releases lock
```

В параллели работает poller `useDaemonStatus` (1s): `wailsAPI.getStatus()` → `setIsConnected`, `setActiveProxy`, `setStats`, `setSpeedHistory`. Гонка с `toggleConnection` гасится `statusGenerationRef`.

### 11.3 Import URI
```
AddProxyView.handleVpnUriSubmit
  ↓
  isSubscriptionURL(text) ? wailsAPI.fetchSubscription(text) → Go-parser
  isEncryptedSubscription(text) ? wailsAPI.parseSubscriptionText(text) → Go (RVSUB1)
  else: parseProxies(text) — **JS-parser**
  ↓
  pendingProxies set → ProtocolSelectionModal opens
  ↓
  handleConfirmImport(protocol):
    if all sub one URL → wailsAPI.addSubscription(label, url, allowInsecure, source)
    else → handleBulkSaveProxies (через ConfigContext → useAppConfig)
  ↓
  setProxies → useEffect → wailsAPI.syncProxies → Go saves config
```

### 11.4 Deep-link `resultv://`
```
Go startup: detect launch with resultv:// arg → after frontend ready: EventsEmit("deeplink:received", payload)
  ↓
ConfigContext useEffect listens → setPendingDeepLink(text), setPendingDeepLinkSource(src)
  ↓
DeepLinkImportModal useEffect fires on pendingDeepLink change
  → stage="loading"
  → parses (subscription/encrypted/raw URI)
  → stage="preview" → ProtocolSelectionModal
  → user confirms → handleConfirm → addSubscription/bulkSave → stage="saving" → close
```

`reqId` ref (`DeepLinkImportModal.jsx:98`) гасит гонку при быстрой смене ссылки.

### 11.5 Update available
```
useCheckUpdate fetches manifest (Go-cached or remote)
  ↓
compareVersions(local, remote.version) === -1
  ↓
hasPlatformAsset ? UpdaterModal : UpdateNotificationModal
  ↓
[UpdaterModal] startUpdate → Go downloads, verifies SHA256, installs
  Events: update:progress, update:verifying, update:installing, update:failed
  ↓
On success: Go restarts app
On failure: phase="failed" → user can Retry or Browser fallback
```

---

## 12. Найденные проблемы и предложения

### High приоритет

1. **`AddProxyView.jsx` 2168 строк** — нечитаемо.
   Разбить на:
   - `AddProxyView/index.jsx` (диспетчер: edit vs add).
   - `AddProxyView/PlainProxyForm.jsx` (HTTP/HTTPS/SOCKS5).
   - `AddProxyView/VpnUriImport.jsx` (URI/subscription textarea + button).
   - `AddProxyView/forms/VlessVmessTrojanForm.jsx`.
   - `AddProxyView/forms/WireGuardForm.jsx` + `AmneziaEditor.jsx`.
   - `AddProxyView/forms/Hysteria2Form.jsx`.
   - `AddProxyView/forms/NaiveProxyForm.jsx`.
   - `AddProxyView/FileClipboardImport.jsx`.
   Большинство дублей формы для edit/add можно объединить через `mode = 'edit'|'create'` пропс.

2. **`proxyParser.js` дублирует `internal/proxy/uriparser.go`** (853 vs 1794 строк) — два source of truth.
   Парсеры расходятся в деталях (Trojan extras, alpn). Решение: добавить `App.ParseProxyURI(text) ([]ProxyEntry, error)` биндинг → удалить `parseProxies` из JS (оставить только клиентские helpers — `isVpnType`, `formatProxyDisplayName`, `getProtocolLabel`, `mergeSubscriptionRefreshCountries`, `subscriptionLabelFromURL`, `parseProxyExtra`, `readVpnTransportFieldsFromExtra`, `applyVpnTransportFieldsToExtra`, `sanitizeVpnExtraForEdit`, `rebuildSubscriptionsFromProxies`).

3. **`useAppConfig.js` — God-hook 462 строк**.
   Разделить:
   - `useAppConfigStorage` — proxies/routingRules/settings/subscriptions + persist через `saveConfig`.
   - `useAppDialog` — appDialog state + showAlertDialog/showConfirmDialog.
   - `useSubscriptionRefresh` — 6h interval refresh.
   - `useSettingsActions` — `updateSetting` со всеми ветками.

4. **Захардкоженные русские строки** (нарушение i18n):
   - `MainLayout.jsx:90,102,111,117` — мобильная nav, нет `t()`.
   - `useAppConfig.js:223,259,275` — alert-сообщения «Ошибка автостарта», «Ошибка Kill Switch» без `t()`.
   - Все `addLog()` пишут русский — `LogsView.translateLog` страдает от substring-matching.
   - Многочисленные `t(key) || "Русский fallback"` (HomeView, ProtocolWarningModal, AddProxyView): при пустом en-key fallback показывает русский.
   - **Решение**: миграция `addLog` на `{ key, params }`-форму; убрать `|| ".."` fallback’и; добавить мобильный nav в локали.

5. **`Connect` параметры в `wailsAPI.js`** — имена параметров устаревшие (`proxyStr, options, mode, processName`), фактически — `(candidate, routingRules, killswitch, adblock)`. Не баг, но критически вводит в заблуждение. Минут 5 поправить.

6. **Production-режим — `console.error/log` вызовы** во всех файлах `wailsAPI.js`, `useAppConfig.js`, `useDaemonStatus.js` остаются в build. Логи могут содержать чувствительные данные (subscription URL, proxy IP). Решение: обернуть в `import.meta.env.DEV` или удалить.

7. **`ProxyListView` — нет виртуализации**. При 500+ прокси (большая подписка) рендерится 500+ карточек → лаги при сортировке/поиске. Решение: `react-window` или `@tanstack/react-virtual`.

### Medium

8. **Контексты не мемоизированы** — `ConfigContext.jsx:73-83`, `ConnectionContext.jsx:114-137` создают `value`-объект inline на каждом рендере провайдера. Это инвалидирует контекст для всех потребителей.
   ```jsx
   const value = useMemo(() => ({...}), [deps]);
   ```

9. **`useDaemonStatus` dependency array** (`useDaemonStatus.js:233-249`) — `daemonStatus`, `settings`, `activeProxy` в зависимостях → эффект пересоздаётся каждые ~1 секунду (после поллера). Каждый раз `clearInterval` + новый `setInterval`. Решение: использовать `useRef` для актуальных значений + один setInterval на mount.

10. **`useDaemonControl` дублирование `toggleConnection`/`selectAndConnect`** — внутри 90% общий код (`useDaemonControl.js:110-371`). Выделить `attemptConnect(candidates, isAuto, addLog, ...)`.

11. **`AppDialogModal` не закрывается по Esc**. Только клик вне модалки или кнопки. Решение: `useEffect` подписывается на `keydown` Esc.

12. **`PBKDF2 100k iterations` в `crypto.js`** — нижняя граница. OWASP 2023 рекомендует 600k. На пользовательском пароле это даёт окно для брутфорса.

13. **Дублирование `ALLOWED_DOWNLOAD_HOSTS`** в `UpdaterModal.jsx:25` и `UpdateNotificationModal.jsx:28`. Вынести в `utils/updateSafety.js`.

14. **Два независимых пути экспорта конфига**: frontend AES-GCM (Web Crypto) vs Go ExportConfig (`RESULTPROXY2:`). Решить, который основной; удалить или переключить SettingsView на бэкенд-экспорт (тогда формат единый, HWID-binding можно реализовать без user-пароля).

15. **`SettingsView` использует `document.createElement('a')` для скачивания** (`SettingsView.jsx:91-102`) вместо native Wails `SaveFileDialog`. Это работает, но менее интегрировано (имя файла, расширение).

16. **`useDaemonPing` использует `flushSync` для pendingPingIds** (`useDaemonPing.js:98-100`). `flushSync` — anti-pattern в современном React, может ронять Concurrent-Mode оптимизации. Здесь, кажется, обязательно из-за того, что иначе кнопка не успевает «загореться» в pending. Альтернатива: разделить state на «инициирован» и «выполнен».

17. **Несинхронизированные `setSettings` в `SettingsView.handleImportClick`** — после import не происходит applyMode/syncProxies для autostart/killswitch (см. 4.4).

### Low

18. **Глобальный `outline: none !important`** (`App.css:27-38`, `MainLayout.jsx:56-71`) — нарушает a11y. Клавиатурные пользователи не видят фокус. Решение: убрать глобал, оставить только `:focus:not(:focus-visible)` стиль для скрытия mouse-focus.

19. **`detectCountry` не покрывает все приватные диапазоны** (`network.js:21-41`): нет 172.16-31, fd::/8, fe80::/10 IPv6.

20. **Нет тестов** во `frontend/`. Сложный `mergeSubscriptionRefreshCountries`, `compareVersions`, `pingSort.getPingSortMetric` без unit-тестов.

21. **Формат даты подписки** — захардкож DD.MM.YY (`ProxyListView.jsx:638-646`). Использовать `Intl.DateTimeFormat(i18n.language)`.

22. **`HoverMarquee` слушает `window.resize` для каждого экземпляра** (`HoverMarquee.jsx:33-37`). При 100+ карточках — 100 листенеров. Лучше через `ResizeObserver` или один глобальный listener в провайдере.

23. **Cloudflare-CDN зависимость для флагов** (`FlagIcon.jsx:51`) — если приложение запущено офлайн/блокируется CDN, флаги не покажутся. Уже есть fallback на text, но при первой загрузке висит. Решение: бандлить SVG локально.

24. **Нет CSP** — в `wails.json` нет CSP, в `index.html` тоже. При деплое расширений / сторонних скриптов это создаёт XSS-вектор.

25. **`a11y` — `aria-modal="true"`/`role="dialog"` не выставлены** на `AppDialogModal`, `ProtocolSelectionModal`, `UpdaterModal`. Screen reader не объявит модалку.

26. **`useId` используется только в `AppSelect`** — для остальных модалок дублируется hardcoded `htmlFor`/`id`. Не критично, но непоследовательно.

27. **`useConfigContext().value.t = useTranslation().t` не повторно используется** — каждый компонент вызывает `useTranslation()` сам. Это норма для i18next, но можно протащить через провайдер для perf.

### XSS / безопасность

28. **`dangerouslySetInnerHTML`, `eval`, `new Function`, `innerHTML` — отсутствуют** (`Grep` подтвердил). Это хороший знак.
29. **`window.open(downloadUrl, "_blank", "noopener,noreferrer")`** (`UpdateNotificationModal.jsx:96`) — корректно ограничивает opener.
30. **`isSafeDownloadURL`** проверяет host из allow-list — защита от рогового update.json. Корректная защита.

---

## TL;DR — Приоритетные действия

1. **Разбить `AddProxyView` (2168 LOC)** на 6-8 компонентов.
2. **Удалить `proxyParser.js` парсер**, использовать Go `ParseProxyURI`-биндинг.
3. **Разделить `useAppConfig` (462 LOC)** на 4 хука.
4. **Починить i18n**: убрать хардкод в MainLayout/useAppConfig/useDaemonControl, перевести `addLog` на ключи.
5. **Мемоизировать context value** (ConfigContext, ConnectionContext).
6. **Виртуализация ProxyListView** для подписок >100 серверов.
7. **Убрать `console.*` в production** или обернуть в DEV-guard.
8. **Поднять PBKDF2 итерации** до 600k в `crypto.js` (или унифицировать с Go-экспортом).
9. **Восстановить focus-стили** (a11y).
10. **Добавить unit-тесты** для `compareVersions`, `mergeSubscriptionRefreshCountries`, `getPingSortMetric`, `parseProxies`.
