import React, { useState, useEffect, useRef } from "react";

// ⚠️ ВАЖНО ДЛЯ ВАШЕГО ПК: Раскомментируйте эту строку, чтобы работало ваше лого:
// import logo from "./assets/logo.png";

// ⚠️ И удалите вот эту строку-заглушку (она нужна только чтобы песочница не выдавала ошибку):
const logo =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";

import {
  Globe,
  Plus,
  List,
  Settings,
  Power,
  Trash2,
  Activity,
  Lock,
  Server,
  Terminal,
  Pencil,
  Download,
  Upload,
  Split,
  ShoppingCart,
  ExternalLink,
  Copy,
  Check,
  Shield,
  ChevronDown,
} from "lucide-react";

// Минималистичный график скорости
const SpeedChart = ({ data, color }) => {
  if (!data || data.length < 2) return null;
  const max = Math.max(...data, 1024);
  const points = data
    .map(
      (val, i) => `${(i / (data.length - 1)) * 100},${25 - (val / max) * 25}`,
    )
    .join(" ");

  return (
    <svg
      viewBox="0 0 100 28"
      className="w-full h-8 mt-3 overflow-visible opacity-90"
    >
      <polyline
        fill="none"
        stroke={color}
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        points={points}
        className="transition-all duration-500"
      />
    </svg>
  );
};

// Умный компонент для отображения настоящих PNG-флагов, обходящий ограничения Windows
const FlagIcon = ({ code, className = "" }) => {
  const [imgError, setImgError] = useState(false);

  if (!code || code === "unknown" || code === "🌐")
    return <Globe className="w-6 h-6 text-zinc-500 shrink-0" />;
  if (code === "local" || code === "🏠")
    return <Server className="w-6 h-6 text-zinc-500 shrink-0" />;

  let isoCode = null;

  if (/^[a-zA-Z]{2}$/.test(code)) {
    isoCode = code.toLowerCase();
  } else {
    const clean = code.replace(/[\uFE0F]/g, "").trim();
    if (clean.length > 0) {
      const cp1 = clean.codePointAt(0);
      if (cp1 >= 0x1f1e6 && cp1 <= 0x1f1ff) {
        const cp2 = clean.codePointAt(2);
        if (cp2 >= 0x1f1e6 && cp2 <= 0x1f1ff) {
          isoCode =
            String.fromCharCode(cp1 - 0x1f1e6 + 97) +
            String.fromCharCode(cp2 - 0x1f1e6 + 97);
        }
      }
    }
  }

  if (isoCode && !imgError) {
    return (
      <img
        src={`https://flagcdn.com/w40/${isoCode}.png`}
        srcSet={`https://flagcdn.com/w80/${isoCode}.png 2x`}
        alt={isoCode.toUpperCase()}
        className={`block shrink-0 object-contain ${className}`}
        onError={() => setImgError(true)}
      />
    );
  }

  return (
    <span className="text-xl leading-none shrink-0 font-sans">{code}</span>
  );
};

