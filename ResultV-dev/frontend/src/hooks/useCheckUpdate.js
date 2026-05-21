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

import { useState, useEffect } from "react";
import { compareVersions } from "../utils/versionCheck";
import { GetVersion } from "../../wailsjs/go/main/App";

const UPDATE_URL =
    "https://raw.githubusercontent.com/AandStep/ResultV/main/update.json";


async function resolveLocalVersion() {
    try {
        if (typeof window !== "undefined" && window.go?.main?.App?.GetVersion) {
            const v = await GetVersion();
            if (v && String(v).trim()) {
                return String(v).trim();
            }
        }
    } catch {
        
    }
    if (typeof __APP_VERSION__ !== "undefined" && __APP_VERSION__) {
        return String(__APP_VERSION__).trim();
    }
    return "0.0.0";
}

async function fetchRemoteManifest() {
    if (
        typeof window !== "undefined" &&
        window.go?.main?.App?.GetUpdateManifest
    ) {
        try {
            const manifest = await window.go.main.App.GetUpdateManifest();
            if (manifest && typeof manifest === "object") {
                return manifest;
            }
        } catch {
            
        }
    }

    const cacheBuster = `?_t=${Date.now()}`;
    const remoteResponse = await fetch(`${UPDATE_URL}${cacheBuster}`);
    return remoteResponse.json();
}

export const useCheckUpdate = () => {
    const [updateAvailable, setUpdateAvailable] = useState(false);
    const [latestVersionData, setLatestVersionData] = useState(null);
    const [hasPlatformAsset, setHasPlatformAsset] = useState(false);
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

                const remoteData = await fetchRemoteManifest();

                setLatestVersionData(remoteData);

                // True when the manifest has at least one platform asset filled in.
                // The Go backend decides which specific asset to use at download time.
                const platformsPopulated =
                    remoteData.platforms != null &&
                    Object.values(remoteData.platforms).some((a) => a?.url && a?.sha256);
                setHasPlatformAsset(platformsPopulated);

                if (localVersion && remoteData.version) {
                    const isNewer = compareVersions(localVersion, remoteData.version) === -1;
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

    return { updateAvailable, latestVersionData, currentVersion, loading, hasPlatformAsset };
};
