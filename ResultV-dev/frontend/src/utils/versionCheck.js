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

export const compareVersions = (v1, v2) => {
  if (!v1 || !v2) return 0;

  const parseVersion = (raw) => {
    const normalized = String(raw).trim().replace(/^v/i, "");
    const [coreRaw, preRaw = ""] = normalized.split("-", 2);
    const core = coreRaw.split(".").map((part) => {
      const n = Number.parseInt(part, 10);
      return Number.isFinite(n) ? n : 0;
    });
    const pre = preRaw
      ? preRaw.split(".").map((part) => {
          if (/^\d+$/.test(part)) return { type: "num", value: Number.parseInt(part, 10) };
          return { type: "str", value: part.toLowerCase() };
        })
      : [];
    return { core, pre };
  };

  const a = parseVersion(v1);
  const b = parseVersion(v2);
  const coreLen = Math.max(a.core.length, b.core.length);

  for (let i = 0; i < coreLen; i++) {
    const x = a.core[i] ?? 0;
    const y = b.core[i] ?? 0;
    if (x > y) return 1;
    if (x < y) return -1;
  }

  const aPre = a.pre.length > 0;
  const bPre = b.pre.length > 0;
  if (!aPre && !bPre) return 0;
  if (!aPre && bPre) return 1;
  if (aPre && !bPre) return -1;

  const preLen = Math.max(a.pre.length, b.pre.length);
  for (let i = 0; i < preLen; i++) {
    const x = a.pre[i];
    const y = b.pre[i];
    if (!x && !y) return 0;
    if (!x) return -1;
    if (!y) return 1;
    if (x.type === y.type) {
      if (x.value > y.value) return 1;
      if (x.value < y.value) return -1;
      continue;
    }
    // Semver rule: numeric prerelease identifiers have lower precedence
    // than non-numeric identifiers.
    return x.type === "num" ? -1 : 1;
  }

  return 0;
};
