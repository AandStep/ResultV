import { useCallback } from "react";
import { apiFetch } from "./useLogs";

export const useDaemonControl = (
  isConnected,
  setIsConnected,
  activeProxy,
  setActiveProxy,
  failedProxy,
  setFailedProxy,
  proxies,
  routingRules,
  settings,
  daemonStatus,
  isSwitchingRef,
  addLog,
) => {
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
        const resData = await res.json().catch(() => ({}));
        addLog("Соединение установлено.", "success");
        if (resData.dnsLeakWarning) {
          addLog(
            "⚠️ DNS-утечка: HTTP-прокси не проксирует DNS-запросы. Используйте SOCKS5 для полной защиты.",
            "warning",
          );
        }
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
    setIsConnected,
    setActiveProxy,
    setFailedProxy,
    isSwitchingRef,
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

        const resData = await res.json().catch(() => ({}));
        setIsConnected(true);
        addLog(`Успешно переключено на ${proxy.name}`, "success");
        if (resData.dnsLeakWarning) {
          addLog(
            "⚠️ DNS-утечка: HTTP-прокси не проксирует DNS-запросы. Используйте SOCKS5 для полной защиты.",
            "warning",
          );
        }

        setTimeout(() => {
          isSwitchingRef.current = false;
        }, 2000);
      } catch (error) {
        isSwitchingRef.current = false;
        setFailedProxy(proxy);
        addLog(`Сбой подключения: ${error.message}`, "error");
      }
    },
    [
      activeProxy,
      isConnected,
      routingRules,
      settings,
      addLog,
      setActiveProxy,
      setFailedProxy,
      setIsConnected,
      isSwitchingRef,
    ],
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
    [
      activeProxy,
      isConnected,
      failedProxy,
      addLog,
      setActiveProxy,
      setIsConnected,
      setFailedProxy,
      isSwitchingRef,
    ],
  );

  return { toggleConnection, selectAndConnect, deleteProxy };
};
