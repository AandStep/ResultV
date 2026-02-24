const { app, BrowserWindow, Tray, Menu, nativeImage } = require("electron");
const path = require("path");

// ПРИНУДИТЕЛЬНО ЗАДАЕМ ПАПКУ ПРИЛОЖЕНИЯ
app.setPath("userData", path.join(app.getPath("appData"), "resultProxy"));

const express = require("express");
const cors = require("cors");
const { exec } = require("child_process");
const os = require("os");
const ProxyChain = require("proxy-chain");
const net = require("net");
const { SocksClient } = require("socks");
const fs = require("fs");
const http = require("http"); // Добавлен модуль для запросов к API геолокации

const server = express();
server.use(cors());
server.use(express.json());

const configPath = path.join(app.getPath("userData"), "proxy_config.json");

let systemState = {
  isConnected: false,
  activeProxy: null,
  bytesSent: 0,
  bytesReceived: 0,
  speedReceived: 0,
  speedSent: 0,
  isProxyDead: false,
  killSwitch: false,
};

// --- СИСТЕМА ЛОГОВ БЭКЕНДА ---
let backendLogs = [];

function addBackendLog(msg, type = "info") {
  const time = new Date().toLocaleTimeString();
  // Добавляем в начало массива, храним только последние 100 логов
  backendLogs.unshift({ timestamp: Date.now(), time, msg, type });
  if (backendLogs.length > 100) backendLogs.pop();
  console.log(`[${time}] ${msg}`);
}

server.get("/api/logs", (req, res) => {
  res.json(backendLogs);
});
// -----------------------------

try {
  if (fs.existsSync(configPath)) {
    const savedConfig = JSON.parse(fs.readFileSync(configPath, "utf8"));
    if (savedConfig.settings && savedConfig.settings.killswitch) {
      systemState.killSwitch = true;
    }
  }
} catch (e) {
  addBackendLog(
    "Конфиг не найден, используются настройки по умолчанию.",
    "warning",
  );
}

let localProxyServer = null;
let localSocksServer = null;
let sessionStartStats = { received: 0, sent: 0 };
let lastTickStats = { received: 0, sent: 0, time: Date.now() };

let mainWindow = null;
let appTray = null;
let uiProxies = [];

const BLOCKED_RESOURCES = [
  "instagram.com",
  "facebook.com",
  "twitter.com",
  "x.com",
  "t.me",
  "discord.com",
  "netflix.com",
];

// --- КЭШИРОВАНИЕ И ПРОВЕРКА ПРОЦЕССОВ (.EXE БЕЛЫЙ СПИСОК) ---
let processTreeCache = {};
let isCaching = false;

setInterval(() => {
  if (isCaching) return;
  if (
    systemState.isConnected &&
    systemState.activeProxy?.rules?.appWhitelist?.length > 0
  ) {
    isCaching = true;
    exec("wmic process get Name,ParentProcessId,ProcessId", (err, stdout) => {
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
    });
  }
}, 5000);

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

// Асинхронная проверка: принадлежит ли порт приложению из белого списка
async function checkAppWhitelist(remotePort, appWhitelist, targetHost) {
  if (!appWhitelist || appWhitelist.length === 0 || !remotePort) return false;
  return new Promise((resolve) => {
    exec(`netstat -ano | findstr ":${remotePort}"`, async (err, stdout) => {
      if (err || !stdout) {
        return resolve(false);
      }

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

      if (!pid || isNaN(pid) || pid === "0") {
        return resolve(false);
      }

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
        addBackendLog(
          `[БЕЛЫЙ СПИСОК EXE] Пропуск напрямую: ${targetHost} (Цепочка: ${chain.join(" <- ")})`,
          "warning",
        );
      } else {
        const chainStr =
          chain.length > 0
            ? chain.join(" <- ")
            : `PID:${pid} (Системный/Защищенный процесс)`;
        addBackendLog(
          `[EXE DEBUG] ${targetHost} (Процесс: ${chainStr}) не в белом списке. Идет в прокси.`,
          "info",
        );
      }

      resolve(foundAppName);
    });
  });
}
// -------------------------------------------------------------

const applyKillSwitch = () => {
  addBackendLog(
    "[KILL SWITCH] Активирована полная блокировка интернета!",
    "error",
  );
  const command = `reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 1 /f && reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /t REG_SZ /d "127.0.0.1:65535" /f && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /f 2>nul & ipconfig /flushdns`;
  exec(command);
};

