import { create } from 'zustand';

type LogEntry = {
    timestamp: number;
    time: string;
    msg: string;
    type: string;
};

type LogStore = {
    logs: LogEntry[];
    backendLogs: LogEntry[];
    addLog: (msg: string, type?: string) => void;
    setBackendLogs: (logs: LogEntry[]) => void;
    startPolling: () => () => void;
};

export const useLogStore = create<LogStore>((set, get) => ({
    logs: [
        {
            timestamp: Date.now(),
            time: new Date().toLocaleTimeString(),
            msg: 'Интерфейс запущен. Загрузка конфигурации...',
            type: 'info',
        },
    ],
    backendLogs: [],

    addLog: (msg, type = 'info') => {
        set(state => ({
            logs: [
                {
                    timestamp: Date.now(),
                    time: new Date().toLocaleTimeString(),
                    msg,
                    type,
                },
                ...state.logs,
            ].slice(0, 50),
        }));
    },

    setBackendLogs: backendLogs => set({ backendLogs }),

    startPolling: () => {
        const { apiFetch } = require('../services/api');
        const interval = setInterval(async () => {
            try {
                const res = await apiFetch('/api/logs');
                if (res.ok) {
                    const data = await res.json();
                    get().setBackendLogs(data);
                }
            } catch { }
        }, 1500);

        return () => clearInterval(interval);
    },
}));
