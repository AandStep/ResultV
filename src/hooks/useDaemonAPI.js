import { useState, useEffect, useRef, useCallback } from "react";
import { apiFetch } from "./useLogs";

export const useDaemonAPI = ({
  proxies,
  routingRules,
  settings,
  addLog,
  isConfigLoaded,
}) => {
  const [isConnected, setIsConnected] = useState(false);
  const [isProxyDead, setIsProxyDead] = useState(false);
  const [failedProxy, setFailedProxy] = useState(null);
  const [activeProxy, setActiveProxy] = useState(null);

  const [stats, setStats] = useState({ download: 0, upload: 0 });
  const [speedHistory, setSpeedHistory] = useState({
    down: new Array(20).fill(0),
    up: new Array(20).fill(0),
  });
  const [pings, setPings] = useState({});
  const [daemonStatus, setDaemonStatus] = useState("checking");

  const isSwitchingRef = useRef(false);
  const prevProxyDead = useRef(false);

  useEffect(() => {
    if (!isConfigLoaded || proxies.length === 0) return;

    const fetchPings = async () => {
      const newPings = {};
      for (const p of proxies) {
        try {
          const res = await apiFetch(`/api/ping`, {
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
        const res = await apiFetch(`/api/status`);
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
  }, [isConnected, daemonStatus, proxies, failedProxy, addLog]);

  const toggleConnection = useCallback(async () => {
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
        await apiFetch(`/api/disconnect`, { method: "POST" });
        addLog("Отключено успешно.", "success");
        setIsConnected(false);
      } else {
        addLog(`Подключение к ${targetProxy.name}...`, "info");
        setActiveProxy(targetProxy);

        const res = await apiFetch(`/api/connect`, {
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
  }, [
    addLog,
    daemonStatus,
    activeProxy,
    proxies,
    isConnected,
    routingRules,
    settings,
  ]);

  const selectAndConnect = useCallback(
    async (proxy, forceReconnect = false, setActiveTab) => {
      if (!forceReconnect && activeProxy?.id === proxy.id && isConnected)
        return;

      try {
        isSwitchingRef.current = true;
        setFailedProxy(null);
        if (setActiveTab) setActiveTab("home");
        setActiveProxy(proxy);
        addLog(`Переключение на: ${proxy.name}...`, "info");

        if (isConnected) {
          await apiFetch(`/api/disconnect`, { method: "POST" });
          setIsConnected(false);
        }

        const res = await apiFetch(`/api/connect`, {
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
    },
    [activeProxy, isConnected, routingRules, settings, addLog],
  );

  const deleteProxy = useCallback(
    async (id, setProxies) => {
      const isDeletingActive = activeProxy?.id === id;
      setProxies((prev) => prev.filter((p) => p.id !== id));

      if (isDeletingActive) {
        if (isConnected) {
          isSwitchingRef.current = true;
          addLog("Активный сервер удален. Разрыв соединения...", "info");
          try {
            await apiFetch(`/api/disconnect`, { method: "POST" });
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
    },
    [activeProxy, isConnected, failedProxy, addLog],
  );

  return {
    isConnected,
    isProxyDead,
    failedProxy,
    setFailedProxy,
    activeProxy,
    setActiveProxy,
    stats,
    speedHistory,
    pings,
    daemonStatus,
    toggleConnection,
    selectAndConnect,
    deleteProxy,
  };
};