const disableProxyClean = () => {
  addBackendLog(
    "[СИСТЕМА] Очистка настроек прокси из реестра Windows...",
    "info",
  );
  const command = `reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /f 2>nul && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /f 2>nul && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v AutoConfigURL /f 2>nul & ipconfig /flushdns`;
  exec(command);
};

const pingProxy = (host, port) => {
  return new Promise((resolve) => {
    const start = Date.now();
    const socket = new net.Socket();
    socket.setTimeout(2000);

    socket.on("connect", () => {
      const ping = Date.now() - start;
      socket.destroy();
      resolve({ alive: true, ping });
    });

    socket.on("timeout", () => {
      socket.destroy();
      resolve({ alive: false, ping: 0 });
    });

    socket.on("error", () => {
      socket.destroy();
      resolve({ alive: false, ping: 0 });
    });

    socket.connect(port, host);
  });
};

setInterval(async () => {
  if (systemState.isConnected && systemState.activeProxy) {
    const { alive } = await pingProxy(
      systemState.activeProxy.ip,
      systemState.activeProxy.port,
    );
    const wasDead = systemState.isProxyDead;
    systemState.isProxyDead = !alive;

    if (systemState.isProxyDead && !wasDead && systemState.killSwitch) {
      applyKillSwitch();
    } else if (!systemState.isProxyDead && wasDead && systemState.killSwitch) {
      addBackendLog(
        "[KILL SWITCH] Связь восстановлена. Возвращаем доступ.",
        "success",
      );
      await setSystemProxy(true, systemState.activeProxy, true);
    }
  } else {
    systemState.isProxyDead = false;
  }
}, 3000);

const getNetworkTraffic = () => {
  return new Promise((resolve) => {
    if (os.platform() === "win32") {
      exec("netstat -e", (err, stdout) => {
        if (!err) {
          const lines = stdout.split("\n");
          for (let l of lines) {
            const parts = l.trim().split(/\s+/);
            if (parts.length >= 3) {
              const val1 = parseInt(parts[parts.length - 2], 10);
              const val2 = parseInt(parts[parts.length - 1], 10);
              if (!isNaN(val1) && !isNaN(val2)) {
                resolve({ received: val1, sent: val2 });
                return;
              }
            }
          }
        }
        resolve({ received: 0, sent: 0 });
      });
    } else {
      resolve({ received: 0, sent: 0 });
    }
  });
};

