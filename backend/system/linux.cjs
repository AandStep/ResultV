const { exec } = require("child_process");
const fs = require("fs");

let processTreeCache = {};
let isCaching = false;

// Получение информации о процессе Linux
async function getProcessInfo(pid) {
  return new Promise((resolve) => {
    exec(`ps -p ${pid} -o ppid=,comm=`, (err, out) => {
      if (err || !out) return resolve(null);
      const parts = out.trim().split(/\s+/);
      if (parts.length >= 2) {
        const ppid = parts[0];
        const name = parts.slice(1).join(" ").split("/").pop().toLowerCase();
        return resolve({ name, ppid });
      }
      resolve(null);
    });
  });
}

module.exports = {
  // 1. Фоновый сборщик процессов Linux
  startProcessCacheInterval: (getState) => {
    setInterval(() => {
      if (isCaching) return;
      const state = getState();

      if (
        state.isConnected &&
        state.activeProxy?.rules?.appWhitelist?.length > 0
      ) {
        isCaching = true;
        exec("ps -eo pid=,ppid=,comm=", (err, stdout) => {
          isCaching = false;
          if (!err && stdout) {
            const lines = stdout.split("\n").filter(Boolean);
            const newCache = {};
            lines.forEach((line) => {
              const parts = line.trim().split(/\s+/);
              if (parts.length >= 3) {
                const pid = parts[0];
                const ppid = parts[1];
                const name = parts
                  .slice(2)
                  .join(" ")
                  .split("/")
                  .pop()
                  .toLowerCase();
                newCache[pid] = { name, ppid };
              }
            });
            if (Object.keys(newCache).length > 0) processTreeCache = newCache;
          }
        });
      }
    }, 5000);
  },

  // 2. Быстрое чтение сетевой статистики напрямую из ядра Linux
  getNetworkTraffic: () => {
    return new Promise((resolve) => {
      try {
        const data = fs.readFileSync("/proc/net/dev", "utf8");
        const lines = data.split("\n");
        let received = 0,
          sent = 0;

        // Пропускаем первые 2 строки с заголовками
        for (let i = 2; i < lines.length; i++) {
          const line = lines[i].trim();
          if (!line || line.startsWith("lo:")) continue; // Игнорируем loopback (локальный) интерфейс

          const parts = line.split(/:?\s+/);
          if (parts.length >= 10) {
            received += parseInt(parts[1]) || 0; // rx_bytes
            sent += parseInt(parts[9]) || 0; // tx_bytes
          }
        }
        resolve({ received, sent });
      } catch (e) {
        resolve({ received: 0, sent: 0 });
      }
    });
  },

  // 3. Настройка системного прокси через gsettings (работает в GNOME, Cinnamon, Budgie, и т.д.)
  setSystemProxy: async (
    proxyIp,
    proxyPort,
    proxyType,
    whitelist,
    logCallback,
  ) => {
    return new Promise((resolve) => {
      let commands = [];

      // Формируем список исключений в формате GSettings: ['localhost', '127.0.0.1']
      let bypassArray = ["'localhost'", "'127.0.0.0/8'", "'::1'"];
      if (whitelist && whitelist.length > 0) {
        whitelist.forEach((domain) => {
          const clean = domain.replace(/\*/g, "");
          bypassArray.push(`'${clean}'`);
        });
      }
      const bypassStr = `[${bypassArray.join(", ")}]`;

      commands.push(`gsettings set org.gnome.system.proxy mode 'manual'`);
      commands.push(
        `gsettings set org.gnome.system.proxy ignore-hosts "${bypassStr}"`,
      );

      if (proxyType === "SOCKS5") {
        commands.push(
          `gsettings set org.gnome.system.proxy.socks host '${proxyIp}'`,
        );
        commands.push(
          `gsettings set org.gnome.system.proxy.socks port ${proxyPort}`,
        );
        commands.push(`gsettings set org.gnome.system.proxy.http host ''`);
        commands.push(`gsettings set org.gnome.system.proxy.https host ''`);
      } else {
        commands.push(
          `gsettings set org.gnome.system.proxy.http host '${proxyIp}'`,
        );
        commands.push(
          `gsettings set org.gnome.system.proxy.http port ${proxyPort}`,
        );
        commands.push(
          `gsettings set org.gnome.system.proxy.https host '${proxyIp}'`,
        );
        commands.push(
          `gsettings set org.gnome.system.proxy.https port ${proxyPort}`,
        );
        if (proxyType === "ALL") {
          commands.push(
            `gsettings set org.gnome.system.proxy.socks host '${proxyIp}'`,
          );
          commands.push(
            `gsettings set org.gnome.system.proxy.socks port ${proxyPort}`,
          );
        }
      }

      // На Linux ошибки gsettings часто бывают, если среда не GNOME (например KDE),
      // поэтому мы просто игнорируем stderr и продолжаем работу
      exec(commands.join(" && "), () => {
        if (logCallback)
          logCallback(
            `[СИСТЕМА] Прокси применен к Linux (gsettings).`,
            "success",
          );
        resolve();
      });
    });
  },

  // 4. Отключение прокси
  disableSystemProxy: async (logCallback) => {
    if (logCallback)
      logCallback("[СИСТЕМА] Очистка настроек прокси Linux...", "info");
    return new Promise((resolve) => {
      exec(`gsettings set org.gnome.system.proxy mode 'none'`, () => resolve());
    });
  },

  // 5. Kill Switch (мертвый порт)
  applyKillSwitch: async (logCallback) => {
    if (logCallback)
      logCallback(
        "[KILL SWITCH] Активирована полная блокировка интернета!",
        "error",
      );
    return new Promise((resolve) => {
      const cmds = [
        `gsettings set org.gnome.system.proxy mode 'manual'`,
        `gsettings set org.gnome.system.proxy.socks host '127.0.0.1'`,
        `gsettings set org.gnome.system.proxy.socks port 65535`,
        `gsettings set org.gnome.system.proxy.http host '127.0.0.1'`,
        `gsettings set org.gnome.system.proxy.http port 65535`,
      ];
      exec(cmds.join(" && "), () => resolve());
    });
  },

  // 6. Проверка процесса (lsof)
  checkAppWhitelist: async (
    remotePort,
    appWhitelist,
    targetHost,
    logCallback,
  ) => {
    if (!appWhitelist || appWhitelist.length === 0 || !remotePort) return false;

    return new Promise((resolve) => {
      exec(
        `lsof -nP -iTCP:${remotePort} -sTCP:ESTABLISHED`,
        async (err, stdout) => {
          if (err || !stdout) return resolve(false);

          const lines = stdout.trim().split("\n");
          if (lines.length < 2) return resolve(false);

          const parts = lines[1].trim().split(/\s+/);
          const pid = parts[1];

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

            // Отрезаем .exe для кроссплатформенной работы белых списков
            const matchedApp = appWhitelist.find((app) => {
              const cleanApp = app.toLowerCase().replace(".exe", "");
              return info.name === cleanApp || info.name.includes(cleanApp);
            });

            if (matchedApp) {
              foundAppName = info.name;
              break;
            }
            currentPid = info.ppid;
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
