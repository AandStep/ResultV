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

import { useState, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import wailsAPI from "../utils/wailsAPI";

export const useDaemonStatus = (
    isConnected,
    setIsConnected,
    setIsConnecting,
    proxies,
    failedProxy,
    setFailedProxy,
    setActiveProxy,
    isSwitchingRef,
    addLog,
    activeProxy,
    statusGenerationRef,
    showAlertDialog,
) => {
    const { t } = useTranslation();
    const [isProxyDead, setIsProxyDead] = useState(false);
    const [stats, setStats] = useState({ download: 0, upload: 0 });
    const [speedHistory, setSpeedHistory] = useState({
        down: new Array(20).fill(0),
        up: new Array(20).fill(0),
    });
    const [daemonStatus, setDaemonStatus] = useState("checking");
    const prevProxyDead = useRef(false);
    const emergencyPopupShownRef = useRef(false);
    const emergencyActionInFlightRef = useRef(false);
    const statusErrorStreakRef = useRef(0);
    const daemonStatusRef = useRef(daemonStatus);
    daemonStatusRef.current = daemonStatus;

    useEffect(() => {
        const resolveActiveProxy = (data) => {
            if (!data.currentProxy) return;
            const currentID = String(data.currentProxy.id || "").trim();
            const currentIP = String(data.currentProxy.ip || "").trim().toLowerCase();
            const currentType = String(data.currentProxy.type || "").trim().toLowerCase();
            const currentPort = Number(data.currentProxy.port || 0);
            const localMatchedProxy = proxies.find((p) => {
                if (p.type?.toUpperCase() === "SECTION") {
                    return false;
                }
                const proxyID = String(p.id || "").trim();
                if (currentID && proxyID && proxyID === currentID) {
                    return true;
                }
                const proxyIP = String(p.ip || "").trim().toLowerCase();
                const proxyType = String(p.type || "").trim().toLowerCase();
                const proxyPort = Number(p.port || 0);
                return (
                    proxyIP === currentIP &&
                    proxyPort === currentPort &&
                    proxyType === currentType
                );
            });

            let resolvedProxy = localMatchedProxy || data.currentProxy;

            if (activeProxy && activeProxy.type?.toUpperCase() !== "AUTO") {
                const activeIP = String(activeProxy.ip || "").trim().toLowerCase();
                const activePort = Number(activeProxy.port || 0);
                const activeType = String(activeProxy.type || "").trim().toLowerCase();
                if (
                    activeIP === currentIP &&
                    activePort === currentPort &&
                    activeType === currentType
                ) {
                    resolvedProxy = activeProxy;
                }
            } else if (localMatchedProxy && activeProxy?.type?.toUpperCase() === "AUTO") {
                try {
                    const extra =
                        typeof activeProxy.extra === "string"
                            ? JSON.parse(activeProxy.extra)
                            : activeProxy.extra || {};
                    const memberIds = (extra.members || []).map(String);
                    if (memberIds.includes(String(localMatchedProxy.id))) {
                        resolvedProxy = activeProxy;
                    }
                } catch (e) {
                    // ignore
                }
            }

            if (
                resolvedProxy &&
                String(resolvedProxy.type || "").toUpperCase() !== "SECTION"
            ) {
                setActiveProxy(resolvedProxy);
            }
        };

        const processStatus = (data, genAtStart) => {
            if (statusGenerationRef && statusGenerationRef.current !== genAtStart) {
                return;
            }
            if (!data || typeof data !== "object") {
                throw new Error("invalid daemon status payload");
            }

            statusErrorStreakRef.current = 0;
            if (daemonStatusRef.current !== "online") {
                setDaemonStatus("online");
            }

            const connected = !!data.isConnected;
            const establishing = !!data.isEstablishing;
            const sessionActive = connected || establishing;
            const proxyDead = !!data.isProxyDead;
            const killSwitchEmergency = !!data.killSwitchEmergency;

            setIsProxyDead(proxyDead);
            if (sessionActive && !proxyDead) {
                setFailedProxy(null);
            }

            if (establishing) {
                setIsConnecting(false);
            }

            const killSwitchTriggered =
                killSwitchEmergency && !isSwitchingRef.current;

            if (
                killSwitchTriggered &&
                !emergencyPopupShownRef.current &&
                !emergencyActionInFlightRef.current &&
                typeof showAlertDialog === "function"
            ) {
                emergencyPopupShownRef.current = true;
                showAlertDialog({
                    title: t("killswitchPopup.title"),
                    message: t("killswitchPopup.message"),
                    variant: "danger",
                    confirmText: t("killswitchPopup.confirm"),
                    onConfirmAction: async () => {
                        if (emergencyActionInFlightRef.current) return;
                        emergencyActionInFlightRef.current = true;
                        addLog(
                            "[KILL SWITCH] Подтверждено: отключаем сервер и снимаем блокировку фаервола.",
                            "warning",
                        );
                        try {
                            await wailsAPI.disconnect();
                        } catch (e) {
                            // best effort
                        }
                        try {
                            await wailsAPI.toggleKillSwitch(false);
                            setIsConnected(false);
                            setIsProxyDead(false);
                            setFailedProxy(null);
                            addLog(
                                "[KILL SWITCH] Сервер отключён, правила фаервола сняты. Настройка Kill Switch активна.",
                                "success",
                            );
                        } catch (e) {
                            addLog(
                                `[KILL SWITCH] Ошибка снятия блокировки: ${e?.message || e}`,
                                "error",
                            );
                        } finally {
                            emergencyActionInFlightRef.current = false;
                        }
                    },
                });
            }
            if ((sessionActive && !proxyDead) || !killSwitchEmergency) {
                emergencyPopupShownRef.current = false;
            }
            if (connected) {
                if (data.isProxyDead && !prevProxyDead.current) {
                    addLog(
                        `Внимание: Узел ${
                            data.currentProxy?.ip || ""
                        } перестал отвечать! (Kill Switch: ${data.killSwitchActive})`,
                        "error",
                    );
                } else if (!data.isProxyDead && prevProxyDead.current) {
                    addLog(`Связь с узлом восстановлена.`, "success");
                }
            }
            prevProxyDead.current = !!data.isProxyDead;

            const allowStatus = !isSwitchingRef.current || establishing;
            if (allowStatus) {
                setIsConnected(sessionActive);
                resolveActiveProxy(data);
            }

            setStats({ download: data.bytesReceived, upload: data.bytesSent });
            setSpeedHistory((h) => ({
                down: [...h.down.slice(1), data.speedReceived || 0],
                up: [...h.up.slice(1), data.speedSent || 0],
            }));
        };

        const fetchStatus = async () => {
            const genAtStart = statusGenerationRef ? statusGenerationRef.current : 0;
            try {
                const data = await wailsAPI.getStatus();
                processStatus(data, genAtStart);
            } catch (error) {
                statusErrorStreakRef.current += 1;
                if (statusErrorStreakRef.current >= 3) {
                    setDaemonStatus("offline");
                    if (isConnected) setIsConnected(false);
                }
            }
        };

        const onStatusUpdate = (data) => {
            try {
                const genAtStart = statusGenerationRef
                    ? statusGenerationRef.current
                    : 0;
                processStatus(data, genAtStart);
            } catch (error) {
                console.error("status:update handler error:", error);
            }
        };

        EventsOn("status:update", onStatusUpdate);
        fetchStatus();
        const interval = setInterval(fetchStatus, 1000);
        return () => {
            clearInterval(interval);
            EventsOff("status:update");
        };
    }, [
        isConnected,
        proxies,
        failedProxy,
        addLog,
        setIsConnected,
        setIsConnecting,
        setActiveProxy,
        setFailedProxy,
        isSwitchingRef,
        activeProxy,
        statusGenerationRef,
        showAlertDialog,
        t,
    ]);

    return { isProxyDead, stats, speedHistory, daemonStatus };
};
