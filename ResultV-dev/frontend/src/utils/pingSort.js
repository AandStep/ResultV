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

/** @param {unknown} raw */
export function parseExtra(raw) {
    if (Array.isArray(raw) && typeof raw[0] === "number") {
        try {
            return JSON.parse(String.fromCharCode(...raw));
        } catch {
            return {};
        }
    }
    if (typeof raw === "string") {
        try {
            return JSON.parse(raw);
        } catch {
            return {};
        }
    }
    return raw || {};
}

const ERR_ORDER = {
    Timeout: 1,
    Refused: 2,
    Unreachable: 3,
    Closed: 4,
    Error: 5,
    Unavailable: 6,
};

const SECTION_RANK = 1e12;
const UNKNOWN_RANK = 1e11;

/**
 * Lower = better latency for ascending sort.
 * @param {{ id?: string, type?: string, extra?: unknown }} proxy
 * @param {Record<string, string>} pings
 */
export function getPingSortMetric(proxy, pings) {
    const t = proxy.type?.toUpperCase() || "";
    if (t === "SECTION") return SECTION_RANK;

    if (t === "AUTO") {
        const extra = parseExtra(proxy.extra);
        const memberIds = (extra?.members || []).map(String);
        const values = memberIds
            .map((id) => pings[id])
            .filter((p) => p && /^\d+/.test(String(p)));
        if (!values.length) return UNKNOWN_RANK;
        const best = values
            .map((v) => parseInt(String(v), 10))
            .reduce((a, b) => Math.min(a, b), Infinity);
        return Number.isFinite(best) ? best : UNKNOWN_RANK;
    }

    const v = pings[proxy.id];
    if (v == null || v === "") return UNKNOWN_RANK;
    if (/^\d+/.test(String(v))) return parseInt(String(v), 10);
    if (v === "Online") return 500_000;
    const err = ERR_ORDER[v];
    if (err != null) return 700_000 + err * 100;
    return UNKNOWN_RANK;
}

/**
 * Same ordering rules as proxy list / home dropdown.
 * @param {unknown[]} list
 * @param {string} sortBy
 * @param {Record<string, string>} pings
 */
export function sortProxiesByOption(list, sortBy, pings) {
    const result = [...list];
    if (sortBy === "country") {
        result.sort((a, b) => (a.country || "").localeCompare(b.country || ""));
    } else if (sortBy === "type") {
        result.sort((a, b) => (a.type || "").localeCompare(b.type || ""));
    } else if (sortBy === "newest") {
        result.reverse();
    } else if (sortBy === "provider") {
        result.sort((a, b) => (a.provider || "").localeCompare(b.provider || ""));
    } else if (sortBy === "ping") {
        result.sort(
            (a, b) => getPingSortMetric(a, pings) - getPingSortMetric(b, pings),
        );
    }
    return result;
}
