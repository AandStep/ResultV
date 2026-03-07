const express = require("express");
const cors = require("cors");
const https = require("https");
const { app } = require("electron");
const {
  validateIp,
  validatePort,
  sanitizeProxy,
  validateRules,
} = require("./validators.cjs");

// ═══════════════════════════════════════════════════════════
// Rate Limiting (in-memory, без внешних зависимостей)
// ═══════════════════════════════════════════════════════════
const AUTH_FAIL_LIMIT = 10; // Макс. неудачных попыток
const AUTH_FAIL_WINDOW_MS = 60000; // Окно в 60 секунд
const AUTH_BLOCK_DURATION_MS = 300000; // Блокировка на 5 минут

const authFailures = new Map(); // ip -> { count, firstFailAt }
const blockedIps = new Map(); // ip -> unblockAt

function isRateLimited(ip) {
  const blockedUntil = blockedIps.get(ip);
  if (blockedUntil) {
    if (Date.now() < blockedUntil) return true;
    blockedIps.delete(ip);
  }
  return false;
}

function recordAuthFailure(ip) {
  const now = Date.now();
  let record = authFailures.get(ip);

  if (!record || now - record.firstFailAt > AUTH_FAIL_WINDOW_MS) {
    record = { count: 1, firstFailAt: now };
  } else {
    record.count++;
  }

  authFailures.set(ip, record);

  if (record.count >= AUTH_FAIL_LIMIT) {
    blockedIps.set(ip, now + AUTH_BLOCK_DURATION_MS);
    authFailures.delete(ip);
  }
}

// ═══════════════════════════════════════════════════════════
// GEO-API helper (HTTPS only, цепочка fallback'ов)
// ═══════════════════════════════════════════════════════════
function httpsGet(url, timeoutMs = 3000) {
  return new Promise((resolve, reject) => {
    const req = https.get(url, (res) => {
      let data = "";
      res.on("data", (chunk) => (data += chunk));
      res.on("end", () => {
        try {
          resolve(JSON.parse(data));
        } catch (e) {
          reject(new Error("JSON parse error"));
        }
      });
    });
    req.on("error", reject);
    req.setTimeout(timeoutMs, () => {
      req.destroy();
      reject(new Error("timeout"));
    });
  });
}

async function detectCountryBackend(cleanIp) {
  // 1. iplocation.net (HTTPS)
  try {
    const data = await httpsGet(`https://api.iplocation.net/?ip=${cleanIp}`);
    if (
      data &&
      data.country_code2 &&
      data.country_code2 !== "-" &&
      data.country_code2.length === 2
    ) {
      return data.country_code2.toLowerCase();
    }
  } catch (e) {}

  // 2. geojs.io (HTTPS)
  try {
    const data = await httpsGet(
      `https://get.geojs.io/v1/ip/country/${cleanIp}.json`,
    );
    if (data && data.country_code && data.country_code.length === 2) {
      return data.country_code.toLowerCase();
    }
  } catch (e) {}

  // 3. country.is (HTTPS)
  try {
    const data = await httpsGet(`https://api.country.is/${cleanIp}`);
    if (data && data.country && data.country.length === 2) {
      return data.country.toLowerCase();
    }
  } catch (e) {}

  return null;
}

// ═══════════════════════════════════════════════════════════
// API Server
// ═══════════════════════════════════════════════════════════
class ApiServer {
  constructor(
    loggerService,
    stateStore,
    configManager,
    proxyManager,
    trayManager,
    trafficMonitor,
    systemAdapter,
  ) {
    this.logger = loggerService;
    this.stateStore = stateStore;
    this.configManager = configManager;
    this.proxyManager = proxyManager;
    this.trayManager = trayManager;
    this.trafficMonitor = trafficMonitor;
    this.systemAdapter = systemAdapter;

    this.app = express();
    this.port = 14080;
  }

