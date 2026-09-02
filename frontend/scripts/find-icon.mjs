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
 * Ищет, какой иконке из @mdi/js соответствует SVG, отданный Figma.
 *
 * Сравнивать пути как строки бесполезно: Figma при экспорте разворачивает
 * дуги (A) в кубические кривые (C). Поэтому сравнивается форма — точки на
 * равных долях длины контура и симметричное chamfer-расстояние.
 *
 * Запуск:
 *   node scripts/find-icon.mjs <url-или-хеш-ассета> [ещё...]
 * Хеш берётся из ответа get_design_context:
 *   const imgIcons = "http://localhost:3845/assets/<хеш>.svg"
 */

import * as mdi from '@mdi/js';
import { svgPathProperties } from 'svg-path-properties';

const SAMPLES = 300;

function sample(d, size = 24) {
  const p = new svgPathProperties(d);
  const len = p.getTotalLength();
  const k = 24 / size;
  const out = [];
  for (let i = 0; i < SAMPLES; i++) {
    const q = p.getPointAtLength((len * i) / SAMPLES);
    out.push([q.x * k, q.y * k]);
  }
  return out;
}

function chamfer(a, b) {
  const oneWay = (from, to) => {
    let sum = 0;
    for (const p of from) {
      let min = Infinity;
      for (const q of to) {
        const d = (p[0] - q[0]) ** 2 + (p[1] - q[1]) ** 2;
        if (d < min) min = d;
      }
      sum += Math.sqrt(min);
    }
    return sum / from.length;
  };
  return (oneWay(a, b) + oneWay(b, a)) / 2;
}

const library = [];
for (const [name, path] of Object.entries(mdi)) {
  if (!name.startsWith('mdi') || typeof path !== 'string') continue;
  try { library.push([name, sample(path)]); } catch {}
}

const args = process.argv.slice(2);
if (!args.length) {
  console.error('нужен хотя бы один хеш или URL ассета');
  process.exit(2);
}

for (const arg of args) {
  const url = arg.startsWith('http') ? arg : `http://localhost:3845/assets/${arg}.svg`;
  let svg;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    svg = await res.text();
  } catch (err) {
    console.log(`${arg}\n  не скачался: ${err.message} (запущен ли Figma?)\n`);
    continue;
  }

  const size = Number((svg.match(/width="(\d+)"/) || [, 24])[1]);
  const paths = [...svg.matchAll(/<path[^>]*\sd="([^"]+)"/g)].map((m) => m[1]);
  const fills = [...svg.matchAll(/<path[^>]*\sfill="([^"]+)"/g)].map((m) => m[1]);
  const opacity = (svg.match(/fill-opacity="([^"]+)"/) || [, '1'])[1];
  if (!paths.length) { console.log(`${arg}\n  в SVG нет путей\n`); continue; }

  const mine = sample(paths.join(' '), size);
  const top = library
    .map(([name, pts]) => [name, chamfer(mine, pts)])
    .sort((a, b) => a[1] - b[1])
    .slice(0, 3);

  console.log(`${arg}  (${size}px, заливка ${fills.join(',')} @ ${opacity})`);
  for (const [name, d] of top) {
    const verdict = d < 0.06 ? 'совпало' : d < 0.6 ? 'похоже' : 'нет';
    console.log(`  ${d.toFixed(4)}  ${name.padEnd(28)} ${verdict}`);
  }
  console.log();
}
