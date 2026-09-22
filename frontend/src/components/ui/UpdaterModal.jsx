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

import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Badge, Button, Dialog, Icon } from "../kit";
import { wailsAPI } from "../../utils/wailsAPI";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import "./UpdaterModal.css";

/*
 * update.json приезжает с raw.githubusercontent.com, поэтому подменённая
 * ветка или MITM с чужим корневым сертификатом могут вписать в downloadUrl
 * что угодно. Список хостов ограничивает последствия: адрес вне его никуда
 * не ведёт.
 */
const ALLOWED_DOWNLOAD_HOSTS = new Set([
  "result-proxy.ru",
  "www.result-proxy.ru",
  "github.com",
]);

function isSafeDownloadURL(raw) {
  if (!raw || typeof raw !== "string") return false;
  let u;
  try {
    u = new URL(raw);
  } catch {
    return false;
  }
  if (u.protocol !== "https:") return false;
  return ALLOWED_DOWNLOAD_HOSTS.has(u.hostname.toLowerCase());
}

/* Фазы, на которых окно ничем не занято и его можно закрыть. */
const IDLE_PHASES = new Set(["idle", "failed"]);

/* Фазы ожидания: значок окна дышит, кнопок нет. */
const BUSY_PHASES = new Set(["verifying", "installing", "restarting"]);

const PHASE_TITLES = {
  idle: ["update.title", "Доступно обновление"],
  downloading: ["update.downloading", "Скачивание..."],
  verifying: ["update.verifying", "Проверка целостности..."],
  installing: ["update.installing", "Установка..."],
  restarting: ["update.restarting", "Перезапуск..."],
  failed: ["update.failed", "Ошибка обновления"],
};

const BUSY_BODIES = {
  verifying: ["update.verifying_body", "Проверяем целостность загруженного файла..."],
  installing: ["update.installing_body", "Устанавливаем обновление, подождите..."],
  restarting: ["update.restarting_body", "Перезапуск приложения..."],
};

/**
 * UpdaterModal — обновление целиком внутри приложения:
 *   idle → downloading → verifying → installing → restarting
 *                                              ↘ failed
 *
 * Props:
 *   currentVersion — установленная версия
 *   latestVersion  — версия из манифеста
 *   downloadUrl    — запасной адрес для браузера
 *   onClose        — закрытие; доступно только в idle и failed
 */
