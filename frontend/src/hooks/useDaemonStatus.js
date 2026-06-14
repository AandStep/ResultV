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
    const [uptime, setUptime] = useState(0);
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
            // Prefer an ID match over the IP+Port+Type fallback. When a
            // subscription returns the same backend twice (once as an auto
            // member, once as a standalone individual), two entries share
            // IP+Port+Type but have distinct IDs — the user's CLICK is
            // identified by ID, so the ID lane is the only correct match.
            // .find() iterates in array order; if currentID is set and any
            // entry matches it, that entry wins before the IP fallback even
            // considers a same-backend duplicate sitting earlier in the array.
            const localMatchedProxy = proxies.find((p) => {
                if (p.type?.toUpperCase() === "SECTION") {
                    return false;
                }
                const proxyID = String(p.id || "").trim();
                if (currentID && proxyID && proxyID === currentID) {
                    return true;
                }
                if (currentID) {
                    // We have a currentID — only an ID match is authoritative.
                    // Refuse IP+Port+Type fallback for this entry; it would
                    // pick a same-backend twin and silently show the wrong row.
                    return false;
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

            // Stickiness: keep activeProxy stable across status ticks so the
            // UI doesn't flicker between same-IP twins. BUT skip the sticky
            // override when we have a definitive ID match — the user's click
            // explicitly targeted that ID, so showing the previous activeProxy
            // (which had matching IP+Port+Type but a different ID) would be
            // the exact "click X, display Y" bug we just fixed in the tray.
            const haveDefinitiveIDMatch =
                currentID &&
                localMatchedProxy &&
                String(localMatchedProxy.id || "").trim() === currentID;

            if (
                activeProxy &&
                activeProxy.type?.toUpperCase() !== "AUTO" &&
                !haveDefinitiveIDMatch
            ) {
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

            // Mirror the backend's establishing phase into the spinner while no
            // user-initiated control op owns it. The engine has booted but
            // runPostStartProbe is still verifying end-to-end traffic (1-3s for
            // WG/Hysteria/VLESS handshakes), so we keep the spinner up — but it
            // must also come back DOWN when establishing ends, not only go up.
            // A hot-reload reconnect (rules/adblock change) goes
            // establishing→connected with no control op in flight; the old
            // raise-only code then left isConnecting stuck true forever, so the
            // UI showed an endless "connecting" spinner even though traffic
            // already flowed.
            //
            // Skip when a user-initiated control op is in flight — in that case
            // useDaemonControl owns the spinner and we'd otherwise stomp its
            // setIsConnecting(false) right after a disconnect.
            if (!isSwitchingRef.current) {
                setIsConnecting(establishing);
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
                    // Subscription servers must not reveal the provider's backend
                    // address; only manual servers (no subscriptionUrl) show the IP.
                    const nodeAddr = data.currentProxy?.subscriptionUrl
                        ? ""
                        : data.currentProxy?.ip || "";
                    addLog(
                        `Внимание: Узел ${nodeAddr} перестал отвечать! (Kill Switch: ${data.killSwitchActive})`,
                        "error",
                    );
                } else if (!data.isProxyDead && prevProxyDead.current) {
                    addLog(`Связь с узлом восстановлена.`, "success");
                }
            }
            prevProxyDead.current = !!data.isProxyDead;

            const allowStatus = !isSwitchingRef.current || establishing;
            if (allowStatus) {
                // STRICT: only the backend's "fully connected" flag drives the
                // green/Connected indicator. Treating "establishing" as
                // "connected" caused the UI to claim success 1-3s before the
                // backend's post-start probe finished — which for WG/AWG (where
                // probe time is longest and traffic can still collapse after
                // handshake) meant the user saw "Connected" right before the
                // tunnel died. Spinner stays via setIsConnecting(true) above.
                setIsConnected(connected);
                resolveActiveProxy(data);
            }

            setUptime(typeof data.uptime === "number" ? data.uptime : 0);
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

    return { isProxyDead, stats, speedHistory, daemonStatus, uptime };
};
