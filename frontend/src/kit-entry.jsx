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
 * Отдельная точка входа для витрины кита: kit.html.
 * Приложение её не подключает, в основной бандл она не попадает.
 */

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import ShowcaseApp from "./views/ShowcaseApp.jsx";

createRoot(document.getElementById("root")).render(
  <StrictMode>
    <ShowcaseApp />
  </StrictMode>
);
