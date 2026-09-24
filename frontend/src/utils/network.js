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

import wailsAPI from "./wailsAPI";


export const detectCountry = async (ip, { fresh = false } = {}) => {
    try {
        let cleanIp = ip.split(":")[0];
        if (
            cleanIp === "127.0.0.1" ||
            cleanIp === "localhost" ||
            cleanIp.startsWith("192.168.") ||
            cleanIp.startsWith("10.") 
        ) {
            return "local";
        }

        const countryCode = fresh
            ? await wailsAPI.redetectCountry(cleanIp)
            : await wailsAPI.detectCountry(cleanIp);
        if (countryCode && countryCode !== "Unknown" && countryCode !== "🌐" && countryCode !== "🏠") {
            return countryCode; 
        }
    } catch (error) {
        console.error("DetectCountry error:", error);
    }
    return "unknown";
};

const REDETECT_PARALLEL = 6;

/*
 * Страна узла сохраняется при добавлении и дальше только наследуется, так что
 * подсеть, перепроданная в другую страну, носила бы старый флаг вечно.
 * Возвращает host → страна только для удачных ответов: при сбое у узла
 * остаётся прежний флаг, а не «unknown».
 */
export const redetectCountries = async (proxies, { fresh = false } = {}) => {
    const hosts = [
        ...new Set(
            proxies
                .filter((p) => p.ip && !["AUTO", "SECTION"].includes(String(p.type || "").toUpperCase()))
                .map((p) => p.ip),
        ),
    ];
    const found = new Map();
    let next = 0;
    const worker = async () => {
        while (next < hosts.length) {
            const host = hosts[next++];
            const code = await detectCountry(host, { fresh });
            if (code !== "unknown" && code !== "local") found.set(host, code);
        }
    };
    await Promise.all(Array.from({ length: Math.min(REDETECT_PARALLEL, hosts.length) }, worker));
    return found;
};

export const applyCountries = (proxies, found) =>
    found.size === 0
        ? proxies
        : proxies.map((p) =>
              found.has(p.ip) && found.get(p.ip) !== p.country ? { ...p, country: found.get(p.ip) } : p,
          );
