/*
 * Copyright (C) 2026 ResultProxy
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

import { useCallback } from "react";
import wailsAPI from "../utils/wailsAPI";

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
    updateSetting,
    daemonStatus,
    isSwitchingRef,
    addLog,
) => {
    const toggleConnection = useCallback(async () => {
        if (daemonStatus !== "online") {
            addLog("Служба недоступна.", "error");
            return;
        }

        const targetProxy = activeProxy || proxies.find(p => p.id === settings?.lastSelectedProxyId) || proxies[0];
        if (proxies.length === 0 || !targetProxy) return;

        try {
            isSwitchingRef.current = true;
            setFailedProxy(null);

            if (isConnected) {
                addLog("Отключение...", "info");
                await wailsAPI.disconnect();
                addLog("Отключено успешно.", "success");
                setIsConnected(false);
            } else {
                addLog(`Подключение к ${targetProxy.name}...`, "info");
                setActiveProxy(targetProxy);
                if (settings?.lastSelectedProxyId !== targetProxy.id) {
                    updateSetting("lastSelectedProxyId", targetProxy.id);
                }

                const res = await wailsAPI.connect(
                    { ...targetProxy, port: parseInt(targetProxy.port, 10) || 0 },
                    routingRules,
                    settings.killswitch || false,
                    settings.adblock || false
                );

                if (!res.success) {
                    const reason = res.reason ? ` Причина: ${res.reason}` : "";
                    throw new Error((res.message || "Unknown proxy connection error") + reason);
                }

                addLog("Соединение установлено.", "success");
                if (res.tunnelFailed) {
                    addLog(`Туннелирование не запущено: ${res.reason || "неизвестная причина"}`, "warning");
                    if (res.fallbackUsed) {
                        addLog("Подключение работает в fallback-режиме без TUN.", "warning");
                    }
                }
                
                // Show GPO warning if the core hit a GPO block
                // (Wails events: we actually might emit this from app.go or manager.go)
                setIsConnected(true);
            }

            setTimeout(() => {
                isSwitchingRef.current = false;
            }, 3000);
        } catch (error) {
            isSwitchingRef.current = false;
            setFailedProxy(targetProxy);
            addLog(`Сбой: ${error.message || error}`, "error");
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
        updateSetting
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
                if (settings?.lastSelectedProxyId !== proxy.id) {
                    updateSetting("lastSelectedProxyId", proxy.id);
                }
                addLog(`Переключение на: ${proxy.name}...`, "info");

                if (isConnected) {
                    await wailsAPI.disconnect();
                    setIsConnected(false);
                }

                const res = await wailsAPI.connect(
                    { ...proxy, port: parseInt(proxy.port, 10) || 0 },
                    routingRules,
                    settings.killswitch || false,
                    settings.adblock || false
                );

                if (!res.success) {
                    const reason = res.reason ? ` Причина: ${res.reason}` : "";
                    throw new Error((res.message || "Ошибка смены прокси: Узел отклонил подключение") + reason);
                }

                setIsConnected(true);
                addLog(`Успешно переключено на ${proxy.name}`, "success");
                if (res.tunnelFailed) {
                    addLog(`Туннелирование не запущено: ${res.reason || "неизвестная причина"}`, "warning");
                    if (res.fallbackUsed) {
                        addLog("Подключение работает в fallback-режиме без TUN.", "warning");
                    }
                }

                setTimeout(() => {
                    isSwitchingRef.current = false;
                }, 2000);
            } catch (error) {
                isSwitchingRef.current = false;
                setFailedProxy(proxy);
                addLog(`Сбой подключения: ${error.message || error}`, "error");
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
            updateSetting
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
                        await wailsAPI.disconnect();
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
