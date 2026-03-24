/*
 * Copyright (C) 2026 ResultProxy
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

import React, { createContext, useContext, useState, useRef } from "react";
import { useConfigContext } from "./ConfigContext";
import { useLogContext } from "./LogContext";
import { useDaemonPing } from "../hooks/useDaemonPing";
import { useDaemonStatus } from "../hooks/useDaemonStatus";
import { useDaemonControl } from "../hooks/useDaemonControl";

const ConnectionContext = createContext();

export const ConnectionProvider = ({ children }) => {
  const { proxies, routingRules, settings, updateSetting, isConfigLoaded } =
    useConfigContext();
  const { addLog } = useLogContext();

  const [isConnected, setIsConnected] = useState(false);
  const [failedProxy, setFailedProxy] = useState(null);
  const [activeProxy, setActiveProxy] = useState(null);

  const isSwitchingRef = useRef(false);

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
  );

  const { toggleConnection, selectAndConnect, deleteProxy } = useDaemonControl(
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
  );

  const value = {
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
