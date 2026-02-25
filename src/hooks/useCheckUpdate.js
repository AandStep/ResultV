import { useState, useEffect } from "react";
import { compareVersions } from "../utils/versionCheck";
import { DAEMON_URL } from "./useLogs";

// Замените на реальный URL вашего JSON-файла с информацией о версии
const UPDATE_URL =
  "https://raw.githubusercontent.com/AandStep/ResultProxy/dev/update.json";

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
        const localResponse = await fetch(`${DAEMON_URL}/api/version`);
        const localData = await localResponse.json();
        const localVersion = localData.version;
        setCurrentVersion(localVersion);

        const remoteResponse = await fetch(UPDATE_URL);
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
