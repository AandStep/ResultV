const { app, ipcMain, dialog } = require("electron");
const path = require("path");
const { execSync } = require("child_process");

// Устанавливаем принудительно директорию с данными
app.setPath("userData", path.join(app.getPath("appData"), "resultProxy"));

// Защита от двойного запуска
const gotTheLock = app.requestSingleInstanceLock();

if (!gotTheLock) {
  app.quit();
  process.exit(0);
}

// ---------------------------------------------------------
// Проверка наличия прав администратора (Windows)
// ---------------------------------------------------------
function isAdmin() {
  if (process.platform !== "win32") return false;
  try {
    execSync("net session", { stdio: "ignore" });
    return true;
  } catch (e) {
    return false;
  }
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
const systemAdapter = SystemFactory.getAdapter(loggerService);
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

ipcMain.on("is-admin", (event) => {
  event.returnValue = isAdmin();
});

ipcMain.on("restart-as-admin", () => {
  if (process.platform === "win32") {
    const exePath = app.isPackaged ? process.execPath : process.argv[0];
    const args = app.isPackaged ? [] : process.argv.slice(1);

    try {
      const { exec } = require("child_process");

      // Явно освобождаем Lock, чтобы новый запущенный класс не закрылся
      // сразу же, подумав, что старая копия всё ещё работает.
      app.releaseSingleInstanceLock();

      // Запоминаем выбор пользователя: запускать всегда от админа
      if (typeof systemAdapter.setRunAsAdminFlag === "function") {
        systemAdapter.setRunAsAdminFlag(true);
      }

      const argStr = args.length > 0 ? `-ArgumentList '${args.join(" ")}'` : "";
      const psCmd = `powershell -WindowStyle Hidden -Command "Start-Process -FilePath '${exePath}' ${argStr} -Verb RunAs"`;

      exec(psCmd, (error) => {
        if (!error) {
          app.isQuitting = true;
          app.quit();
        } else {
          loggerService.log(
            `[СИСТЕМА] Ошибка UAC запроса или отмена: ${error.message}`,
            "error",
          );
          // Возвращаем Lock, так как пользователь отменил запрос, и текущее приложение продолжает работу
          app.requestSingleInstanceLock();
        }
      });
    } catch (e) {
      loggerService.log(
        `[СИСТЕМА] Ошибка запуска с правами админа: ${e.message}`,
        "error",
      );
      app.requestSingleInstanceLock();
    }
  }
});

app.on("second-instance", () => {
  windowManager.show();
});

app.whenReady().then(async () => {
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
    if (process.platform === "win32" && !isAdmin()) {
      const { dialog } = require("electron");
      const result = dialog.showMessageBoxSync({
        type: "warning",
        title: "Требуются права администратора",
        message:
          "Функция Kill Switch включена, но приложению не хватает прав администратора для работы с файрволом.\n\nПерезапустить приложение от имени администратора?",
        buttons: ["Да, перезапустить", "Нет, отключить Kill Switch"],
        defaultId: 0,
        cancelId: 1,
      });

      if (result === 0) {
        // Перезапуск с правами админа
        const exePath = app.isPackaged ? process.execPath : process.argv[0];
        const args = app.isPackaged ? [] : process.argv.slice(1);

        let shouldQuit = false;
        try {
          const { exec } = require("child_process");

          app.releaseSingleInstanceLock();

          // Запоминаем выбор пользователя: запускать всегда от админа
          if (typeof systemAdapter.setRunAsAdminFlag === "function") {
            systemAdapter.setRunAsAdminFlag(true);
          }

          const argStr =
            args.length > 0 ? `-ArgumentList '${args.join(" ")}'` : "";
          const psCmd = `powershell -WindowStyle Hidden -Command "Start-Process -FilePath '${exePath}' ${argStr} -Verb RunAs"`;

          shouldQuit = await new Promise((resolve) => {
            exec(psCmd, (error) => {
              if (!error) {
                resolve(true); // Успех, нужно завершить это приложение
              } else {
                loggerService.log(
                  `[СИСТЕМА] Ошибка UAC запроса или отмена: ${error.message}`,
                  "error",
                );
                // Если пользователь отменил UAC - продолжаем загрузку с выключенным kill switch
                app.requestSingleInstanceLock();
                config.settings.killswitch = false;
                configManager.save(config);
                stateStore.update({ killSwitch: false });
                loggerService.log(
                  "Kill Switch отключен (отмена UAC).",
                  "warning",
                );
                resolve(false); // Отказ, продолжаем загрузку как обычно
              }
            });
          });
        } catch (e) {
          loggerService.log(
            `[СИСТЕМА] Ошибка запуска с админ правами: ${e.message}`,
            "error",
          );
          app.requestSingleInstanceLock();
        }

        if (shouldQuit) {
          app.isQuitting = true;
          app.quit();
          return; // прерываем дальнейшую загрузку
        }
      } else {
        // Отключаем Kill Switch и продолжаем запуск
        config.settings.killswitch = false;
        configManager.save(config);
        stateStore.update({ killSwitch: false });
        loggerService.log("Kill Switch отключен пользователем.", "warning");
      }
    } else {
      stateStore.update({ killSwitch: true });
    }
  } else {
    loggerService.log(
      "Конфиг не найден, используются настройки по умолчанию.",
      "warning",
    );
  }

  // Удаляем старые правила файрвола, которые могли остаться от предыдущего аварийного завершения
  if (typeof systemAdapter.removeKillSwitchFirewall === "function") {
    systemAdapter.removeKillSwitchFirewall().catch(() => {});
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

let isForceQuitting = false;

app.on("before-quit", async (event) => {
  if (isForceQuitting) return;
  event.preventDefault();

  app.isQuitting = true;
  trafficMonitor.stop();

  try {
    if (
      systemAdapter &&
      typeof systemAdapter.disableSystemProxySync === "function"
    ) {
      systemAdapter.disableSystemProxySync();
    } else {
      await systemAdapter.disableSystemProxy();
    }

    // Принудительно гасим серверы и убираем правила файрвола
    if (typeof systemAdapter.removeKillSwitchFirewall === "function") {
      await systemAdapter.removeKillSwitchFirewall();
    }
    await proxyManager.setSystemProxy(false);
  } catch (e) {
    loggerService.log(`[СИСТЕМА] Ошибка при выходе: ${e.message}`, "error");
  } finally {
    isForceQuitting = true;
    app.quit();
  }
});
