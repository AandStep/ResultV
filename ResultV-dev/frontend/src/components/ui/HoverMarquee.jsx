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

import React, { useRef, useState, useEffect } from "react";

export const HoverMarquee = ({ text, className = "" }) => {
  const containerRef = useRef(null);
  const textRef = useRef(null);
  const [overflow, setOverflow] = useState(0);

  const checkOverflow = () => {
    if (containerRef.current && textRef.current) {
      const scrollWidth = textRef.current.scrollWidth;
      const clientWidth = containerRef.current.clientWidth;
      setOverflow(Math.max(0, scrollWidth - clientWidth));
    }
  };

  useEffect(() => {
    checkOverflow();
    window.addEventListener("resize", checkOverflow);
    return () => window.removeEventListener("resize", checkOverflow);
  }, [text]);

  const handleMouseEnter = () => {
    checkOverflow();
  };

  return (
    <div
      ref={containerRef}
      onMouseEnter={handleMouseEnter}
      className={`overflow-hidden whitespace-nowrap w-full ${className}`}
    >
      <div
        ref={textRef}
        className={
          overflow > 0
            ? "block w-full truncate group-hover/card:inline-block group-hover/card:w-max group-hover/card:overflow-visible group-hover/card:text-clip group-hover/card:animate-marquee"
            : "block w-full truncate"
        }
        style={overflow > 0 ? { "--scroll-amount": `-${overflow}px` } : {}}
      >
        {text}
      </div>
    </div>
  );
};
