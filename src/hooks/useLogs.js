import { useState, useEffect, useCallback } from "react";

export const DAEMON_URL = "http://127.0.0.1:14080";

export const apiFetch = async (endpoint, options = {}) => {
  const token = window.electronAPI ? window.electronAPI.getApiToken() : "";
  const headers = {
    ...options.headers,
    Authorization: `Bearer ${token}`,
  };
  return fetch(`${DAEMON_URL}${endpoint}`, { ...options, headers });
};

export const useLogs = () => {
  const [logs, setLogs] = useState([
    {
      timestamp: Date.now(),
      time: new Date().toLocaleTimeString(),
      msg: "Интерфейс запущен. Загрузка конфигурации...",
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
      ].slice(0, 50),
    );
  }, []);

  useEffect(() => {
    let interval;
    const fetchLogs = async () => {
      try {
        const res = await apiFetch(`/api/logs`);
        if (res.ok) {
          const data = await res.json();
          setBackendLogs(data);
        }
      } catch (e) {}
    };

    fetchLogs();
    interval = setInterval(fetchLogs, 1500);

    return () => clearInterval(interval);
  }, []);

  return { logs, backendLogs, addLog };
};
