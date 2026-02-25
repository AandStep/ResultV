import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { DownloadCloud, X } from "lucide-react";

const UpdateNotificationModal = ({
  currentVersion,
  latestVersion,
  downloadUrl,
  onClose,
}) => {
  const { t } = useTranslation();

  if (!latestVersion) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-zinc-950 border border-zinc-800 rounded-2xl shadow-2xl max-w-sm w-full p-6 animate-fade-in-up">
        <div className="flex justify-between items-start mb-4">
          <div className="flex items-center gap-3 text-[#00A819]">
            <DownloadCloud size={24} />
            <h3 className="text-xl font-bold text-white">
              {t("update.title", "Доступно обновление")}
            </h3>
          </div>
          <button
            onClick={onClose}
            className="text-zinc-500 hover:text-[#00A819] transition-colors outline-none focus:outline-none focus:ring-0"
          >
            <X size={20} />
          </button>
        </div>

        <p className="text-zinc-400 mb-6 whitespace-pre-wrap">
          {t(
            "update.message",
            "У вас установлена версия {{current}}, доступна новая версия {{latest}}.",
            {
              current: currentVersion,
              latest: latestVersion,
            },
          )}
        </p>

        <div className="flex gap-3">
          <button
            onClick={onClose}
            className="flex-1 py-3 px-4 rounded-xl bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-[#00A819] transition-all text-sm font-bold outline-none focus:outline-none"
          >
            {t("update.later", "Позже")}
          </button>

          <button
            onClick={() => {
              if (downloadUrl) {
                window.open(downloadUrl, "_blank");
              } else {
                // Если есть уже модалка для выбора ОС, можно триггерить её
                document.dispatchEvent(new CustomEvent("open-download-modal"));
              }
              onClose();
            }}
            className="flex-1 py-3 px-4 rounded-xl bg-[#007E3A] hover:bg-[#00A819] text-white transition-all text-sm font-bold border-transparent outline-none focus:outline-none"
          >
            {t("update.download", "Обновить")}
          </button>
        </div>
      </div>
    </div>
  );
};

export default UpdateNotificationModal;
