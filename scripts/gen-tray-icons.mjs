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

// Атласы картинок для пунктов меню трея: internal/system/trayicons.
//
// Флаги — тот же набор flag-icons 7.2.3, что рисует главное окно, значок
// авто-группы — mdiCreation, как у строки авто-сервера в окне, глобус — как
// у сервера без страны. Атлас на каждый размер SM_CXSMICON, клетки
// квадратные: флаг уже стоит в клетке по центру и со скруглёнными углами.
//
// Run:
//   npm pack flag-icons@7.2.3 && tar -xzf flag-icons-7.2.3.tgz
//   npm install @resvg/resvg-js@2.6.2
//   node scripts/gen-tray-icons.mjs <package/flags/4x3> <node_modules/@resvg/resvg-js>
import fs from "node:fs";
import path from "node:path";
import zlib from "node:zlib";
import { createRequire } from "node:module";
import { mdiCreation } from "../frontend/node_modules/@mdi/js/mdi.js";

const [flagsDir, resvgDir] = process.argv.slice(2);
if (!flagsDir || !resvgDir) {
  console.error("usage: node scripts/gen-tray-icons.mjs <flags/4x3 dir> <@resvg/resvg-js dir>");
  process.exit(1);
}
const { Resvg } = createRequire(import.meta.url)(path.resolve(resvgDir));

const SIZES = [16, 20, 24, 28, 32, 40, 48];
const COLUMNS = 16;
const OUT = path.join(path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1")), "..", "internal", "system", "trayicons");

const AUTO_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#00a819" d="${mdiCreation}"/></svg>`;
const GLOBE_SVG =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#9aa0a6" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
  '<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>';

const codes = fs
  .readdirSync(flagsDir)
  .map((f) => /^([a-z]{2})\.svg$/.exec(f)?.[1])
  .filter((c) => c && c !== "xx")
  .sort();
const cells = [
  { name: "auto", svg: AUTO_SVG, flag: false },
  { name: "globe", svg: GLOBE_SVG, flag: false },
  ...codes.map((c) => ({ name: c, svg: fs.readFileSync(path.join(flagsDir, `${c}.svg`), "utf8"), flag: true })),
];

function render(svg, width) {
  const img = new Resvg(svg, { fitTo: { mode: "width", value: width } }).render();
  return { w: img.width, h: img.height, px: img.pixels };
}

// Доля пикселя (x, y) внутри прямоугольника w×h со скруглением r, 4×4 выборки.
function roundedCoverage(x, y, w, h, r) {
  let inside = 0;
  for (let sy = 0; sy < 4; sy++) {
    for (let sx = 0; sx < 4; sx++) {
      const px = x + (sx + 0.5) / 4;
      const py = y + (sy + 0.5) / 4;
      const cx = Math.min(Math.max(px, r), w - r);
      const cy = Math.min(Math.max(py, r), h - r);
      if ((px - cx) ** 2 + (py - cy) ** 2 <= r * r) inside++;
    }
  }
  return inside / 16;
}

function crc32(buf) {
  return zlib.crc32(buf) >>> 0;
}

function chunk(type, data) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body));
  return Buffer.concat([len, body, crc]);
}

// Пиксели premultiplied (так отдаёт resvg) -> PNG с обычной альфой.
function encodePNG(w, h, premul) {
  const raw = Buffer.alloc((w * 4 + 1) * h);
  for (let y = 0; y < h; y++) {
    raw[y * (w * 4 + 1)] = 0;
    for (let x = 0; x < w; x++) {
      const s = (y * w + x) * 4;
      const d = y * (w * 4 + 1) + 1 + x * 4;
      const a = premul[s + 3];
      for (let c = 0; c < 3; c++) raw[d + c] = a ? Math.min(255, Math.round((premul[s + c] * 255) / a)) : 0;
      raw[d + 3] = a;
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", ihdr),
    chunk("IDAT", zlib.deflateSync(raw, { level: 9 })),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

fs.mkdirSync(OUT, { recursive: true });
fs.writeFileSync(path.join(OUT, "atlas.txt"), cells.map((c) => c.name).join("\n") + "\n");

for (const size of SIZES) {
  const rows = Math.ceil(cells.length / COLUMNS);
  const aw = COLUMNS * size;
  const ah = rows * size;
  const atlas = Buffer.alloc(aw * ah * 4);

  cells.forEach((cell, i) => {
    const img = render(cell.svg, size);
    const ox = (i % COLUMNS) * size;
    const oy = Math.floor(i / COLUMNS) * size + Math.floor((size - img.h) / 2);
    const radius = size / 12;
    for (let y = 0; y < img.h; y++) {
      for (let x = 0; x < img.w; x++) {
        const k = cell.flag ? roundedCoverage(x, y, img.w, img.h, radius) : 1;
        const s = (y * img.w + x) * 4;
        const d = ((oy + y) * aw + ox + x) * 4;
        for (let c = 0; c < 4; c++) atlas[d + c] = Math.round(img.px[s + c] * k);
      }
    }
  });

  const file = path.join(OUT, `atlas_${size}.png`);
  fs.writeFileSync(file, encodePNG(aw, ah, atlas));
  console.log(`${path.basename(file)}: ${cells.length} cells, ${fs.statSync(file).size} bytes`);
}
