/*
 * Copyright (C) 2026 ResultProxy
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

const { exec, execSync } = require("child_process");
const { getSafeOSWhitelist } = require("../utils/domain.cjs");

// Локальный кэш процессов для быстродействия белых списков
let processTreeCache = {};
let isCaching = false;

// Очередь для пакетного запуска netstat, предотвращающая CPU-голодание (Event Loop starvation)
let netstatQueue = {};
let isNetstatRunning = false;

function fetchPidsForPorts() {
  if (isNetstatRunning || Object.keys(netstatQueue).length === 0) return;
  isNetstatRunning = true;
  
  const currentBatch = { ...netstatQueue };
  netstatQueue = {};
  
  exec("netstat -ano -p tcp", { maxBuffer: 1024 * 1024 * 50 }, (err, stdout) => {
    isNetstatRunning = false;
    const lines = stdout ? stdout.trim().split(/\r?\n/) : [];
    const pidMap = {};
    for (const line of lines) {
      const parts = line.trim().split(/\s+/).filter(Boolean);
      if (parts.length >= 4) {
        pidMap[parts[1]] = parts[parts.length - 1]; // "IP:Port" -> "PID"
      }
    }
    
    for (const port in currentBatch) {
      let foundPid = null;
      for (const localAddr in pidMap) {
        if (localAddr.endsWith(`:${port}`)) {
          foundPid = pidMap[localAddr];
          break;
        }
      }
      currentBatch[port].forEach(resolve => resolve(foundPid));
    }
    
    if (Object.keys(netstatQueue).length > 0) {
      setTimeout(fetchPidsForPorts, 50);
    }
  });
}

function getPidForPort(port) {
  return new Promise(resolve => {
    if (!netstatQueue[port]) {
      netstatQueue[port] = [];
    }
    netstatQueue[port].push(resolve);
    
    if (!isNetstatRunning) {
      setTimeout(fetchPidsForPorts, 20); // небольшая задержка, чтобы собрать все входящие запросы в 1 команду
    }
  });
}

// Умная функция получения информации о процессе с fallback'ами
async function getProcessInfo(pid) {
  return new Promise((resolve) => {
    exec(
      `wmic process where processid=${pid} get Name,ParentProcessId`,
      { maxBuffer: 1024 * 1024 * 50 },
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
        exec(
          `tasklist /fi "PID eq ${pid}" /fo csv /nh`,
          { maxBuffer: 1024 * 1024 * 50 },
          (err2, out2) => {
            if (!err2 && out2 && out2.includes(pid.toString())) {
              const match = out2.match(/"([^"]+)"/);
              if (match) {
                return resolve({ name: match[1].toLowerCase(), ppid: "0" });
              }
            }
            resolve(null);
          },
        );
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
          { maxBuffer: 1024 * 1024 * 50 },
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
        const safeList = getSafeOSWhitelist(whitelist);
        const bypassStr = safeList.map((d) => `*.${d};*${d}*`).join(";");
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

  // 4. Очистка прокси из системы (Асинхронно, для обычной работы)
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

  // 5. ЖЕСТКАЯ СИНХРОННАЯ ОЧИСТКА (Вызывается только при завершении работы/выключении ПК)
  disableSystemProxySync: () => {
    try {
      // execSync блокирует поток, заставляя ОС дождаться выполнения очистки реестра перед "смертью" процесса
      execSync(
        'reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f',
        { stdio: "ignore" },
      );
      execSync(
        'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /f 2>nul',
        { stdio: "ignore" },
      );
      execSync(
        'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /f 2>nul',
        { stdio: "ignore" },
      );
    } catch (e) {
      // Игнорируем ошибки (например, если ключа и так нет), чтобы не крашить завершение работы
    }
  },

  // 6. Жесткая блокировка интернета (Kill Switch)
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

  // 7. Проверка процесса по белому списку EXE
  checkAppWhitelist: async (
    remotePort,
    appWhitelist,
    targetHost,
    logCallback,
  ) => {
    if (!appWhitelist || appWhitelist.length === 0 || !remotePort) return false;

    const pid = await getPidForPort(remotePort);
    if (!pid || isNaN(pid) || pid === "0") return false;

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

          return foundAppName;
  },

  // 8. Установка/снятие флага "Запускать от имени администратора" в реестре Windows
  setRunAsAdminFlag: (enable) => {
    return new Promise((resolve) => {
      const { app } = require("electron");

      if (!app.isPackaged) {
        return resolve();
      }

      const exePath = app.getPath("exe");
      const regPath =
        "HKCU\\Software\\Microsoft\\Windows NT\\CurrentVersion\\AppCompatFlags\\Layers";
      const command = enable
        ? `reg add "${regPath}" /v "${exePath}" /t REG_SZ /d "~ RUNASADMIN" /f`
        : `reg delete "${regPath}" /v "${exePath}" /f`;

      exec(command, (err) => {
        if (err) {
          console.error(
            `[СИСТЕМА] Ошибка при установке флага RUNASADMIN: ${err.message}`,
          );
        }
        resolve();
      });
    });
  },

  // 9. Работа с планировщиком задач (schtasks) для автостарта с админ-правами
  enableTaskAutostart: (exePath, args = []) => {
    return new Promise((resolve, reject) => {
      const taskName = "ResultProxyAutostart";
      const argsStr = args.length > 0 ? ` ${args.join(" ")}` : "";

      // Проверка на права администратора перед созданием задачи
      try {
        execSync("net session", { stdio: "ignore" });
      } catch (e) {
        return reject(
          new Error(
            "Требуются права администратора для создания задачи в планировщике.",
          ),
        );
      }

      // Мы используем schtasks для создания задачи, которая запускается при входе пользователя
      // Используем двойные кавычки для пути к EXE, экранируя их для cmd
      const taskCmd = `\\"${exePath.replace(/\//g, "\\")}\\"${argsStr}`;
      const command = `schtasks /create /tn "${taskName}" /tr "${taskCmd}" /sc ONLOGON /rl HIGHEST /f`;

      exec(command, (err, stdout, stderr) => {
        if (err) {
          console.error(`[СИСТЕМА] Ошибка создания задачи schtasks: ${stderr}`);
          return reject(new Error(stderr || err.message));
        }
        console.log(`[СИСТЕМА] Задача автостарта успешно создана: ${stdout}`);
        resolve();
      });
    });
  },

  disableTaskAutostart: () => {
    return new Promise((resolve) => {
      const taskName = "ResultProxyAutostart";
      exec(`schtasks /delete /tn "${taskName}" /f`, (err) => {
        if (err) {
          console.warn(
            `[СИСТЕМА] Не удалось удалить задачу автостарта (возможно, ее нет).`,
          );
        }
        resolve();
      });
    });
  },
};
