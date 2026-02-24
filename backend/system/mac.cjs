const { exec } = require("child_process");

// Локальный кэш процессов для быстродействия белых списков
let processTreeCache = {};
let isCaching = false;

// Вспомогательная функция для получения активных сетевых адаптеров Mac (Wi-Fi, Ethernet)
// Мы игнорируем отключенные сервисы (содержат *) и виртуальные мосты
async function getActiveServices() {
  return new Promise((resolve) => {
    exec("networksetup -listallnetworkservices", (err, stdout) => {
      if (err) return resolve(["Wi-Fi"]);
      const services = stdout
        .split("\n")
        .map((l) => l.trim())
        .filter(
          (l) =>
            l &&
            !l.includes("*") &&
            l !== "An asterisk (*) denotes that a network service is disabled.",
        );
      // Если почему-то список пуст, возвращаем стандартные значения
      resolve(services.length > 0 ? services : ["Wi-Fi", "Ethernet"]);
    });
  });
}

// Получение информации о процессе Mac (Имя и родительский ID)
async function getProcessInfo(pid) {
  return new Promise((resolve) => {
    // ps выводит PPID и путь к команде
    exec(`ps -p ${pid} -o ppid=,comm=`, (err, out) => {
      if (err || !out) return resolve(null);

      const match = out.trim().match(/^(\d+)\s+(.+)$/);
      if (match) {
        const ppid = match[1];
        const comm = match[2];

        // Вытаскиваем имя из пути (например, /Applications/Discord.app/Contents/MacOS/Discord -> discord)
        const parts = comm.split("/");
        let name = parts[parts.length - 1].toLowerCase();

        // Если это приложение .app, берем его нормальное имя
        if (comm.includes(".app/")) {
          const appMatch = comm.match(/\/([^/]+)\.app\//);
          if (appMatch) name = appMatch[1].toLowerCase();
        }

        resolve({ name, ppid });
      } else {
        resolve(null);
      }
    });
  });
}

module.exports = {
  // 1. Фоновый сборщик процессов macOS
  startProcessCacheInterval: (getState) => {
    setInterval(() => {
      if (isCaching) return;
      const state = getState();

      if (
        state.isConnected &&
        state.activeProxy?.rules?.appWhitelist?.length > 0
      ) {
        isCaching = true;
        exec("ps -Ao pid=,ppid=,comm=", (err, stdout) => {
          isCaching = false;
          if (!err && stdout) {
            const lines = stdout
              .split("\n")
              .map((l) => l.trim())
              .filter(Boolean);
            const newCache = {};
            lines.forEach((line) => {
              const match = line.match(/^(\d+)\s+(\d+)\s+(.+)$/);
              if (match) {
                const pid = match[1];
                const ppid = match[2];
                const comm = match[3];

                let name = comm.split("/").pop().toLowerCase();
                if (comm.includes(".app/")) {
                  const appMatch = comm.match(/\/([^/]+)\.app\//);
                  if (appMatch) name = appMatch[1].toLowerCase();
                }
                newCache[pid] = { name, ppid };
              }
            });
            if (Object.keys(newCache).length > 0) processTreeCache = newCache;
          }
        });
      }
    }, 5000);
  },

  // 2. Статистика сети для графика скорости (парсинг netstat Mac)
  getNetworkTraffic: () => {
    return new Promise((resolve) => {
      exec("netstat -ibn", (err, stdout) => {
        if (err) return resolve({ received: 0, sent: 0 });
        let received = 0,
          sent = 0;
        const lines = stdout.split("\n");
        lines.forEach((line) => {
          // Берем только физические адаптеры (en0 - Wi-Fi, en1 - Ethernet и т.д.)
          if (line.startsWith("en")) {
            const parts = line.trim().split(/\s+/);
            // Проверяем строку с MAC-адресом (в ней правильная статистика байтов в macOS)
            if (parts[2] && parts[2].includes("Link#")) {
              received += parseInt(parts[6]) || 0; // Входящие байты (Ibytes)
              sent += parseInt(parts[9]) || 0; // Исходящие байты (Obytes)
            }
          }
        });
        resolve({ received, sent });
      });
    });
  },

  // 3. Установка системного прокси через networksetup
  setSystemProxy: async (
    proxyIp,
    proxyPort,
    proxyType,
    whitelist,
    logCallback,
  ) => {
    const services = await getActiveServices();
    let commands = [];

    // Стандартные исключения macOS
    let bypassStr = "*.local 169.254/16 127.0.0.1 localhost";
    if (whitelist && whitelist.length > 0) {
      // macOS networksetup требует пробелов между доменами
      bypassStr +=
        " " + whitelist.map((d) => (d.includes("*") ? d : `*${d}*`)).join(" ");
    }

    for (const service of services) {
      commands.push(
        `networksetup -setproxybypassdomains "${service}" ${bypassStr}`,
      );

      if (proxyType === "SOCKS5") {
        commands.push(
          `networksetup -setsocksfirewallproxy "${service}" ${proxyIp} ${proxyPort}`,
        );
        commands.push(
          `networksetup -setsocksfirewallproxystate "${service}" on`,
        );
        // Отключаем HTTP на случай, если он был включен
        commands.push(`networksetup -setwebproxystate "${service}" off`);
        commands.push(`networksetup -setsecurewebproxystate "${service}" off`);
      } else {
        commands.push(
          `networksetup -setwebproxy "${service}" ${proxyIp} ${proxyPort}`,
        );
        commands.push(
          `networksetup -setsecurewebproxy "${service}" ${proxyIp} ${proxyPort}`,
        );
        commands.push(`networksetup -setwebproxystate "${service}" on`);
        commands.push(`networksetup -setsecurewebproxystate "${service}" on`);

        if (proxyType === "ALL") {
          commands.push(
            `networksetup -setsocksfirewallproxy "${service}" ${proxyIp} ${proxyPort}`,
          );
          commands.push(
            `networksetup -setsocksfirewallproxystate "${service}" on`,
          );
        } else {
          commands.push(
            `networksetup -setsocksfirewallproxystate "${service}" off`,
          );
        }
      }
    }

    return new Promise((resolve, reject) => {
      // Используем && чтобы команды выполнялись последовательно
      exec(commands.join(" && "), (error) => {
        if (error) {
          if (logCallback)
            logCallback(
              `[ОШИБКА macOS] Не удалось применить настройки: ${error.message}`,
              "error",
            );
          return reject(error);
        }
        if (logCallback)
          logCallback(
            `[СИСТЕМА] Прокси применен к macOS (Интерфейсы: ${services.join(", ")}).`,
            "success",
          );
        resolve();
      });
    });
  },

  // 4. Отключение прокси
  disableSystemProxy: async (logCallback) => {
    if (logCallback)
      logCallback("[СИСТЕМА] Очистка настроек прокси macOS...", "info");
    const services = await getActiveServices();
    let commands = [];

    for (const service of services) {
      commands.push(`networksetup -setwebproxystate "${service}" off`);
      commands.push(`networksetup -setsecurewebproxystate "${service}" off`);
      commands.push(
        `networksetup -setsocksfirewallproxystate "${service}" off`,
      );
      commands.push(`networksetup -setproxybypassdomains "${service}" Empty`);
    }

    return new Promise((resolve) => {
      // Используем ; чтобы продолжить выполнение даже если один из сервисов выдаст ошибку
      exec(commands.join(" ; "), () => resolve());
    });
  },

  // 5. Kill Switch (направляем трафик в "черную дыру")
  applyKillSwitch: async (logCallback) => {
    if (logCallback)
      logCallback(
        "[KILL SWITCH] Активирована полная блокировка интернета!",
        "error",
      );
    const services = await getActiveServices();
    let commands = [];

    for (const service of services) {
      commands.push(`networksetup -setwebproxy "${service}" 127.0.0.1 65535`);
      commands.push(
        `networksetup -setsecurewebproxy "${service}" 127.0.0.1 65535`,
      );
      commands.push(
        `networksetup -setsocksfirewallproxy "${service}" 127.0.0.1 65535`,
      );
      commands.push(`networksetup -setwebproxystate "${service}" on`);
      commands.push(`networksetup -setsecurewebproxystate "${service}" on`);
      commands.push(`networksetup -setsocksfirewallproxystate "${service}" on`);
    }

    return new Promise((resolve) => {
      exec(commands.join(" && "), () => resolve());
    });
  },

  // 6. Проверка процесса через lsof
  checkAppWhitelist: async (
    remotePort,
    appWhitelist,
    targetHost,
    logCallback,
  ) => {
    if (!appWhitelist || appWhitelist.length === 0 || !remotePort) return false;

    return new Promise((resolve) => {
      // Ищем PID процесса, который установил TCP соединение на нужный порт
      exec(
        `lsof -n -P -iTCP:${remotePort} -sTCP:ESTABLISHED`,
        async (err, stdout) => {
          if (err || !stdout) return resolve(false);

          const lines = stdout.trim().split("\n");
          if (lines.length < 2) return resolve(false);

          // Берем вторую строку (первая - это заголовки таблицы lsof)
          const parts = lines[1].trim().split(/\s+/);
          const pid = parts[1]; // Второй столбец — это PID

          if (!pid || isNaN(pid)) return resolve(false);

          let currentPid = pid;
          let foundAppName = false;
          let depth = 0;
          let chain = [];

          while (
            currentPid &&
            currentPid !== "0" &&
            currentPid !== "1" &&
            depth < 10
          ) {
            let info = processTreeCache[currentPid];
            if (!info) {
              info = await getProcessInfo(currentPid);
              if (info) processTreeCache[currentPid] = info;
            }
            if (!info) break;

            chain.push(info.name);

            // Умное сравнение: отрезаем .exe, чтобы правила из Windows работали на Mac
            const matchedApp = appWhitelist.find((app) => {
              const cleanApp = app.toLowerCase().replace(".exe", "");
              return info.name === cleanApp || info.name.includes(cleanApp);
            });

            if (matchedApp) {
              foundAppName = info.name;
              break;
            }
            currentPid = info.ppid; // Идем вверх по дереву процессов
            depth++;
          }

          if (foundAppName) {
            if (logCallback)
              logCallback(
                `[БЕЛЫЙ СПИСОК APP] Пропуск напрямую: ${targetHost} (Цепочка: ${chain.join(" <- ")})`,
                "warning",
              );
          } else {
            const chainStr =
              chain.length > 0 ? chain.join(" <- ") : `PID:${pid}`;
            if (logCallback)
              logCallback(
                `[APP DEBUG] ${targetHost} (Процесс: ${chainStr}) не в белом списке.`,
                "info",
              );
          }

          resolve(foundAppName);
        },
      );
    });
  },
};
