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

// Max simultaneous in-flight probes per run. The probes spend almost all their
// time waiting on the network (ICMP/TCP/UDP timeouts up to ~2s each), so running
// them sequentially makes the startup sweep over a large subscription take
// dozens of seconds. A bounded pool keeps the wall-clock time roughly
// targets/PING_CONCURRENCY × timeout while not opening hundreds of sockets at
// once. A small manual ping (one group) still completes in a single wave.
const PING_CONCURRENCY = 16;

/** @param {unknown} data */
function pingResultToLabel(data) {
    if (data && data.reachable) {
        if (typeof data.latencyMs === "number") {
            if (data.latencyMs > 0) {
                return `${data.latencyMs}ms`;
            } else if (data.latencyMs === 0) {
                return "<1ms";
            } else if (data.latencyMs < 0) {
                return "—";
            }
        }
        return "—";
    }
    const reason = data?.reason || "";
    if (reason === "timeout") return "Timeout";
    if (reason === "connection_refused") return "Refused";
    if (reason === "network_unreachable" || reason === "no_route_to_host") {
        return "Unreachable";
    }
    if (reason === "connection_closed") return "Closed";
    if (reason === "dns_unresolved") return "DNS";
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
    // All ids with a ping in flight (background sweep + manual). Drives the
    // per-card "Опрос" label and isPingPending.
    const [pendingPingIds, setPendingPingIds] = useState(() => new Set());
    // Ids that belong to a *user-initiated* run only. The per-group ping button
    // pulses/disables off this set, so a background sweep (which touches every
    // server) never lights up every group's button, and clicking one group's
    // button leaves the others untouched.
    const [manualPendingIds, setManualPendingIds] = useState(() => new Set());

    const proxiesRef = useRef(proxies);
    proxiesRef.current = proxies;

    // Count of runs in flight (any kind) and of manual runs, so concurrent
    // manual pings of different groups each clear the global flags only once the
    // last one finishes — and a manual ping is never dropped just because the
    // background sweep happens to be running.
    const activeRunsRef = useRef(0);
    const manualRunsRef = useRef(0);

    const runPing = useCallback(async (optionalIds, options = {}) => {
        const userInitiated = options.userInitiated === true;
        const list = proxiesRef.current;
        if (!isConfigLoaded || list.length === 0) return;

        // The automatic background sweep must not stack on itself or on a manual
        // run; a user-initiated ping always proceeds — different groups ping
        // concurrently, each tracking only its own ids.
        if (!userInitiated && activeRunsRef.current > 0) {
            return;
        }

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

        if (targets.length === 0) return;

        const targetIds = targets.map((p) => String(p.id));

        activeRunsRef.current += 1;
        setIsPinging(true);
        if (userInitiated) {
            manualRunsRef.current += 1;
            setIsManualPinging(true);
        }

        flushSync(() => {
            setPendingPingIds((prev) => {
                const n = new Set(prev);
                targetIds.forEach((id) => n.add(id));
                return n;
            });
            if (userInitiated) {
                setManualPendingIds((prev) => {
                    const n = new Set(prev);
                    targetIds.forEach((id) => n.add(id));
                    return n;
                });
            }
        });

        try {
            // Bounded worker pool: probes are network-bound, so running them in
            // parallel (instead of one-at-a-time) cuts the sweep from
            // targets×timeout down to ~targets/PING_CONCURRENCY×timeout. Plain
            // index handoff is safe — JS runs these to completion between awaits,
            // so no two workers ever grab the same target.
            let next = 0;
            const worker = async () => {
                while (next < targets.length) {
                    const p = targets[next++];
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
                    if (userInitiated) {
                        setManualPendingIds((prev) => {
                            const n = new Set(prev);
                            n.delete(sid);
                            return n;
                        });
                    }
                }
            };

            const poolSize = Math.min(PING_CONCURRENCY, targets.length);
            await Promise.all(
                Array.from({ length: poolSize }, () => worker()),
            );
        } finally {
            activeRunsRef.current = Math.max(0, activeRunsRef.current - 1);
            if (activeRunsRef.current === 0) {
                setIsPinging(false);
            }
            if (userInitiated) {
                manualRunsRef.current = Math.max(0, manualRunsRef.current - 1);
                if (manualRunsRef.current === 0) {
                    setIsManualPinging(false);
                }
            }
            // Defensive: release any ids this run still owns (early error paths).
            setPendingPingIds((prev) => {
                if (prev.size === 0) return prev;
                const n = new Set(prev);
                targetIds.forEach((id) => n.delete(id));
                return n;
            });
            if (userInitiated) {
                setManualPendingIds((prev) => {
                    if (prev.size === 0) return prev;
                    const n = new Set(prev);
                    targetIds.forEach((id) => n.delete(id));
                    return n;
                });
            }
        }
    }, [isConfigLoaded]);

    useEffect(() => {
        if (!isConfigLoaded || proxies.length === 0) return;

        runPing(undefined, { userInitiated: false });
    }, [proxies, isConfigLoaded, runPing]);

    const refreshPings = useCallback(
        async (ids) => {
            await runPing(ids, { userInitiated: true });
        },
        [runPing],
    );

    const pingPendingFor = useCallback((proxy, idSet) => {
        if (!proxy || idSet.size === 0) return false;
        const t = proxy.type?.toUpperCase();
        if (t === "SECTION") return false;
        if (t === "AUTO") {
            const extra = parseExtra(proxy.extra);
            const memberIds = (extra?.members || []).map(String);
            return memberIds.some((id) => idSet.has(id));
        }
        return idSet.has(String(proxy.id));
    }, []);

    const isPingPending = useCallback(
        (proxy) => pingPendingFor(proxy, pendingPingIds),
        [pingPendingFor, pendingPingIds],
    );

    // True only while a *user-initiated* ping covering this proxy is in flight —
    // background sweeps don't count, so the per-group ping button reflects only
    // the group the user actually clicked.
    const isManualPingPending = useCallback(
        (proxy) => pingPendingFor(proxy, manualPendingIds),
        [pingPendingFor, manualPendingIds],
    );

    return {
        pings,
        refreshPings,
        isPinging,
        isManualPinging,
        pendingPingIds,
        isPingPending,
        isManualPingPending,
    };
};
