const { exec } = require("child_process");

// Локальный кэш процессов для быстродействия белых списков
let processTreeCache = {};
let isCaching = false;

// Умная функция получения информации о процессе с fallback'ами
async function getProcessInfo(pid) {
  return new Promise((resolve) => {
    exec(
      `wmic process where processid=${pid} get Name,ParentProcessId`,
      (err, out) => {
        if (!err && out) {
          const lines = out
            .split("\n")
            .map((l) => l.replace(/\r/g, "").trim())
            .filter((l) => l.length > 0);
          if (lines.length >= 2) {
            const match = lines[1].match(/^(.+?)\s+(\d+)$/);
            if (match) {
              return resolve({
                name: match[1].trim().toLowerCase(),
                ppid: match[2],
              });
            }
          }
        }
        exec(`tasklist /fi "PID eq ${pid}" /fo csv /nh`, (err2, out2) => {
          if (!err2 && out2 && out2.includes(pid.toString())) {
            const match = out2.match(/"([^"]+)"/);
            if (match) {
              return resolve({ name: match[1].toLowerCase(), ppid: "0" });
            }
          }
          resolve(null);
        });
      },
    );
  });
}

module.exports = {
  // 1. Фоновый сборщик процессов (запускается ядром)
  startProcessCacheInterval: (getState) => {
    setInterval(() => {
      if (isCaching) return;
      const state = getState();

      if (
        state.isConnected &&
        state.activeProxy?.rules?.appWhitelist?.length > 0
      ) {
        isCaching = true;
        exec(
          "wmic process get Name,ParentProcessId,ProcessId",
          (err, stdout) => {
            isCaching = false;
            if (!err && stdout) {
              const lines = stdout
                .split("\n")
                .map((l) => l.replace(/\r/g, "").trim())
                .filter((l) => l.length > 0);
              const newCache = {};
              for (let i = 1; i < lines.length; i++) {
                const match = lines[i].match(/(.+?)\s+(\d+)\s+(\d+)$/);
                if (match) {
                  newCache[match[3]] = {
                    name: match[1].trim().toLowerCase(),
                    ppid: match[2],
                  };
                }
              }
              if (Object.keys(newCache).length > 0) processTreeCache = newCache;
            }
          },
        );
      }
    }, 5000);
  },

  // 2. Получение сетевой статистики Windows
  getNetworkTraffic: () => {
    return new Promise((resolve) => {
      exec("netstat -e", (err, stdout) => {
        if (!err) {
          const lines = stdout.split("\n");
          for (let l of lines) {
            const parts = l.trim().split(/\s+/);
            if (parts.length >= 3) {
              const val1 = parseInt(parts[parts.length - 2], 10);
              const val2 = parseInt(parts[parts.length - 1], 10);
              if (!isNaN(val1) && !isNaN(val2)) {
                return resolve({ received: val1, sent: val2 });
              }
            }
          }
        }
        resolve({ received: 0, sent: 0 });
      });
    });
  },

  // 3. Установка прокси в систему
  setSystemProxy: (proxyIp, proxyPort, proxyType, whitelist, logCallback) => {
    return new Promise((resolve, reject) => {
      let proxyStr = "";
      if (proxyType === "SOCKS5") {
        proxyStr = `socks=${proxyIp}:${proxyPort}`;
      } else if (proxyType === "ALL") {
        proxyStr = `${proxyIp}:${proxyPort}`;
      } else {
        proxyStr = `http=${proxyIp}:${proxyPort};https=${proxyIp}:${proxyPort}`;
      }

      let override = "<local>";
      if (whitelist && whitelist.length > 0) {
        const bypassStr = whitelist.map((d) => `*.${d};*${d}*`).join(";");
        override = `${bypassStr};<local>`;
      }

      const command = `reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 1 /f && reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /t REG_SZ /d "${proxyStr}" /f && reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /t REG_SZ /d "${override}" /f && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v AutoConfigURL /f 2>nul & ipconfig /flushdns`;

      exec(command, (error) => {
        if (error) return reject(error);
        if (logCallback)
          logCallback(
            `[СИСТЕМА] Прокси применен к Windows успешно.`,
            "success",
          );
        resolve();
      });
    });
  },

  // 4. Очистка прокси из системы
  disableSystemProxy: (logCallback) => {
    return new Promise((resolve) => {
      if (logCallback)
        logCallback(
          "[СИСТЕМА] Очистка настроек прокси из реестра Windows...",
          "info",
        );
      const command = `reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /f 2>nul && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /f 2>nul && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v AutoConfigURL /f 2>nul & ipconfig /flushdns`;
      exec(command, () => resolve());
    });
  },

  // 5. Жесткая блокировка интернета
  applyKillSwitch: (logCallback) => {
    return new Promise((resolve) => {
      if (logCallback)
        logCallback(
          "[KILL SWITCH] Активирована полная блокировка интернета!",
          "error",
        );
      const command = `reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 1 /f && reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /t REG_SZ /d "127.0.0.1:65535" /f && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /f 2>nul & ipconfig /flushdns`;
      exec(command, () => resolve());
    });
  },

  // 6. Проверка процесса по белому списку EXE
  checkAppWhitelist: async (
    remotePort,
    appWhitelist,
    targetHost,
    logCallback,
  ) => {
    if (!appWhitelist || appWhitelist.length === 0 || !remotePort) return false;

    return new Promise((resolve) => {
      exec(`netstat -ano | findstr ":${remotePort}"`, async (err, stdout) => {
        if (err || !stdout) return resolve(false);

        const lines = stdout.trim().split(/\r?\n/);
        let pid = null;
        for (let line of lines) {
          const parts = line.trim().split(/\s+/).filter(Boolean);
          if (parts.length >= 4) {
            let currentPid = parts[parts.length - 1];
            let localAddr = parts[1];
            if (localAddr.endsWith(`:${remotePort}`)) {
              pid = currentPid;
              break;
            }
          }
        }

        if (!pid || isNaN(pid) || pid === "0") return resolve(false);

        let currentPid = pid;
        let foundAppName = false;
        let depth = 0;
        let chain = [];

        while (currentPid && currentPid !== "0" && depth < 10) {
          let info = processTreeCache[currentPid];
          if (!info) {
            info = await getProcessInfo(currentPid);
            if (info) processTreeCache[currentPid] = info;
          }
          if (!info) break;

          chain.push(info.name);

          const matchedApp = appWhitelist.find(
            (app) =>
              info.name === app.toLowerCase() ||
              info.name.includes(app.toLowerCase()),
          );
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
              `[БЕЛЫЙ СПИСОК EXE] Пропуск напрямую: ${targetHost} (Цепочка: ${chain.join(" <- ")})`,
              "warning",
            );
        } else {
          const chainStr =
            chain.length > 0
              ? chain.join(" <- ")
              : `PID:${pid} (Системный/Защищенный процесс)`;
          if (logCallback)
            logCallback(
              `[EXE DEBUG] ${targetHost} (Процесс: ${chainStr}) не в белом списке. Идет в прокси.`,
              "info",
            );
        }

        resolve(foundAppName);
      });
    });
  },
};
