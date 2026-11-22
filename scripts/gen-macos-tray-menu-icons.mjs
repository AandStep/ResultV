// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Render UI-kit icons as Retina template images for the native macOS menu.
// Usage: node scripts/gen-macos-tray-menu-icons.mjs <path-to-@resvg/resvg-js>
import { readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";
import { ICONS } from "../frontend/src/components/kit/icons.js";

const [rendererPath] = process.argv.slice(2);
if (!rendererPath) {
  throw new Error("Pass the path to @resvg/resvg-js.");
}
const { Resvg } = createRequire(import.meta.url)(resolve(rendererPath));

for (const name of ["power", "list", "home", "close"]) {
  const { path, offset: [x, y] = [0, 0] } = ICONS[name];
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="${x} ${y} 24 24"><path d="${path}"/></svg>`;
  const output = new URL(`../internal/system/trayicons/menu_${name}.png`, import.meta.url);
  writeFileSync(output, new Resvg(svg).render().asPng());
}

// Cut the mark out of the shield so AppKit can tint it as a template image.
const logo = readFileSync(new URL("../public/logo.svg", import.meta.url), "utf8");
const logoPaths = Array.from(logo.matchAll(/<path d="([^"]+)"/g), ([, path]) => path);
if (logoPaths.length !== 3) {
  throw new Error("Expected the ResultV shield, letter and dot.");
}
const trayLogo = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="-101 0 1008 1008"><path fill-rule="evenodd" d="${logoPaths.join(" ")}"/></svg>`;
writeFileSync(new URL("../public/mac-tray-logo.png", import.meta.url), new Resvg(trayLogo).render().asPng());
