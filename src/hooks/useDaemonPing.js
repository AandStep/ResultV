import { useState, useEffect } from "react";
import { apiFetch } from "./useLogs";

export const useDaemonPing = (proxies, isConfigLoaded) => {
  const [pings, setPings] = useState({});

  useEffect(() => {
    if (!isConfigLoaded || proxies.length === 0) return;

    const fetchPings = async () => {
      const newPings = {};
      for (const p of proxies) {
        try {
          const res = await apiFetch(`/api/ping`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ ip: p.ip, port: p.port }),
          });
          if (res.ok) {
            const data = await res.json();
            newPings[p.id] = data.alive ? `${data.ping}ms` : "Timeout";
          } else {
            newPings[p.id] = "Error";
          }
        } catch {
          newPings[p.id] = "Error";
        }
      }
      setPings(newPings);
    };

    fetchPings();
    const interval = setInterval(fetchPings, 10000);
    return () => clearInterval(interval);
  }, [proxies, isConfigLoaded]);

  return pings;
};
