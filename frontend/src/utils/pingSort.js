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
 *
 * For AUTO rows, ranks by the same chosen-node RTT autoRowPingLabel displays
 * once it is known, falling back to the member minimum otherwise — see
 * autoRowPingLabel's own doc comment for why the minimum alone is a poor
 * number (one unusually fast member sets it for the whole group even though
 * it may never be the one actually picked). Without this, once the AUTO row
 * DISPLAYS the chosen node's RTT (see GetAutoGroupStatus/autoStatusById) it
 * would still SORT as if it were the group minimum — a proxy list that
 * visibly reads 180ms sorting ahead of one that reads 90ms.
 *
 * @param {{ id?: string, type?: string, extra?: unknown }} proxy
 * @param {Record<string, string>} pings
 * @param {Record<string, { known?: boolean, rttMs?: number }>} [autoStatusById]
 */
export function getPingSortMetric(proxy, pings, autoStatusById) {
    const t = proxy.type?.toUpperCase() || "";
    if (t === "SECTION") return SECTION_RANK;

    if (t === "AUTO") {
        const status = autoStatusById?.[proxy.id];
        if (status?.known && typeof status.rttMs === "number") {
            return status.rttMs;
        }
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
    if (v === "Unknown") return 500_000;
    const err = ERR_ORDER[v];
    if (err != null) return 700_000 + err * 100;
    return UNKNOWN_RANK;
}

/**
 * Ping label for an AUTO row.
 *
 * Falls back to the group minimum only when the chosen node is unknown (no
 * connection yet). The minimum is a poor number on its own: one unusually fast
 * member sets it for the whole group even though it may never be picked.
 *
 * @param {{ extra?: unknown }} proxy
 * @param {Record<string, string>} pings
 * @param {{ known?: boolean, rttMs?: number }} [autoStatus]
 */
export function autoRowPingLabel(proxy, pings, autoStatus) {
    if (autoStatus?.known && typeof autoStatus.rttMs === "number") {
        return `${autoStatus.rttMs}ms`;
    }
    const extra = parseExtra(proxy.extra);
    const memberIds = (extra?.members || []).map(String);
    const values = memberIds
        .map((id) => pings[id])
        .filter((p) => p && /^\d+/.test(String(p)));
    if (!values.length) return null;
    return values
        .slice()
        .sort((a, b) => parseInt(a, 10) - parseInt(b, 10))[0];
}

/**
 * Same ordering rules as proxy list / home dropdown.
 * @param {unknown[]} list
 * @param {string} sortBy
 * @param {Record<string, string>} pings
 * @param {Record<string, { known?: boolean, rttMs?: number }>} [autoStatusById]
 */
export function sortProxiesByOption(list, sortBy, pings, autoStatusById) {
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
            (a, b) =>
                getPingSortMetric(a, pings, autoStatusById) -
                getPingSortMetric(b, pings, autoStatusById),
        );
    }
    return result;
}
