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

import React, { useEffect, useId, useMemo, useRef, useState } from "react";
import { ChevronDown } from "lucide-react";

const AppSelect = ({
  value,
  options,
  onChange,
  placeholder = "",
  disabled = false,
  className = "",
  buttonClassName = "",
  listClassName = "",
  optionClassName = "",
  align = "right",
  ariaLabel,
}) => {
  const rootRef = useRef(null);
  const buttonRef = useRef(null);
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const listId = useId();

  const normalizedOptions = useMemo(
    () =>
      Array.isArray(options)
        ? options.map((opt) =>
            typeof opt === "string" ? { value: opt, label: opt } : opt,
          )
        : [],
    [options],
  );

  const selectedIndex = useMemo(
    () => normalizedOptions.findIndex((opt) => opt.value === value),
    [normalizedOptions, value],
  );

  const selectedLabel =
    selectedIndex >= 0
      ? normalizedOptions[selectedIndex]?.label
      : placeholder || "";

  useEffect(() => {
    if (!isOpen) return;
    const handlePointerDown = (event) => {
      if (rootRef.current && !rootRef.current.contains(event.target)) {
        setIsOpen(false);
      }
    };
    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    if (selectedIndex >= 0) {
      setActiveIndex(selectedIndex);
      return;
    }
    setActiveIndex(normalizedOptions.length > 0 ? 0 : -1);
  }, [isOpen, selectedIndex, normalizedOptions.length]);

  const openMenu = () => {
    if (disabled || normalizedOptions.length === 0) return;
    setIsOpen(true);
  };

  const closeMenu = () => setIsOpen(false);

  const toggleMenu = () => {
    if (disabled || normalizedOptions.length === 0) return;
    setIsOpen((prev) => !prev);
  };

  const selectByIndex = (index) => {
    if (index < 0 || index >= normalizedOptions.length) return;
    const next = normalizedOptions[index];
    if (next?.value !== value) onChange?.(next.value);
    closeMenu();
    buttonRef.current?.focus();
  };

  const handleButtonKeyDown = (event) => {
    if (disabled) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (!isOpen) {
        openMenu();
        return;
      }
      setActiveIndex((prev) =>
        prev < 0 ? 0 : Math.min(prev + 1, normalizedOptions.length - 1),
      );
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      if (!isOpen) {
        openMenu();
        return;
      }
      setActiveIndex((prev) => (prev <= 0 ? 0 : prev - 1));
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (!isOpen) {
        openMenu();
        return;
      }
      if (activeIndex >= 0) selectByIndex(activeIndex);
      return;
    }
    if (event.key === "Escape") {
      if (isOpen) {
        event.preventDefault();
        closeMenu();
      }
    }
  };

  const handleListKeyDown = (event) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((prev) =>
        prev < 0 ? 0 : Math.min(prev + 1, normalizedOptions.length - 1),
      );
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((prev) => (prev <= 0 ? 0 : prev - 1));
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      if (activeIndex >= 0) selectByIndex(activeIndex);
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      closeMenu();
      buttonRef.current?.focus();
    }
  };

  return (
    <div ref={rootRef} className={`relative ${className}`}>
      <button
        ref={buttonRef}
        type="button"
        disabled={disabled}
        onClick={toggleMenu}
        onKeyDown={handleButtonKeyDown}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-controls={listId}
        aria-label={ariaLabel}
        className={`group w-full inline-flex items-center justify-between gap-2 rounded-xl border border-zinc-800 bg-zinc-900 px-3 py-2 text-sm text-white transition-colors hover:border-zinc-700 disabled:opacity-50 disabled:cursor-not-allowed outline-none focus:outline-none focus:ring-2 focus:ring-[#00A819]/35 ${buttonClassName}`}
      >
        <span className="truncate">{selectedLabel}</span>
        <ChevronDown
          className={`h-4 w-4 shrink-0 text-zinc-400 transition-transform ${isOpen ? "rotate-180" : ""}`}
        />
      </button>

      {isOpen && (
        <div
          className={`absolute z-30 mt-2 min-w-full overflow-hidden rounded-xl border border-zinc-700/70 bg-zinc-900 shadow-2xl ${align === "right" ? "right-0" : "left-0"} ${listClassName}`}
        >
          <ul
            id={listId}
            role="listbox"
            tabIndex={-1}
            onKeyDown={handleListKeyDown}
            aria-activedescendant={
              activeIndex >= 0 ? `${listId}-option-${activeIndex}` : undefined
            }
            className="max-h-64 overflow-auto py-1 outline-none"
          >
            {normalizedOptions.map((opt, index) => {
              const selected = opt.value === value;
              const active = index === activeIndex;
              return (
                <li key={opt.value} role="none">
                  <button
                    id={`${listId}-option-${index}`}
                    type="button"
                    role="option"
                    aria-selected={selected}
                    className={`w-full px-3 py-2 text-left text-sm transition-colors ${selected ? "bg-[#00A819]/15 text-[#00A819]" : active ? "bg-zinc-800 text-white" : "text-zinc-200 hover:bg-zinc-800 hover:text-white"} ${optionClassName}`}
                    onMouseEnter={() => setActiveIndex(index)}
                    onClick={() => selectByIndex(index)}
                  >
                    {opt.label}
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
};

export default AppSelect;