  start() {
    const corsOptions = {
      origin: ["http://localhost:5173", "file://", "electron://"],
      optionsSuccessStatus: 200,
    };
    this.app.use(cors(corsOptions));
    this.app.use(express.json());

    // Authorization middleware с rate limiting
    const authManager = require("../core/auth.manager.cjs");
    this.app.use((req, res, next) => {
      // Allow preflight options
      if (req.method === "OPTIONS") return next();

      const clientIp = req.ip || req.connection.remoteAddress || "unknown";

      // Проверка rate limit
      if (isRateLimited(clientIp)) {
        return res
          .status(429)
          .json({ error: "Too many failed requests. Try again later." });
      }

      if (!authManager.verifyRequest(req)) {
        recordAuthFailure(clientIp);
        return res.status(401).json({ error: "Unauthorized access" });
      }
      next();
    });

    // Registering Routes
    this._registerRoutes();

    this.app.listen(this.port, "127.0.0.1", () => {
      this.logger.log(
        `API Server listening at http://127.0.0.1:${this.port}`,
        "info",
      );
    });
  }

  _registerRoutes() {
    this.app.get("/api/logs", (req, res) => {
      res.json(this.logger.getLogs());
    });

    this.app.post("/api/detect-country", async (req, res) => {
      const { ip } = req.body;
      if (!ip) return res.json({ country: "🌐" });

      let cleanIp = ip.split(":")[0];

      // Валидация IP
      if (!validateIp(cleanIp)) {
        return res.json({ country: "🌐" });
      }

      if (
        cleanIp === "127.0.0.1" ||
        cleanIp === "localhost" ||
        cleanIp.startsWith("192.168.") ||
        cleanIp.startsWith("10.")
      ) {
        return res.json({ country: "🏠" });
      }

      const country = await detectCountryBackend(cleanIp);
      res.json({ country: country || "🌐" });
    });

    this.app.get("/api/config", (req, res) => {
      res.json(this.configManager.getConfig());
    });

    this.app.post("/api/config", (req, res) => {
      try {
        this.configManager.save(req.body);
        res.json({ success: true });
      } catch (err) {
        res.status(500).json({ error: err.message });
      }
    });

    this.app.get("/api/status", async (req, res) => {
      const latestState = this.stateStore.getState();
      res.json({
        isConnected: latestState.isConnected,
        isProxyDead: latestState.isProxyDead,
        activeProxy: latestState.activeProxy,
        bytesReceived: latestState.bytesReceived,
        bytesSent: latestState.bytesSent,
        speedReceived: latestState.speedReceived,
        speedSent: latestState.speedSent,
      });
    });

    this.app.post("/api/ping", async (req, res) => {
      const { ip, port } = req.body;

      // Валидация входных данных
      if (!validateIp(ip)) {
        return res.status(400).json({ error: "Невалидный IP адрес" });
      }
      if (!validatePort(port)) {
        return res.status(400).json({ error: "Невалидный порт" });
      }

      const result = await this.trafficMonitor.pingProxy(ip, port);
      res.json(result);
    });

    this.app.post("/api/killswitch", async (req, res) => {
      const enable = !!req.body.enable;
      this.stateStore.update({ killSwitch: enable });

      const state = this.stateStore.getState();

      // Проверка прав администратора
      let needsAdmin = false;
      if (enable && process.platform === "win32") {
        const { execSync } = require("child_process");
        try {
          execSync("net session", { stdio: "ignore" });
        } catch (e) {
          needsAdmin = true;
        }
      }

      if (state.killSwitch && state.isProxyDead && state.isConnected) {
        await this.proxyManager.applyKillSwitch();
      } else if (!state.killSwitch && state.isProxyDead && state.isConnected) {
        this.logger.log(
          "[KILL SWITCH] Отключен вручную. Снимаем блокировку.",
          "info",
        );
        await this.proxyManager.setSystemProxy(true, state.activeProxy, true);
      }

      res.json({ success: true, needsAdmin });
    });

    this.app.post("/api/sync-proxies", (req, res) => {
      this.stateStore.update({ uiProxies: req.body || [] });
      this.trayManager.updateMenu();
      res.sendStatus(200);
    });

    this.app.post("/api/connect", async (req, res) => {
      this.logger.log("--- НОВЫЙ ЗАПРОС НА ПОДКЛЮЧЕНИЕ ---", "info");
      try {
        const proxy = req.body;

        // Валидация прокси-объекта
        const validation = sanitizeProxy(proxy);
        if (!validation.valid) {
          return res.status(400).json({ error: validation.error });
        }

        this.stateStore.update({ killSwitch: !!proxy.killSwitch });

        const { alive } = await this.trafficMonitor.pingProxy(
          proxy.ip,
          proxy.port,
        );
        const state = this.stateStore.getState();

        if (!alive) {
          if (state.killSwitch) {
            await this.proxyManager.applyKillSwitch();
            this.stateStore.update({
              isConnected: true,
              activeProxy: proxy,
              isProxyDead: true,
            });
            this.trayManager.updateMenu();
            return res.status(200).json({ success: true });
          } else {
            throw new Error(`Узел ${proxy.ip}:${proxy.port} не отвечает.`);
          }
        }

        const stats = await this.systemAdapter.getNetworkTraffic();
        const sessionStartStats = {
          received: stats.received || 0,
          sent: stats.sent || 0,
        };

        this.stateStore.update({
          sessionStartStats,
          lastTickStats: { ...sessionStartStats, time: Date.now() },
          bytesReceived: 0,
          bytesSent: 0,
        });

        await this.proxyManager.setSystemProxy(true, proxy);

        this.stateStore.update({
          isConnected: true,
          activeProxy: proxy,
          isProxyDead: false,
        });
        this.trayManager.updateMenu();

        // DNS leak warning: HTTP-прокси не проксируют DNS-запросы
        const proxyType = (proxy.type || "HTTP").toUpperCase();
        const dnsLeakWarning = proxyType !== "SOCKS5";

        res.status(200).json({ success: true, dnsLeakWarning });
      } catch (err) {
        this.logger.log(`Ошибка подключения: ${err.message}`, "error");
        res.status(500).json({ error: err.message });
      }
    });

    this.app.post("/api/update-rules", async (req, res) => {
      try {
        // Валидация rules
        const validation = validateRules(req.body);
        if (!validation.valid) {
          return res.status(400).json({ error: validation.error });
        }

        const state = this.stateStore.getState();
        if (state.isConnected && state.activeProxy && !state.isProxyDead) {
          const updatedProxy = { ...state.activeProxy, rules: req.body };
          this.stateStore.update({ activeProxy: updatedProxy });
          await this.proxyManager.setSystemProxy(true, updatedProxy, true);
        }
        res.status(200).json({ success: true });
      } catch (err) {
        res.status(500).json({ error: err.message });
      }
    });

    this.app.post("/api/disconnect", async (req, res) => {
      this.logger.log("--- ЗАПРОС НА ОТКЛЮЧЕНИЕ ---", "info");
      try {
        await this.proxyManager.setSystemProxy(false);
        this.stateStore.update({
          isConnected: false,
          activeProxy: null,
          isProxyDead: false,
        });
        this.trayManager.updateMenu();
        res.status(200).json({ success: true });
      } catch (err) {
        this.logger.log(`Ошибка отключения: ${err.message}`, "error");
        res.status(500).json({ error: err.message });
      }
    });

    this.app.post("/api/autostart", (req, res) => {
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

    this.app.get("/api/platform", (req, res) => {
      const os = require("os");
      res.json({ platform: os.platform() });
    });

    this.app.get("/api/version", (req, res) => {
      let version = app.getVersion();
      if (!app.isPackaged) {
        try {
          const path = require("path");
          const fs = require("fs");
          const packageJsonPath = path.join(__dirname, "../../package.json");
          const packageJson = JSON.parse(
            fs.readFileSync(packageJsonPath, "utf-8"),
          );
          version = packageJson.version;
        } catch (e) {
          console.error("Не удалось прочитать package.json:", e);
        }
      }
      res.json({ version });
    });

    this.app.get("/api/admin-status", (req, res) => {
      const { execSync } = require("child_process");
      let isAdmin = false;
      if (process.platform === "win32") {
        try {
          execSync("net session", { stdio: "ignore" });
          isAdmin = true;
        } catch (e) {}
      } else {
        // На Linux/macOS проверяем uid === 0
        isAdmin = process.getuid && process.getuid() === 0;
      }
      res.json({ isAdmin, platform: process.platform });
    });
  }
}

module.exports = ApiServer;