const setSystemProxy = async (
  enable,
  proxy = null,
  updateRegistryOnly = false,
) => {
  let command = "";

  if (localProxyServer && !updateRegistryOnly) {
    await localProxyServer.close(true);
    localProxyServer = null;
  }
  if (localSocksServer && !updateRegistryOnly) {
    localSocksServer.close();
    localSocksServer = null;
  }

  if (os.platform() === "win32") {
    if (enable && proxy) {
      let proxyIp = proxy.ip;
      let proxyPort = proxy.port;
      let proxyType = proxy.type || "HTTP";
      const rules = proxy.rules || {
        mode: "global",
        whitelist: ["localhost", "127.0.0.1"],
        appWhitelist: [],
      };

      if (proxyType === "SOCKS5" || (proxy.username && proxy.password)) {
        if (!updateRegistryOnly) {
          if (proxyType === "SOCKS5") {
            addBackendLog(
              `[МОСТ SOCKS5] Запуск локального туннеля на 127.0.0.1:14081`,
              "info",
            );

            localSocksServer = net.createServer((client) => {
              let step = 0;
              let buffer = Buffer.alloc(0);
              let isSocks5 = false;
              let isSocks4 = false;
              let isHttpConnect = false;

              const onData = async (data) => {
                client.pause();
                try {
                  buffer = Buffer.concat([buffer, data]);

                  if (step === 0) {
                    if (buffer.length < 1) {
                      client.resume();
                      return;
                    }

                    if (buffer[0] === 0x05) {
                      isSocks5 = true;
                      if (buffer.length < 2) {
                        client.resume();
                        return;
                      }
                      const numMethods = buffer[1];
                      if (buffer.length < 2 + numMethods) {
                        client.resume();
                        return;
                      }

                      client.write(Buffer.from([0x05, 0x00]));
                      buffer = buffer.slice(2 + numMethods);
                      step = 1;
                      if (buffer.length === 0) {
                        client.resume();
                        return;
                      }
                    } else if (buffer[0] === 0x04) {
                      isSocks4 = true;
                      step = 1;
                    } else if (buffer[0] === 0x43) {
                      isHttpConnect = true;
                      step = 1;
                    } else {
                      client.resume();
                      return client.end();
                    }
                  }

                  if (step === 1) {
                    let dstHost, dstPort, offset, successResponse;

                    if (isSocks5) {
                      if (buffer.length < 4) {
                        client.resume();
                        return;
                      }
                      if (buffer[0] !== 0x05 || buffer[1] !== 0x01) {
                        client.resume();
                        return client.end();
                      }

                      const atyp = buffer[3];
                      offset = 4;

                      if (atyp === 0x01) {
                        if (buffer.length < offset + 4 + 2) {
                          client.resume();
                          return;
                        }
                        dstHost = `${buffer[offset]}.${buffer[offset + 1]}.${buffer[offset + 2]}.${buffer[offset + 3]}`;
                        offset += 4;
                      } else if (atyp === 0x03) {
                        if (buffer.length < offset + 1) {
                          client.resume();
                          return;
                        }
                        const len = buffer[offset];
                        if (buffer.length < offset + 1 + len + 2) {
                          client.resume();
                          return;
                        }
                        dstHost = buffer
                          .slice(offset + 1, offset + 1 + len)
                          .toString();
                        offset += 1 + len;
                      } else {
                        client.resume();
                        return client.end();
                      }

                      dstPort = buffer.readUInt16BE(offset);
                      offset += 2;
                      successResponse = Buffer.from([
                        0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
                        0x00,
                      ]);
                    } else if (isSocks4) {
                      if (buffer.length < 9) {
                        client.resume();
                        return;
                      }
                      const nullIdx = buffer.indexOf(0x00, 8);
                      if (nullIdx === -1) {
                        client.resume();
                        return;
                      }

                      dstPort = buffer.readUInt16BE(2);
                      const ip1 = buffer[4],
                        ip2 = buffer[5],
                        ip3 = buffer[6],
                        ip4 = buffer[7];
                      dstHost = `${ip1}.${ip2}.${ip3}.${ip4}`;
                      offset = nullIdx + 1;

                      if (ip1 === 0 && ip2 === 0 && ip3 === 0 && ip4 !== 0) {
                        const domainNullIdx = buffer.indexOf(0x00, offset);
                        if (domainNullIdx === -1) {
                          client.resume();
                          return;
                        }
                        dstHost = buffer
                          .slice(offset, domainNullIdx)
                          .toString();
                        offset = domainNullIdx + 1;
                      }
                      successResponse = Buffer.from([
                        0x00, 0x5a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                      ]);
                    } else if (isHttpConnect) {
                      const reqStr = buffer.toString();
                      const headerEnd = reqStr.indexOf("\r\n\r\n");
                      if (headerEnd === -1) {
                        client.resume();
                        return;
                      }

                      const lines = reqStr.split("\r\n");
                      const match = lines[0].match(/CONNECT\s+([^:]+):(\d+)/);
                      if (!match) {
                        client.resume();
                        return client.end();
                      }

                      dstHost = match[1];
                      dstPort = parseInt(match[2], 10);
                      offset = headerEnd + 4;
                      successResponse = Buffer.from(
                        "HTTP/1.1 200 Connection Established\r\n\r\n",
                      );
                    }

                    client.removeListener("data", onData);
                    const remainingData = buffer.slice(offset);

                    const currentRules = systemState.activeProxy?.rules || {
                      mode: "global",
                      whitelist: ["localhost", "127.0.0.1"],
                      appWhitelist: [],
                    };

                    let isAppWhitelisted = false;
                    let appName = "";
                    if (
                      currentRules.appWhitelist &&
                      currentRules.appWhitelist.length > 0
                    ) {
                      const clientPort = client.remotePort;
                      const matchedApp = await checkAppWhitelist(
                        clientPort,
                        currentRules.appWhitelist,
                        dstHost,
                      );
                      if (matchedApp) {
                        isAppWhitelisted = true;
                        appName = matchedApp;
                      }
                    }

                    const isWhitelisted =
                      currentRules.whitelist &&
                      currentRules.whitelist.some((d) => dstHost.includes(d));
                    const isBlocked = BLOCKED_RESOURCES.some((d) =>
                      dstHost.includes(d),
                    );

                    let useProxy =
                      currentRules.mode === "smart"
                        ? isBlocked
                        : !isWhitelisted;

                    if (isAppWhitelisted) {
                      useProxy = false;
                    } else if (useProxy) {
                      addBackendLog(
                        `[ПРОКСИ] ${dstHost}:${dstPort} -> ${proxy.ip}`,
                        "success",
                      );
                    }

                    if (!useProxy) {
                      const directSocket = net.connect(dstPort, dstHost, () => {
                        client.write(successResponse);
                        if (remainingData.length > 0)
                          directSocket.write(remainingData);
                        directSocket.pipe(client);
                        client.pipe(directSocket);
                      });
                      directSocket.on("error", (err) => client.end());
                      client.on("error", () => directSocket.end());
                    } else {
                      SocksClient.createConnection({
                        proxy: {
                          host: proxy.ip,
                          port: parseInt(proxy.port),
                          type: 5,
                          userId: proxy.username || undefined,
                          password: proxy.password || undefined,
                        },
                        command: "connect",
                        destination: { host: dstHost, port: dstPort },
                      })
                        .then((info) => {
                          client.write(successResponse);
                          if (remainingData.length > 0)
                            info.socket.write(remainingData);
                          info.socket.pipe(client);
                          client.pipe(info.socket);

                          info.socket.on("error", (err) => client.end());
                          client.on("error", () => info.socket.end());
                        })
                        .catch((err) => {
                          client.end();
                        });
                    }
                  }
                } catch (e) {
                  client.end();
                }
                client.resume();
              };
              client.on("data", onData);
              client.on("error", () => {});
            });

            await new Promise((resolve) =>
              localSocksServer.listen(14081, "127.0.0.1", resolve),
            );
            proxyIp = "127.0.0.1";
            proxyPort = "14081";
            proxyType = "ALL";
          } else {
            addBackendLog(
              "[МОСТ HTTP] Настройка локального HTTP туннеля для авторизации...",
              "info",
            );
            const encUser = encodeURIComponent(proxy.username);
            const encPass = encodeURIComponent(proxy.password);
            const upstreamUrl = `http://${encUser}:${encPass}@${proxy.ip}:${proxy.port}`;

            localProxyServer = new ProxyChain.Server({
              port: 14081,
              prepareRequestFunction: async ({
                hostname,
                port,
                isHttp,
                request,
              }) => {
                const currentRules = systemState.activeProxy?.rules || {
                  mode: "global",
                  whitelist: ["localhost", "127.0.0.1"],
                  appWhitelist: [],
                };

                if (
                  currentRules.appWhitelist &&
                  currentRules.appWhitelist.length > 0
                ) {
                  const clientPort =
                    request.socket?.remotePort ||
                    request.connection?.remotePort;
                  const appName = await checkAppWhitelist(
                    clientPort,
                    currentRules.appWhitelist,
                    hostname,
                  );
                  if (appName) {
                    return { requestAuthentication: false };
                  }
                }

                if (
                  currentRules.whitelist &&
                  currentRules.whitelist.length > 0
                ) {
                  if (currentRules.whitelist.some((d) => hostname.includes(d)))
                    return { requestAuthentication: false };
                }
                if (currentRules.mode === "smart") {
                  if (!BLOCKED_RESOURCES.some((d) => hostname.includes(d)))
                    return { requestAuthentication: false };
                }

                addBackendLog(`[ПРОКСИ] ${hostname} -> ${proxy.ip}`, "success");
                return {
                  requestAuthentication: false,
                  upstreamProxyUrl: upstreamUrl,
                };
              },
            });

            localProxyServer.on("serverError", (err) => {});
            await localProxyServer.listen();

            proxyIp = "127.0.0.1";
            proxyPort = "14081";
            proxyType = "ALL";
          }
        } else {
          proxyIp = "127.0.0.1";
          proxyPort = "14081";
          proxyType = "ALL";
        }
      }

      let proxyStr = "";
      if (proxyType === "SOCKS5") {
        proxyStr = `socks=${proxyIp}:${proxyPort}`;
      } else if (proxyType === "ALL") {
        proxyStr = `${proxyIp}:${proxyPort}`;
      } else {
        proxyStr = `http=${proxyIp}:${proxyPort};https=${proxyIp}:${proxyPort}`;
      }

      let override = "<local>";
      if (rules.whitelist && rules.whitelist.length > 0) {
        const bypassStr = rules.whitelist.map((d) => `*.${d};*${d}*`).join(";");
        override = `${bypassStr};<local>`;
      }

      if (!updateRegistryOnly)
        addBackendLog(
          `[СИСТЕМА] Прокси применен к Windows успешно.`,
          "success",
        );

      command = `reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 1 /f && reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /t REG_SZ /d "${proxyStr}" /f && reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /t REG_SZ /d "${override}" /f && reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v AutoConfigURL /f 2>nul & ipconfig /flushdns`;
    } else {
      disableProxyClean();
      return Promise.resolve();
    }
  }

  return new Promise((resolve, reject) => {
    if (!command) return resolve();
    exec(command, (error) => {
      if (error) return reject(error);
      resolve();
    });
  });
};

