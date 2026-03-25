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

const SocksServer = require("./socks.server.cjs");
const HttpServer = require("./http.server.cjs");

class ProxyManager {
  constructor(loggerService, systemAdapter, stateStore) {
    this.logger = loggerService;
    this.systemAdapter = systemAdapter;
    this.stateStore = stateStore;

    this.socksServer = new SocksServer(
      loggerService,
      systemAdapter,
      stateStore,
    );
    this.httpServer = new HttpServer(loggerService, systemAdapter, stateStore);
  }

  async setSystemProxy(enable, proxy = null, updateRegistryOnly = false) {
    if (!updateRegistryOnly) {
      await this.httpServer.stop();
      this.socksServer.stop();
    }

    let proxyIp = "127.0.0.1";
    let proxyPort = "14081";
    let proxyType = "ALL";
    let rules = {
      mode: "global",
      whitelist: ["localhost", "127.0.0.1"],
      appWhitelist: [],
    };
    let gpoConflict = false;

    if (enable && proxy) {
      proxyIp = proxy.ip;
      proxyPort = proxy.port;
      proxyType = proxy.type || "HTTP";
      rules = proxy.rules || rules;

      // If SOCKS5 or HTTP with Auth requires a local bridge
      if (proxyType === "SOCKS5" || (proxy.username && proxy.password)) {
        if (!updateRegistryOnly) {
          if (proxyType === "SOCKS5") {
            const result = await this.socksServer.start(proxy);
            proxyIp = result.host;
            proxyPort = result.port;
            proxyType = "ALL";
          } else {
            const result = await this.httpServer.start(proxy);
            proxyIp = result.host;
            proxyPort = result.port;
            proxyType = "ALL";
          }
        } else {
          proxyIp = "127.0.0.1";
          proxyPort = "14081";
          proxyType = "ALL";
        }
      }

      const proxyResult = await this.systemAdapter.setSystemProxy(
        proxyIp,
        proxyPort,
        proxyType,
        rules.whitelist,
        !updateRegistryOnly ? this.logger.log.bind(this.logger) : null,
      );

      // Если системный адаптер вернул информацию о GPO-конфликте
      if (proxyResult && proxyResult.gpoConflict) {
        gpoConflict = true;
      }
    } else {
      await this.systemAdapter.disableSystemProxy(
        this.logger.log.bind(this.logger),
      );
    }

    return { gpoConflict };
  }

  async applyKillSwitch() {
    await this.systemAdapter.applyKillSwitch(this.logger.log.bind(this.logger));
  }
}

module.exports = ProxyManager;
