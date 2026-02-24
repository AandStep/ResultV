const { app, BrowserWindow, Tray, Menu, nativeImage } = require("electron");
const path = require("path");

// ПРИНУДИТЕЛЬНО ЗАДАЕМ ПАПКУ ПРИЛОЖЕНИЯ
app.setPath("userData", path.join(app.getPath("appData"), "resultProxy"));

// === ЗАЩИТА ОТ ДВОЙНОГО ЗАПУСКА (SINGLE INSTANCE LOCK) ===
const gotTheLock = app.requestSingleInstanceLock();

let mainWindow = null;
let appTray = null;

if (!gotTheLock) {
  // Если копия уже запущена, закрываем эту новую попытку старта моментально
  app.quit();
  process.exit(0);
} else {
  // Если это первая копия, слушаем попытки запуска вторых копий
  app.on("second-instance", (event, commandLine, workingDirectory) => {
    if (mainWindow) {
      if (!mainWindow.isVisible()) mainWindow.show();
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.focus();
    }
  });
}
// ==========================================================

const express = require("express");
const cors = require("cors");
const os = require("os");
const ProxyChain = require("proxy-chain");
const net = require("net");
const { SocksClient } = require("socks");
const fs = require("fs");
const http = require("http");

// === ПОДКЛЮЧЕНИЕ СИСТЕМНЫХ АДАПТЕРОВ ===
let systemCore = null;

if (os.platform() === "win32") {
  systemCore = require("./system/windows.cjs");
} else if (os.platform() === "darwin") {
  systemCore = require("./system/mac.cjs");
} else if (os.platform() === "linux") {
  // Подключаем модуль для Linux
  systemCore = require("./system/linux.cjs");
} else {
  // Временная заглушка для совсем экзотических ОС
  systemCore = {
    startProcessCacheInterval: () => {},
    getNetworkTraffic: async () => ({ received: 0, sent: 0 }),
    applyKillSwitch: (logCallback) => {
      if (logCallback)
        logCallback("Killswitch пока не поддерживается на этой ОС", "warning");
      return Promise.resolve();
    },
    disableSystemProxy: (logCallback) => {
      if (logCallback)
        logCallback(
          "Очистка прокси пока не реализована для этой ОС",
          "warning",
        );
      return Promise.resolve();
    },
    setSystemProxy: (ip, port, type, wl, logCallback) => {
      if (logCallback)
        logCallback(
          `Авто-настройка не поддерживается. Настройте прокси вручную: ${ip}:${port}`,
          "warning",
        );
      return Promise.resolve();
    },
    checkAppWhitelist: async () => false,
  };
}
// =======================================

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
  backendLogs.unshift({ timestamp: Date.now(), time, msg, type });
  if (backendLogs.length > 100) backendLogs.pop();
  console.log(`[${time}] ${msg}`);
}

// Запускаем фоновый сборщик процессов (через Адаптер)
systemCore.startProcessCacheInterval(() => ({
  isConnected: systemState.isConnected,
  activeProxy: systemState.activeProxy,
}));

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
      await systemCore.applyKillSwitch(addBackendLog);
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

const setSystemProxy = async (
  enable,
  proxy = null,
  updateRegistryOnly = false,
) => {
  if (localProxyServer && !updateRegistryOnly) {
    await localProxyServer.close(true);
    localProxyServer = null;
  }
  if (localSocksServer && !updateRegistryOnly) {
    localSocksServer.close();
    localSocksServer = null;
  }

  let proxyIp = "127.0.0.1";
  let proxyPort = "14081";
  let proxyType = "ALL";
  let rules = {
    mode: "global",
    whitelist: ["localhost", "127.0.0.1"],
    appWhitelist: [],
  };

  if (enable && proxy) {
    proxyIp = proxy.ip;
    proxyPort = proxy.port;
    proxyType = proxy.type || "HTTP";
    rules = proxy.rules || rules;

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
                      dstHost = buffer.slice(offset, domainNullIdx).toString();
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
                  if (
                    currentRules.appWhitelist &&
                    currentRules.appWhitelist.length > 0
                  ) {
                    const clientPort = client.remotePort;
                    // Вызов системного адаптера
                    const matchedApp = await systemCore.checkAppWhitelist(
                      clientPort,
                      currentRules.appWhitelist,
                      dstHost,
                      addBackendLog,
                    );
                    if (matchedApp) {
                      isAppWhitelisted = true;
                    }
                  }

                  const isWhitelisted =
                    currentRules.whitelist &&
                    currentRules.whitelist.some((d) => dstHost.includes(d));
                  const isBlocked = BLOCKED_RESOURCES.some((d) =>
                    dstHost.includes(d),
                  );

                  let useProxy =
                    currentRules.mode === "smart" ? isBlocked : !isWhitelisted;

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
                  request.socket?.remotePort || request.connection?.remotePort;
                // Вызов системного адаптера
                const appName = await systemCore.checkAppWhitelist(
                  clientPort,
                  currentRules.appWhitelist,
                  hostname,
                  addBackendLog,
                );
                if (appName) {
                  return { requestAuthentication: false };
                }
              }

              if (currentRules.whitelist && currentRules.whitelist.length > 0) {
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

    // Вызов системного адаптера для установки прокси в ОС
    await systemCore.setSystemProxy(
      proxyIp,
      proxyPort,
      proxyType,
      rules.whitelist,
      !updateRegistryOnly ? addBackendLog : null,
    );
  } else {
    // Вызов системного адаптера для очистки прокси в ОС
    await systemCore.disableSystemProxy(addBackendLog);
  }
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

          const stats = await systemCore.getNetworkTraffic();
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
          if (!alive && systemState.killSwitch)
            await systemCore.applyKillSwitch(addBackendLog);
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

server.get("/api/logs", (req, res) => {
  res.json(backendLogs);
});

server.post("/api/detect-country", (req, res) => {
  const { ip } = req.body;
  if (!ip) return res.json({ country: "🌐" });

  let cleanIp = ip.split(":")[0];

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
            res.json({ country: parsed.countryCode.toLowerCase() });
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
    // Использование адаптера для получения статистики
    const currentStats = await systemCore.getNetworkTraffic();
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

server.post("/api/killswitch", async (req, res) => {
  systemState.killSwitch = !!req.body.enable;

  if (
    systemState.killSwitch &&
    systemState.isProxyDead &&
    systemState.isConnected
  ) {
    await systemCore.applyKillSwitch(addBackendLog);
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
        await systemCore.applyKillSwitch(addBackendLog);
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

    const stats = await systemCore.getNetworkTraffic();
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
    await systemCore.disableSystemProxy(addBackendLog);

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
      ? path.join(__dirname, "../public", "logo.png")
      : path.join(__dirname, "../dist", "logo.png");

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
      : `file://${path.join(__dirname, "../dist/index.html")}`,
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
      ? path.join(__dirname, "../public", "logo.png")
      : path.join(__dirname, "../dist", "logo.png");

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

app.on("before-quit", async () => {
  app.isQuitting = true;
  await systemCore.disableSystemProxy();
});
