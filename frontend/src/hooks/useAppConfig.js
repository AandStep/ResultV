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

import { useState, useEffect, useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import wailsAPI from "../utils/wailsAPI";
import { detectCountry } from "../utils/network";
import { mergeSubscriptionRefreshCountries } from "../utils/proxyParser";

export const useAppConfig = (addLog) => {
    const { t } = useTranslation();
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
        mode: "proxy",
        language: "ru",
        theme: "dark",
        localPort: 0,
        listenLan: false,
        dnsServers: [],
    });
    const [showProtocolModal, setShowProtocolModal] = useState(false);
    const [platform, setPlatform] = useState("windows");
    const [subscriptions, setSubscriptions] = useState([]);
    const modeApplyingRef = useRef(false);
    const [appDialog, setAppDialog] = useState({
        isOpen: false,
        title: "",
        message: "",
        variant: "info",
        showCancel: false,
        confirmText: "",
        cancelText: "",
    });
    const confirmResolverRef = useRef(null);
    const dialogConfirmRef = useRef(null);

    const resetDialog = useCallback(() => ({
        isOpen: false,
        title: "",
        message: "",
        variant: "info",
        showCancel: false,
        confirmText: "",
        cancelText: "",
    }), []);

    const closeAppDialog = useCallback((confirmed = false) => {
        dialogConfirmRef.current = null;
        if (confirmResolverRef.current) {
            confirmResolverRef.current(confirmed);
            confirmResolverRef.current = null;
        }
        setAppDialog(resetDialog());
    }, [resetDialog]);

    const showAlertDialog = useCallback((options = {}) => {
        if (confirmResolverRef.current) {
            confirmResolverRef.current(false);
            confirmResolverRef.current = null;
        }
        dialogConfirmRef.current =
            typeof options.onConfirmAction === "function"
                ? options.onConfirmAction
                : null;
        setAppDialog({
            isOpen: true,
            title: options.title || "",
            message: options.message || "",
            variant: options.variant || "info",
            showCancel: false,
            confirmText: options.confirmText || "",
            cancelText: "",
        });
    }, []);

    const handleAppDialogConfirm = useCallback(async () => {
        const fn = dialogConfirmRef.current;
        dialogConfirmRef.current = null;
        if (confirmResolverRef.current) {
            confirmResolverRef.current(true);
            confirmResolverRef.current = null;
        }
        setAppDialog(resetDialog());
        if (typeof fn === "function") {
            try {
                await fn();
            } catch (e) {
                console.error(e);
            }
        }
    }, [resetDialog]);

    const showConfirmDialog = useCallback((options = {}) => {
        dialogConfirmRef.current = null;
        if (confirmResolverRef.current) {
            confirmResolverRef.current(false);
            confirmResolverRef.current = null;
        }

        return new Promise((resolve) => {
            confirmResolverRef.current = resolve;
            setAppDialog({
                isOpen: true,
                title: options.title || "",
                message: options.message || "",
                variant: options.variant || "warning",
                showCancel: true,
                confirmText: options.confirmText || "",
                cancelText: options.cancelText || "",
            });
        });
    }, []);

    const persistSettings = useCallback(async (nextSettings) => {
        await wailsAPI.saveConfig({
            proxies: proxies.map(p => ({ ...p, port: parseInt(p.port, 10) || 0, id: String(p.id) })),
            routingRules,
            settings: nextSettings,
            subscriptions,
        });
    }, [proxies, routingRules, subscriptions]);

    useEffect(() => {
        const loadInitialConfig = async () => {
            try {
                const plat = await wailsAPI.getPlatform();
                setPlatform(plat);

                const config = await wailsAPI.getConfig();
                if (config) {
                    if (config.proxies && Array.isArray(config.proxies)) {
                        setProxies(config.proxies.map(p => ({ ...p, port: parseInt(p.port, 10) || 0, id: String(p.id) })));
                    }
                    if (config.routingRules) {
                        setRoutingRules(config.routingRules);
                    }
                    if (config.settings) {
                        setSettings(config.settings);
                    }
                    if (config.subscriptions && Array.isArray(config.subscriptions)) {
                        setSubscriptions(config.subscriptions);
                    }
                    setIsConfigLoaded(true);
                    addLog("Конфигурация успешно загружена.", "success");
                }
            } catch (err) {
                console.error("Failed to load config:", err);
                setIsConfigLoaded(true);
                addLog(`Служба недоступна (${err.toString()}). Используются базовые настройки.`, "error");
            }
        };

        loadInitialConfig();
    }, [addLog]);

    
    useEffect(() => {
        if (!isConfigLoaded) return;
        wailsAPI.updateRules(routingRules).catch(err => console.error("UpdateRules err:", err));
    }, [routingRules, isConfigLoaded]);

    useEffect(() => {
        if (!isConfigLoaded) return;
        const sanitizedProxies = proxies.map(p => ({ ...p, port: parseInt(p.port, 10) || 0, id: String(p.id) }));
        wailsAPI.syncProxies(sanitizedProxies).catch(err => console.error("SyncProxies err:", err));
    }, [proxies, isConfigLoaded]);

    const updateSetting = useCallback(async (key, value) => {
        const previousValue = settings[key];

        if (key === "mode") {
            if (modeApplyingRef.current) return;
            modeApplyingRef.current = true;
            setSettings((prev) => ({ ...prev, [key]: value }));
            try {
                const result = await wailsAPI.applyMode(value);
                if (!result?.success) {
                    if (result?.errorCode === "tun_privileges") {
                        setSettings((prev) => ({
                            ...prev,
                            [key]: previousValue,
                        }));
                        addLog(
                            result?.message ||
                                t("tunnel.adminMessage"),
                            "error",
                        );
                        showAlertDialog({
                            title: t("tunnel.adminTitle"),
                            message: t("tunnel.adminMessage"),
                            variant: "warning",
                            confirmText: t("tunnel.restartAsAdmin"),
                            onConfirmAction: () => wailsAPI.restartAsAdmin(),
                        });
                        return;
                    }
                    throw new Error(
                        result?.message || "Не удалось применить режим",
                    );
                }
                if (result.tunnelFailed) {
                    const reason = result.reason || "неизвестная причина";
                    addLog(`Туннелирование не запущено: ${reason}`, "warning");
                    if (result.fallbackUsed) {
                        addLog("Применен fallback: подключение продолжено без TUN.", "warning");
                    }
                } else {
                    addLog(`Режим ${value} применен`, "success");
                }
            } catch (err) {
                setSettings((prev) => ({ ...prev, [key]: previousValue }));
                addLog(`Ошибка применения режима: ${err?.message || err}`, "error");
            } finally {
                modeApplyingRef.current = false;
            }
            return;
        }

        if (key === "autostart") {
            const nextSettings = { ...settings, [key]: value };
            setSettings(nextSettings);
            try {
                await wailsAPI.setAutostart(value);
                await persistSettings(nextSettings);
            } catch (err) {
                console.error("Autostart error:", err);
                const rollbackSettings = { ...settings, [key]: previousValue };
                setSettings(rollbackSettings);
                await persistSettings(rollbackSettings).catch(console.error);
                showAlertDialog({
                    title: "Ошибка автостарта",
                    message: `Не удалось изменить настройку автозапуска.\n\n${err}`,
                    variant: "danger",
                });
            }
        } else if (key === "killswitch") {
            const nextSettings = { ...settings, [key]: value };
            setSettings(nextSettings);
            wailsAPI.toggleKillSwitch(value).then(() => {
                return persistSettings(nextSettings);
            }).catch(async err => {
                console.error("Kill switch error:", err);
                const rollbackSettings = { ...settings, [key]: previousValue };
                setSettings(rollbackSettings);
                await persistSettings(rollbackSettings).catch(console.error);
                showAlertDialog({
                    title: "Ошибка Kill Switch",
                    message: `Не удалось изменить состояние Kill Switch.\n\n${err}`,
                    variant: "danger",
                });
            });
        } else if (key === "adblock") {
            const nextSettings = { ...settings, [key]: value };
            setSettings(nextSettings);
            persistSettings(nextSettings).catch(console.error);
            wailsAPI.toggleAdBlock(value).catch(err => console.error("Ad block error:", err));
        } else if (key === "dnsServers") {
            const normalized = Array.isArray(value)
                ? value
                    .map((v) => String(v || "").trim())
                    .filter(Boolean)
                    .filter((v, idx, arr) => arr.indexOf(v) === idx)
                : [];
            const nextSettings = { ...settings, [key]: normalized };
            setSettings(nextSettings);
            try {
                await persistSettings(nextSettings);
                const status = await wailsAPI.getStatus();
                if (status?.currentProxy) {
                    await wailsAPI.applyMode(nextSettings.mode || "proxy");
                }
            } catch (err) {
                console.error("DNS settings error:", err);
                const rollbackSettings = { ...settings, [key]: previousValue };
                setSettings(rollbackSettings);
                await persistSettings(rollbackSettings).catch(console.error);
            }
        } else {
            const nextSettings = { ...settings, [key]: value };
            setSettings(nextSettings);
            persistSettings(nextSettings).catch(console.error);
        }
    }, [settings, addLog, showAlertDialog, persistSettings, t]);

    
    useEffect(() => {
        if (!isConfigLoaded || subscriptions.length === 0) return;

        const refreshAll = async () => {
            for (const sub of subscriptions) {
                try {
                    const updated = await wailsAPI.refreshSubscription(sub.id);
                    if (updated?.length) {
                        setProxies((prev) => {
                            const filtered = prev.filter((p) => p.subscriptionUrl !== sub.url);
                            const merged = mergeSubscriptionRefreshCountries(prev, updated, sub.url);
                            return [...filtered, ...merged];
                        });
                        addLog(`Подписка "${sub.name}" обновлена: ${updated.length} серверов`, "success");
                    }
                } catch (err) {
                    console.error("Subscription refresh error:", err);
                }
            }
            try {
                const cfg = await wailsAPI.getConfig();
                if (cfg?.subscriptions) setSubscriptions(cfg.subscriptions);
            } catch (e) {
                console.error("getConfig after subscription refresh:", e);
            }
        };

        const interval = setInterval(refreshAll, 6 * 60 * 60 * 1000);
        return () => clearInterval(interval);
    }, [isConfigLoaded, subscriptions, addLog]);

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

            const finalProxy = { ...proxyData, country: countryCode, port: parseInt(proxyData.port, 10) || 0, id: String(proxyData.id || Date.now()) };

            if (proxyData.id) {
                setProxies((prev) =>
                    prev.map((p) => (String(p.id) === finalProxy.id ? finalProxy : p))
                );
                if (String(failedProxy?.id) === finalProxy.id) setFailedProxy(null);
                addLog(`Профиль "${proxyData.name}" обновлен.`, "success");

                if (String(activeProxy?.id) === finalProxy.id) {
                    setActiveProxy(finalProxy);
                    if (isConnected) {
                        addLog("Применение новых настроек, переподключение...", "info");
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
                setProxies((prev) => [...prev, finalProxy]);
                addLog(`Новый профиль "${proxyData.name}" добавлен.`, "success");
                setActiveTab("list");
            }
            setEditingProxy(null);
        },
        [addLog]
    );

    const handleBulkSaveProxies = useCallback(
        async (proxiesData, setActiveTab, defaultProtocol) => {
            const VPN_TYPES = ["SS", "VMESS", "VLESS", "TROJAN", "WIREGUARD", "AMNEZIAWG", "HYSTERIA2"];
            const now = Date.now();
            const finalProxies = await Promise.all(
                proxiesData.map(async (p, index) => {
                    const countryCode = await detectCountry(p.ip);
                    const isVpn = VPN_TYPES.includes(p.type);
                    return {
                        ...p,
                        id: String(p.id || now + index),
                        country: p.country && p.country !== "unknown" && p.country !== "\u{1F310}" ? p.country : countryCode,
                        type: isVpn ? p.type : (defaultProtocol || p.type || "HTTP"),
                        port: parseInt(p.port, 10) || 0,
                    };
                })
            );

            setProxies((prev) => {
                const VPN_SET = new Set(["SS", "VMESS", "VLESS", "TROJAN", "WIREGUARD", "AMNEZIAWG", "HYSTERIA2"]);
                const newKeys = new Set(
                    finalProxies
                        .filter((p) => VPN_SET.has(p.type))
                        .map((p) => `${p.ip}:${p.port}:${p.type}`)
                );
                const filtered = prev.filter((p) => {
                    if (!VPN_SET.has(p.type)) return true;
                    return !newKeys.has(`${p.ip}:${p.port}:${p.type}`);
                });
                return [...filtered, ...finalProxies];
            });
            addLog(`Добавлено ${finalProxies.length} новых прокси.`, "success");
            setActiveTab("list");
        },
        [addLog]
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
        subscriptions,
        setSubscriptions,
        appDialog,
        showAlertDialog,
        showConfirmDialog,
        closeAppDialog,
        handleAppDialogConfirm,
    };
};
