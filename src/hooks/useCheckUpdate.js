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

import { useState, useEffect } from "react";
import { compareVersions } from "../utils/versionCheck";
import { apiFetch } from "./useLogs";

// Замените на реальный URL вашего JSON-файла с информацией о версии
const UPDATE_URL =
  "https://raw.githubusercontent.com/AandStep/ResultProxy/main/update.json";

export const useCheckUpdate = () => {
  const [updateAvailable, setUpdateAvailable] = useState(false);
  const [latestVersionData, setLatestVersionData] = useState(null);
  const [currentVersion, setCurrentVersion] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const checkUpdate = async () => {
      try {
        setLoading(true);
        // 1. Получаем текущую версию из локального API Electron
        const localResponse = await apiFetch(`/api/version`);
        const localData = await localResponse.json();
        const localVersion = localData.version;
        setCurrentVersion(localVersion);

        // Добавляем timestamp для обхода жесткого кэша GitHub Raw (до 5 минут)
        const cacheBuster = `?_t=${Date.now()}`;
        const remoteResponse = await fetch(`${UPDATE_URL}${cacheBuster}`);
        const remoteData = await remoteResponse.json();

        setLatestVersionData(remoteData);

        // 3. Сравниваем
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
