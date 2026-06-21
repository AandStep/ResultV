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
import { useConnectionContext } from "../../context/ConnectionContext";
import logo from "../../assets/logo.png";

export const MobileHeader = () => {
  const { isConnected, isProxyDead } = useConnectionContext();

  return (
    <div className="md:hidden flex items-center justify-between p-4 bg-zinc-900 border-b border-zinc-800 sticky top-0 z-10">
      <div className="flex items-center space-x-2">
        <img
          src={logo}
          alt="ResultV"
          className="w-6 h-6 drop-shadow-[0_0_8px_rgba(0,126,58,0.5)]"
        />
        <span className="text-lg font-bold text-white">ResultV</span>
      </div>
      {isConnected && (
        <div
          className={`w-3 h-3 rounded-full animate-pulse ${
            isProxyDead ? "bg-rose-500" : "bg-[#007E3A]"
          }`}
        ></div>
      )}
    </div>
  );
};
