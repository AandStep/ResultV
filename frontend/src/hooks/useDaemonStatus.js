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
import wailsAPI from "../utils/wailsAPI";

export const useDaemonStatus = (
    isConnected,
    setIsConnected,
    proxies,
    failedProxy,
    setFailedProxy,
    setActiveProxy,
    isSwitchingRef,
    addLog,
) => {
    const [isProxyDead, setIsProxyDead] = useState(false);
    const [stats, setStats] = useState({ download: 0, upload: 0 });
    const [speedHistory, setSpeedHistory] = useState({
        down: new Array(20).fill(0),
        up: new Array(20).fill(0),
    });
    const [daemonStatus, setDaemonStatus] = useState("checking");
    const prevProxyDead = useRef(false);

    useEffect(() => {
        let interval;
        const fetchStatus = async () => {
            try {
                const data = await wailsAPI.getStatus();
                if (daemonStatus !== "online") setDaemonStatus("online");

                setIsProxyDead(!!data.isProxyDead);
                if (data.isConnected) {
                    setFailedProxy(null);
                }

                if (data.isConnected) {
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

                if (!isSwitchingRef.current) {
                    setIsConnected(data.isConnected);
                    if (data.currentProxy) {
                        const currentID = String(data.currentProxy.id || "").trim();
                        const currentIP = String(data.currentProxy.ip || "").trim().toLowerCase();
                        const currentType = String(data.currentProxy.type || "").trim().toLowerCase();
                        const currentPort = Number(data.currentProxy.port || 0);
                        const localMatchedProxy = proxies.find(
                            (p) => {
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
                            },
                        );
                        setActiveProxy(localMatchedProxy || data.currentProxy);
                    }
                }

                setStats({ download: data.bytesReceived, upload: data.bytesSent });
                setSpeedHistory((h) => ({
                    down: [...h.down.slice(1), data.speedReceived || 0],
                    up: [...h.up.slice(1), data.speedSent || 0],
                }));
                
            } catch (error) {
                setDaemonStatus("offline");
                if (isConnected) setIsConnected(false);
            }
        };

        fetchStatus();
        interval = setInterval(fetchStatus, 1000);
        return () => clearInterval(interval);
    }, [
        isConnected,
        daemonStatus,
        proxies,
        failedProxy,
        addLog,
        setIsConnected,
        setActiveProxy,
        setFailedProxy,
        isSwitchingRef,
    ]);

    return { isProxyDead, stats, speedHistory, daemonStatus };
};
