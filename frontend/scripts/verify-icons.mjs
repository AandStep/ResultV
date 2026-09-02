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
 * Доказательство того, что иконки из @mdi/js совпадают с макетом.
 *
 * Побайтово сравнивать пути нельзя: Figma при экспорте разворачивает дуги (A)
 * в кубические кривые (C), поэтому одна и та же иконка записана по-разному.
 * Сравниваем геометрию: берём точки на равных долях длины контура и считаем
 * симметричное chamfer-расстояние — оно не зависит ни от направления обхода,
 * ни от того, с какой вершины путь начался.
 *
 * Запуск: npm run verify:icons
 */

import { svgPathProperties } from 'svg-path-properties';
import { ICONS as LIB } from '../src/components/kit/icons.js';
import { ICONS as FIG } from '../src/components/kit/icons.figma.js';

const SAMPLES = 600;
// Порог. Расхождение выше — повод разбираться, ниже — погрешность
// пересчёта дуг в безье на стороне Figma (см. комментарий в icons.js).
const TOLERANCE = 0.06;

function sample(d, size = 24, dx = 0, dy = 0, flipY = false) {
  const p = new svgPathProperties(d);
  const len = p.getTotalLength();
  const k = 24 / size;
  const out = [];
  for (let i = 0; i < SAMPLES; i++) {
    const q = p.getPointAtLength((len * i) / SAMPLES);
    const y = q.y * k + dy;
    // Часть иконок отражена по вертикали внутри компонента Figma, а экспорт
    // отдаёт путь без отражения. Отражаем эталон, чтобы сравнивать одно и то же.
    out.push([q.x * k + dx, flipY ? 24 - y : y]);
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

let failed = 0;
let checked = 0;
const skipped = [];
const pending = [];

for (const [name, lib] of Object.entries(LIB)) {
  const fig = FIG[name];
  if (!fig) {
    // Эталон ещё не снят с макета, но соответствие доказано вручную
    // (`npm run find:icon`). Это долг, а не расхождение.
    if (lib.reference === 'pending') {
      pending.push(name);
      continue;
    }
    console.error(`  ${name}: нет эталона в icons.figma.js`);
    failed++;
    continue;
  }
  // Логотип Telegram взят из самого макета — сверять его не с чем.
  if (lib.source === 'figma') {
    skipped.push(name);
    continue;
  }
  const [dx, dy] = lib.offset ?? [0, 0];
  for (const state of lib.states) {
    const libPath = typeof lib.path === 'string' ? lib.path : lib.path[state];
    const figPath = typeof fig.d === 'string' ? fig.d : fig.d[state];
    // Состояния, отличающиеся только цветом, делят одну геометрию.
    if (!libPath || !figPath) continue;
    if (typeof lib.path === 'string' && state !== lib.states[0]) continue;

    const d = chamfer(sample(figPath, fig.size, dx, dy, lib.flipY), sample(libPath));
    checked++;
    const ok = d <= TOLERANCE;
    if (!ok) failed++;
    const label = typeof lib.path === 'string' ? name : `${name}/${state}`;
    console.log(`  ${ok ? 'OK  ' : 'FAIL'} ${label.padEnd(18)} ${d.toFixed(5)} px`);
  }
}

console.log(`\nсверено путей: ${checked}, расхождений: ${failed}, порог ${TOLERANCE} px (сетка 24)`);
if (skipped.length) console.log(`взято прямо из макета, сверять не с чем: ${skipped.join(', ')}`);
if (pending.length) console.log(`эталон из макета ещё не снят: ${pending.join(', ')}`);
process.exit(failed ? 1 : 0);
