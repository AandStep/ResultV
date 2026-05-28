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

import { useState, useEffect, useCallback, useRef } from "react";
import { flushSync } from "react-dom";
import wailsAPI from "../utils/wailsAPI";
import { parseExtra } from "../utils/pingSort";

/** @param {unknown} data */
function pingResultToLabel(data) {
    if (data && data.reachable) {
        if (typeof data.latencyMs === "number" && data.latencyMs > 0) {
            return `${data.latencyMs}ms`;
        }
        return "Online";
    }
    const reason = data?.reason || "";
    if (reason === "timeout") return "Timeout";
    if (reason === "connection_refused") return "Refused";
    if (reason === "network_unreachable" || reason === "no_route_to_host") {
        return "Unreachable";
    }
    if (reason === "connection_closed") return "Closed";
    if (reason === "error" || reason === "probe_error") return "Error";
    return "Unavailable";
}

/**
 * @param {unknown[]} proxies
 * @param {boolean} isConfigLoaded
 * @returns {{
 *   pings: Record<string, string>,
 *   refreshPings: (ids?: Iterable<string>|string[]) => Promise<void>,
 *   isPinging: boolean,
 *   isManualPinging: boolean,
 *   pendingPingIds: ReadonlySet<string>,
 *   isPingPending: (proxy: unknown) => boolean,
 * }}
 */
export const useDaemonPing = (proxies, isConfigLoaded) => {
    const [pings, setPings] = useState({});
    const [isPinging, setIsPinging] = useState(false);
    const [isManualPinging, setIsManualPinging] = useState(false);
    const [pendingPingIds, setPendingPingIds] = useState(() => new Set());

    const proxiesRef = useRef(proxies);
    proxiesRef.current = proxies;

    const inFlightRef = useRef(false);

    const runPing = useCallback(async (optionalIds, options = {}) => {
        const userInitiated = options.userInitiated === true;
        const list = proxiesRef.current;
        if (!isConfigLoaded || list.length === 0) return;

        if (inFlightRef.current) {
            return;
        }
        inFlightRef.current = true;

        const idSet =
            optionalIds != null
                ? new Set([...optionalIds].map((id) => String(id)))
                : null;

        const targets = [];
        for (const p of list) {
            if (idSet && !idSet.has(String(p.id))) continue;
            if (p.type?.toUpperCase() === "AUTO") continue;
            if (p.type?.toUpperCase() === "SECTION") continue;
            targets.push(p);
        }

        if (targets.length === 0) {
            inFlightRef.current = false;
            return;
        }

        setIsPinging(true);
        if (userInitiated) {
            setIsManualPinging(true);
        }

        flushSync(() => {
            setPendingPingIds(new Set(targets.map((p) => String(p.id))));
        });

        try {
            for (const p of targets) {
                const sid = String(p.id);

                let value = "Error";
                try {
                    const data = await wailsAPI.ping(
                        p.ip,
                        parseInt(p.port, 10) || 0,
                        p.type || "",
                    );
                    value = pingResultToLabel(data);
                } catch {
                    value = "Error";
                }

                setPings((prev) => ({ ...prev, [p.id]: value }));

                setPendingPingIds((prev) => {
                    const n = new Set(prev);
                    n.delete(sid);
                    return n;
                });
            }
        } finally {
            inFlightRef.current = false;
            setIsPinging(false);
            if (userInitiated) {
                setIsManualPinging(false);
            }
            setPendingPingIds(new Set());
        }
    }, [isConfigLoaded]);

    useEffect(() => {
        if (!isConfigLoaded || proxies.length === 0) return;

        runPing(undefined, { userInitiated: false });
        const interval = setInterval(() => {
            runPing(undefined, { userInitiated: false });
        }, 60000);
        return () => clearInterval(interval);
    }, [proxies, isConfigLoaded, runPing]);

    const refreshPings = useCallback(
        async (ids) => {
            await runPing(ids, { userInitiated: true });
        },
        [runPing],
    );

    const isPingPending = useCallback(
        (proxy) => {
            if (!proxy || pendingPingIds.size === 0) return false;
            const t = proxy.type?.toUpperCase();
            if (t === "SECTION") return false;
            if (t === "AUTO") {
                const extra = parseExtra(proxy.extra);
                const memberIds = (extra?.members || []).map(String);
                return memberIds.some((id) => pendingPingIds.has(id));
            }
            return pendingPingIds.has(String(proxy.id));
        },
        [pendingPingIds],
    );

    return {
        pings,
        refreshPings,
        isPinging,
        isManualPinging,
        pendingPingIds,
        isPingPending,
    };
};
