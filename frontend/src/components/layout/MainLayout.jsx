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

import { TitleBar } from "./TitleBar";

/*
 * Оболочка окна. Страницы нового дизайна рисуют себя целиком — свой сайдбар,
 * свои отступы и свою прокрутку, — поэтому здесь остаётся ровно то, что
 * общее для всех: сброс браузерных умолчаний и заголовок окна с его
 * кнопками, которого в макете нет.
 */
export const MainLayout = ({ children }) => {
  const resetCss = `
        * {
          outline: none !important;
          -webkit-tap-highlight-color: transparent !important;
        }
        button { border-color: transparent; }
        button:hover, a:hover {
          border-color: transparent;
        }
        button:focus, input:focus, a:focus {
          outline: none !important;
          box-shadow: none !important;
        }
        :root { --bs-primary: transparent; }
        .scrollbar-hide::-webkit-scrollbar { display: none; }
  `;

  return (
    <div className="fixed inset-0 overflow-hidden select-none">
      <style>{resetCss}</style>
      <TitleBar overlay />
      {children}
    </div>
  );
};