function updateTrayMenu() {
  if (!appTray) return;

  const menuTemplate = [
    {
      label: systemState.isConnected
        ? `Подключено: ${systemState.activeProxy?.name}`
        : "Отключено",
      enabled: false,
    },
    { type: "separator" },
    { label: "Развернуть окно", click: () => mainWindow && mainWindow.show() },
    { type: "separator" },
  ];

  if (uiProxies.length > 0) {
    menuTemplate.push({ label: "Сохраненные серверы", enabled: false });
    uiProxies.forEach((p) => {
      const isCurrent =
        systemState.isConnected && systemState.activeProxy?.id === p.id;
      menuTemplate.push({
        label: `${isCurrent ? "✓ " : "  "} ${p.name}`,
        click: async () => {
          if (isCurrent) return;

          const { alive } = await pingProxy(p.ip, p.port);
          if (!alive && !systemState.killSwitch) {
            return;
          }

          const stats = await getNetworkTraffic();
          sessionStartStats = {
            received: stats.received || 0,
            sent: stats.sent || 0,
          };
          lastTickStats = { ...sessionStartStats, time: Date.now() };
          systemState = { ...systemState, bytesReceived: 0, bytesSent: 0 };

          await setSystemProxy(true, p);
          systemState = {
            ...systemState,
            isConnected: true,
            activeProxy: p,
            isProxyDead: !alive,
          };
          if (!alive && systemState.killSwitch) applyKillSwitch();
          updateTrayMenu();
        },
      });
    });
    menuTemplate.push({ type: "separator" });
  }

  if (systemState.isConnected) {
    menuTemplate.push({
      label: "Отключить защиту",
      click: async () => {
        await setSystemProxy(false);
        systemState.isConnected = false;
        systemState.activeProxy = null;
        systemState.isProxyDead = false;
        updateTrayMenu();
      },
    });
  }

  menuTemplate.push({
    label: "Выход",
    click: () => {
      app.isQuitting = true;
      app.quit();
    },
  });
  appTray.setContextMenu(Menu.buildFromTemplate(menuTemplate));
}

