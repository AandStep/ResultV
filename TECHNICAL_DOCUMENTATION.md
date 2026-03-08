# Техническая документация: ResultProxy

Данный документ содержит подробное техническое описание архитектуры, компонентов и процессов работы десктопного приложения **ResultProxy** (управление прокси-серверами).

## 1. Общее описание

**ResultProxy** — кроссплатформенное десктопное приложение для управления подключениями к прокси-серверам. Приложение позволяет удобно переключаться между различными прокси (HTTP/HTTPS, SOCKS5), управлять правилами маршрутизации (глобальный и smart-режим), использовать Kill Switch и мониторить сетевой трафик в реальном времени.

**Стек технологий:**

- **Окружение:** Node.js, Electron 40
- **Backend (Main Process):** `express` 5, `cors`, `proxy-chain`, `socks`
- **Frontend (Renderer Process):** React 19, Vite 7, TailwindCSS 3, `i18next` / `react-i18next` (локализация ru/en), `lucide-react` (иконки)
- **Сборка:** `electron-builder`, `concurrently`, `wait-on`, `cross-env`
- **Безопасность:** AES-256-GCM (конфиг), токен-авторизация API, Context Isolation

---

## 2. Архитектура приложения

Приложение построено по классической модели Electron с разделёнными Backend (Main Process) и Frontend (Renderer Process). Для коммуникации между процессами используется **локальный HTTP API (Express)**, а не стандартный IPC Bridge Electron. Единственное исключение — передача токена авторизации через IPC (`preload.cjs`).

### 2.1. Взаимодействие Frontend и Backend

1. **Backend (Electron Main Process)** поднимает Express-сервер на `127.0.0.1:14080`.
2. **Frontend (React)** отправляет авторизованные REST API запросы через обёртку `apiFetch()`, которая автоматически прикрепляет Bearer-токен из `window.electronAPI.getApiToken()`.
3. Это решение обеспечивает высокую модульность и возможность прозрачного тестирования бекенда через Postman или cURL.

### 2.2. Безопасность коммуникации

- **Токен-авторизация:** При запуске `AuthManager` генерирует случайный UUID-токен (`crypto.randomUUID()`). Токен передаётся во Frontend через IPC-канал `get-api-token` (синхронный `ipcRenderer.sendSync`). Все HTTP-запросы от Frontend содержат заголовок `Authorization: Bearer <token>`, который верифицируется middleware в Express.
- **Context Isolation:** В `preload.cjs` используется `contextBridge.exposeInMainWorld` для безопасного экспорта минимального API (`getApiToken`) в Renderer Process. В BrowserWindow `nodeIntegration: false`, `contextIsolation: true`.

---

## 3. Детальное описание Backend (Electron Main)

Точка входа: `backend/electron-main.cjs`

Код бекенда построен с использованием паттерна **Dependency Injection**, где модули получают зависимости через конструкторы. Это решает проблему циклических зависимостей и упрощает поддержку. Порядок инициализации строго определён в `electron-main.cjs`.

### 3.1. Основные модули Backend

#### `config/crypto.service.cjs` (CryptoService)

Сервис шифрования конфигурации, привязанный к аппаратному идентификатору машины:

- **Hardware ID:** Получает уникальный идентификатор машины через системные команды:
  - Windows: `MachineGuid` из реестра
  - macOS: `IOPlatformUUID` через `ioreg`
  - Linux: `/etc/machine-id`
- **Fallback:** Если HW ID недоступен, генерируется случайный UUID и сохраняется в `%APPDATA%/resultProxy/.machine-fallback-id`.
- **Ключ шифрования:** 256-бит, получен через `SHA-256` от `machineId + "_ResultProxy_SafeVault_v1"`.
- **Алгоритм:** AES-256-GCM с случайным IV (16 байт). Зашифрованные данные хранятся в JSON с полями `_isSecure`, `iv`, `data`, `authTag`.
- **Обратная совместимость:** Если файл не зашифрован (нет `_isSecure`), парсится как обычный JSON.

#### `config/config.manager.cjs` (ConfigManager)

Читает, валидирует и сохраняет конфигурацию приложения в зашифрованный файл `proxy_config.json` в `userData` Electron:

