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

import React, { createContext, useContext, useState, useRef, useEffect } from "react";
import { useConfigContext } from "./ConfigContext";
import { useLogContext } from "./LogContext";
import { useDaemonPing } from "../hooks/useDaemonPing";
import { useDaemonStatus } from "../hooks/useDaemonStatus";
import { useDaemonControl } from "../hooks/useDaemonControl";

const ConnectionContext = createContext();

export const ConnectionProvider = ({ children }) => {
    const {
        proxies,
        routingRules,
        settings,
        updateSetting,
        isConfigLoaded,
        showAlertDialog,
        isApplyingMode,
    } = useConfigContext();
    const { addLog } = useLogContext();

    const [isConnected, setIsConnected] = useState(false);
    const [isConnecting, setIsConnecting] = useState(false);
    const [failedProxy, setFailedProxy] = useState(null);
    const [activeProxy, setActiveProxy] = useState(null);

    const isSwitchingRef = useRef(false);

    // Mirror mode-apply state into the connection flags so the UI shows
    // "reconnecting" instead of briefly flipping to "disconnected" while
    // the backend disconnect/connect cycle runs.
    useEffect(() => {
        if (isApplyingMode) {
            isSwitchingRef.current = true;
            setIsConnecting(true);
        } else {
            isSwitchingRef.current = false;
            setIsConnecting(false);
        }
    }, [isApplyingMode]);

    const pings = useDaemonPing(proxies, isConfigLoaded);

    const { isProxyDead, stats, speedHistory, daemonStatus } = useDaemonStatus(
        isConnected,
        setIsConnected,
        proxies,
        failedProxy,
        setFailedProxy,
        setActiveProxy,
        isSwitchingRef,
        addLog,
        settings,
        activeProxy,
    );

    const { disconnectOnly, toggleConnection, selectAndConnect, deleteProxy, cancelConnect } = useDaemonControl(
        isConnected,
        setIsConnected,
        setIsConnecting,
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
    );

    const value = {
        isConnected,
        isConnecting,
        isProxyDead,
        failedProxy,
        setFailedProxy,
        activeProxy,
        setActiveProxy,
        stats,
        speedHistory,
        pings,
        daemonStatus,
        disconnectOnly,
        toggleConnection,
        selectAndConnect,
        deleteProxy,
        cancelConnect,
    };

    return (
        <ConnectionContext.Provider value={value}>
            {children}
        </ConnectionContext.Provider>
    );
};

export const useConnectionContext = () => {
    const context = useContext(ConnectionContext);
    if (!context)
        throw new Error(
            "useConnectionContext must be used within ConnectionProvider",
        );
    return context;
};
