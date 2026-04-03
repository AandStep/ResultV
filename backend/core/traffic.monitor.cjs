/*
 * Copyright (C) 2026 ResultProxy
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

const net = require("net");

class TrafficMonitor {
  constructor(loggerService, stateStore, proxyManager, systemAdapter) {
    this.logger = loggerService;
    this.stateStore = stateStore;
    this.proxyManager = proxyManager;
    this.systemAdapter = systemAdapter;
    this.intervalId = null;
    this.trafficIntervalId = null;
  }

  start() {
    if (this.intervalId) return;

    this.intervalId = setInterval(async () => {
      const state = this.stateStore.getState();

      if (state.isConnected && state.activeProxy) {
        const { alive } = await this.pingProxy(
          state.activeProxy.ip,
          state.activeProxy.port,
        );
        const wasDead = state.isProxyDead;

        this.stateStore.update({ isProxyDead: !alive });

        if (!alive && !wasDead && state.killSwitch) {
          await this.proxyManager.applyKillSwitch();
        } else if (alive && wasDead && state.killSwitch) {
          this.logger.log(
            "[KILL SWITCH] Связь восстановлена. Возвращаем доступ.",
            "success",
          );
          await this.proxyManager.setSystemProxy(true, state.activeProxy, true);
        }
      } else {
        this.stateStore.update({ isProxyDead: false });
      }
    }, 3000);

    this.trafficIntervalId = setInterval(async () => {
      const state = this.stateStore.getState();

      if (state.isConnected) {
        const now = Date.now();
        const currentStats = await this.systemAdapter.getNetworkTraffic();
        const timeDiff = (now - state.lastTickStats.time) / 1000;

        if (timeDiff <= 0) return;

        let dRec = (currentStats.received || 0) - state.lastTickStats.received;
        let dSent = (currentStats.sent || 0) - state.lastTickStats.sent;

        if (dRec < 0) dRec = 0;
        if (dSent < 0) dSent = 0;

        const newBytesReceived = state.bytesReceived + dRec;
        const newBytesSent = state.bytesSent + dSent;

        let sRec = dRec > 0 ? dRec / timeDiff : 0;
        let sSent = dSent > 0 ? dSent / timeDiff : 0;

        const newSpeedReceived = sRec > 2048 ? sRec : 0;
        const newSpeedSent = sSent > 2048 ? sSent : 0;

        this.stateStore.update({
          bytesReceived: newBytesReceived,
          bytesSent: newBytesSent,
          speedReceived: newSpeedReceived,
          speedSent: newSpeedSent,
          lastTickStats: {
            received: currentStats.received || 0,
            sent: currentStats.sent || 0,
            time: now,
          },
        });
      } else {
        // Убеждаемся, что скорость показывается 0, если мы не подключены
        if (state.speedReceived > 0 || state.speedSent > 0) {
          this.stateStore.update({
            speedReceived: 0,
            speedSent: 0,
          });
        }
      }
    }, 1000);
  }

  stop() {
    if (this.intervalId) {
      clearInterval(this.intervalId);
      this.intervalId = null;
    }
    if (this.trafficIntervalId) {
      clearInterval(this.trafficIntervalId);
      this.trafficIntervalId = null;
    }
  }

  pingProxy(host, port) {
    return new Promise((resolve) => {
      const start = Date.now();
      const socket = new net.Socket();
      socket.setTimeout(5000);

      socket.on("connect", () => {
        const ping = Date.now() - start;
        socket.destroy();
        resolve({ alive: true, ping });
      });

      socket.on("timeout", () => {
        socket.destroy();
        resolve({ alive: false, ping: 0 });
      });

      socket.on("error", () => {
        socket.destroy();
        resolve({ alive: false, ping: 0 });
      });

      socket.connect(port, host);
    });
  }
}

module.exports = TrafficMonitor;
