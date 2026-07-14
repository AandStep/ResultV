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

import React, { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { DownloadCloud, X, RefreshCw, ExternalLink, CheckCircle, Loader } from "lucide-react";
import { wailsAPI } from "../../utils/wailsAPI";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";

// Hosts allowed to open in browser as fallback
const ALLOWED_DOWNLOAD_HOSTS = new Set([
  "result-proxy.ru",
  "www.result-proxy.ru",
  "github.com",
]);

function isSafeDownloadURL(raw) {
  if (!raw || typeof raw !== "string") return false;
  let u;
  try { u = new URL(raw); } catch { return false; }
  if (u.protocol !== "https:") return false;
  return ALLOWED_DOWNLOAD_HOSTS.has(u.hostname.toLowerCase());
}

function formatBytes(bytes) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
}

function formatSpeed(bps) {
  if (bps < 1024) return `${bps.toFixed(0)} Б/с`;
  if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(0)} КБ/с`;
  return `${(bps / (1024 * 1024)).toFixed(1)} МБ/с`;
}

/**
 * UpdaterModal — handles the full in-app update lifecycle:
 *   idle → downloading → verifying → installing → restarting
 *                                                ↘ failed
 *
 * Props:
 *   currentVersion  — installed version string
 *   latestVersion   — new version string
 *   downloadUrl     — browser fallback URL
 *   onClose         — dismiss callback (only available in idle / failed states)
 */
const UpdaterModal = ({ currentVersion, latestVersion, downloadUrl, onClose }) => {
  const { t } = useTranslation();
  const [phase, setPhase] = useState("idle"); // idle | downloading | verifying | installing | restarting | failed
  const [progress, setProgress] = useState({ downloaded: 0, total: 0, speedBps: 0 });
  const [errorInfo, setErrorInfo] = useState(null); // { stage, message }

  // Subscribe to Wails backend events
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
    const onVerified  = () => setPhase("verifying"); // brief, transitions to installing next
    const onInstalling = () => setPhase("installing");
    const onFailed = (data) => {
      setErrorInfo({ stage: data?.stage || "unknown", message: data?.message || "Unknown error" });
      setPhase("failed");
    };

    EventsOn("update:progress",   onProgress);
    EventsOn("update:verifying",  onVerifying);
    EventsOn("update:verified",   onVerified);
    EventsOn("update:installing", onInstalling);
    EventsOn("update:failed",     onFailed);

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
      document.dispatchEvent(new CustomEvent("open-download-modal"));
    }
    onClose();
  }, [downloadUrl, onClose]);

  const handleRetry = useCallback(() => {
    setPhase("idle");
    setErrorInfo(null);
  }, []);

  if (!latestVersion) return null;

  const pct = progress.total > 0 ? Math.round((progress.downloaded / progress.total) * 100) : 0;
  const canClose = phase === "idle" || phase === "failed";
  const canCancel = phase === "downloading";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-zinc-950 border border-zinc-800 rounded-3xl shadow-2xl max-w-sm w-full p-6 animate-fade-in-up">

        {/* Header */}
        <div className="flex justify-between items-start mb-4">
          <div className="flex items-center gap-3 text-[#00A819]">
            {phase === "installing" || phase === "restarting"
              ? <Loader size={24} className="animate-spin" />
              : <DownloadCloud size={24} />}
            <h3 className="text-xl font-bold text-white">
              {phase === "idle"    && t("update.title", "Доступно обновление")}
              {phase === "downloading" && t("update.downloading", "Скачивание...")}
              {phase === "verifying"   && t("update.verifying", "Проверка целостности...")}
              {phase === "installing"  && t("update.installing", "Установка...")}
              {phase === "restarting"  && t("update.restarting", "Перезапуск...")}
              {phase === "failed"      && t("update.failed", "Ошибка обновления")}
            </h3>
          </div>
          {canClose && (
            <button
              onClick={onClose}
              className="text-zinc-500 hover:text-[#00A819] transition-colors outline-none focus:outline-none focus:ring-0"
            >
              <X size={20} />
            </button>
          )}
        </div>

        {/* Body */}
        {phase === "idle" && (
          <p className="text-zinc-500 text-sm mb-6 whitespace-pre-wrap">
            {t("update.message", "У вас установлена версия {{current}}, доступна новая версия {{latest}}.", {
              current: currentVersion,
              latest: latestVersion,
            })}
          </p>
        )}

        {phase === "downloading" && (
          <div className="mb-6 space-y-3">
            <div className="flex justify-between text-xs text-zinc-400">
              <span>{formatBytes(progress.downloaded)} / {progress.total > 0 ? formatBytes(progress.total) : "…"}</span>
              <span>{formatSpeed(progress.speedBps)}</span>
            </div>
            <div className="w-full bg-zinc-800 rounded-full h-2">
              <div
                className="bg-[#00A819] h-2 rounded-full transition-all duration-300"
                style={{ width: `${pct}%` }}
              />
            </div>
            <p className="text-center text-zinc-500 text-xs">{pct}%</p>
          </div>
        )}

        {(phase === "verifying" || phase === "installing" || phase === "restarting") && (
          <div className="flex items-center gap-3 mb-6 text-zinc-400 text-sm">
            <Loader size={16} className="animate-spin text-[#00A819] shrink-0" />
            <span>
              {phase === "verifying"  && t("update.verifying_body", "Проверяем целостность загруженного файла...")}
              {phase === "installing" && t("update.installing_body", "Устанавливаем обновление, подождите...")}
              {phase === "restarting" && t("update.restarting_body", "Перезапуск приложения...")}
            </span>
          </div>
        )}

        {phase === "failed" && (
          <div className="mb-6 space-y-2">
            <p className="text-red-400 text-sm">
              {t("update.error_stage", "Этап: {{stage}}", { stage: errorInfo?.stage })}
            </p>
            <p className="text-zinc-400 text-xs break-words">{errorInfo?.message}</p>
          </div>
        )}

        {/* Footer buttons */}
        <div className="flex gap-3">
          {phase === "idle" && (
            <>
              <button
                onClick={onClose}
                className="flex-1 py-3 px-4 rounded-xl bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-[#00A819] transition-all font-bold outline-none focus:outline-none"
              >
                {t("update.later", "Позже")}
              </button>
              <button
                onClick={handleStartUpdate}
                className="flex-1 py-3 px-4 rounded-xl bg-[#007E3A] hover:bg-[#00A819] text-white transition-all font-bold border-transparent outline-none focus:outline-none"
              >
                {t("update.download", "Обновить")}
              </button>
            </>
          )}

          {canCancel && (
            <button
              onClick={handleCancel}
              className="flex-1 py-3 px-4 rounded-xl bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-red-500 transition-all font-bold outline-none focus:outline-none"
            >
              {t("update.cancel", "Отмена")}
            </button>
          )}

          {phase === "failed" && (
            <>
              <button
                onClick={handleRetry}
                className="flex-1 py-3 px-4 rounded-xl bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-[#00A819] transition-all font-bold outline-none focus:outline-none flex items-center justify-center gap-2"
              >
                <RefreshCw size={14} />
                {t("update.retry", "Повторить")}
              </button>
              <button
                onClick={handleBrowserFallback}
                className="flex-1 py-3 px-4 rounded-xl bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-zinc-500 transition-all font-bold outline-none focus:outline-none flex items-center justify-center gap-2"
              >
                <ExternalLink size={14} />
                {t("update.browser", "В браузере")}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
};

export default UpdaterModal;
