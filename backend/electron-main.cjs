const { app, ipcMain } = require("electron");
const path = require("path");

// Устанавливаем принудительно директорию с данными
app.setPath("userData", path.join(app.getPath("appData"), "resultProxy"));

// Защита от двойного запуска
const gotTheLock = app.requestSingleInstanceLock();

if (!gotTheLock) {
  app.quit();
  process.exit(0);
}

// ---------------------------------------------------------
// 1. Dependency Injection - Импорт всех модулей
// ---------------------------------------------------------
const loggerService = require("./core/logger.service.cjs");
const authManager = require("./core/auth.manager.cjs");
const stateStore = require("./core/state.store.cjs");
const configManager = require("./config/config.manager.cjs");
const SystemFactory = require("./system/system.factory.cjs");
const ProxyManager = require("./proxy/proxy.manager.cjs");
const TrafficMonitor = require("./core/traffic.monitor.cjs");
const ApiServer = require("./api/express.server.cjs");
const WindowManager = require("./electron/window.manager.cjs");
const TrayManager = require("./electron/tray.manager.cjs");

// ---------------------------------------------------------
// 2. Инициализация (Сборка приложения)
// ---------------------------------------------------------
const systemAdapter = SystemFactory.getAdapter();
const windowManager = new WindowManager();
const proxyManager = new ProxyManager(loggerService, systemAdapter, stateStore);

// Важно передать proxyManager и trafficMonitor в TrayManager, а TrayManager в ApiServer
// Для решения циклических зависимостей, мы создаем их, а потом связываем если нужно,
// либо передаем ссылки.
const trafficMonitor = new TrafficMonitor(
  loggerService,
  stateStore,
  proxyManager,
  systemAdapter,
);
const trayManager = new TrayManager(
  stateStore,
  proxyManager,
  systemAdapter,
  windowManager,
  trafficMonitor,
  loggerService,
);
const apiServer = new ApiServer(
  loggerService,
  stateStore,
  configManager,
  proxyManager,
  trayManager,
  trafficMonitor,
  systemAdapter,
);

// ---------------------------------------------------------
// 3. Жизненный цикл Electron
// ---------------------------------------------------------

ipcMain.on("get-api-token", (event) => {
  event.returnValue = authManager.getToken();
});

app.on("second-instance", () => {
  windowManager.show();
});

app.whenReady().then(() => {
  // ПРЕДОХРАНИТЕЛЬ: ОЧИСТКА ПРИ ЗАПУСКЕ (Синхронно)
  if (
    systemAdapter &&
    typeof systemAdapter.disableSystemProxySync === "function"
  ) {
    systemAdapter.disableSystemProxySync();
  }

  // 1. Инициализируем конфиг
  configManager.init(app.getPath("userData"));
  const config = configManager.getConfig();
  if (config && config.settings && config.settings.killswitch) {
    stateStore.update({ killSwitch: true });
  } else {
    loggerService.log(
      "Конфиг не найден, используются настройки по умолчанию.",
      "warning",
    );
  }

  // 2. Запускаем сборщик процессов для белого списка
  systemAdapter.startProcessCacheInterval(() => stateStore.getState());

  // 3. Стартуем слои
  windowManager.create();
  trayManager.init();
  apiServer.start();
  trafficMonitor.start();
});

// ---------------------------------------------------------
// 4. Ловушки завершения работы
// ---------------------------------------------------------
app.on("session-end", () => {
  if (
    systemAdapter &&
    typeof systemAdapter.disableSystemProxySync === "function"
  ) {
    systemAdapter.disableSystemProxySync();
  }
});

app.on("before-quit", async () => {
  app.isQuitting = true;
  trafficMonitor.stop();

  if (
    systemAdapter &&
    typeof systemAdapter.disableSystemProxySync === "function"
  ) {
    systemAdapter.disableSystemProxySync();
  } else {
    await systemAdapter.disableSystemProxy();
  }

  // Принудительно гасим серверы
  await proxyManager.setSystemProxy(false);
});
