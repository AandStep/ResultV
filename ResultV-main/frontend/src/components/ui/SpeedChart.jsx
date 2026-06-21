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

export const SpeedChart = ({ data, color, fillHeight = true }) => {
  if (!data || data.length < 2) return null;
  const max = Math.max(...data, 1024);
  const points = data
    .map(
      (val, i) => `${(i / (data.length - 1)) * 100},${22 - (val / max) * 22}`,
    )
    .join(" ");

  if (!fillHeight) {
    return (
      <div className="mt-3 w-full shrink-0">
        <svg
          viewBox="0 0 100 26"
          preserveAspectRatio="none"
          className="block h-7 w-full overflow-visible opacity-90"
        >
          <polyline
            fill="none"
            stroke={color}
            strokeWidth="2.25"
            strokeLinecap="round"
            strokeLinejoin="round"
            points={points}
            vectorEffect="non-scaling-stroke"
            className="transition-all duration-500"
          />
        </svg>
      </div>
    );
  }

  return (
    <div className="mt-3 flex w-full min-h-0 flex-1 flex-col">
      <svg
        viewBox="0 0 100 26"
        preserveAspectRatio="none"
        className="block h-full w-full min-h-[3rem] flex-1 overflow-visible opacity-90"
      >
        <polyline
          fill="none"
          stroke={color}
          strokeWidth="2.25"
          strokeLinecap="round"
          strokeLinejoin="round"
          points={points}
          vectorEffect="non-scaling-stroke"
          className="transition-all duration-500"
        />
      </svg>
    </div>
  );
};
