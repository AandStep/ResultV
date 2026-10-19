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

/*
 * Тема интерфейса: "dark", "light" или "system".
 *
 * Источник правды — настройка `theme` в конфиге, но конфиг приходит позже
 * первого кадра. Поэтому выбор дублируется в localStorage и применяется до
 * рендера, иначе светлая тема каждый запуск начиналась бы со вспышки тёмной.
 */

const STORAGE_KEY = "rv-theme";
const systemLight = window.matchMedia?.("(prefers-color-scheme: light)");

let preference = "dark";

const normalize = (value) =>
  value === "light" || value === "system" ? value : "dark";

const resolve = (pref) =>
  pref === "system" ? (systemLight?.matches ? "light" : "dark") : pref;

function paintWindow(theme) {
  const rt = window.runtime;
  if (!rt) return;
  try {
    if (theme === "light") rt.WindowSetLightTheme?.();
    else rt.WindowSetDarkTheme?.();
    const hex = getComputedStyle(document.documentElement)
      .getPropertyValue("--rv-black")
      .trim();
    const m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex);
    if (m) {
      rt.WindowSetBackgroundColour?.(
        parseInt(m[1], 16),
        parseInt(m[2], 16),
        parseInt(m[3], 16),
        255,
      );
    }
  } catch {
    // Окно без рантайма Wails (витрина кита) — красить нечего.
  }
}

function paint() {
  const root = document.documentElement;
  const theme = resolve(preference);
  if (root.dataset.theme === theme) return;

  root.classList.add("rv-theme-switching");
  root.dataset.theme = theme;
  paintWindow(theme);
  // Переходы возвращаются, когда новые цвета уже отрисованы.
  requestAnimationFrame(() =>
    requestAnimationFrame(() => root.classList.remove("rv-theme-switching")),
  );
}

export function applyTheme(value) {
  preference = normalize(value);
  try {
    localStorage.setItem(STORAGE_KEY, preference);
  } catch {
    // Без хранилища тема всё равно применится, просто первый кадр будет тёмным.
  }
  paint();
}

export function initTheme() {
  let stored = null;
  try {
    stored = localStorage.getItem(STORAGE_KEY);
  } catch {
    stored = null;
  }
  const fromQuery = new URLSearchParams(window.location.search).get("theme");
  preference = normalize(fromQuery || stored);
  paint();
  systemLight?.addEventListener?.("change", () => {
    if (preference === "system") paint();
  });
}
