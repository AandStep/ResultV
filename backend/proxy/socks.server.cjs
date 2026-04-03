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
const { SocksClient } = require("socks");
const { isWhitelisted } = require("../utils/domain.cjs");
const { isAdDomain } = require("../utils/adblock.cjs");
const { isBlockedDomain } = require("../utils/blocked.cjs");

class SocksServer {
  constructor(loggerService, systemAdapter, stateStore) {
    this.logger = loggerService;
    this.systemAdapter = systemAdapter;
    this.stateStore = stateStore;
    this.server = null;
    this.port = 14081;
    this.host = "127.0.0.1";
  }

  start(proxyConfig) {
    return new Promise((resolve, reject) => {
      if (this.server) {
        this.stop();
      }

      this.logger.log(
        `[МОСТ SOCKS5] Запуск локального туннеля на ${this.host}:${this.port}`,
        "info",
      );

      this.server = net.createServer((client) => {
        let step = 0;
        let buffer = Buffer.alloc(0);
        let isSocks5 = false;
        let isSocks4 = false;
        let isHttpConnect = false;

        const onData = async (data) => {
          client.pause();
          try {
            buffer = Buffer.concat([buffer, data]);

            if (step === 0) {
              if (buffer.length < 1) {
                client.resume();
                return;
              }

              if (buffer[0] === 0x05) {
                isSocks5 = true;
                if (buffer.length < 2) {
                  client.resume();
                  return;
                }
                const numMethods = buffer[1];
                if (buffer.length < 2 + numMethods) {
                  client.resume();
                  return;
                }

                client.write(Buffer.from([0x05, 0x00]));
                buffer = buffer.slice(2 + numMethods);
                step = 1;
                if (buffer.length === 0) {
                  client.resume();
                  return;
                }
              } else if (buffer[0] === 0x04) {
                isSocks4 = true;
                step = 1;
              } else if (buffer[0] === 0x43) {
                isHttpConnect = true;
                step = 1;
              } else {
                client.resume();
                return client.end();
              }
            }

            if (step === 1) {
              let dstHost, dstPort, offset, successResponse;

              if (isSocks5) {
                if (buffer.length < 4) {
                  client.resume();
                  return;
                }
                if (buffer[0] !== 0x05 || buffer[1] !== 0x01) {
                  client.resume();
                  return client.end();
                }

                const atyp = buffer[3];
                offset = 4;

                if (atyp === 0x01) {
                  if (buffer.length < offset + 4 + 2) {
                    client.resume();
                    return;
                  }
                  dstHost = `${buffer[offset]}.${buffer[offset + 1]}.${buffer[offset + 2]}.${buffer[offset + 3]}`;
                  offset += 4;
                } else if (atyp === 0x03) {
                  if (buffer.length < offset + 1) {
                    client.resume();
                    return;
                  }
                  const len = buffer[offset];
                  if (buffer.length < offset + 1 + len + 2) {
                    client.resume();
                    return;
                  }
                  dstHost = buffer
                    .slice(offset + 1, offset + 1 + len)
                    .toString();
                  offset += 1 + len;
                } else {
                  client.resume();
                  return client.end();
                }

                dstPort = buffer.readUInt16BE(offset);
                offset += 2;
                successResponse = Buffer.from([
                  0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                ]);
              } else if (isSocks4) {
                if (buffer.length < 9) {
                  client.resume();
                  return;
                }
                const nullIdx = buffer.indexOf(0x00, 8);
                if (nullIdx === -1) {
                  client.resume();
                  return;
                }

                dstPort = buffer.readUInt16BE(2);
                const ip1 = buffer[4],
                  ip2 = buffer[5],
                  ip3 = buffer[6],
                  ip4 = buffer[7];
                dstHost = `${ip1}.${ip2}.${ip3}.${ip4}`;
                offset = nullIdx + 1;

                if (ip1 === 0 && ip2 === 0 && ip3 === 0 && ip4 !== 0) {
                  const domainNullIdx = buffer.indexOf(0x00, offset);
                  if (domainNullIdx === -1) {
                    client.resume();
                    return;
                  }
                  dstHost = buffer.slice(offset, domainNullIdx).toString();
                  offset = domainNullIdx + 1;
                }
                successResponse = Buffer.from([
                  0x00, 0x5a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                ]);
              } else if (isHttpConnect) {
                const reqStr = buffer.toString();
                const headerEnd = reqStr.indexOf("\r\n\r\n");
                if (headerEnd === -1) {
                  client.resume();
                  return;
                }

                const lines = reqStr.split("\r\n");
                const match = lines[0].match(/CONNECT\s+([^:]+):(\d+)/);
                if (!match) {
                  client.resume();
                  return client.end();
                }

                dstHost = match[1];
                dstPort = parseInt(match[2], 10);
                offset = headerEnd + 4;
                successResponse = Buffer.from(
                  "HTTP/1.1 200 Connection Established\r\n\r\n",
                );
              }

              client.removeListener("data", onData);
              const remainingData = buffer.slice(offset);

              // Ad Blocker: блокировка рекламных доменов (Ghostery)
              const adblockEnabled = this.stateStore.getState().adblock;
              if (adblockEnabled && isAdDomain(dstHost)) {
                this.logger.log(
                  `[ADBLOCK] Заблокировано: ${dstHost}`,
                  "warning",
                );
                client.write(successResponse);
                client.end();
                client.resume();
                return;
              }

              const activeProxy = this.stateStore.getState().activeProxy;
              const currentRules = activeProxy?.rules || {
                mode: "global",
                whitelist: ["localhost", "127.0.0.1"],
                appWhitelist: [],
              };

              let isAppWhitelisted = false;
              if (
                currentRules.appWhitelist &&
                currentRules.appWhitelist.length > 0
              ) {
                const clientPort = client.remotePort;
                const matchedApp = await this.systemAdapter.checkAppWhitelist(
                  clientPort,
                  currentRules.appWhitelist,
                  dstHost,
                  this.logger.log.bind(this.logger),
                );
                if (matchedApp) {
                  isAppWhitelisted = true;
                }
              }

              const isWhitelistedDomain =
                currentRules.whitelist &&
                isWhitelisted(dstHost, currentRules.whitelist);
              const { isWhitelisted: isBypass, matchingRules } = isWhitelisted(
                dstHost,
                currentRules.whitelist,
              );
              const isBlocked = isBlockedDomain(dstHost);

              let useProxy = false;
              let reason = "";

              if (currentRules.mode === "smart") {
                if (!isBypass && matchingRules.length > 0) {
                  useProxy = true;
                  reason = `Nested exception found: [${matchingRules.join(", ")}]`;
                } else if (isBypass) {
                  useProxy = isBlocked;
                  reason = isBlocked
                    ? "Blocked resource in Smart mode"
                    : "Bypass (Whitelisted)";
                } else {
                  useProxy = isBlocked;
                  reason = isBlocked
                    ? "Blocked resource (No match)"
                    : "Direct (Not blocked)";
                }
              } else {
                useProxy = !isBypass;
                reason = isBypass
                  ? "Whitelisted (Bypass)"
                  : "Not whitelisted (Proxy)";
              }

              if (isAppWhitelisted) {
                useProxy = false;
                this.logger.log(
                  `[МОСТ] BYPASS: ${dstHost} (App Whitelisted)`,
                  "info",
                );
              } else if (useProxy) {
                this.logger.log(
                  `[ПРОКСИ] ${dstHost}:${dstPort} -> ${proxyConfig.ip} (${reason})`,
                  "success",
                );
              }

              const setupPipeline = (src, dst) => {
                src.pipe(dst);
                dst.pipe(src);
                
                const cleanup = () => {
                  if (!src.destroyed) src.destroy();
                  if (!dst.destroyed) dst.destroy();
                };
                
                src.on("error", cleanup);
                dst.on("error", cleanup);
                
                src.setTimeout(300000, cleanup);
                dst.setTimeout(300000, cleanup);
                
                src.setKeepAlive(true, 30000);
                dst.setKeepAlive(true, 30000);
              };

              if (!useProxy) {
                const directSocket = net.connect(dstPort, dstHost, () => {
                  client.write(successResponse);
                  if (remainingData.length > 0)
                    directSocket.write(remainingData);
                  setupPipeline(client, directSocket);
                });
                directSocket.on("error", () => client.destroy());
              } else {
                SocksClient.createConnection({
                  proxy: {
                    host: proxyConfig.ip,
                    port: parseInt(proxyConfig.port),
                    type: 5,
                    userId: proxyConfig.username || undefined,
                    password: proxyConfig.password || undefined,
                  },
                  command: "connect",
                  destination: { host: dstHost, port: dstPort },
                })
                  .then((info) => {
                    client.write(successResponse);
                    if (remainingData.length > 0)
                      info.socket.write(remainingData);
                    setupPipeline(client, info.socket);
                  })
                  .catch(() => {
                    client.destroy();
                  });
              }
            }
          } catch (e) {
            client.destroy();
          }
        };
        client.on("data", onData);
        client.on("error", () => {});
      });

      this.server.listen(this.port, this.host, () => {
        resolve({ host: this.host, port: this.port });
      });

      this.server.on("error", (err) => {
        reject(err);
      });
    });
  }

  stop() {
    if (this.server) {
      this.server.close();
      this.server = null;
    }
  }
}

module.exports = SocksServer;
