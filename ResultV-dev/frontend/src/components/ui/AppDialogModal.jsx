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

import React from "react";
import { AlertTriangle, Info, X } from "lucide-react";
import { useTranslation } from "react-i18next";

const VARIANT_STYLES = {
  info: {
    icon: Info,
    iconClass: "text-[#00A819]",
    confirmClass:
      "bg-[#007E3A] hover:bg-[#00A819] text-white border-transparent",
  },
  warning: {
    icon: AlertTriangle,
    iconClass: "text-amber-400",
    confirmClass:
      "bg-amber-500/20 hover:bg-amber-500/30 text-amber-300 border-amber-500/40",
  },
  danger: {
    icon: AlertTriangle,
    iconClass: "text-rose-400",
    confirmClass:
      "bg-rose-600 hover:bg-rose-500 text-white border-rose-500",
  },
};

const AppDialogModal = ({
  isOpen = false,
  title = "",
  message = "",
  variant = "info",
  showCancel = false,
  confirmText,
  cancelText,
  onConfirm,
  onClose,
}) => {
  const { t } = useTranslation();

  if (!isOpen) return null;

  const safeVariant = VARIANT_STYLES[variant] ? variant : "info";
  const style = VARIANT_STYLES[safeVariant];
  const Icon = style.icon;

  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center p-4 bg-black/40">
      <div
        aria-hidden
        className="absolute inset-0"
        onClick={onClose}
      />

      <div className="relative bg-zinc-950 border border-zinc-800 rounded-3xl shadow-2xl max-w-md w-full p-6 animate-fade-in-up">
        <button
          type="button"
          onClick={onClose}
          className="absolute right-4 top-4 text-zinc-500 hover:text-zinc-200 transition-colors outline-none focus:outline-none focus:ring-0"
          aria-label={t("common.close", "Закрыть")}
        >
          <X size={20} />
        </button>

        <div className="flex items-center gap-3 mb-4 pr-8">
          <Icon size={22} className={style.iconClass} />
          <h3 className="text-xl font-bold text-white">
            {title || t("common.notice", "Уведомление")}
          </h3>
        </div>

        <p className="text-zinc-400 text-sm mb-6 whitespace-pre-wrap">
          {message}
        </p>

        <div className="flex gap-3">
          {showCancel && (
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-3 px-4 rounded-xl bg-zinc-900 border border-zinc-800 text-zinc-300 hover:text-white hover:border-zinc-700 transition-all font-bold outline-none focus:outline-none"
            >
              {cancelText || t("common.cancel", "Отмена")}
            </button>
          )}
          <button
            type="button"
            onClick={onConfirm}
            className={`py-3 px-4 rounded-xl border transition-all font-bold outline-none focus:outline-none ${showCancel ? "flex-1" : "w-full"} ${style.confirmClass}`}
          >
            {confirmText ||
              (showCancel
                ? t("common.confirm", "Подтвердить")
                : t("common.ok", "OK"))}
          </button>
        </div>
      </div>
    </div>
  );
};

export default AppDialogModal;