export default function App() {
  const [isConfigLoaded, setIsConfigLoaded] = useState(false);
  const [activeTab, setActiveTab] = useState("home");
  const [isConnected, setIsConnected] = useState(false);
  const [isProxyDead, setIsProxyDead] = useState(false);
  const [failedProxy, setFailedProxy] = useState(null);
  const [activeProxy, setActiveProxy] = useState(null);
  const [editingProxy, setEditingProxy] = useState(null);

  const isSwitchingRef = useRef(false);
  const prevProxyDead = useRef(false);

  const [proxies, setProxies] = useState([]);

  const [routingRules, setRoutingRules] = useState({
    mode: "global",
    whitelist: ["localhost", "127.0.0.1"],
    appWhitelist: [],
  });

  const [settings, setSettings] = useState({
    autostart: false,
    killswitch: false,
  });

  const [stats, setStats] = useState({ download: 0, upload: 0 });
  const [speedHistory, setSpeedHistory] = useState({
    down: new Array(20).fill(0),
    up: new Array(20).fill(0),
  });

  const [pings, setPings] = useState({});
  const [daemonStatus, setDaemonStatus] = useState("checking");
  const DAEMON_URL = "http://127.0.0.1:14080";

  const [logs, setLogs] = useState([
    {
      timestamp: Date.now(),
      time: new Date().toLocaleTimeString(),
      msg: "Интерфейс запущен. Загрузка конфигурации...",
      type: "info",
    },
  ]);

  const [backendLogs, setBackendLogs] = useState([]);

  const addLog = (msg, type = "info") => {
    setLogs((prev) =>
      [
        {
          timestamp: Date.now(),
          time: new Date().toLocaleTimeString(),
          msg,
          type,
        },
        ...prev,
      ].slice(0, 50),
    );
  };

  useEffect(() => {
    fetch(`${DAEMON_URL}/api/config`)
      .then((res) => res.json())
      .then((data) => {
        if (data.proxies && data.proxies.length > 0) {
          setProxies(data.proxies);
        }
        if (data.routingRules) setRoutingRules(data.routingRules);
        if (data.settings) setSettings(data.settings);

        setIsConfigLoaded(true);
        addLog("Конфигурация успешно загружена.", "success");
      })
      .catch(() => {
        setIsConfigLoaded(true);
        addLog("Служба недоступна. Используются базовые настройки.", "error");
      });
  }, []);

  useEffect(() => {
    let interval;
    const fetchLogs = async () => {
      try {
        const res = await fetch(`${DAEMON_URL}/api/logs`);
        if (res.ok) {
          const data = await res.json();
          setBackendLogs(data);
        }
      } catch (e) {}
    };

    fetchLogs();
    interval = setInterval(fetchLogs, 1500);

    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (!isConfigLoaded) return;
    fetch(`${DAEMON_URL}/api/config`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ proxies, routingRules, settings }),
    }).catch(() => {});
  }, [proxies, routingRules, settings, isConfigLoaded]);

  useEffect(() => {
    if (!isConfigLoaded) return;
    fetch(`${DAEMON_URL}/api/update-rules`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(routingRules),
    }).catch(() => {});
  }, [routingRules, isConfigLoaded]);

  useEffect(() => {
    if (!isConfigLoaded) return;
    fetch(`${DAEMON_URL}/api/sync-proxies`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(proxies),
    }).catch(() => {});
  }, [proxies, isConfigLoaded]);

  useEffect(() => {
    if (!isConfigLoaded || proxies.length === 0) return;

    const fetchPings = async () => {
      const newPings = {};
      for (const p of proxies) {
        try {
          const res = await fetch(`${DAEMON_URL}/api/ping`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ ip: p.ip, port: p.port }),
          });
          if (res.ok) {
            const data = await res.json();
            newPings[p.id] = data.alive ? `${data.ping}ms` : "Timeout";
          } else {
            newPings[p.id] = "Error";
          }
        } catch {
          newPings[p.id] = "Error";
        }
      }
      setPings(newPings);
    };

    fetchPings();
    const interval = setInterval(fetchPings, 10000);
    return () => clearInterval(interval);
  }, [proxies, isConfigLoaded]);

  useEffect(() => {
    let interval;
    const fetchStatus = async () => {
      try {
        const res = await fetch(`${DAEMON_URL}/api/status`);
        if (res.ok) {
          const data = await res.json();
          if (daemonStatus !== "online") setDaemonStatus("online");

          setIsProxyDead(!!data.isProxyDead);
          if (data.isConnected) {
            setFailedProxy(null);
          }

          if (data.isConnected) {
            if (data.isProxyDead && !prevProxyDead.current) {
              addLog(
                `Внимание: Узел ${data.activeProxy?.ip || ""} перестал отвечать!`,
                "error",
              );
            } else if (!data.isProxyDead && prevProxyDead.current) {
              addLog(`Связь с узлом восстановлена.`, "success");
            }
          }
          prevProxyDead.current = !!data.isProxyDead;

          if (!isSwitchingRef.current) {
            setIsConnected(data.isConnected);
            if (data.activeProxy) {
              const localMatchedProxy = proxies.find(
                (p) =>
                  p.id === data.activeProxy.id || p.ip === data.activeProxy.ip,
              );
              setActiveProxy(localMatchedProxy || data.activeProxy);
            } else {
              if (!failedProxy) setActiveProxy(null);
            }
          }

          setStats({ download: data.bytesReceived, upload: data.bytesSent });
          setSpeedHistory((h) => ({
            down: [...h.down.slice(1), data.speedReceived || 0],
            up: [...h.up.slice(1), data.speedSent || 0],
          }));
        }
      } catch (error) {
        setDaemonStatus("offline");
        if (isConnected) setIsConnected(false);
      }
    };

    fetchStatus();
    interval = setInterval(fetchStatus, 2000);
    return () => clearInterval(interval);
  }, [isConnected, daemonStatus, proxies, failedProxy]);

  const updateSetting = (key, value) => {
    setSettings((prev) => ({ ...prev, [key]: value }));

    if (key === "autostart") {
      fetch(`${DAEMON_URL}/api/autostart`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enable: value }),
      }).catch(() => {});
    } else if (key === "killswitch") {
      fetch(`${DAEMON_URL}/api/killswitch`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enable: value }),
      }).catch(() => {});
    }
  };

  const toggleConnection = async () => {
    if (daemonStatus !== "online") {
      addLog("Служба недоступна.", "error");
      return;
    }

    const targetProxy = activeProxy || proxies[0];
    if (proxies.length === 0 || !targetProxy) return;

    try {
      isSwitchingRef.current = true;
      setFailedProxy(null);

      if (isConnected) {
        addLog("Отключение...", "info");
        await fetch(`${DAEMON_URL}/api/disconnect`, { method: "POST" });
        addLog("Отключено успешно.", "success");
        setIsConnected(false);
      } else {
        addLog(`Подключение к ${targetProxy.name}...`, "info");
        setActiveProxy(targetProxy);

        const res = await fetch(`${DAEMON_URL}/api/connect`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            ...targetProxy,
            rules: routingRules,
            killSwitch: settings.killswitch,
          }),
        });

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${res.status}`);
        }
        addLog("Соединение установлено.", "success");
        setIsConnected(true);
      }

      setTimeout(() => {
        isSwitchingRef.current = false;
      }, 3000);
    } catch (error) {
      isSwitchingRef.current = false;
      setFailedProxy(targetProxy);
      addLog(`Сбой: ${error.message}`, "error");
    }
  };

  const selectAndConnect = async (proxy, forceReconnect = false) => {
    if (!forceReconnect && activeProxy?.id === proxy.id && isConnected) return;

    try {
      isSwitchingRef.current = true;
      setFailedProxy(null);
      setActiveTab("home");
      setActiveProxy(proxy);
      addLog(`Переключение на: ${proxy.name}...`, "info");

      if (isConnected) {
        await fetch(`${DAEMON_URL}/api/disconnect`, { method: "POST" });
        setIsConnected(false);
      }

      const res = await fetch(`${DAEMON_URL}/api/connect`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...proxy,
          rules: routingRules,
          killSwitch: settings.killswitch,
        }),
      });

      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || "Ошибка смены прокси");
      }

      setIsConnected(true);
      addLog(`Успешно переключено на ${proxy.name}`, "success");

      setTimeout(() => {
        isSwitchingRef.current = false;
      }, 2000);
    } catch (error) {
      isSwitchingRef.current = false;
      setFailedProxy(proxy);
      addLog(`Сбой подключения: ${error.message}`, "error");
    }
  };

  const detectCountry = async (ip) => {
    try {
      let cleanIp = ip.split(":")[0];
      if (
        cleanIp === "127.0.0.1" ||
        cleanIp === "localhost" ||
        cleanIp.startsWith("192.168.")
      ) {
        return "local";
      }
      const res = await fetch(
        `http://ip-api.com/json/${cleanIp}?fields=countryCode`,
      );
      const data = await res.json();
      return data.countryCode ? data.countryCode.toLowerCase() : "unknown";
    } catch (error) {
      return "unknown";
    }
  };

  const handleSaveProxy = async (proxyData) => {
    const countryCode = await detectCountry(proxyData.ip);
    const finalProxy = { ...proxyData, country: countryCode };

    if (proxyData.id) {
      setProxies(proxies.map((p) => (p.id === proxyData.id ? finalProxy : p)));
      if (failedProxy?.id === proxyData.id) setFailedProxy(null);
      addLog(`Профиль "${proxyData.name}" обновлен.`, "success");

      if (activeProxy?.id === proxyData.id) {
        setActiveProxy(finalProxy);
        if (isConnected) {
          addLog("Применение новых настроек, перезапуск...", "info");
          setTimeout(() => {
            selectAndConnect(finalProxy, true);
          }, 100);
        } else {
          setActiveTab("list");
        }
      } else {
        setActiveTab("list");
      }
    } else {
      setProxies([...proxies, { ...finalProxy, id: Date.now() }]);
      addLog(`Новый профиль "${proxyData.name}" добавлен.`, "success");
      setActiveTab("list");
    }
    setEditingProxy(null);
  };

  const deleteProxy = async (id) => {
    const isDeletingActive = activeProxy?.id === id;
    setProxies((prev) => prev.filter((p) => p.id !== id));

    if (isDeletingActive) {
      if (isConnected) {
        isSwitchingRef.current = true;
        addLog("Активный сервер удален. Разрыв соединения...", "info");
        try {
          await fetch(`${DAEMON_URL}/api/disconnect`, { method: "POST" });
          addLog("Отключено успешно.", "success");
        } catch (e) {}
        setIsConnected(false);
        setActiveProxy(null);
        setTimeout(() => {
          isSwitchingRef.current = false;
        }, 2000);
      } else {
        setActiveProxy(null);
      }
    }
    if (failedProxy?.id === id) setFailedProxy(null);
  };

  const editProxy = (proxy) => {
    setEditingProxy(proxy);
    setActiveTab("add");
  };

  const formatBytes = (bytes) => {
    if (!bytes || bytes === 0) return "0.00 MB";
    const mb = bytes / (1024 * 1024);
    if (mb < 1024) return mb.toFixed(2) + " MB";
    return (mb / 1024).toFixed(2) + " GB";
  };

  const formatSpeed = (bytesPerSec) => {
    if (!bytesPerSec || bytesPerSec === 0) return "0.0 KB/s";
    const kb = bytesPerSec / 1024;
    if (kb < 1024) return kb.toFixed(1) + " KB/s";
    return (kb / 1024).toFixed(1) + " MB/s";
  };

  if (!isConfigLoaded) {
    return (
      <div className="fixed inset-0 flex flex-col items-center justify-center bg-zinc-950">
        <div className="relative flex items-center justify-center">
          <img
            src={logo}
            alt="ResultProxy"
            className="w-10 h-10 absolute drop-shadow-[0_0_15px_rgba(0,126,58,0.8)] z-10"
          />
          <div className="w-20 h-20 border-4 border-zinc-800 border-t-[#00A819] rounded-full animate-spin"></div>
        </div>
        <p className="text-zinc-500 mt-6 font-medium animate-pulse">
          Загрузка конфигурации...
        </p>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 flex bg-zinc-950 text-zinc-200 font-sans overflow-hidden select-none">
      <style>{`
        * { 
          outline: none !important; 
          -webkit-tap-highlight-color: transparent !important; 
        }
        button { border-color: transparent; }
        button:hover, a:hover { 
          border-color: transparent;
        }
        button:focus, input:focus, a:focus { 
          outline: none !important; 
          box-shadow: none !important; 
        }
        :root { --bs-primary: transparent; }
        .scrollbar-hide::-webkit-scrollbar { display: none; }
      `}</style>

      <div className="hidden md:flex flex-col w-64 bg-zinc-900 border-r border-zinc-800 shrink-0">
        <div className="p-6 flex items-center space-x-3">
          <img
            src={logo}
            alt="ResultProxy"
            className="w-8 h-8 drop-shadow-[0_0_10px_rgba(0,126,58,0.5)]"
          />
          <span className="text-xl font-bold text-white">ResultProxy</span>
        </div>

        {daemonStatus === "offline" && (
          <div className="mx-4 mb-2 p-2 bg-rose-500/10 border border-rose-500/20 rounded-lg flex items-center text-xs text-rose-400">
            <Activity className="w-4 h-4 mr-2 shrink-0" />
            <span>Служба не отвечает</span>
          </div>
        )}

        <nav className="flex-1 flex flex-col px-4 py-4 overflow-y-auto scrollbar-hide">
          <div className="space-y-1">
            <NavItem
              icon={<Activity />}
              label="Главная"
              isActive={activeTab === "home"}
              onClick={() => setActiveTab("home")}
            />
            <NavItem
              icon={<ShoppingCart />}
              label="Купить прокси"
              isActive={activeTab === "buy"}
              onClick={() => setActiveTab("buy")}
            />
            <NavItem
              icon={<Plus />}
              label="Добавить"
              isActive={activeTab === "add"}
              onClick={() => {
                setEditingProxy(null);
                setActiveTab("add");
              }}
            />
            <NavItem
              icon={<List />}
              label="Список прокси"
              isActive={activeTab === "list"}
              onClick={() => setActiveTab("list")}
            />
            <NavItem
              icon={<Split />}
              label="Умные правила"
              isActive={activeTab === "rules"}
              onClick={() => setActiveTab("rules")}
            />
            <NavItem
              icon={<Terminal />}
              label="Журнал логов"
              isActive={activeTab === "logs"}
              onClick={() => setActiveTab("logs")}
            />
          </div>

          <div className="mt-auto pt-4 space-y-1">
            <NavItem
              icon={<Settings />}
              label="Настройки"
              isActive={activeTab === "settings"}
              onClick={() => setActiveTab("settings")}
            />
          </div>
        </nav>

        <div className="p-4 border-t border-zinc-800 text-xs text-zinc-500 text-center">
          Версия 1.5.0 (stable)
        </div>
      </div>

      <div className="flex-1 flex flex-col relative overflow-y-auto min-w-0">
        <div className="md:hidden flex items-center justify-between p-4 bg-zinc-900 border-b border-zinc-800 sticky top-0 z-10">
          <div className="flex items-center space-x-2">
            <img
              src={logo}
              alt="ResultProxy"
              className="w-6 h-6 drop-shadow-[0_0_8px_rgba(0,126,58,0.5)]"
            />
            <span className="text-lg font-bold text-white">ResultProxy</span>
          </div>
          {isConnected && (
            <div
              className={`w-3 h-3 rounded-full animate-pulse ${isProxyDead ? "bg-rose-500" : "bg-[#007E3A]"}`}
            ></div>
          )}
        </div>

        <div className="p-4 md:p-10 w-full max-w-[1600px] mx-auto pb-24 md:pb-10">
          {activeTab === "home" && (
            <HomeView
              isConnected={isConnected}
              isProxyDead={isProxyDead}
              failedProxy={failedProxy}
              setFailedProxy={setFailedProxy}
              toggleConnection={toggleConnection}
              activeProxy={failedProxy || activeProxy || proxies[0]}
              stats={stats}
              speedHistory={speedHistory}
              formatBytes={formatBytes}
              formatSpeed={formatSpeed}
              hasProxies={proxies.length > 0}
              proxies={proxies}
              pings={pings}
              selectAndConnect={selectAndConnect}
              goToBuy={() => setActiveTab("buy")}
              goToAdd={() => {
                setEditingProxy(null);
                setActiveTab("add");
              }}
              onEditProxy={editProxy}
              goToProxyList={() => setActiveTab("list")}
            />
          )}
          {activeTab === "list" && (
            <ProxyListView
              proxies={proxies}
              deleteProxy={deleteProxy}
              editProxy={editProxy}
              selectAndConnect={selectAndConnect}
              activeProxy={activeProxy}
              isConnected={isConnected}
              pings={pings}
            />
          )}
          {activeTab === "rules" && (
            <RulesView rules={routingRules} setRules={setRoutingRules} />
          )}
          {activeTab === "add" && (
            <AddProxyView
              handleSaveProxy={handleSaveProxy}
              editingProxy={editingProxy}
              onCancel={() => setActiveTab("list")}
            />
          )}
          {activeTab === "buy" && <BuyProxyView />}
          {activeTab === "logs" && (
            <LogsView logs={logs} backendLogs={backendLogs} />
          )}
          {activeTab === "settings" && (
            <SettingsView
              proxies={proxies}
              setProxies={setProxies}
              routingRules={routingRules}
              setRoutingRules={setRoutingRules}
              settings={settings}
              setSettings={setSettings}
              updateSetting={updateSetting}
            />
          )}
        </div>
      </div>

      <div className="md:hidden absolute bottom-0 w-full bg-zinc-900 border-t border-zinc-800 flex justify-around p-2 z-20 pb-safe">
        <MobileNavItem
          icon={<Activity />}
          label="Главная"
          isActive={activeTab === "home"}
          onClick={() => setActiveTab("home")}
        />
        <MobileNavItem
          icon={<ShoppingCart />}
          label="Купить"
          isActive={activeTab === "buy"}
          onClick={() => setActiveTab("buy")}
        />
        <MobileNavItem
          icon={<Plus />}
          label="Добавить"
          isActive={activeTab === "add"}
          onClick={() => {
            setEditingProxy(null);
            setActiveTab("add");
          }}
        />
        <MobileNavItem
          icon={<List />}
          label="Прокси"
          isActive={activeTab === "list"}
          onClick={() => setActiveTab("list")}
        />
        <MobileNavItem
          icon={<Settings />}
          label="Настройки"
          isActive={activeTab === "settings"}
          onClick={() => setActiveTab("settings")}
        />
      </div>
    </div>
  );
}

