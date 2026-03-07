import { create } from 'zustand';
import { apiFetch } from '../services/api';
import type { ProxyItem, RoutingRules } from './configStore';

type Stats = { download: number; upload: number };
type SpeedHistory = { down: number[]; up: number[] };

type ConnectionStore = {
    isConnected: boolean;
    isProxyDead: boolean;
    failedProxy: ProxyItem | null;
    activeProxy: ProxyItem | null;
    stats: Stats;
    speedHistory: SpeedHistory;
    pings: Record<number, string>;
    daemonStatus: 'checking' | 'online' | 'offline';
    isSwitching: boolean;

    setFailedProxy: (p: ProxyItem | null) => void;
    setActiveProxy: (p: ProxyItem | null) => void;

    toggleConnection: (
        proxies: ProxyItem[],
        routingRules: RoutingRules,
        killswitch: boolean,
        addLog: (msg: string, type: string) => void,
    ) => Promise<void>;

    selectAndConnect: (
        proxy: ProxyItem,
        routingRules: RoutingRules,
        killswitch: boolean,
        addLog: (msg: string, type: string) => void,
        forceReconnect?: boolean,
    ) => Promise<void>;

    deleteProxy: (
        id: number,
        setProxies: (fn: (prev: ProxyItem[]) => ProxyItem[]) => void,
        addLog: (msg: string, type: string) => void,
    ) => Promise<void>;

    startStatusPolling: (
        proxies: ProxyItem[],
        addLog: (msg: string, type: string) => void,
    ) => () => void;

    startPingPolling: (proxies: ProxyItem[]) => () => void;
};

