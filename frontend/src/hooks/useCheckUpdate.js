/*
 * Copyright (C) 2026 ResultProxy
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

import { useState, useEffect } from "react";
import { compareVersions } from "../utils/versionCheck";
import { GetVersion } from "../../wailsjs/go/main/App";

const UPDATE_URL =
    "https://raw.githubusercontent.com/AandStep/ResultProxy/main/update.json";

/** Текущая версия: из Go (встроенный wails.json) в Wails; иначе __APP_VERSION__ из Vite. */
async function resolveLocalVersion() {
    try {
        if (typeof window !== "undefined" && window.go?.main?.App?.GetVersion) {
            const v = await GetVersion();
            if (v && String(v).trim()) {
                return String(v).trim();
            }
        }
    } catch {
        // npm run dev в браузере без Wails
    }
    if (typeof __APP_VERSION__ !== "undefined" && __APP_VERSION__) {
        return String(__APP_VERSION__).trim();
    }
    return "0.0.0";
}

export const useCheckUpdate = () => {
    const [updateAvailable, setUpdateAvailable] = useState(false);
    const [latestVersionData, setLatestVersionData] = useState(null);
    const [currentVersion, setCurrentVersion] = useState(() =>
        typeof __APP_VERSION__ !== "undefined" ? String(__APP_VERSION__) : "",
    );
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        const checkUpdate = async () => {
            try {
                setLoading(true);
                const localVersion = await resolveLocalVersion();
                setCurrentVersion(localVersion);

                const cacheBuster = `?_t=${Date.now()}`;
                const remoteResponse = await fetch(`${UPDATE_URL}${cacheBuster}`);
                const remoteData = await remoteResponse.json();

                setLatestVersionData(remoteData);

                if (localVersion && remoteData.version) {
                    const isNewer =
                        compareVersions(localVersion, remoteData.version) === -1;
                    setUpdateAvailable(isNewer);
                }
            } catch (error) {
                console.error("Ошибка проверки обновлений:", error);
            } finally {
                setLoading(false);
            }
        };

        checkUpdate();
    }, []);

    return { updateAvailable, latestVersionData, currentVersion, loading };
};
