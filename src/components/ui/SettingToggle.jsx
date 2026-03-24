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

import React from "react";

export const SettingToggle = ({ title, description, isOn, onToggle }) => {
  return (
    <div
      className="flex items-center justify-between p-6 bg-zinc-900 rounded-3xl border border-zinc-800 cursor-pointer hover:border-zinc-700 transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
      onClick={onToggle}
    >
      <div className="pr-6">
        <h4 className="text-white font-bold text-lg">{title}</h4>
        <p className="text-zinc-500 text-sm mt-1">{description}</p>
      </div>
      <div
        className={`relative w-14 h-7 rounded-full transition-colors duration-300 ease-in-out shrink-0 ${isOn ? "bg-[#007E3A]" : "bg-zinc-700"}`}
      >
        <div
          className={`absolute top-1 left-1 bg-white w-5 h-5 rounded-full transition-transform duration-300 ease-in-out ${isOn ? "transform translate-x-7" : ""}`}
        ></div>
      </div>
    </div>
  );
};
