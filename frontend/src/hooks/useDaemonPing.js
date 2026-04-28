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

import { useState, useEffect } from "react";
import wailsAPI from "../utils/wailsAPI";

export const useDaemonPing = (proxies, isConfigLoaded) => {
    const [pings, setPings] = useState({});

    useEffect(() => {
        if (!isConfigLoaded || proxies.length === 0) return;

        const fetchPings = async () => {
            const newPings = {};
            for (const p of proxies) {
                if (p.type?.toUpperCase() === "AUTO") continue;
                try {
                    const data = await wailsAPI.ping(p.ip, parseInt(p.port, 10) || 0, p.type || "");
                    if (data && data.reachable) {
                        if (typeof data.latencyMs === "number" && data.latencyMs > 0) {
                            newPings[p.id] = `${data.latencyMs}ms`;
                        } else {
                            newPings[p.id] = "Online";
                        }
                    } else {
                        const reason = data?.reason || "";
                        if (reason === "timeout") {
                            newPings[p.id] = "Timeout";
                        } else if (reason === "connection_refused") {
                            newPings[p.id] = "Refused";
                        } else if (reason === "network_unreachable" || reason === "no_route_to_host") {
                            newPings[p.id] = "Unreachable";
                        } else if (reason === "connection_closed") {
                            newPings[p.id] = "Closed";
                        } else if (reason === "error" || reason === "probe_error") {
                            newPings[p.id] = "Error";
                        } else {
                            newPings[p.id] = "Unavailable";
                        }
                    }
                } catch {
                    newPings[p.id] = "Error";
                }
            }
            setPings(newPings);
        };

        fetchPings();
        const interval = setInterval(fetchPings, 15000);
        return () => clearInterval(interval);
    }, [proxies, isConfigLoaded]);

    return pings;
};