- При загрузке: расшифровка через `CryptoService.decrypt()`.
- При сохранении: шифрование через `CryptoService.encrypt()`.
- Значения по умолчанию: `routingRules.mode = "global"`, `whitelist = ["localhost", "127.0.0.1"]`, `appWhitelist = []`.

#### `core/auth.manager.cjs` (AuthManager)

Генерирует одноразовый UUID-токен при запуске приложения. Предоставляет метод `verifyRequest(req)` для проверки HTTP-заголовка `Authorization: Bearer <token>`.

#### `core/state.store.cjs` (StateStore)

In-Memory хранилище текущего состояния, наследующее `EventEmitter`. Эмитит событие `change` при каждом обновлении. Хранимые поля:

| Поле                | Тип       | Описание                                    |
| ------------------- | --------- | ------------------------------------------- |
| `isConnected`       | `boolean` | Активно ли подключение к прокси             |
| `activeProxy`       | `object`  | Данные текущего активного прокси            |
| `bytesSent`         | `number`  | Отправлено байт за сессию                   |
| `bytesReceived`     | `number`  | Получено байт за сессию                     |
| `speedReceived`     | `number`  | Текущая скорость скачивания (byte/s)        |
| `speedSent`         | `number`  | Текущая скорость отправки (byte/s)          |
| `isProxyDead`       | `boolean` | Прокси перестал отвечать на ping            |
| `killSwitch`        | `boolean` | Активен ли Kill Switch                      |
| `uiProxies`         | `array`   | Кеш списка прокси из UI (для Tray)          |
| `lastTickStats`     | `object`  | Данные последнего тика мониторинга трафика  |
| `sessionStartStats` | `object`  | Данные сетевых интерфейсов на старте сессии |

#### `core/logger.service.cjs` (LoggerService)

Централизованное логгирование с кольцевым буфером (максимум 100 записей). Каждая запись содержит `timestamp`, `time`, `msg`, `type`. Вывод дублируется в консоль и доступен через API `/api/logs`.

#### `core/traffic.monitor.cjs` (TrafficMonitor)

Два параллельных интервала:

1. **Ping-мониторинг** (каждые 3 секунды):
   - TCP-сокет с таймаутом 2 секунды к `activeProxy.ip:port`.
   - При падении пинга: устанавливает `isProxyDead: true`, при активном Kill Switch вызывает `ProxyManager.applyKillSwitch()`.
   - При восстановлении пинга: автоматически переподключает прокси через `ProxyManager.setSystemProxy()`.

2. **Мониторинг трафика** (каждую секунду):
   - Опрашивает системные сетевые интерфейсы через `SystemAdapter.getNetworkTraffic()`.
   - Рассчитывает дельту трафика и мгновенную скорость (с порогом > 2048 byte/s для фильтрации шума).

#### `proxy/proxy.manager.cjs` (ProxyManager)

Управляет локальными прокси-серверами-мостами:

- **Логика моста:** Если прокси требует авторизацию (username/password) или тип SOCKS5, поднимает локальный мост на `127.0.0.1:14081`:
  - **SOCKS5** → `SocksServer` (raw TCP сервер с ручным SOCKS5 хендшейком)
  - **HTTP с Auth** → `HttpServer` (через `proxy-chain`)
  - **HTTP без Auth** → прямое использование без моста
- После запуска моста прописывает `127.0.0.1:14081` как системный прокси через `SystemAdapter`.
- Метод `setSystemProxy(enable, proxy, updateRegistryOnly)` — `updateRegistryOnly` позволяет обновить только реестр (при горячем обновлении правил без перезапуска серверов).
- Метод `applyKillSwitch()` — делегирует блокировку трафика системному адаптеру.

#### `proxy/http.server.cjs` (HttpServer)

Локальный HTTP-прокси мост на `proxy-chain`. Поддерживает:

- **Routing Rules:** Проверяет каждый запрос через `prepareRequestFunction`:
  - `appWhitelist` — проверка процесса-источника запроса через `SystemAdapter.checkAppWhitelist()`.
  - `whitelist` — доменный белый список (прямое соединение без прокси).
  - `smart-режим` — проксирование только заблокированных ресурсов (Instagram, Facebook, Twitter/X, Telegram, Discord, Netflix).
  - `global-режим` — весь трафик через прокси.

