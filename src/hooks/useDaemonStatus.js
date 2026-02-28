import { useState, useEffect, useRef } from "react";
import { apiFetch } from "./useLogs";

export const useDaemonStatus = (
  isConnected,
  setIsConnected,
  proxies,
  failedProxy,
  setFailedProxy,
  setActiveProxy,
  isSwitchingRef,
  addLog,
) => {
  const [isProxyDead, setIsProxyDead] = useState(false);
  const [stats, setStats] = useState({ download: 0, upload: 0 });
  const [speedHistory, setSpeedHistory] = useState({
    down: new Array(20).fill(0),
    up: new Array(20).fill(0),
  });
  const [daemonStatus, setDaemonStatus] = useState("checking");
  const prevProxyDead = useRef(false);

  useEffect(() => {
    let interval;
    const fetchStatus = async () => {
      try {
        const res = await apiFetch(`/api/status`);
        if (res.ok) {
          const data = await res.json();
          if (daemonStatus !== "online") setDaemonStatus("online");

          setIsProxyDead(!!data.isProxyDead);
          if (data.isConnected) {
            setFailedProxy(null);
          }

          if (data.isConnected) {
            if (data.isProxyDead && !prevProxyDead.current) {
              addLog(
                `Внимание: Узел ${
                  data.activeProxy?.ip || ""
                } перестал отвечать!`,
                "error",
              );
            } else if (!data.isProxyDead && prevProxyDead.current) {
              addLog(`Связь с узлом восстановлена.`, "success");
            }
          }
          prevProxyDead.current = !!data.isProxyDead;

          if (!isSwitchingRef.current) {
            setIsConnected(data.isConnected);
            if (data.activeProxy) {
              const localMatchedProxy = proxies.find(
                (p) =>
                  p.id === data.activeProxy.id || p.ip === data.activeProxy.ip,
              );
              setActiveProxy(localMatchedProxy || data.activeProxy);
            }
          }

          setStats({ download: data.bytesReceived, upload: data.bytesSent });
          setSpeedHistory((h) => ({
            down: [...h.down.slice(1), data.speedReceived || 0],
            up: [...h.up.slice(1), data.speedSent || 0],
          }));
        }
      } catch (error) {
        setDaemonStatus("offline");
        if (isConnected) setIsConnected(false);
      }
    };

    fetchStatus();
    interval = setInterval(fetchStatus, 1000);
    return () => clearInterval(interval);
  }, [
    isConnected,
    daemonStatus,
    proxies,
    failedProxy,
    addLog,
    setIsConnected,
    setActiveProxy,
    setFailedProxy,
    isSwitchingRef,
  ]);

  return { isProxyDead, stats, speedHistory, daemonStatus };
};
