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
import { useTranslation } from "react-i18next";
import { FlagIcon } from "./FlagIcon";

export const LanguageSwitcher = () => {
  const { i18n } = useTranslation();

  const toggleLanguage = () => {
    
    const nextLang = i18n.language?.startsWith("ru") ? "en" : "ru";
    i18n.changeLanguage(nextLang);
  };

  const isRu = i18n.language?.startsWith("ru");

  return (
    <button
      onClick={toggleLanguage}
      className="flex items-center justify-center p-1.5 rounded-xl hover:bg-zinc-800 transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none shrink-0"
      title={isRu ? "Switch to English" : "Переключить на русский"}
    >
      <FlagIcon
        code={isRu ? "RU" : "US"}
        className="h-4 w-auto rounded-[2px] shadow-sm object-contain"
      />
    </button>
  );
};
