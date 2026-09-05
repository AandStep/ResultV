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
    /*
     * Раскрытость бокового меню живёт здесь, а не в самом меню: каждая
     * страница нового дизайна рисует свой экземпляр AppSidebar, и при
     * переходе меню размонтировалось вместе со страницей — а с ним и его
     * состояние, из-за чего раскрытое меню схлопывалось на каждом переходе.
     */
    const [sidebarOpen, setSidebarOpen] = useState(false);
    const [editingProxy, setEditingProxy] = useState(null);
    const [pendingDeepLink, setPendingDeepLink] = useState("");
    const [pendingDeepLinkSource, setPendingDeepLinkSource] = useState("");

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
        // Ссылка всегда лежит в очереди на стороне Go, а событие — только
        // звонок «приходи за ней». Так она не теряется, если события ещё
        // некому слушать (холодный старт), и не задваивается: очередь
        // забирается разом.
        const takeDeepLink = async (event) => {
            const queued = await wailsAPI.takePendingDeepLink();
            const text = String(queued?.payload || event?.payload || "").trim();
            if (!text) return;
            const source = queued?.payload ? queued?.source : event?.source;
            setPendingDeepLink(text);
            setPendingDeepLinkSource(typeof source === "string" ? source : "");
        };

        EventsOn("deeplink:received", takeDeepLink);
        EventsOn("deeplink:error", (msg) => {
            const text = typeof msg === "string" ? msg : JSON.stringify(msg);
            addLog(`Ошибка ссылки resultv://: ${text}`, "error");
        });
        EventsOn("config:updated", (cfg) => {
            applyConfig(cfg);
        });

        // Ссылка, с которой приложение запустили, ждёт в очереди с самого
        // старта: события тогда слушать было некому.
        takeDeepLink();

        // И ещё раз — когда окно возвращается на экран. Пока оно свёрнуто или
        // спрятано в трей, WebView2 усыпляет страницу, и событие до неё может
        // не доехать вовсе; очередь на стороне Go в этот момент уже полна, и
        // забрать её надо самим. Пусто — вернётся пусто, лишним не будет.
        const onWake = () => {
            if (document.visibilityState === "visible") takeDeepLink();
        };
        window.addEventListener("focus", onWake);
        document.addEventListener("visibilitychange", onWake);

        return () => {
            EventsOff("deeplink:received");
            EventsOff("deeplink:error");
            EventsOff("config:updated");
            window.removeEventListener("focus", onWake);
            document.removeEventListener("visibilitychange", onWake);
        };

    }, [addLog, config.setProxies, config.setSubscriptions]);

    const value = {
        ...config,
        activeTab,
        setActiveTab,
        sidebarOpen,
        setSidebarOpen,
        editingProxy,
        setEditingProxy,
        pendingDeepLink,
        setPendingDeepLink,
        pendingDeepLinkSource,
        setPendingDeepLinkSource,
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
