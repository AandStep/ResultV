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

import React, { createContext, useContext, useEffect, useState } from "react";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import { useAppConfig } from "../hooks/useAppConfig";
import { useLogContext } from "./LogContext";
import wailsAPI from "../utils/wailsAPI";

const ConfigContext = createContext();

export const ConfigProvider = ({ children }) => {
    const { addLog } = useLogContext();
    const config = useAppConfig(addLog);
    const [activeTab, setActiveTab] = useState("home");
    const [editingProxy, setEditingProxy] = useState(null);
    const [pendingDeepLink, setPendingDeepLink] = useState("");

    useEffect(() => {
        const applyConfig = (cfg) => {
            if (!cfg) return;
            if (Array.isArray(cfg.proxies) && config.setProxies) {
                config.setProxies(
                    cfg.proxies.map((p) => ({
                        ...p,
                        port: parseInt(p.port, 10) || 0,
                        id: String(p.id),
                    })),
                );
            }
            if (Array.isArray(cfg.subscriptions) && config.setSubscriptions) {
                config.setSubscriptions(cfg.subscriptions);
            }
        };
        EventsOn("deeplink:received", (payload) => {
            const text = String(payload?.payload || "").trim();
            if (!text) return;
            setPendingDeepLink(text);
        });
        EventsOn("deeplink:error", (msg) => {
            const text = typeof msg === "string" ? msg : JSON.stringify(msg);
            addLog(`Ошибка ссылки resultv://: ${text}`, "error");
        });
        EventsOn("config:updated", (cfg) => {
            applyConfig(cfg);
        });
        return () => {
            EventsOff("deeplink:received");
            EventsOff("deeplink:error");
            EventsOff("config:updated");
        };

    }, [addLog, config.setProxies, config.setSubscriptions]);

    const value = {
        ...config,
        activeTab,
        setActiveTab,
        editingProxy,
        setEditingProxy,
        pendingDeepLink,
        setPendingDeepLink,
    };

    return (
        <ConfigContext.Provider value={value}>
            {children}
        </ConfigContext.Provider>
    );
};

export const useConfigContext = () => {
    const context = useContext(ConfigContext);
    if (!context) throw new Error("useConfigContext must be used within ConfigProvider");
    return context;
};
