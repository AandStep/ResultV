const express = require("express");
const cors = require("cors");
const http = require("http");
const { app } = require("electron");

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

    // Authorization middleware
    const authManager = require("../core/auth.manager.cjs");
    this.app.use((req, res, next) => {
      // Allow preflight options
      if (req.method === "OPTIONS") return next();

      if (!authManager.verifyRequest(req)) {
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

    this.app.post("/api/detect-country", (req, res) => {
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
        .get(
          `http://ip-api.com/json/${cleanIp}?fields=countryCode`,
          (apiRes) => {
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
          },
        )
        .on("error", () => {
          res.json({ country: "🌐" });
        });
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
      const result = await this.trafficMonitor.pingProxy(ip, port);
      res.json(result);
    });

    this.app.post("/api/killswitch", async (req, res) => {
      const enable = !!req.body.enable;
      this.stateStore.update({ killSwitch: enable });

      const state = this.stateStore.getState();

      if (state.killSwitch && state.isProxyDead && state.isConnected) {
        await this.proxyManager.applyKillSwitch();
      } else if (!state.killSwitch && state.isProxyDead && state.isConnected) {
        this.logger.log(
          "[KILL SWITCH] Отключен вручную. Снимаем блокировку.",
          "info",
        );
        await this.proxyManager.setSystemProxy(true, state.activeProxy, true);
      }

      res.json({ success: true });
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
        res.status(200).json({ success: true });
      } catch (err) {
        this.logger.log(`Ошибка подключения: ${err.message}`, "error");
        res.status(500).json({ error: err.message });
      }
    });

    this.app.post("/api/update-rules", async (req, res) => {
      try {
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
      // В режиме разработки app.getVersion() может возвращать версию самого Electron (например 40.6.0),
      // так как точка входа находится в папке backend.
      // Поэтому для гарантии отдаём версию из package.json в dev режиме:
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
  }
}

module.exports = ApiServer;
