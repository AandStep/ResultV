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
 * Порядок групп серверов, заданный перетаскиванием на странице серверов.
 * Ключ группы — id подписки, у своих серверов — "my". Группа, которой в
 * сохранённом порядке нет (подписку добавили позже), встаёт сразу за той,
 * за которой шла бы без порядка: новая подписка не улетает в конец.
 */
export function orderGroups(groups, order) {
  if (!Array.isArray(order) || order.length === 0) return groups;
  const byKey = new Map(groups.map((g) => [String(g.key), g]));
  const out = order.map(String).filter((k) => byKey.has(k)).map((k) => byKey.get(k));
  const placed = new Set(out.map((g) => String(g.key)));

  groups.forEach((g, i) => {
    const key = String(g.key);
    if (placed.has(key)) return;
    let at = 0;
    for (let j = i - 1; j >= 0; j--) {
      const prev = out.indexOf(groups[j]);
      if (prev !== -1) {
        at = prev + 1;
        break;
      }
    }
    out.splice(at, 0, g);
    placed.add(key);
  });
  return out;
}