#### `proxy/socks.server.cjs` (SocksServer)

Raw TCP сервер с ручной реализацией SOCKS5 протокола. Поддерживает полный handshake (версия, методы аутентификации, CONNECT-запрос). Проксирует соединения через `SocksClient` к удалённому SOCKS5 серверу. Применяет те же правила маршрутизации (whitelist, appWhitelist, smart-режим).

#### `system/system.factory.cjs` (SystemFactory)

Паттерн **Factory + Facade**, предоставляющий единый интерфейс для трёх ОС:

```
system/
├── system.factory.cjs          # Factory + Facade
├── network/
│   ├── BaseNetworkManager.cjs  # Абстрактный базовый класс
│   ├── WindowsNetwork.cjs      # Сбор трафика через netstat/typeperf
│   ├── MacNetwork.cjs          # Сбор трафика через netstat -ib
│   └── LinuxNetwork.cjs        # Сбор трафика через /proc/net/dev
├── proxy/
│   ├── BaseProxyManager.cjs    # Абстрактный базовый класс
│   ├── WindowsProxy.cjs        # Реестр Windows + netsh (Kill Switch)
│   ├── MacProxy.cjs            # networksetup (macOS)
│   └── LinuxProxy.cjs          # gsettings / environment vars
└── process/
    ├── BaseProcessManager.cjs  # Кеш процессов + checkAppWhitelist
    ├── WindowsProcess.cjs      # netstat + tasklist
    ├── MacProcess.cjs          # lsof + ps
    └── LinuxProcess.cjs        # /proc/net/tcp + /proc/pid/cmdline
```

Facade API:

| Метод                                | Описание                                       |
| ------------------------------------ | ---------------------------------------------- |
| `setSystemProxy(ip, port, type, wl)` | Установить системный прокси в ОС               |
| `disableSystemProxy()`               | Отключить системный прокси                     |
| `disableSystemProxySync()`           | Синхронное отключение (при shutdown)           |
| `applyKillSwitch()`                  | Заблокировать весь трафик                      |
| `getNetworkTraffic()`                | Получить входящий/исходящий трафик (bytes)     |
| `startProcessCacheInterval()`        | Запустить периодический кеш процессов          |
| `checkAppWhitelist(port, wl, host)`  | Проверить, находится ли процесс в белом списке |

#### `electron/window.manager.cjs` (WindowManager)

