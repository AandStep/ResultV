/*
 * Copyright (C) 2026 ResultProxy
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

import { useState, useEffect, useCallback } from "react";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import wailsAPI from "../utils/wailsAPI";

export const useLogs = () => {
    const [logs, setLogs] = useState([
        {
            timestamp: Date.now(),
            time: new Date().toLocaleTimeString(),
            msg: "Интерфейс запущен. Подключение к фоновой службе...",
            type: "info",
        },
    ]);
    const [backendLogs, setBackendLogs] = useState([]);

    const addLog = useCallback((msg, type = "info") => {
        setLogs((prev) =>
            [
                {
                    timestamp: Date.now(),
                    time: new Date().toLocaleTimeString(),
                    msg,
                    type,
                },
                ...prev,
            ].slice(0, 50), // Keep last 50 entries
        );
    }, []);

    useEffect(() => {
        // Fetch initial bulk logs
        const fetchInitialLogs = async () => {
            try {
                const recentLogs = await wailsAPI.getLogs(1, 100);
                if (recentLogs && recentLogs.items) {
                    setBackendLogs(recentLogs.items);
                }
            } catch (err) {
                console.error("Failed to fetch initial logs:", err);
            }
        };
        fetchInitialLogs();

        // Listen for new logs pushed from backend securely
        EventsOn("log", (logEntry) => {
            // Update backend logs
            setBackendLogs((prevLogs) => [logEntry, ...prevLogs].slice(0, 500)); // Keep last 500
        });

        return () => {
            EventsOff("log");
        };
    }, []);

    return { logs, backendLogs, addLog };
};
