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

import React, { useEffect, useState } from "react";
import {
  Environment,
  Quit,
  WindowIsMaximised,
  WindowMinimise,
  WindowToggleMaximise,
} from "../../../wailsjs/runtime/runtime";

const dragStyle = {
  "--wails-draggable": "drag",
  WebkitAppRegion: "drag",
};
const noDragStyle = {
  "--wails-draggable": "no-drag",
  WebkitAppRegion: "no-drag",
};

const ICON_SIZE = 12;
const STROKE_WIDTH = 2;
const iconStyle = {
  width: `${ICON_SIZE}px`,
  height: `${ICON_SIZE}px`,
  display: "block",
  flexShrink: 0,
};

const ControlButton = ({ onClick, danger, ariaLabel, children }) => (
  <button
    type="button"
    aria-label={ariaLabel}
    onClick={onClick}
    style={noDragStyle}
    className={`h-full w-[46px] flex items-center justify-center rounded-none border-0 bg-transparent p-0 text-zinc-300 transition-colors hover:border-transparent ${
      danger
        ? "hover:bg-[#c42b1c] hover:text-white"
        : "hover:bg-white/[0.06] active:bg-white/[0.04]"
    }`}
  >
    {children}
  </button>
);

export const TitleBar = () => {
  const [platform, setPlatform] = useState(null);
  const [isMaximised, setIsMaximised] = useState(false);

  useEffect(() => {
    let cancelled = false;
    Environment().then((env) => {
      if (!cancelled) setPlatform(env.platform);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (platform !== "windows") return undefined;
    let cancelled = false;
    const sync = async () => {
      const v = await WindowIsMaximised();
      if (!cancelled) setIsMaximised(v);
    };
    sync();
    const id = window.setInterval(sync, 500);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [platform]);

  if (platform !== "windows") return null;

  const handleDragDoubleClick = (e) => {
    if (e.target === e.currentTarget) {
      WindowToggleMaximise();
    }
  };

  return (
    <div
      style={dragStyle}
      onDoubleClick={handleDragDoubleClick}
      className="flex h-8 w-full shrink-0 items-stretch justify-end bg-zinc-900 select-none"
    >
      <ControlButton ariaLabel="Свернуть" onClick={() => WindowMinimise()}>
        <svg viewBox="0 0 22 22" fill="none" className="shrink-0" style={iconStyle}>
          <path d="M2 11h18" stroke="currentColor" strokeWidth={STROKE_WIDTH} />
        </svg>
      </ControlButton>

      <ControlButton
        ariaLabel={isMaximised ? "Восстановить" : "Развернуть"}
        onClick={() => WindowToggleMaximise()}
      >
        {isMaximised ? (
          <svg viewBox="0 0 22 22" fill="none" className="shrink-0" style={iconStyle}>
            <rect
              x="2.5"
              y="5.5"
              width="14"
              height="14"
              stroke="currentColor"
              strokeWidth={STROKE_WIDTH}
              fill="none"
            />
            <path
              d="M5.5 5.5V2.5h14v14h-3"
              stroke="currentColor"
              strokeWidth={STROKE_WIDTH}
              fill="none"
            />
          </svg>
        ) : (
          <svg viewBox="0 0 22 22" fill="none" className="shrink-0" style={iconStyle}>
            <rect
              x="2.5"
              y="2.5"
              width="17"
              height="17"
              stroke="currentColor"
              strokeWidth={STROKE_WIDTH}
              fill="none"
            />
          </svg>
        )}
      </ControlButton>

      <ControlButton danger ariaLabel="Закрыть" onClick={() => Quit()}>
        <svg viewBox="0 0 22 22" fill="none" className="shrink-0" style={iconStyle}>
          <path
            d="M2 2l18 18M20 2L2 20"
            stroke="currentColor"
            strokeWidth={STROKE_WIDTH}
          />
        </svg>
      </ControlButton>
    </div>
  );
};
