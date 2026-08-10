/*
 * Copyright (C) 2026 ResultV
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

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import wailsAPI from "../utils/wailsAPI";

export const useDaemonControl = (
    isConnected,
    setIsConnected,
    setIsConnecting,
    setIsDisconnecting,
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
    showAlertDialog,
    pings,
    statusGenerationRef,
) => {
    const { t } = useTranslation();

    const AUTO_MAX_ATTEMPTS = 5;

    const bumpGen = () => {
        if (statusGenerationRef) statusGenerationRef.current += 1;
    };

    // Ranking lives in the backend (App.ResolveAutoCandidates) so the tray and
    // the UI cannot drift apart. This used to sort AUTO members by the cached
    // ping sweep, which measured a bare TCP connect and knew nothing about
    // past connect failures.
    const getConnectCandidates = useCallback(async (proxyToResolve) => {
        if (proxyToResolve?.type?.toUpperCase() !== "AUTO") {
            return [proxyToResolve];
        }
        const ranked = await wailsAPI.resolveAutoCandidates(proxyToResolve.id);
        return ranked.length > 0 ? ranked : [proxyToResolve];
    }, []);

    const isTerminalErrorCode = (code) =>
        code === "tun_privileges" || code === "proxy_not_supported";

    const disconnectOnly = useCallback(async () => {
        if (isSwitchingRef.current) return;

        try {
            bumpGen();
            isSwitchingRef.current = true;
            setIsConnecting(false);
            setIsDisconnecting(true);
            addLog("Отключение...", "info");
            await wailsAPI.disconnect();
            setIsConnected(false);
            setFailedProxy(null);
            addLog("Отключено успешно.", "success");
        } catch (error) {
            addLog(`Сбой отключения: ${error.message || error}`, "error");
        } finally {
            bumpGen();
            setIsDisconnecting(false);
            isSwitchingRef.current = false;
        }
    }, [addLog, isSwitchingRef, setFailedProxy, setIsConnected, setIsConnecting, setIsDisconnecting]);

    const toggleConnection = useCallback(async () => {
        if (isSwitchingRef.current) return;
        if (daemonStatus !== "online") {
            addLog("Служба недоступна.", "error");
            return;
        }

        const targetProxy =
            activeProxy ||
            proxies.find((p) => String(p.id) === String(settings?.lastSelectedProxyId)) ||
            proxies[0];
        if (proxies.length === 0 || !targetProxy) return;

        try {
            bumpGen();
            isSwitchingRef.current = true;
            setFailedProxy(null);

            if (isConnected) {
                setIsConnecting(false);
                setIsDisconnecting(true);
                addLog("Отключение...", "info");
                await wailsAPI.disconnect();
                addLog("Отключено успешно.", "success");
                setIsConnected(false);
                setIsDisconnecting(false);
            } else {
                setIsConnecting(true);
                if (targetProxy?.type?.toUpperCase() === "SECTION") {
                    setIsConnecting(false);
                    showAlertDialog({
                        title: t("common.notice"),
                        message:
                            t("proxyList.sectionNoConnect") ||
                            "This row is a subscription group label — pick a server below.",
                        variant: "info",
                    });
                    bumpGen();
                    isSwitchingRef.current = false;
                    return;
                }
                const isAuto = targetProxy?.type?.toUpperCase() === "AUTO";
                const candidates = (await getConnectCandidates(targetProxy)).slice(0, isAuto ? AUTO_MAX_ATTEMPTS : 1);
                addLog(`Подключение к ${targetProxy.name}...`, "info");
                setActiveProxy(targetProxy);
                if (String(settings?.lastSelectedProxyId) !== String(targetProxy.id)) {
                    updateSetting("lastSelectedProxyId", targetProxy.id);
                }

                let res = null;
                for (let i = 0; i < candidates.length; i++) {
                    const candidate = candidates[i];
                    if (isAuto && i > 0) {
                        const label = candidate.name || `${candidate.ip}:${candidate.port}`;
                        addLog(`Auto: пробуем следующий узел (${label})...`, "info");
                        try { await wailsAPI.disconnect(); } catch {}
                    }
                    res = await wailsAPI.connect(
                        { ...candidate, port: parseInt(candidate.port, 10) || 0, id: targetProxy.id, name: targetProxy.name },
                        routingRules,
                        settings.killswitch || false
                    );
                    // Whether a connect actually succeeded is the most honest
                    // signal about a node, and it used to be discarded. Only
                    // AUTO reports: a single server has nothing to rank against.
                    if (isAuto) {
                        await wailsAPI.reportAutoConnectOutcome(
                            targetProxy.id,
                            candidate.ip,
                            parseInt(candidate.port, 10) || 0,
                            !!res.success,
                        );
                    }
                    if (res.success) break;
                    if (isTerminalErrorCode(res.errorCode)) break;
                    if (!isAuto) break;
                }

                if (!res?.success) {
                    if (res?.errorCode === "tun_privileges") {
                        setIsConnecting(false);
                        addLog(
                            res.message || t("tunnel.adminMessage"),
                            "error",
                        );
                        showAlertDialog({
                            title: t("tunnel.adminTitle"),
                            message: t("tunnel.adminMessage"),
                            variant: "warning",
                            confirmText: t("tunnel.restartAsAdmin"),
                            onConfirmAction: () => wailsAPI.restartAsAdmin(),
                        });
                        bumpGen();
                        isSwitchingRef.current = false;
                        return;
                    }
                    const reason = res?.reason ? ` Причина: ${res.reason}` : "";
                    const code = res?.errorCode ? ` Код: ${res.errorCode}` : "";
                    const prefix = isAuto && candidates.length > 1
                        ? `Auto: все ${candidates.length} попытки не удались. `
                        : "";
                    throw new Error(prefix + (res?.message || "Unknown proxy connection error") + code + reason);
                }

                addLog("Соединение установлено.", "success");
                if (res.tunnelFailed) {
                    addLog(`Туннелирование не запущено: ${res.reason || "неизвестная причина"}`, "warning");
                    if (res.fallbackUsed) {
                        addLog("Подключение работает в fallback-режиме без TUN.", "warning");
                    }
                }



                setIsConnected(true);
                setIsConnecting(false);
            }

            bumpGen();
            isSwitchingRef.current = false;
        } catch (error) {
            bumpGen();
            isSwitchingRef.current = false;
            setIsConnected(false);
            setIsConnecting(false);
            setIsDisconnecting(false);
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
        setIsConnecting,
        setIsDisconnecting,
        updateSetting,
        showAlertDialog,
        t,
        getConnectCandidates,
    ]);

    const selectAndConnect = useCallback(
        async (proxy, forceReconnect = false, setActiveTab) => {
            if (isSwitchingRef.current) return;
            if (!forceReconnect && activeProxy?.id === proxy.id && isConnected)
                return;

            if (proxy?.type?.toUpperCase() === "SECTION") {
                showAlertDialog({
                    title: t("common.notice"),
                    message:
                        t("proxyList.sectionNoConnect") ||
                        "This row is a subscription group label — pick a server below.",
                    variant: "info",
                });
                return;
            }

            try {
                bumpGen();
                isSwitchingRef.current = true;
                setFailedProxy(null);
                if (setActiveTab) setActiveTab("home");

                const isAuto = proxy?.type?.toUpperCase() === "AUTO";
                const candidates = (await getConnectCandidates(proxy)).slice(0, isAuto ? AUTO_MAX_ATTEMPTS : 1);

                setActiveProxy(proxy);
                if (String(settings?.lastSelectedProxyId) !== String(proxy.id)) {
                    updateSetting("lastSelectedProxyId", proxy.id);
                }
                addLog(`Переключение на: ${proxy.name}...`, "info");

                if (isConnected) {
                    setIsDisconnecting(true);
                    await wailsAPI.disconnect();
                    setIsConnected(false);
                    setIsDisconnecting(false);
                }

                setIsConnecting(true);
                let res = null;
                for (let i = 0; i < candidates.length; i++) {
                    const candidate = candidates[i];
                    if (isAuto && i > 0) {
                        const label = candidate.name || `${candidate.ip}:${candidate.port}`;
                        addLog(`Auto: пробуем следующий узел (${label})...`, "info");
                        try { await wailsAPI.disconnect(); } catch {}
                    }
                    res = await wailsAPI.connect(
                        { ...candidate, port: parseInt(candidate.port, 10) || 0, id: proxy.id, name: proxy.name },
                        routingRules,
                        settings.killswitch || false
                    );
                    // Whether a connect actually succeeded is the most honest
                    // signal about a node, and it used to be discarded. Only
                    // AUTO reports: a single server has nothing to rank against.
                    if (isAuto) {
                        await wailsAPI.reportAutoConnectOutcome(
                            proxy.id,
                            candidate.ip,
                            parseInt(candidate.port, 10) || 0,
                            !!res.success,
                        );
                    }
                    if (res.success) break;
                    if (isTerminalErrorCode(res.errorCode)) break;
                    if (!isAuto) break;
                }

                if (!res?.success) {
                    if (res?.errorCode === "tun_privileges") {
                        setIsConnecting(false);
                        addLog(
                            res.message || t("tunnel.adminMessage"),
                            "error",
                        );
                        showAlertDialog({
                            title: t("tunnel.adminTitle"),
                            message: t("tunnel.adminMessage"),
                            variant: "warning",
                            confirmText: t("tunnel.restartAsAdmin"),
                            onConfirmAction: () => wailsAPI.restartAsAdmin(),
                        });
                        bumpGen();
                        isSwitchingRef.current = false;
                        return;
                    }
                    const reason = res?.reason ? ` Причина: ${res.reason}` : "";
                    const code = res?.errorCode ? ` Код: ${res.errorCode}` : "";
                    const prefix = isAuto && candidates.length > 1
                        ? `Auto: все ${candidates.length} попытки не удались. `
                        : "";
                    throw new Error(prefix + (res?.message || "Ошибка смены прокси: Узел отклонил подключение") + code + reason);
                }

                setIsConnected(true);
                setIsConnecting(false);
                addLog(`Успешно переключено на ${proxy.name}`, "success");
                if (res.tunnelFailed) {
                    addLog(`Туннелирование не запущено: ${res.reason || "неизвестная причина"}`, "warning");
                    if (res.fallbackUsed) {
                        addLog("Подключение работает в fallback-режиме без TUN.", "warning");
                    }
                }

                bumpGen();
                isSwitchingRef.current = false;
            } catch (error) {
                bumpGen();
                isSwitchingRef.current = false;
                setIsConnected(false);
                setIsConnecting(false);
                setIsDisconnecting(false);
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
            setIsConnecting,
            setIsDisconnecting,
            isSwitchingRef,
            updateSetting,
            showAlertDialog,
            t,
            getConnectCandidates,
        ],
    );

    const deleteProxy = useCallback(
        async (id, setProxies) => {
            const isDeletingActive = activeProxy?.id === id;
            setProxies((prev) => prev.filter((p) => p.id !== id));

            if (isDeletingActive) {
                if (isConnected) {
                    bumpGen();
                    isSwitchingRef.current = true;
                    setIsConnecting(false);
                    setIsDisconnecting(true);
                    addLog("Активный сервер удален. Разрыв соединения...", "info");
                    try {
                        await wailsAPI.disconnect();
                        addLog("Отключено успешно.", "success");
                    } catch (e) {}
                    setIsConnected(false);
                    setActiveProxy(null);
                    setIsDisconnecting(false);
                    bumpGen();
                    isSwitchingRef.current = false;
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
            setIsConnecting,
            setIsDisconnecting,
            setFailedProxy,
            isSwitchingRef,
        ],
    );

    const cancelConnect = useCallback(async () => {
        // Full disconnect on cancel: backend CancelConnect aborts the probe ctx,
        // and Disconnect additionally stops the engine and clears sys proxy —
        // without this, sing-box keeps running and the next Connect fails with
        // "engine already running".
        bumpGen();
        isSwitchingRef.current = true;
        try {
            await wailsAPI.disconnect();
        } catch (e) {
            // ignore
        }
        setIsConnected(false);
        setIsConnecting(false);
        setIsDisconnecting(false);
        setFailedProxy(null);
        bumpGen();
        isSwitchingRef.current = false;
    }, [setIsConnecting, setIsDisconnecting, setIsConnected, setFailedProxy, isSwitchingRef]);

    return {
        disconnectOnly,
        toggleConnection,
        selectAndConnect,
        deleteProxy,
        cancelConnect,
    };
};