// === БАЗА ДАННЫХ И API ===

// Новый маршрут: Определение страны по IP
server.post("/api/detect-country", (req, res) => {
  const { ip } = req.body;
  if (!ip) return res.json({ country: "🌐" });

  // Очищаем IP от порта, если он есть
  let cleanIp = ip.split(":")[0];

  // Для локальных адресов возвращаем домик
  if (
    cleanIp === "127.0.0.1" ||
    cleanIp === "localhost" ||
    cleanIp.startsWith("192.168.")
  ) {
    return res.json({ country: "🏠" });
  }

  http
    .get(`http://ip-api.com/json/${cleanIp}?fields=countryCode`, (apiRes) => {
      let data = "";
      apiRes.on("data", (chunk) => (data += chunk));
      apiRes.on("end", () => {
        try {
          const parsed = JSON.parse(data);
          if (parsed.countryCode) {
            // Переводим 2 буквы (например "US") в эмодзи-флаг
            const codePoints = parsed.countryCode
              .toUpperCase()
              .split("")
              .map((char) => 127397 + char.charCodeAt(0));
            const emoji = String.fromCodePoint(...codePoints);
            res.json({ country: emoji });
          } else {
            res.json({ country: "🌐" });
          }
        } catch (e) {
          res.json({ country: "🌐" });
        }
      });
    })
    .on("error", () => {
      res.json({ country: "🌐" });
    });
});