export const useConnectionStore = create<ConnectionStore>((set, get) => ({
    isConnected: false,
    isProxyDead: false,
    failedProxy: null,
    activeProxy: null,
    stats: { download: 0, upload: 0 },
    speedHistory: { down: new Array(20).fill(0), up: new Array(20).fill(0) },
    pings: {},
    daemonStatus: 'checking',
    isSwitching: false,

    setFailedProxy: failedProxy => set({ failedProxy }),
    setActiveProxy: activeProxy => set({ activeProxy }),

    toggleConnection: async (proxies, routingRules, killswitch, addLog) => {
        const { daemonStatus, activeProxy, isConnected } = get();
        if (daemonStatus !== 'online') {
            addLog('Служба недоступна.', 'error');
            return;
        }

        const targetProxy = activeProxy || proxies[0];
        if (proxies.length === 0 || !targetProxy) return;

        try {
            set({ isSwitching: true, failedProxy: null });

            if (isConnected) {
                addLog('Отключение...', 'info');
                await apiFetch('/api/disconnect', { method: 'POST' });
                addLog('Отключено успешно.', 'success');
                set({ isConnected: false });
            } else {
                addLog(`Подключение к ${targetProxy.name}...`, 'info');
                set({ activeProxy: targetProxy });

                const res = await apiFetch('/api/connect', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        ...targetProxy,
                        rules: routingRules,
                        killSwitch: killswitch,
                    }),
                });

                if (!res.ok) {
                    const errData = await res.json().catch(() => ({}));
                    throw new Error(errData.error || `HTTP ${res.status}`);
                }
                const resData = await res.json().catch(() => ({}));
                addLog('Соединение установлено.', 'success');
                if (resData.dnsLeakWarning) {
                    addLog(
                        '⚠️ DNS-утечка: HTTP-прокси не проксирует DNS-запросы. Используйте SOCKS5 для полной защиты.',
                        'warning',
                    );
                }
                set({ isConnected: true });
            }

            setTimeout(() => set({ isSwitching: false }), 3000);
        } catch (error: any) {
            set({ isSwitching: false, failedProxy: targetProxy });
            addLog(`Сбой: ${error.message}`, 'error');
        }
    },

    selectAndConnect: async (
        proxy,
        routingRules,
        killswitch,
        addLog,
        forceReconnect = false,
    ) => {
        const { activeProxy, isConnected } = get();
        if (!forceReconnect && activeProxy?.id === proxy.id && isConnected) return;

        try {
            set({ isSwitching: true, failedProxy: null, activeProxy: proxy });
            addLog(`Переключение на: ${proxy.name}...`, 'info');

            if (isConnected) {
                await apiFetch('/api/disconnect', { method: 'POST' });
                set({ isConnected: false });
            }

            const res = await apiFetch('/api/connect', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    ...proxy,
                    rules: routingRules,
                    killSwitch: killswitch,
                }),
            });

            if (!res.ok) {
                const errData = await res.json().catch(() => ({}));
                throw new Error(errData.error || 'Ошибка смены прокси');
            }

            const resData = await res.json().catch(() => ({}));
            set({ isConnected: true });
            addLog(`Успешно переключено на ${proxy.name}`, 'success');
            if (resData.dnsLeakWarning) {
                addLog(
                    '⚠️ DNS-утечка: HTTP-прокси не проксирует DNS-запросы. Используйте SOCKS5 для полной защиты.',
                    'warning',
                );
            }

            setTimeout(() => set({ isSwitching: false }), 2000);
        } catch (error: any) {
            set({ isSwitching: false, failedProxy: proxy });
            addLog(`Сбой подключения: ${error.message}`, 'error');
        }
    },

    deleteProxy: async (id, setProxies, addLog) => {
        const { activeProxy, isConnected, failedProxy } = get();
        const isDeletingActive = activeProxy?.id === id;
        setProxies(prev => prev.filter(p => p.id !== id));

        if (isDeletingActive) {
            if (isConnected) {
                set({ isSwitching: true });
                addLog('Активный сервер удален. Разрыв соединения...', 'info');
                try {
                    await apiFetch('/api/disconnect', { method: 'POST' });
                    addLog('Отключено успешно.', 'success');
                } catch { }
                set({ isConnected: false, activeProxy: null });
                setTimeout(() => set({ isSwitching: false }), 2000);
            } else {
                set({ activeProxy: null });
            }
        }
        if (failedProxy?.id === id) set({ failedProxy: null });
    },

    startStatusPolling: (proxies, addLog) => {
        let prevProxyDead = false;

        const fetchStatus = async () => {
            try {
                const res = await apiFetch('/api/status');
                if (res.ok) {
                    const data = await res.json();
                    const { daemonStatus, isSwitching, failedProxy } = get();
                    if (daemonStatus !== 'online') set({ daemonStatus: 'online' });

                    set({ isProxyDead: !!data.isProxyDead });
                    if (data.isConnected) set({ failedProxy: null });

                    if (data.isConnected) {
                        if (data.isProxyDead && !prevProxyDead) {
                            addLog(
                                `Внимание: Узел ${data.activeProxy?.ip || ''} перестал отвечать!`,
                                'error',
                            );
                        } else if (!data.isProxyDead && prevProxyDead) {
                            addLog('Связь с узлом восстановлена.', 'success');
                        }
                    }
                    prevProxyDead = !!data.isProxyDead;

                    if (!isSwitching) {
                        set({ isConnected: data.isConnected });
                        if (data.activeProxy) {
                            const localMatch = proxies.find(
                                p =>
                                    p.id === data.activeProxy.id ||
                                    p.ip === data.activeProxy.ip,
                            );
                            set({ activeProxy: localMatch || data.activeProxy });
                        }
                    }

                    set(state => ({
                        stats: { download: data.bytesReceived, upload: data.bytesSent },
                        speedHistory: {
                            down: [...state.speedHistory.down.slice(1), data.speedReceived || 0],
                            up: [...state.speedHistory.up.slice(1), data.speedSent || 0],
                        },
                    }));
                }
            } catch {
                set({ daemonStatus: 'offline' });
                if (get().isConnected) set({ isConnected: false });
            }
        };

        fetchStatus();
        const interval = setInterval(fetchStatus, 1000);
        return () => clearInterval(interval);
    },

    startPingPolling: proxies => {
        const fetchPings = async () => {
            const newPings: Record<number, string> = {};
            for (const p of proxies) {
                try {
                    const res = await apiFetch('/api/ping', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ ip: p.ip, port: p.port }),
                    });
                    if (res.ok) {
                        const data = await res.json();
                        newPings[p.id] = data.alive ? `${data.ping}ms` : 'Timeout';
                    } else {
                        newPings[p.id] = 'Error';
                    }
                } catch {
                    newPings[p.id] = 'Error';
                }
            }
            set({ pings: newPings });
        };

        fetchPings();
        const interval = setInterval(fetchPings, 10000);
        return () => clearInterval(interval);
    },
}));
