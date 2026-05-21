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
import { Info } from "lucide-react";
import { useTranslation } from "react-i18next";

const ProtocolWarningModal = ({ isOpen, onClose }) => {
  const { t } = useTranslation();

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center p-4 animate-in fade-in duration-300">
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="relative bg-zinc-900 border border-zinc-800 w-full max-w-md p-6 rounded-3xl shadow-2xl animate-in zoom-in-95 duration-300 flex flex-col items-center text-center space-y-6">
        <div className="w-16 h-16 bg-[#007E3A]/10 rounded-full flex items-center justify-center">
          <Info className="w-8 h-8 text-[#007E3A]" />
        </div>
        <div className="space-y-2">
          <h3 className="text-xl font-bold text-white">
            {t("add.protocolWarningTitle") || "Важное уточнение"}
          </h3>
          <p className="text-zinc-400 text-sm leading-relaxed">
            {t("add.protocolWarning")}
          </p>
        </div>
        <button
          onClick={onClose}
          className="w-full bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-4 rounded-2xl transition-all shadow-lg shadow-[#007E3A]/20"
        >
          {t("add.gotIt") || "Понятно"}
        </button>
      </div>
    </div>
  );
};

export default ProtocolWarningModal;
