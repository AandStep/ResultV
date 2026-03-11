import { useState, useEffect, useCallback } from "react";
import { apiFetch, DAEMON_URL } from "./useLogs";
import { detectCountry } from "../utils/network";

export const useAppConfig = (addLog) => {
  const [isConfigLoaded, setIsConfigLoaded] = useState(false);
  const [proxies, setProxies] = useState([]);
  const [routingRules, setRoutingRules] = useState({
    mode: "global",
    whitelist: ["localhost", "127.0.0.1"],
    appWhitelist: [],
  });
  const [settings, setSettings] = useState({
    autostart: false,
    killswitch: false,
    adblock: false,
  });
  const [showProtocolModal, setShowProtocolModal] = useState(false);
  const [platform, setPlatform] = useState("win32");

  useEffect(() => {
    apiFetch(`/api/platform`)
      .then((res) => res.json())
      .then((data) => {
        if (data.platform) setPlatform(data.platform);
      })
      .catch(() => {});

    apiFetch(`/api/config`)
      .then((res) => res.json())
      .then((data) => {
        if (
          data.proxies &&
          Array.isArray(data.proxies) &&
          data.proxies.length > 0
        ) {
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
  }, [addLog]);

  useEffect(() => {
    if (!isConfigLoaded) return;
    apiFetch(`/api/config`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ proxies, routingRules, settings }),
    }).catch(() => {});
  }, [proxies, routingRules, settings, isConfigLoaded]);

  useEffect(() => {
    if (!isConfigLoaded) return;
    apiFetch(`/api/update-rules`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(routingRules),
    }).catch(() => {});
  }, [routingRules, isConfigLoaded]);

  useEffect(() => {
    if (!isConfigLoaded) return;
    apiFetch(`/api/sync-proxies`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(proxies),
    }).catch(() => {});
  }, [proxies, isConfigLoaded]);

  const updateSetting = useCallback((key, value) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
    if (key === "autostart" || key === "killswitch" || key === "adblock") {
      apiFetch(`/api/${key}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enable: value }),
      })
        .then(async (res) => {
          if (!res.ok) {
            const data = await res.json().catch(() => ({}));
            if (key === "autostart") {
              setSettings((prev) => ({ ...prev, autostart: !value }));
              alert(
                `Ошибка настройки автостарта:\n\n${data.error || "Неизвестная ошибка"}`,
              );

              if (data.needsAdmin && window.electronAPI?.restartAsAdmin) {
                const restart = window.confirm(
                  "Для настройки автостарта с правами администратора требуется перезапуск. Перезапустить сейчас?",
                );
                if (restart) window.electronAPI.restartAsAdmin();
              }
            }
            return;
          }
          if (key === "killswitch" && value) {
            const data = await res.json().catch(() => ({}));
            if (data.needsAdmin && window.electronAPI?.restartAsAdmin) {
              const restart = window.confirm(
                "Kill Switch работает эффективнее с правами администратора (полная блокировка через файрвол).\n\nПерезапустить приложение с правами администратора?",
              );
              if (restart) {
                window.electronAPI.restartAsAdmin();
              }
            }
          }
        })
        .catch(() => {
          if (key === "autostart") {
            setSettings((prev) => ({ ...prev, autostart: !value }));
          }
        });
    }
  }, []);

  const handleSaveProxy = useCallback(
    async (
      proxyData,
      activeProxy,
      failedProxy,
      setFailedProxy,
      setActiveProxy,
      isConnected,
      selectAndConnect,
      setActiveTab,
      setEditingProxy,
    ) => {
      let countryCode = await detectCountry(proxyData.ip);

      if (
        countryCode === "unknown" &&
        proxyData.country &&
        proxyData.country !== "🌐" &&
        proxyData.country !== "unknown"
      ) {
        countryCode = proxyData.country;
      }

      const finalProxy = { ...proxyData, country: countryCode };

      if (proxyData.id) {
        setProxies((prev) =>
          prev.map((p) => (p.id === proxyData.id ? finalProxy : p)),
        );
        if (failedProxy?.id === proxyData.id) setFailedProxy(null);
        addLog(`Профиль "${proxyData.name}" обновлен.`, "success");

        if (activeProxy?.id === proxyData.id) {
          setActiveProxy(finalProxy);
          if (isConnected) {
            addLog("Применение новых настроек, перезапуск...", "info");
            setTimeout(() => {
              selectAndConnect(finalProxy, true);
            }, 100);
            setActiveTab("list");
          } else {
            setActiveTab("list");
          }
        } else {
          setActiveTab("list");
        }
      } else {
        setProxies((prev) => [...prev, { ...finalProxy, id: Date.now() }]);
        addLog(`Новый профиль "${proxyData.name}" добавлен.`, "success");
        setActiveTab("list");
      }
      setEditingProxy(null);
    },
    [addLog],
  );

  const handleBulkSaveProxies = useCallback(
    async (proxiesData, setActiveTab, defaultProtocol) => {
      const now = Date.now();
      const finalProxies = await Promise.all(
        proxiesData.map(async (p, index) => {
          const countryCode = await detectCountry(p.ip);
          return {
            ...p,
            id: now + index,
            country: countryCode,
            type: defaultProtocol || p.type || "HTTP",
          };
        }),
      );

      setProxies((prev) => [...prev, ...finalProxies]);
      addLog(`Добавлено ${finalProxies.length} новых прокси.`, "success");
      setActiveTab("list");
    },
    [addLog],
  );

  return {
    isConfigLoaded,
    proxies,
    setProxies,
    routingRules,
    setRoutingRules,
    settings,
    setSettings,
    updateSetting,
    handleSaveProxy,
    handleBulkSaveProxies,
    showProtocolModal,
    setShowProtocolModal,
    platform,
  };
};