const NavItem = ({ icon, label, isActive, onClick }) => (
  <button
    onClick={onClick}
    className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isActive ? "bg-[#007E3A]/10 text-[#007E3A]" : "text-zinc-400 hover:bg-zinc-800 hover:text-[#00A819]"}`}
  >
    {React.cloneElement(icon, { className: "w-5 h-5" })}
    <span className="font-medium">{label}</span>
  </button>
);

const MobileNavItem = ({ icon, label, isActive, onClick }) => (
  <button
    onClick={onClick}
    className={`flex flex-col items-center p-2 min-w-[64px] border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isActive ? "text-[#007E3A]" : "text-zinc-500 hover:text-[#00A819]"}`}
  >
    {React.cloneElement(icon, { className: "w-6 h-6 mb-1" })}
    <span className="text-[10px] font-medium">{label}</span>
  </button>
);

const HomeView = ({
  isConnected,
  isProxyDead,
  failedProxy,
  setFailedProxy,
  toggleConnection,
  activeProxy,
  stats,
  speedHistory,
  formatBytes,
  formatSpeed,
  hasProxies,
  proxies,
  pings,
  selectAndConnect,
  goToBuy,
  goToAdd,
  onEditProxy,
  goToProxyList,
}) => {
  const isError = !!failedProxy;
  const [isProxyListOpen, setIsProxyListOpen] = useState(false);

  return (
    <div className="flex flex-col items-center justify-center min-h-[75vh] space-y-10 animate-in fade-in zoom-in-95 duration-300">
      <div className="text-center space-y-2">
        <h2
          className={`text-3xl font-bold ${isConnected ? (isProxyDead ? "text-rose-500" : "text-[#007E3A]") : isError ? "text-rose-500" : "text-zinc-400"}`}
        >
          {isConnected
            ? isProxyDead
              ? "Связь потеряна"
              : "Защищено"
            : isError
              ? "Ошибка подключения"
              : "Не защищено"}
        </h2>
        <p className="text-zinc-500 text-md">
          {isConnected
            ? isProxyDead
              ? "Прокси-сервер не отвечает. Ожидание восстановления..."
              : "Ваш трафик маршрутизируется через прокси."
            : isError
              ? "Узел недоступен. Проверьте данные или выберите другой сервер."
              : "Ваш реальный IP-адрес виден."}
        </p>
      </div>

      <div className="relative group my-8">
        <div
          className={`absolute inset-0 rounded-full blur-2xl transition-all duration-700 ${isConnected ? (isProxyDead ? "bg-rose-500/40 animate-pulse" : "bg-[#007E3A]/40") : isError ? "bg-rose-500/20 animate-pulse" : hasProxies ? "bg-zinc-800/10 group-hover:bg-zinc-800/20" : ""}`}
        ></div>
        <button
          disabled={!hasProxies && !isConnected}
          onClick={
            isError
              ? () => {
                  setFailedProxy(null);
                  toggleConnection();
                }
              : toggleConnection
          }
          className={`relative border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none flex items-center justify-center w-48 h-48 rounded-full transition-all duration-300 transform active:scale-95 ${
            !hasProxies && !isConnected
              ? "bg-zinc-900 border-4 border-zinc-800 text-zinc-600 opacity-50 cursor-not-allowed"
              : isConnected
                ? isProxyDead
                  ? "bg-rose-600 text-white shadow-2xl shadow-rose-500/50"
                  : "bg-[#007E3A] text-zinc-950 shadow-2xl shadow-[#007E3A]/50"
                : isError
                  ? "bg-zinc-900 border-4 border-rose-500/50 text-rose-500 shadow-2xl shadow-rose-500/20"
                  : "bg-gradient-to-br from-zinc-800 to-zinc-900 border-4 border-zinc-800 text-zinc-400 hover:border-[#007E3A] hover:text-[#007E3A] shadow-2xl"
          }`}
        >
          <Power
            className={`w-20 h-20 ${isConnected && !isProxyDead ? "drop-shadow-none" : isConnected || isError ? "drop-shadow-md" : ""}`}
          />
        </button>
      </div>

      {!hasProxies ? (
        <div className="w-full max-w-2xl flex flex-col items-center animate-in fade-in duration-300">
          <p className="text-zinc-400 mb-4 text-center">
            Нет прокси?{" "}
            <span
              onClick={goToBuy}
              className="text-[#007E3A] hover:text-[#00A819] transition-colors cursor-pointer font-medium border-b border-transparent hover:border-[#00A819]"
            >
              Приобретите их со скидкой 5%
            </span>
          </p>
          <div
            onClick={goToAdd}
            className="w-full bg-zinc-900 border border-dashed border-zinc-700 rounded-3xl p-8 flex flex-col items-center justify-center cursor-pointer hover:border-[#007E3A] hover:bg-zinc-800/50 transition-all group outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            <div className="bg-zinc-800 p-4 rounded-full mb-4 text-zinc-400 group-hover:text-[#007E3A] transition-colors">
              <Plus className="w-8 h-8" />
            </div>
            <p className="text-lg font-bold text-white mb-1">Добавить сервер</p>
            <p className="text-sm text-zinc-500">
              Нажмите здесь, чтобы ввести данные вручную
            </p>
          </div>
        </div>
      ) : (
        <div
          className={`w-full max-w-2xl bg-zinc-900 rounded-3xl border flex flex-col overflow-hidden transition-all outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${(isProxyDead && isConnected) || isError ? "border-rose-500/30" : isProxyListOpen ? "border-zinc-700" : "border-zinc-800 hover:border-[#007E3A] hover:bg-zinc-800/50"}`}
        >
          <div
            onClick={() => setIsProxyListOpen(!isProxyListOpen)}
            className="p-5 flex items-center justify-between cursor-pointer transition-all group"
          >
            <div className="flex items-center space-x-5 min-w-0">
              <div
                className={`w-14 h-14 flex items-center justify-center rounded-2xl shrink-0 transition-colors ${isConnected ? (isProxyDead ? "bg-rose-500/20 text-rose-500" : "bg-[#007E3A]/20 text-[#007E3A]") : isError ? "bg-rose-500/10 text-rose-500" : "bg-zinc-800 text-zinc-500 group-hover:bg-zinc-700"}`}
              >
                {activeProxy ? (
                  <FlagIcon
                    code={activeProxy.country}
                    className="w-8 rounded-sm shadow-sm"
                  />
                ) : (
                  <Globe className="w-8 h-8" />
                )}
              </div>
              <div className="min-w-0">
                <p className="text-sm text-zinc-400 mb-1 truncate">
                  Текущий сервер
                </p>
                <p className="text-lg font-bold text-white truncate">
                  {activeProxy ? activeProxy.name : "Нет серверов"}
                </p>
                {activeProxy && (
                  <p className="text-sm text-zinc-500 font-mono mt-1 truncate">
                    {activeProxy.ip}:{activeProxy.port} ({activeProxy.type})
                  </p>
                )}
              </div>
            </div>

            <div className="flex items-center space-x-1 shrink-0 ml-4">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onEditProxy(activeProxy);
                }}
                className={`p-2 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${(isProxyDead && isConnected) || isError ? "text-rose-500/50 hover:text-rose-500 hover:bg-rose-500/10" : "text-zinc-600 hover:text-[#007E3A] hover:bg-[#007E3A]/10"}`}
              >
                <Pencil className="w-5 h-5" />
              </button>
              <div
                className={`p-2 rounded-xl transition-colors text-zinc-500 group-hover:text-zinc-300`}
              >
                <ChevronDown
                  className={`w-5 h-5 transition-transform duration-300 ${isProxyListOpen ? "rotate-180" : ""}`}
                />
              </div>
            </div>
          </div>

          {isProxyListOpen && (
            <div className="bg-zinc-950/50 border-t border-zinc-800/50 p-2 max-h-[280px] overflow-y-auto scrollbar-hide space-y-1 animate-in slide-in-from-top-2 duration-200">
              {proxies.map((proxy) => {
                const isActive = activeProxy?.id === proxy.id;
                return (
                  <div
                    key={proxy.id}
                    onClick={(e) => {
                      e.stopPropagation();
                      selectAndConnect(proxy);
                      setIsProxyListOpen(false);
                    }}
                    className={`flex items-center justify-between p-3 rounded-2xl cursor-pointer transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isActive ? "bg-[#007E3A]/10 border border-[#007E3A]/20" : "bg-zinc-900/50 border border-transparent hover:border-[#00A819]/50 hover:bg-zinc-800/80"}`}
                  >
                    <div className="flex items-center space-x-4 min-w-0">
                      <div className="shrink-0 flex items-center justify-center w-10 h-10 bg-zinc-800/50 rounded-lg border border-zinc-700/50">
                        <FlagIcon
                          code={proxy.country}
                          className="w-6 h-auto rounded-[2px]"
                        />
                      </div>
                      <div className="min-w-0">
                        <h4
                          className={`text-sm font-bold truncate transition-colors ${isActive ? "text-[#00A819]" : "text-white"}`}
                        >
                          {proxy.name}
                        </h4>
                        <p className="text-xs text-zinc-500 font-mono mt-0.5 truncate">
                          {proxy.ip}:{proxy.port}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center space-x-3 shrink-0 ml-3">
                      <div
                        className={`text-xs flex items-center ${pings[proxy.id] === "Timeout" || pings[proxy.id] === "Error" ? "text-rose-500" : "text-zinc-500"}`}
                      >
                        <Activity className="w-3 h-3 mr-1" />{" "}
                        {pings[proxy.id] || "..."}
                      </div>
                      {isActive ? (
                        <div className="w-2 h-2 rounded-full bg-[#00A819] shadow-[0_0_8px_rgba(0,168,25,0.8)]"></div>
                      ) : (
                        <div className="w-2 h-2 rounded-full bg-zinc-700"></div>
                      )}
                    </div>
                  </div>
                );
              })}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  goToProxyList();
                }}
                className="w-full mt-2 py-3 text-sm text-zinc-400 hover:text-white transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                Открыть полный список
              </button>
            </div>
          )}
        </div>
      )}

      {isError ? (
        <div className="flex space-x-4 w-full max-w-2xl animate-in slide-in-from-bottom-4 fade-in duration-300">
          <button
            onClick={() => onEditProxy(activeProxy)}
            className="flex-1 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none bg-zinc-900 border border-zinc-800 hover:border-zinc-700 text-white py-4 rounded-3xl font-bold transition-colors"
          >
            Изменить данные
          </button>
          <button
            onClick={() => {
              setFailedProxy(null);
              goToProxyList();
            }}
            className="flex-1 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none bg-rose-500/10 border border-rose-500/20 hover:bg-rose-500/20 text-rose-500 py-4 rounded-3xl font-bold transition-colors"
          >
            Выбрать другой
          </button>
        </div>
      ) : (
        <div
          className={`w-full max-w-2xl grid grid-cols-2 gap-6 transition-all duration-500 ${isConnected ? "opacity-100 translate-y-0" : "opacity-0 translate-y-4 pointer-events-none"}`}
        >
          <div className="bg-zinc-900 rounded-3xl p-6 border border-zinc-800 flex flex-col min-w-0 relative">
            <div className="flex justify-between items-start w-full">
              <p className="text-sm text-zinc-500 mb-2 truncate font-bold uppercase tracking-widest">
                Загружено
              </p>
              <p
                className={`text-[10px] font-bold ${isProxyDead ? "text-zinc-600" : "text-[#007E3A]"}`}
              >
                {formatSpeed(speedHistory.down[19])}
              </p>
            </div>
            <p
              className={`text-3xl font-bold truncate w-full ${isProxyDead ? "text-zinc-600" : "text-[#007E3A]"}`}
            >
              {formatBytes(stats.download)}
            </p>
            <SpeedChart
              data={speedHistory.down}
              color={isProxyDead ? "#52525b" : "#007E3A"}
            />
          </div>
          <div className="bg-zinc-900 rounded-3xl p-6 border border-zinc-800 flex flex-col min-w-0 relative">
            <div className="flex justify-between items-start w-full">
              <p className="text-sm text-zinc-500 mb-2 truncate font-bold uppercase tracking-widest">
                Отправлено
              </p>
              <p
                className={`text-[10px] font-bold ${isProxyDead ? "text-zinc-600" : "text-[#00A819]"}`}
              >
                {formatSpeed(speedHistory.up[19])}
              </p>
            </div>
            <p
              className={`text-3xl font-bold truncate w-full ${isProxyDead ? "text-zinc-600" : "text-[#00A819]"}`}
            >
              {formatBytes(stats.upload)}
            </p>
            <SpeedChart
              data={speedHistory.up}
              color={isProxyDead ? "#52525b" : "#00A819"}
            />
          </div>
        </div>
      )}
    </div>
  );
};

const BuyProxyView = () => {
  const [linkCopied, setLinkCopied] = useState(false);
  const [promoCopied, setPromoCopied] = useState(false);
  const link = "https://proxy6.net/?r=833290";
  const promoCode = "resultproxy";

  const handleCopyAndGo = () => {
    const el = document.createElement("textarea");
    el.value = link;
    document.body.appendChild(el);
    el.select();
    document.execCommand("copy");
    document.body.removeChild(el);

    setLinkCopied(true);
    setTimeout(() => setLinkCopied(false), 2000);

    window.open(link, "_blank");
  };

  const handleCopyPromo = () => {
    const el = document.createElement("textarea");
    el.value = promoCode;
    document.body.appendChild(el);
    el.select();
    document.execCommand("copy");
    document.body.removeChild(el);

    setPromoCopied(true);
    setTimeout(() => setPromoCopied(false), 2000);
  };

  return (
    <div className="max-w-2xl mx-auto space-y-6 animate-in fade-in duration-300">
      <div>
        <h2 className="text-3xl font-bold text-white">Купить прокси</h2>
        <p className="text-zinc-400 mt-2">
          Надежные IPv4 и IPv6 прокси для любых задач
        </p>
      </div>

      <div className="bg-zinc-900 p-8 rounded-3xl border border-zinc-800 mt-6 text-center">
        <div className="bg-[#007E3A]/10 w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-6">
          <ShoppingCart className="w-8 h-8 text-[#007E3A]" />
        </div>
        <h3 className="text-white font-bold text-xl mb-4">
          Скидка 5% от PROXY6.net
        </h3>
        <p className="text-zinc-400 text-md mb-8 leading-relaxed">
          Зарегистрируйтесь на сайте PROXY6.net по ссылке ниже, чтобы получить
          скидку 5% на покупку прокси.
        </p>

        <div className="space-y-4">
          <button
            onClick={handleCopyAndGo}
            className="w-full relative group border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none flex flex-col sm:flex-row items-center justify-between p-4 bg-zinc-950 border border-zinc-800 hover:border-[#00A819] rounded-2xl transition-all overflow-hidden gap-4 sm:gap-0"
          >
            <div className="absolute inset-0 bg-[#007E3A]/5 opacity-0 group-hover:opacity-100 transition-opacity"></div>
            <div className="flex items-center space-x-4 relative z-10 w-full sm:w-auto overflow-hidden">
              <div className="bg-zinc-900 p-3 rounded-xl border border-zinc-800 shrink-0">
                <ExternalLink className="w-5 h-5 text-zinc-400 group-hover:text-[#00A819] transition-colors" />
              </div>
              <span className="text-zinc-300 font-mono text-sm tracking-wide group-hover:text-white transition-colors truncate">
                {link}
              </span>
            </div>
            <div className="relative z-10 flex items-center justify-center w-full sm:w-auto space-x-2 bg-zinc-900 px-6 py-3 sm:px-4 sm:py-2 rounded-xl border border-zinc-800 group-hover:border-[#00A819]/50 transition-colors shrink-0">
              {linkCopied ? (
                <>
                  <Check className="w-4 h-4 text-[#00A819]" />
                  <span className="text-sm font-medium text-[#00A819]">
                    Скопировано!
                  </span>
                </>
              ) : (
                <>
                  <Copy className="w-4 h-4 text-zinc-400 group-hover:text-[#00A819]" />
                  <span className="text-sm font-medium text-zinc-400 group-hover:text-[#00A819]">
                    Перейти
                  </span>
                </>
              )}
            </div>
          </button>

          <button
            onClick={handleCopyPromo}
            className="w-full relative group border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none flex flex-col sm:flex-row items-center justify-between p-4 bg-zinc-950 border border-zinc-800 hover:border-[#00A819] rounded-2xl transition-all overflow-hidden gap-4 sm:gap-0"
          >
            <div className="absolute inset-0 bg-[#007E3A]/5 opacity-0 group-hover:opacity-100 transition-opacity"></div>
            <div className="flex items-center space-x-4 relative z-10 w-full sm:w-auto overflow-hidden">
              <div className="bg-zinc-900 p-3 rounded-xl border border-zinc-800 shrink-0">
                <Copy className="w-5 h-5 text-zinc-400 group-hover:text-[#00A819] transition-colors" />
              </div>
              <div className="flex flex-col items-start text-left min-w-0">
                <span className="text-xs text-zinc-500 font-medium mb-0.5">
                  Промокод на скидку 5%
                </span>
                <span className="text-zinc-300 font-mono text-sm font-bold tracking-widest group-hover:text-white transition-colors truncate uppercase">
                  {promoCode}
                </span>
              </div>
            </div>
            <div className="relative z-10 flex items-center justify-center w-full sm:w-auto space-x-2 bg-zinc-900 px-6 py-3 sm:px-4 sm:py-2 rounded-xl border border-zinc-800 group-hover:border-[#00A819]/50 transition-colors shrink-0">
              {promoCopied ? (
                <>
                  <Check className="w-4 h-4 text-[#00A819]" />
                  <span className="text-sm font-medium text-[#00A819]">
                    Скопировано!
                  </span>
                </>
              ) : (
                <>
                  <Copy className="w-4 h-4 text-zinc-400 group-hover:text-[#00A819]" />
                  <span className="text-sm font-medium text-zinc-400 group-hover:text-[#00A819]">
                    Копировать
                  </span>
                </>
              )}
            </div>
          </button>
        </div>
      </div>
    </div>
  );
};

const RulesView = ({ rules, setRules }) => {
  const [newDomain, setNewDomain] = useState("");
  const [newApp, setNewApp] = useState("");

  const appFileInputRef = useRef(null);

  const addDomain = () => {
    if (newDomain && !rules.whitelist.includes(newDomain)) {
      setRules({ ...rules, whitelist: [...rules.whitelist, newDomain] });
      setNewDomain("");
    }
  };

  const addSpecificDomain = (domain) => {
    if (!rules.whitelist.includes(domain)) {
      setRules({ ...rules, whitelist: [...rules.whitelist, domain] });
    }
  };

  const addApp = () => {
    if (newApp) {
      let appName = newApp.toLowerCase().trim();
      if (!appName.endsWith(".exe")) appName += ".exe";

      const currentList = rules.appWhitelist || [];
      if (!currentList.includes(appName)) {
        setRules({ ...rules, appWhitelist: [...currentList, appName] });
        setNewApp("");
      }
    }
  };

  const addSpecificApp = (appName) => {
    const currentList = rules.appWhitelist || [];
    if (!currentList.includes(appName)) {
      setRules({ ...rules, appWhitelist: [...currentList, appName] });
    }
  };

  const handleFileSelect = (e) => {
    const file = e.target.files[0];
    if (file) {
      let appName = file.name.toLowerCase();
      if (!appName.endsWith(".exe")) appName += ".exe";
      const currentList = rules.appWhitelist || [];
      if (!currentList.includes(appName)) {
        setRules({ ...rules, appWhitelist: [...currentList, appName] });
      }
      e.target.value = "";
    }
  };

  const popularTlds = ["*.ru", "*.рф", "*.su", "*.by", "*.kz"];
  const popularApps = [
    "steam.exe",
    "discord.exe",
    "telegram.exe",
    "epicgameslauncher.exe",
  ];
  const safeAppWhitelist = rules.appWhitelist || [];

  return (
    <div className="max-w-2xl mx-auto space-y-6 animate-in fade-in duration-300">
      <div>
        <h2 className="text-3xl font-bold text-white">Умная маршрутизация</h2>
        <p className="text-zinc-400 mt-2">
          Выберите сценарий использования туннеля
        </p>
      </div>

      <div className="grid gap-4">
        <div
          onClick={() => setRules({ ...rules, mode: "global" })}
          className={`p-6 rounded-3xl border cursor-pointer transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${rules.mode === "global" ? "border-[#007E3A] bg-[#007E3A]/10" : "border-zinc-800 bg-zinc-900 hover:border-zinc-700"}`}
        >
          <h4 className="text-white font-bold text-lg">Глобальный режим</h4>
          <p className="text-zinc-500 text-sm mt-1">
            Весь системный трафик идет через выбранный прокси-сервер.
          </p>
        </div>
        <div
          onClick={() => setRules({ ...rules, mode: "smart" })}
          className={`p-6 rounded-3xl border cursor-pointer transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${rules.mode === "smart" ? "border-[#00A819] bg-[#00A819]/10" : "border-zinc-800 bg-zinc-900 hover:border-zinc-700"}`}
        >
          <h4 className="text-white font-bold text-lg">Smart (Антизапрет)</h4>
          <p className="text-zinc-500 text-sm mt-1">
            Прокси активен только для заблокированных ресурсов. Остальное
            работает напрямую.
          </p>
        </div>
      </div>

      <div className="bg-zinc-900 p-8 rounded-3xl border border-zinc-800 mt-6">
        <h3 className="text-white font-bold text-lg mb-2">
          Сайты-исключения (Домены)
        </h3>
        <p className="text-zinc-500 text-sm mb-6">
          Сайты из этого списка всегда будут подключаться без прокси. Изменения
          применяются мгновенно.
        </p>
        <div className="flex space-x-3 mb-6">
          <input
            type="text"
            placeholder="Например: yandex.ru"
            className="flex-1 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
            onKeyPress={(e) => e.key === "Enter" && addDomain()}
          />
          <button
            onClick={addDomain}
            className="bg-[#007E3A] hover:bg-[#00A819] text-white px-6 font-bold rounded-xl transition-colors border-none outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            Добавить
          </button>
        </div>
        <div className="flex flex-wrap gap-2 mb-8">
          {rules.whitelist.map((d) => (
            <div
              key={d}
              className="bg-zinc-800 px-4 py-2 rounded-lg flex items-center space-x-3 border border-zinc-700"
            >
              <span className="text-sm text-zinc-300">{d}</span>
              <button
                onClick={() =>
                  setRules({
                    ...rules,
                    whitelist: rules.whitelist.filter((i) => i !== d),
                  })
                }
                className="text-zinc-500 hover:text-rose-500 transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>

        <div className="pt-6 border-t border-zinc-800">
          <p className="text-sm font-medium text-zinc-400 mb-4">
            Быстро добавить в исключения домены:
          </p>
          <div className="flex flex-wrap gap-3">
            {popularTlds.map((tld) => {
              const isAdded = rules.whitelist.includes(tld);
              return (
                <button
                  key={tld}
                  onClick={() => addSpecificDomain(tld)}
                  disabled={isAdded}
                  className={`flex items-center px-4 py-2 rounded-xl text-sm font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
                    isAdded
                      ? "bg-[#007E3A]/10 text-[#007E3A] border-[#007E3A]/20 cursor-default"
                      : "bg-zinc-800 text-zinc-300 border-zinc-700 hover:border-[#00A819] hover:text-white cursor-pointer"
                  }`}
                >
                  {!isAdded && <Plus className="w-4 h-4 mr-2" />}
                  {tld}
                </button>
              );
            })}
          </div>
        </div>
      </div>

      <div className="bg-zinc-900 p-8 rounded-3xl border border-zinc-800 mt-6">
        <h3 className="text-white font-bold text-lg mb-2">
          Приложения-исключения (.EXE)
        </h3>
        <p className="text-zinc-500 text-sm mb-6">
          Трафик этих приложений пойдет напрямую.{" "}
          <strong>
            Дочерние процессы (например, игры из Steam) также пойдут без прокси.
          </strong>
        </p>
        <div className="flex space-x-3 mb-6">
          <input
            type="text"
            placeholder="Например: steam.exe"
            className="flex-1 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
            value={newApp}
            onChange={(e) => setNewApp(e.target.value)}
            onKeyPress={(e) => e.key === "Enter" && addApp()}
          />
          <button
            onClick={addApp}
            className="bg-zinc-800 hover:bg-zinc-700 text-white px-6 font-bold rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none shrink-0"
          >
            Вручную
          </button>

          <input
            type="file"
            accept=".exe"
            className="hidden"
            ref={appFileInputRef}
            onChange={handleFileSelect}
          />
          <button
            onClick={() => appFileInputRef.current.click()}
            className="bg-[#007E3A] hover:bg-[#00A819] text-white px-6 font-bold rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none shrink-0"
          >
            Выбрать .exe
          </button>
        </div>
        <div className="flex flex-wrap gap-2 mb-8">
          {safeAppWhitelist.map((appName) => (
            <div
              key={appName}
              className="bg-zinc-800 px-4 py-2 rounded-lg flex items-center space-x-3 border border-zinc-700"
            >
              <span className="text-sm text-zinc-300 font-mono">{appName}</span>
              <button
                onClick={() =>
                  setRules({
                    ...rules,
                    appWhitelist: safeAppWhitelist.filter((i) => i !== appName),
                  })
                }
                className="text-zinc-500 hover:text-rose-500 transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>

        <div className="pt-6 border-t border-zinc-800">
          <p className="text-sm font-medium text-zinc-400 mb-4">
            Популярные программы для обхода прокси:
          </p>
          <div className="flex flex-wrap gap-3">
            {popularApps.map((appName) => {
              const isAdded = safeAppWhitelist.includes(appName);
              return (
                <button
                  key={appName}
                  onClick={() => addSpecificApp(appName)}
                  disabled={isAdded}
                  className={`flex items-center px-4 py-2 rounded-xl text-sm font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
                    isAdded
                      ? "bg-[#007E3A]/10 text-[#007E3A] border-[#007E3A]/20 cursor-default"
                      : "bg-zinc-800 text-zinc-300 border-zinc-700 hover:border-[#00A819] hover:text-white cursor-pointer"
                  }`}
                >
                  {!isAdded && <Plus className="w-4 h-4 mr-2" />}
                  <span className="font-mono">{appName}</span>
                </button>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
};

const ProxyListView = ({
  proxies,
  deleteProxy,
  editProxy,
  selectAndConnect,
  activeProxy,
  isConnected,
  pings,
}) => (
  <div className="space-y-6 animate-in fade-in duration-300">
    <div>
      <h2 className="text-3xl font-bold text-white">Сохраненные профили</h2>
      <p className="text-zinc-400 mt-2">Управляйте вашими серверами</p>
    </div>

    {proxies.length === 0 ? (
      <div className="text-center py-16 bg-zinc-900 rounded-3xl border border-zinc-800 border-dashed">
        <Server className="w-16 h-16 text-zinc-700 mx-auto mb-4" />
        <p className="text-zinc-400 text-lg">Список прокси пуст.</p>
      </div>
    ) : (
      <div className="grid gap-6 grid-cols-1 sm:grid-cols-[repeat(auto-fit,minmax(320px,1fr))]">
        {proxies.map((proxy) => {
          const isActive = isConnected && activeProxy?.id === proxy.id;
          return (
            <div
              key={proxy.id}
              onClick={() => selectAndConnect(proxy)}
              className={`bg-zinc-900 p-6 rounded-3xl border transition-all flex flex-col cursor-pointer group/card outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isActive ? "border-[#00A819] shadow-[0_0_20px_rgba(0,168,25,0.1)]" : "border-zinc-800 hover:border-[#00A819] hover:bg-zinc-800/30"}`}
            >
              <div className="flex justify-between items-start mb-6 gap-4">
                <div className="flex items-center space-x-4 min-w-0">
                  <div className="shrink-0 flex items-center justify-center w-12 h-12 bg-zinc-800/50 rounded-xl border border-zinc-700/50">
                    <FlagIcon
                      code={proxy.country}
                      className="w-7 rounded-[2px]"
                    />
                  </div>
                  <div className="min-w-0">
                    <h3 className="text-lg font-bold text-white truncate group-hover/card:text-[#00A819] transition-colors">
                      {proxy.name}
                    </h3>
                    <p className="text-sm text-zinc-400 font-mono mt-1 truncate">
                      {proxy.ip}:{proxy.port}
                    </p>
                  </div>
                </div>
                <span className="text-xs font-medium px-2 py-1 rounded bg-zinc-800 text-zinc-300 shrink-0">
                  {proxy.type}
                </span>
              </div>

              <div className="flex items-center justify-between mt-auto pt-2 flex-wrap gap-4">
                <div
                  className={`text-sm flex items-center shrink-0 ${pings[proxy.id] === "Timeout" || pings[proxy.id] === "Error" ? "text-rose-500" : "text-zinc-500"}`}
                >
                  <Activity className="w-4 h-4 mr-1 shrink-0" />{" "}
                  {pings[proxy.id] || "Опрос..."}
                </div>
                <div className="flex space-x-2 shrink-0">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      editProxy(proxy);
                    }}
                    className="p-3 bg-zinc-800 text-zinc-400 hover:text-white hover:bg-zinc-700 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
                  >
                    <Pencil className="w-4 h-4" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      deleteProxy(proxy.id);
                    }}
                    className="p-3 bg-zinc-800 text-zinc-400 hover:text-rose-500 hover:bg-rose-500/10 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      selectAndConnect(proxy);
                    }}
                    className={`px-5 py-2 rounded-xl text-sm font-medium transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isActive ? "bg-[#00A819] text-zinc-950 font-bold" : "bg-[#007E3A]/10 text-[#00A819] hover:bg-[#007E3A]/20"}`}
                  >
                    {isActive ? "Подключено" : "Подключить"}
                  </button>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    )}
  </div>
);

const AddProxyView = ({ handleSaveProxy, editingProxy, onCancel }) => {
  const [formData, setFormData] = useState({
    name: "",
    ip: "",
    port: "",
    type: "SOCKS5",
    username: "",
    password: "",
    country: "🌐",
  });

  useEffect(() => {
    if (editingProxy) setFormData(editingProxy);
    else
      setFormData({
        name: "",
        ip: "",
        port: "",
        type: "SOCKS5",
        username: "",
        password: "",
        country: "🌐",
      });
  }, [editingProxy]);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (formData.ip && formData.port)
      handleSaveProxy({ ...formData, name: formData.name || "Новый сервер" });
  };

  return (
    <div className="max-w-2xl mx-auto space-y-6 animate-in fade-in duration-300">
      <div>
        <h2 className="text-3xl font-bold text-white">
          {editingProxy ? "Редактировать профиль" : "Добавить конфигурацию"}
        </h2>
        <p className="text-zinc-400 mt-2">
          {editingProxy
            ? "Измените данные вашего сервера. Сохранение автоматически перезапустит соединение."
            : "Введите данные сервера вручную"}
        </p>
      </div>
      <form
        onSubmit={handleSubmit}
        className="bg-zinc-900 p-8 rounded-3xl border border-zinc-800 space-y-6"
      >
        <div>
          <label className="block text-sm font-medium text-zinc-400 mb-2">
            Название профиля
          </label>
          <input
            type="text"
            placeholder="Например: Мой личный сервер"
            className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
            value={formData.name || ""}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
          />
        </div>
        <div className="grid grid-cols-3 gap-6">
          <div className="col-span-2">
            <label className="block text-sm font-medium text-zinc-400 mb-2">
              IP-адрес / Хост *
            </label>
            <input
              type="text"
              required
              placeholder="192.168.1.1"
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.ip || ""}
              onChange={(e) => setFormData({ ...formData, ip: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-400 mb-2">
              Порт *
            </label>
            <input
              type="number"
              required
              placeholder="8000"
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.port || ""}
              onChange={(e) =>
                setFormData({ ...formData, port: e.target.value })
              }
            />
          </div>
        </div>
        <div>
          <label className="block text-sm font-medium text-zinc-400 mb-3">
            Протокол
          </label>
          <div className="grid grid-cols-3 gap-3">
            {["HTTP", "HTTPS", "SOCKS5"].map((type) => (
              <button
                type="button"
                key={type}
                onClick={() => setFormData({ ...formData, type })}
                className={`py-3 rounded-xl text-sm font-bold border transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${formData.type === type ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-950 text-zinc-400 border-zinc-800 hover:border-[#00A819]"}`}
              >
                {type}
              </button>
            ))}
          </div>
        </div>
        <div className="pt-6 border-t border-zinc-800">
          <p className="text-sm font-medium text-zinc-400 mb-4 flex items-center">
            <Lock className="w-4 h-4 mr-2" /> Авторизация (опционально)
          </p>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <input
              type="text"
              placeholder="Логин"
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.username || ""}
              onChange={(e) =>
                setFormData({ ...formData, username: e.target.value })
              }
            />
            <input
              type="password"
              placeholder="Пароль"
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.password || ""}
              onChange={(e) =>
                setFormData({ ...formData, password: e.target.value })
              }
            />
          </div>
        </div>
        <div className="pt-6 flex space-x-4">
          {editingProxy && (
            <button
              type="button"
              onClick={onCancel}
              className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-4 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
            >
              Отмена
            </button>
          )}
          <button
            type="submit"
            className="flex-[2] bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-4 rounded-xl transition-colors shadow-lg shadow-[#007E3A]/20 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            {editingProxy ? "Сохранить изменения" : "Сохранить прокси"}
          </button>
        </div>
      </form>
    </div>
  );
};