Управление окном Electron: создание (1050×780), preload, сворачивание в трей при закрытии (перехват `close`), переключение видимости, автоматический выбор URL (localhost:5173 в dev / file:// в production).

#### `electron/tray.manager.cjs` (TrayManager)

Системный трей с контекстным меню:

- Отображение статуса подключения.
- Список сохранённых серверов (из `stateStore.uiProxies`) с возможностью быстрого переключения.
- Кнопки: «Отключить защиту», «Развернуть окно», «Выход».
- Иконка с fallback (встроенный base64 PNG).

#### `electron/preload.cjs`

Минимальный preload-скрипт. Экспортирует единственный метод `electronAPI.getApiToken()` через `contextBridge`.

#### `api/express.server.cjs` (ApiServer)

REST API на Express 5, слушает `127.0.0.1:14080`.

**Middleware:** CORS (origins: `localhost:5173`, `file://`, `electron://`) + JSON parser + Bearer-токен авторизация.

**Эндпоинты:**

| Метод  | Путь                  | Описание                                                       |
| ------ | --------------------- | -------------------------------------------------------------- |
| `GET`  | `/api/status`         | Текущее состояние (подключение, трафик, скорость, dead)        |
| `GET`  | `/api/logs`           | Кольцевой буфер логов (до 100 записей)                         |
| `GET`  | `/api/config`         | Полный конфиг приложения (зашифрованный на диске)              |
| `POST` | `/api/config`         | Сохранить конфиг (шифрование + запись)                         |
| `POST` | `/api/connect`        | Подключиться к прокси                                          |
| `POST` | `/api/disconnect`     | Отключиться от прокси                                          |
| `POST` | `/api/ping`           | Проверить доступность хоста (TCP, таймаут 2с)                  |
| `POST` | `/api/killswitch`     | Включить/выключить Kill Switch                                 |
| `POST` | `/api/sync-proxies`   | Синхронизация списка прокси из UI в Tray                       |
| `POST` | `/api/update-rules`   | Горячее обновление правил маршрутизации                        |
| `POST` | `/api/autostart`      | Управление автозапуском (`app.setLoginItemSettings`)           |
| `POST` | `/api/detect-country` | Гео-определение страны по IP (через ip-api.com)                |
| `GET`  | `/api/platform`       | Текущая ОС (`os.platform()`)                                   |
| `GET`  | `/api/version`        | Версия приложения (dev: из package.json, prod: app.getVersion) |

---

## 4. Детальное описание Frontend (React + Vite)

Точка входа: `src/main.jsx` → `src/App.jsx`

Фронтенд — SPA (Single Page Application), стилизованное с помощью TailwindCSS 3. Дизайн приложения ориентирован на удобство управления списками и настройками.

### 4.1. Архитектура Frontend

#### Контексты (Context API)

Контексты организованы по слоям, каждый отвечает за минимальную зону ответственности. Это исключает нежелательные ререндеры (в отличие от единого `useAppContext`):

| Контекст            | Провайдер            | Описание                                                                                 |
| ------------------- | -------------------- | ---------------------------------------------------------------------------------------- |
| `LogContext`        | `LogProvider`        | Хранит UI-логи (до 50) и backend-логи. Предоставляет `addLog()`.                         |
| `ConfigContext`     | `ConfigProvider`     | Конфигурация (proxies, routingRules, settings), навигация (`activeTab`), `editingProxy`. |
| `ConnectionContext` | `ConnectionProvider` | Состояние подключения, статистика, пинги, управление соединением.                        |

Обёртка `AppProvider` вкладывает их в порядке: `LogProvider` → `ConfigProvider` → `ConnectionProvider`.

#### Хуки (Hooks)

| Хук                | Файл                        | Описание                                                                                                                                                           |
| ------------------ | --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `useLogs`          | `hooks/useLogs.js`          | UI-лог (50 записей LIFO) + backend-лог (polling `/api/logs` каждые 1.5с). Экспортирует `apiFetch()` — глобальную обёртку над `fetch` с авторизацией.               |
| `useAppConfig`     | `hooks/useAppConfig.js`     | Загрузка конфига при старте, автосохранение при изменениях, определение ОС, сохранение/обновление прокси, гео-определение страны.                                  |
| `useDaemonStatus`  | `hooks/useDaemonStatus.js`  | Polling `/api/status` каждую 1с. Трекинг: `isProxyDead`, скорость, статистика, `daemonStatus` (online/offline/checking), история скоростей (20 точек для графика). |
| `useDaemonPing`    | `hooks/useDaemonPing.js`    | Периодический (10с) ping всех сохранённых прокси через `/api/ping`.                                                                                                |
| `useDaemonControl` | `hooks/useDaemonControl.js` | Команды: `toggleConnection`, `selectAndConnect`, `deleteProxy`. Race-condition защита через `isSwitchingRef`.                                                      |
| `useCheckUpdate`   | `hooks/useCheckUpdate.js`   | Проверка обновлений: сравнение локальной версии (`/api/version`) с удалённой (GitHub Raw `update.json`). Semver сравнение через `compareVersions`.                 |

#### Представления (Views) и маршрутизация

Навигация реализована в `App.jsx` через стейт `activeTab` (без `react-router-dom`):

| View            | Tab        | Описание                                                                                                                      |
| --------------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `HomeView`      | `home`     | Главный экран: кнопка подключения, активный прокси, Kill Switch, мониторинг скорости (график SpeedChart), статистика трафика. |
| `ProxyListView` | `list`     | Список прокси: сортировка, фильтрация по IP/имени/стране, отображение пинга, флаги стран (FlagIcon), выбор активного.         |
| `RulesView`     | `rules`    | Настройка правил маршрутизации: режимы (global/smart), белый список доменов, белый список приложений.                         |
| `AddProxyView`  | `add`      | Форма добавления/редактирования прокси (IP, порт, тип, авторизация, имя).                                                     |
| `BuyProxyView`  | `buy`      | Форма покупки прокси.                                                                                                         |
| `LogsView`      | `logs`     | Терминалообразный вывод логов (UI + backend, объединённые по timestamp).                                                      |
| `SettingsView`  | `settings` | Настройки: язык (ru/en), автозапуск, Kill Switch, удаление данных, экспорт/импорт конфигурации.                               |

#### Компоненты

**Layout (`src/components/layout/`):**

| Компонент      | Описание                                             |
| -------------- | ---------------------------------------------------- |
| `MainLayout`   | Основной Layout: Sidebar + область контента.         |
| `Sidebar`      | Боковая навигация с вкладками, статусом подключения. |
| `MobileHeader` | Адаптивный header для мобильных разрешений.          |

**UI (`src/components/ui/`):**

| Компонент                 | Описание                                                   |
| ------------------------- | ---------------------------------------------------------- |
| `FlagIcon`                | Отображение флага страны по ISO-коду (через flagcdn.com).  |
| `LanguageSwitcher`        | Переключатель языков (ru/en) с использованием `i18next`.   |
| `SettingToggle`           | Универсальный toggle-переключатель для настроек.           |
| `SpeedChart`              | Мини-график скорости на основе истории (20 точек, Canvas). |
| `UpdateNotificationModal` | Модальное окно уведомления о доступном обновлении.         |

### 4.2. Утилиты

| Файл                    | Описание                                                                                                                                                      |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `utils/network.js`      | Многоуровневое гео-определение страны по IP через 4 API: `iplocation.net` → `ip-api.com` → `geojs.io` → `country.is`. Таймаут 2с на каждый. Fallback-цепочка. |
| `utils/crypto.js`       | Клиентское шифрование AES-256-GCM через Web Crypto API с PBKDF2 (100 000 итераций). Для экспорта/импорта конфигурации с паролем пользователя.                 |
| `utils/formatters.js`   | `formatBytes()` — MB/GB, `formatSpeed()` — KB/s / MB/s.                                                                                                       |
| `utils/versionCheck.js` | `compareVersions()` — semver-сравнение строк `major.minor.patch`.                                                                                             |

### 4.3. Локализация

Система: `i18next` + `react-i18next` + `i18next-browser-languagedetector`.
Конфигурация: `src/lib/i18n.js`.
Файлы переводов: `src/locales/ru.json`, `src/locales/en.json`.
LanguageDetector автоматически определяет язык при первом запуске.

---

## 5. Процессы (Flow)

### 5.1. Инициализация приложения

1. Electron запускается, устанавливает `userData` путь, запрашивает single instance lock.
2. `AuthManager` генерирует UUID-токен.
3. IPC-хендлер `get-api-token` регистрируется для передачи токена в Renderer.
4. `app.whenReady()`:
   - **Предохранитель:** `disableSystemProxySync()` — очистка системного прокси при запуске (на случай аварийного завершения).
   - Инициализация `ConfigManager` (чтение + расшифровка конфига).
   - Загрузка Kill Switch из сохранённых настроек.
   - Запуск кеша процессов (`startProcessCacheInterval()`).
   - Создание окна, трея, API-сервера, мониторинга трафика.
5. Frontend загружается, `useLogs.apiFetch` получает токен через `window.electronAPI.getApiToken()`.
6. `useAppConfig` загружает конфигурацию через `/api/config` и ОС через `/api/platform`.
7. `useDaemonStatus` начинает polling `/api/status` (1с).
8. `useDaemonPing` начинает polling `/api/ping` для всех прокси (10с).

### 5.2. Поток подключения прокси

1. Пользователь вызывает `toggleConnection()` или `selectAndConnect(proxy)` во Frontend.
2. Frontend отправляет POST `/api/connect` с данными прокси, правилами маршрутизации и флагом killswitch.
3. Express вызывает `TrafficMonitor.pingProxy()` (TCP-соединение с таймаутом 2с).
4. Если прокси недоступен и Kill Switch активен — подключение отмечается как `isProxyDead`, но продолжается (будет ждать восстановления).
5. Если прокси недоступен и Kill Switch неактивен — ошибка.
6. В случае успеха:
   - Обнуляется статистика сессии (фиксируется baseline сетевых интерфейсов).
   - `ProxyManager.setSystemProxy(true, proxy)`:
     - Если SOCKS5 → запускается `SocksServer` на `127.0.0.1:14081`.
     - Если HTTP с авторизацией → запускается `HttpServer` (proxy-chain) на `127.0.0.1:14081`.
     - Если HTTP без авторизации → используется напрямую.
     - `SystemAdapter.setSystemProxy()` прописывает прокси в ОС.
   - Обновляется состояние и Tray меню.
7. Frontend через polling `/api/status` получает обновлённое состояние и отображает трафик/скорость.

### 5.3. Срабатывание Kill Switch

1. `TrafficMonitor` (интервал 3с) обнаруживает, что ping к активному прокси упал.
2. Устанавливается `isProxyDead: true` в `StateStore`.
3. Если `killSwitch === true`:
   - `ProxyManager.applyKillSwitch()` → `SystemAdapter.applyKillSwitch()` блокирует весь сетевой трафик на уровне ОС (Windows: netsh/брандмауэр, macOS: pfctl, Linux: iptables).
4. Frontend через polling получает `isProxyDead: true` и визуально уведомляет пользователя.
5. При восстановлении пинга:
   - `TrafficMonitor` автоматически вызывает `ProxyManager.setSystemProxy(true, activeProxy, true)` (updateRegistryOnly), восстанавливая доступ.
   - `isProxyDead: false` → Frontend обновляет интерфейс.

### 5.4. Горячее обновление правил маршрутизации

1. Frontend обнаруживает изменение `routingRules` через `useEffect` в `useAppConfig`.
2. Автоматически отправляет POST `/api/update-rules` с новыми правилами.
3. Backend обновляет `activeProxy.rules` в `StateStore`.
4. Вызывает `ProxyManager.setSystemProxy(true, updatedProxy, true)` — обновляет только реестр/конфигурацию ОС без перезапуска серверов-мостов.

### 5.5. Режимы маршрутизации

| Режим    | Описание                                                                                                                                    |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `global` | Весь трафик идёт через прокси (кроме whitelist доменов и appWhitelist).                                                                     |
| `smart`  | Через прокси идут только заблокированные ресурсы (Instagram, Facebook, Twitter/X, Telegram, Discord, Netflix). Остальной трафик — напрямую. |

Дополнительно: `whitelist` — список доменов, трафик к которым всегда идёт напрямую. `appWhitelist` — список приложений (по имени процесса), чей трафик идёт напрямую.

### 5.6. Проверка обновлений

1. `useCheckUpdate` при старте запрашивает текущую версию через `/api/version`.
2. Загружает `update.json` с GitHub Raw (с cache buster `?_t=timestamp`).
3. Сравнивает через `compareVersions()` (semver).
4. Если доступна новая версия — отображает `UpdateNotificationModal` с ссылкой на скачивание (dismissable через sessionStorage).

---

## 6. Завершение работы и предохранители

| Событие           | Действие                                                                                       |
| ----------------- | ---------------------------------------------------------------------------------------------- |
| `session-end`     | Синхронное отключение системного прокси (`disableSystemProxySync`).                            |
| `before-quit`     | Остановка TrafficMonitor, синхронное/асинхронное отключение прокси, остановка серверов-мостов. |
| `second-instance` | Показ окна (single instance lock).                                                             |
| `close` (окно)    | Скрытие в трей вместо закрытия (`event.preventDefault()`).                                     |
| Запуск            | Предохранитель: `disableSystemProxySync()` — очистка прокси предыдущей сессии.                 |

---

## 7. Сборка и Деплой

Система собирается с помощью `electron-builder` в исполняемые файлы. Конфигурация в секции `build` файла `package.json`.

**Поддерживаемые платформы:**

| Платформа | Формат           | Команда                 |
| --------- | ---------------- | ----------------------- |
| Windows   | NSIS             | `npm run package`       |
| Linux     | AppImage, tar.gz | `npm run package:linux` |
| macOS     | DMG              | (ручная настройка)      |

**Режим разработки:**

```bash
npm run dev
```

Конкурентно поднимает Vite-сервер (порт 5173) и Electron в dev-режиме (ожидая пока порт Vite станет доступен через `wait-on`).

**Структура production-бандла:**

```
release/
├── dist/         # Скомпилированный React (vite build)
├── backend/      # Backend Node.js модули (as-is)
└── package.json
```

---

## 8. Файловая структура проекта

```
ResultProxy/
├── backend/
│   ├── electron-main.cjs           # Точка входа, DI, жизненный цикл
│   ├── api/
│   │   └── express.server.cjs      # REST API (14 эндпоинтов)
│   ├── config/
│   │   ├── config.manager.cjs      # Менеджер конфигурации
│   │   └── crypto.service.cjs      # AES-256-GCM шифрование
│   ├── core/
│   │   ├── auth.manager.cjs        # Токен-авторизация
│   │   ├── logger.service.cjs      # Логгирование (100 записей)
│   │   ├── state.store.cjs         # In-Memory стейт (EventEmitter)
│   │   └── traffic.monitor.cjs     # Мониторинг пинга и трафика
│   ├── proxy/
│   │   ├── proxy.manager.cjs       # Управление мостами
│   │   ├── http.server.cjs         # HTTP-мост (proxy-chain)
│   │   └── socks.server.cjs        # SOCKS5-мост (raw TCP)
│   ├── electron/
│   │   ├── window.manager.cjs      # Управление окном
│   │   ├── tray.manager.cjs        # Системный трей
│   │   └── preload.cjs             # Context Bridge (getApiToken)
│   └── system/
│       ├── system.factory.cjs      # Factory + Facade
│       ├── network/                # Сбор трафика (Win/Mac/Linux)
│       ├── proxy/                  # Системный прокси (Win/Mac/Linux)
│       └── process/                # Менеджер процессов (Win/Mac/Linux)
├── src/
│   ├── main.jsx                    # Точка входа React
│   ├── App.jsx                     # Маршрутизация (activeTab)
│   ├── context/
│   │   ├── AppContext.jsx          # Обёртка провайдеров
│   │   ├── LogContext.jsx          # Логи
│   │   ├── ConfigContext.jsx       # Конфигурация + навигация
│   │   └── ConnectionContext.jsx   # Подключение + статистика
│   ├── hooks/
│   │   ├── useLogs.js              # Логи + apiFetch()
│   │   ├── useAppConfig.js         # Конфигурация
│   │   ├── useDaemonStatus.js      # Polling статуса (1с)
│   │   ├── useDaemonPing.js        # Polling пингов (10с)
│   │   ├── useDaemonControl.js     # Команды подключения
│   │   ├── useDaemonAPI.js         # Агрегатный хук (если используется)
│   │   └── useCheckUpdate.js       # Проверка обновлений
│   ├── views/                      # 7 view-компонентов
│   ├── components/
│   │   ├── layout/                 # MainLayout, Sidebar, MobileHeader
│   │   └── ui/                     # FlagIcon, LanguageSwitcher, SettingToggle, SpeedChart, UpdateNotificationModal
│   ├── utils/
│   │   ├── network.js              # Гео-определение (4 API fallback)
│   │   ├── crypto.js               # Web Crypto AES-256-GCM (PBKDF2)
│   │   ├── formatters.js           # Форматирование байтов/скорости
│   │   └── versionCheck.js         # Semver сравнение
│   ├── locales/
│   │   ├── ru.json                 # Русская локализация
│   │   └── en.json                 # Английская локализация
│   └── lib/
│       └── i18n.js                 # Конфигурация i18next
├── package.json                    # v2.0.0, зависимости, скрипты сборки
├── vite.config.js                  # Конфигурация Vite
├── tailwind.config.js              # Конфигурация TailwindCSS
└── update.json                     # Данные для проверки обновлений
```