server.get("/api/config", (req, res) => {
  try {
    if (fs.existsSync(configPath)) {
      const data = JSON.parse(fs.readFileSync(configPath, "utf8"));
      if (data.routingRules) {
        if (!data.routingRules.whitelist)
          data.routingRules.whitelist = ["localhost", "127.0.0.1"];
        if (!data.routingRules.appWhitelist)
          data.routingRules.appWhitelist = [];
      }
      res.json(data);
    } else {
      res.json({
        routingRules: {
          mode: "global",
          whitelist: ["localhost", "127.0.0.1"],
          appWhitelist: [],
        },
      });
    }
  } catch (err) {
    res.json({
      routingRules: {
        mode: "global",
        whitelist: ["localhost", "127.0.0.1"],
        appWhitelist: [],
      },
    });
  }
});

server.post("/api/config", (req, res) => {
  try {
    const dir = path.dirname(configPath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }
    fs.writeFileSync(configPath, JSON.stringify(req.body, null, 2));
    res.json({ success: true });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

server.get("/api/status", async (req, res) => {
  if (systemState.isConnected) {
    const now = Date.now();
    const currentStats = await getNetworkTraffic();
    const timeDiff = (now - lastTickStats.time) / 1000;

    let dRec = (currentStats.received || 0) - lastTickStats.received;
    let dSent = (currentStats.sent || 0) - lastTickStats.sent;

    if (dRec < 0) dRec = 0;
    if (dSent < 0) dSent = 0;

    systemState.bytesReceived += dRec;
    systemState.bytesSent += dSent;

    let sRec = dRec > 0 ? dRec / timeDiff : 0;
    let sSent = dSent > 0 ? dSent / timeDiff : 0;

    systemState.speedReceived = sRec > 2048 ? sRec : 0;
    systemState.speedSent = sSent > 2048 ? sSent : 0;

    lastTickStats = {
      received: currentStats.received || 0,
      sent: currentStats.sent || 0,
      time: now,
    };
  } else {
    systemState.speedReceived = 0;
    systemState.speedSent = 0;
  }

  res.json({
    isConnected: systemState.isConnected,
    isProxyDead: systemState.isProxyDead,
    activeProxy: systemState.activeProxy,
    bytesReceived: systemState.bytesReceived,
    bytesSent: systemState.bytesSent,
    speedReceived: systemState.speedReceived,
    speedSent: systemState.speedSent,
  });
});

server.post("/api/ping", async (req, res) => {
  const { ip, port } = req.body;
  const result = await pingProxy(ip, port);
  res.json(result);
});

server.post("/api/killswitch", (req, res) => {
  systemState.killSwitch = !!req.body.enable;

  if (
    systemState.killSwitch &&
    systemState.isProxyDead &&
    systemState.isConnected
  ) {
    applyKillSwitch();
  } else if (
    !systemState.killSwitch &&
    systemState.isProxyDead &&
    systemState.isConnected
  ) {
    addBackendLog(
      "[KILL SWITCH] Отключен вручную. Снимаем блокировку.",
      "info",
    );
    setSystemProxy(true, systemState.activeProxy, true);
  }

  res.json({ success: true });
});

server.post("/api/sync-proxies", (req, res) => {
  uiProxies = req.body || [];
  updateTrayMenu();
  res.sendStatus(200);
});

server.post("/api/connect", async (req, res) => {
  addBackendLog("--- НОВЫЙ ЗАПРОС НА ПОДКЛЮЧЕНИЕ ---", "info");
  try {
    const proxy = req.body;
    systemState.killSwitch = !!proxy.killSwitch;

    const { alive, ping } = await pingProxy(proxy.ip, proxy.port);

    if (!alive) {
      if (systemState.killSwitch) {
        applyKillSwitch();
        systemState = {
          ...systemState,
          isConnected: true,
          activeProxy: proxy,
          isProxyDead: true,
        };
        updateTrayMenu();
        return res.status(200).json({ success: true });
      } else {
        throw new Error(`Узел ${proxy.ip}:${proxy.port} не отвечает.`);
      }
    }

    const stats = await getNetworkTraffic();
    sessionStartStats = {
      received: stats.received || 0,
      sent: stats.sent || 0,
    };
    lastTickStats = { ...sessionStartStats, time: Date.now() };
    systemState.bytesReceived = 0;
    systemState.bytesSent = 0;

    await setSystemProxy(true, proxy);
    systemState = {
      ...systemState,
      isConnected: true,
      activeProxy: proxy,
      isProxyDead: false,
    };
    updateTrayMenu();
    res.status(200).json({ success: true });
  } catch (err) {
    addBackendLog(`Ошибка подключения: ${err.message}`, "error");
    res.status(500).json({ error: err.message });
  }
});

server.post("/api/update-rules", async (req, res) => {
  try {
    if (
      systemState.isConnected &&
      systemState.activeProxy &&
      !systemState.isProxyDead
    ) {
      systemState.activeProxy.rules = req.body;
      await setSystemProxy(true, systemState.activeProxy, true);
    }
    res.status(200).json({ success: true });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

server.post("/api/disconnect", async (req, res) => {
  addBackendLog("--- ЗАПРОС НА ОТКЛЮЧЕНИЕ ---", "info");
  try {
    disableProxyClean();

    if (localProxyServer) {
      await localProxyServer.close(true);
      localProxyServer = null;
    }
    if (localSocksServer) {
      localSocksServer.close();
      localSocksServer = null;
    }

    systemState.isConnected = false;
    systemState.activeProxy = null;
    systemState.isProxyDead = false;
    updateTrayMenu();
    res.status(200).json({ success: true });
  } catch (err) {
    addBackendLog(`Ошибка отключения: ${err.message}`, "error");
    res.status(500).json({ error: err.message });
  }
});

server.post("/api/autostart", (req, res) => {
  try {
    app.setLoginItemSettings({
      openAtLogin: req.body.enable,
      path: process.execPath,
    });
    res.status(200).json({ success: true });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

server.listen(14080, "127.0.0.1");

function createWindow() {
  const iconPath =
    process.env.NODE_ENV === "development"
      ? path.join(__dirname, "src", "assets", "logo.png")
      : path.join(__dirname, "dist", "logo.png");

  mainWindow = new BrowserWindow({
    width: 1050,
    height: 780,
    icon: nativeImage.createFromPath(iconPath),
    autoHideMenuBar: true,
    webPreferences: { nodeIntegration: true, contextIsolation: false },
  });

  mainWindow.loadURL(
    process.env.NODE_ENV === "development"
      ? "http://localhost:5173"
      : `file://${path.join(__dirname, "dist/index.html")}`,
  );

  mainWindow.on("close", function (event) {
    if (!app.isQuitting) {
      event.preventDefault();
      mainWindow.hide();
    }
    return false;
  });
}

app.whenReady().then(() => {
  createWindow();

  const iconPath =
    process.env.NODE_ENV === "development"
      ? path.join(__dirname, "src", "assets", "logo.png")
      : path.join(__dirname, "dist", "logo.png");

  let trayIcon = nativeImage.createFromPath(iconPath);

  if (trayIcon.isEmpty()) {
    const fallbackBase64 =
      "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAZElEQVQ4T2NkoBAwUqifYdQAhoEwMCzIz/9PhH5GUjTjM4jRgFmwgVEQx8I3jGwA3Awm+QUY/v//z4BVA27XkGwALuPRjEExAIfL0DUQsoEuDbiSBaMWUFAzYvU/sYKQ3AAiBwAASiowZf1PzCgAAAAASUVORK5CYII=";
    trayIcon = nativeImage.createFromBuffer(
      Buffer.from(fallbackBase64, "base64"),
    );
    console.log(
      "[ВНИМАНИЕ] Ваша иконка logo.png не найдена по пути: " + iconPath,
    );
  }

  appTray = new Tray(trayIcon);
  appTray.setToolTip("ResultProxy");

  appTray.on("click", () => {
    if (mainWindow) {
      mainWindow.isVisible() ? mainWindow.hide() : mainWindow.show();
    }
  });

  updateTrayMenu();
});

app.on("before-quit", () => {
  app.isQuitting = true;
  disableProxyClean();
});