const LogsView = ({ logs, backendLogs }) => {
  const allLogs = [...logs, ...(backendLogs || [])]
    .sort((a, b) => (b.timestamp || 0) - (a.timestamp || 0))
    .slice(0, 150);

  return (
    <div className="max-w-4xl mx-auto space-y-4 animate-in fade-in duration-300 h-full flex flex-col">
      <div>
        <h2 className="text-3xl font-bold text-white">Журнал событий</h2>
        <p className="text-zinc-400 mt-2">
          Отслеживание системных команд и трафика в реальном времени.
        </p>
      </div>
      <div className="bg-zinc-950 border border-zinc-800 rounded-3xl p-6 flex-1 overflow-y-auto font-mono text-sm scrollbar-hide">
        {allLogs.map((log, i) => (
          <div
            key={i}
            className={`flex items-start space-x-4 border-b border-zinc-800/50 py-3 last:border-0 ${log.type === "error" ? "text-rose-400" : log.type === "success" ? "text-[#007E3A]" : log.type === "warning" ? "text-[#00A819]" : "text-zinc-300"}`}
          >
            <span className="text-zinc-600 shrink-0">[{log.time}]</span>
            <span className="break-words w-full">{log.msg}</span>
          </div>
        ))}
      </div>
    </div>
  );
};