const UpdaterModal = ({ currentVersion, latestVersion, downloadUrl, onClose }) => {
  const { t } = useTranslation();
  const [phase, setPhase] = useState("idle");
  const [progress, setProgress] = useState({ downloaded: 0, total: 0, speedBps: 0 });
  const [errorInfo, setErrorInfo] = useState(null);

  const formatBytes = useCallback(
    (bytes) =>
      bytes < 1024 * 1024
        ? `${(bytes / 1024).toFixed(1)} ${t("units.kb", "кб")}`
        : `${(bytes / (1024 * 1024)).toFixed(1)} ${t("units.mb", "Мб")}`,
    [t],
  );

  const formatSpeed = useCallback(
    (bps) =>
      bps < 1024 * 1024
        ? `${(bps / 1024).toFixed(1)} ${t("units.kbps", "кб/с")}`
        : `${(bps / (1024 * 1024)).toFixed(1)} ${t("units.mbps", "Мб/с")}`,
    [t],
  );

  useEffect(() => {
    const onProgress = (data) => {
      setPhase("downloading");
      setProgress({
        downloaded: data.downloaded || 0,
        total: data.total || 0,
        speedBps: data.speedBps || 0,
      });
    };
    const onVerifying = () => setPhase("verifying");
    const onInstalling = () => setPhase("installing");
    const onFailed = (data) => {
      setErrorInfo({
        stage: data?.stage || "unknown",
        message: data?.message || "Unknown error",
      });
      setPhase("failed");
    };

    EventsOn("update:progress", onProgress);
    EventsOn("update:verifying", onVerifying);
    EventsOn("update:verified", onVerifying);
    EventsOn("update:installing", onInstalling);
    EventsOn("update:failed", onFailed);

    return () => {
      EventsOff("update:progress");
      EventsOff("update:verifying");
      EventsOff("update:verified");
      EventsOff("update:installing");
      EventsOff("update:failed");
    };
  }, []);

  const handleStartUpdate = useCallback(async () => {
    setPhase("downloading");
    setProgress({ downloaded: 0, total: 0, speedBps: 0 });
    setErrorInfo(null);
    try {
      await wailsAPI.startUpdate();
    } catch (e) {
      setErrorInfo({ stage: "start", message: String(e) });
      setPhase("failed");
    }
  }, []);

  const handleCancel = useCallback(async () => {
    await wailsAPI.cancelUpdate();
    setPhase("idle");
  }, []);

  const handleBrowserFallback = useCallback(() => {
    if (isSafeDownloadURL(downloadUrl)) {
      window.open(downloadUrl, "_blank", "noopener,noreferrer");
    } else {
      if (downloadUrl) {
        console.warn("Update downloadUrl rejected by host allow list:", downloadUrl);
      }
      document.dispatchEvent(new CustomEvent("open-download-modal"));
    }
    onClose();
  }, [downloadUrl, onClose]);

  const handleRetry = useCallback(() => {
    setPhase("idle");
    setErrorInfo(null);
  }, []);

  if (!latestVersion) return null;

  const pct =
    progress.total > 0 ? Math.round((progress.downloaded / progress.total) * 100) : 0;
  const failed = phase === "failed";
  const canClose = IDLE_PHASES.has(phase);
  const busy = BUSY_PHASES.has(phase);
  const [titleKey, titleFallback] = PHASE_TITLES[phase] ?? PHASE_TITLES.idle;

  /* Ожидание проходит без кнопок: прервать проверку и установку нельзя. */
  let actions = null;
  if (phase === "idle") {
    actions = (
      <>
        <Button onClick={onClose}>{t("update.later", "Позже")}</Button>
        <Button variant="green" onClick={handleStartUpdate}>
          {t("update.download", "Обновить")}
        </Button>
      </>
    );
  } else if (phase === "downloading") {
    actions = (
      <Button variant="red" onClick={handleCancel}>
        {t("update.cancel", "Отмена")}
      </Button>
    );
  } else if (failed) {
    actions = (
      <>
        <Button
          icon={<Icon name="sync" color="currentColor" />}
          onClick={handleRetry}
        >
          {t("update.retry", "Повторить")}
        </Button>
        <Button
          icon={<Icon name="externallink" color="currentColor" />}
          onClick={handleBrowserFallback}
        >
          {t("update.browser", "В браузере")}
        </Button>
      </>
    );
  }

  return (
    <Dialog
      className={busy ? "rv-dialog--busy" : ""}
      variant={failed ? "error" : "success"}
      icon={failed ? "alert" : "import"}
      title={t(titleKey, titleFallback)}
      subtitle={<Badge color={failed ? "error" : "success"}>{latestVersion}</Badge>}
      onClose={canClose ? onClose : undefined}
      actions={actions}
    >
      {phase === "idle" && (
        <p className="rv-dialog__text">
          {t(
            "update.message",
            "У вас установлена версия {{current}}, доступна новая версия {{latest}}.",
            { current: currentVersion, latest: latestVersion },
          )}
        </p>
      )}

      {phase === "downloading" && (
        <div className="rv-updater__progress">
          <div className="rv-updater__figures">
            <span>
              {formatBytes(progress.downloaded)} /{" "}
              {progress.total > 0 ? formatBytes(progress.total) : "…"}
            </span>
            <span>{formatSpeed(progress.speedBps)}</span>
          </div>
          <div
            className="rv-updater__track"
            role="progressbar"
            aria-valuenow={pct}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div className="rv-updater__fill" style={{ width: `${pct}%` }} />
          </div>
          <p className="rv-updater__percent">{pct}%</p>
        </div>
      )}

      {busy && (
        <p className="rv-dialog__text">
          {t(BUSY_BODIES[phase][0], BUSY_BODIES[phase][1])}
        </p>
      )}

      {failed && (
        <div className="rv-updater__failure">
          <p className="rv-updater__stage">
            {t("update.error_stage", "Этап: {{stage}}", { stage: errorInfo?.stage })}
          </p>
          <p className="rv-updater__reason">{errorInfo?.message}</p>
        </div>
      )}
    </Dialog>
  );
};

export default UpdaterModal;