const SettingsView = ({
  proxies,
  setProxies,
  routingRules,
  setRoutingRules,
  settings,
  setSettings,
  updateSetting,
}) => {
  const fileInputRef = useRef(null);

  const handleExport = () => {
    const fullConfig = {
      proxies,
      routingRules,
      settings,
    };
    const dataStr =
      "data:text/json;charset=utf-8," +
      encodeURIComponent(JSON.stringify(fullConfig, null, 2));
    const downloadAnchorNode = document.createElement("a");
    downloadAnchorNode.setAttribute("href", dataStr);
    downloadAnchorNode.setAttribute("download", "resultproxy-full-config.json");
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    document.body.removeChild(downloadAnchorNode);
  };

  const handleImport = (e) => {
    const fileReader = new FileReader();
    if (!e.target.files[0]) return;
    fileReader.readAsText(e.target.files[0], "UTF-8");
    fileReader.onload = (event) => {
      try {
        const imported = JSON.parse(event.target.result);
        if (Array.isArray(imported)) {
          setProxies(imported);
          alert("Конфигурации серверов успешно импортированы (старый формат)!");
        } else if (imported && typeof imported === "object") {
          if (imported.proxies) setProxies(imported.proxies);
          if (imported.routingRules) setRoutingRules(imported.routingRules);
          if (imported.settings) {
            if (imported.settings.autostart !== undefined) {
              updateSetting("autostart", imported.settings.autostart);
            }
            if (imported.settings.killswitch !== undefined) {
              updateSetting("killswitch", imported.settings.killswitch);
            }
            setSettings(imported.settings);
          }
          alert("Полная конфигурация приложения успешно импортирована!");
        } else {
          alert("Неверный формат файла.");
        }
      } catch (err) {
        alert("Ошибка чтения файла!");
      }
      e.target.value = "";
    };
  };

  return (
    <div className="max-w-3xl mx-auto space-y-10 animate-in fade-in duration-300">
      <div>
        <h2 className="text-3xl font-bold text-white">Настройки приложения</h2>
        <p className="text-zinc-400 mt-2">
          Управление безопасностью и системой
        </p>
      </div>

      <div className="space-y-4">
        <SettingToggle
          title="Запуск при старте системы"
          description="Автоматически открывать приложение при включении ПК."
          isOn={settings.autostart}
          onToggle={() => updateSetting("autostart", !settings.autostart)}
        />
        <SettingToggle
          title="Включить Kill Switch"
          description="Моментально прервать интернет соединение при падении прокси."
          isOn={settings.killswitch}
          onToggle={() => updateSetting("killswitch", !settings.killswitch)}
        />
      </div>

      <div className="p-8 bg-zinc-900 rounded-3xl border border-zinc-800 mt-10">
        <h3 className="text-white font-bold mb-2 text-xl">
          Экспорт / Импорт конфигураций
        </h3>
        <p className="text-zinc-400 mb-6">
          Сохраните полную конфигурацию (серверы, умные правила, настройки) в
          файл, чтобы перенести их на другой ПК.
        </p>
        <div className="flex space-x-4">
          <button
            onClick={handleExport}
            className="flex items-center px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            <Download className="w-5 h-5 mr-2" /> Экспорт
          </button>

          <input
            type="file"
            ref={fileInputRef}
            onChange={handleImport}
            accept=".json"
            className="hidden"
          />
          <button
            onClick={() => fileInputRef.current.click()}
            className="flex items-center px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            <Upload className="w-5 h-5 mr-2" /> Импорт
          </button>
        </div>
      </div>
    </div>
  );
};

const SettingToggle = ({ title, description, isOn, onToggle }) => {
  return (
    <div
      className="flex items-center justify-between p-6 bg-zinc-900 rounded-3xl border border-zinc-800 cursor-pointer hover:border-zinc-700 transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
      onClick={onToggle}
    >
      <div className="pr-6">
        <h4 className="text-white font-bold text-lg">{title}</h4>
        <p className="text-zinc-500 mt-1">{description}</p>
      </div>
      <div
        className={`relative w-14 h-7 rounded-full transition-colors duration-300 ease-in-out shrink-0 ${isOn ? "bg-[#007E3A]" : "bg-zinc-700"}`}
      >
        <div
          className={`absolute top-1 left-1 bg-white w-5 h-5 rounded-full transition-transform duration-300 ease-in-out ${isOn ? "transform translate-x-7" : ""}`}
        ></div>
      </div>
    </div>
  );
};
