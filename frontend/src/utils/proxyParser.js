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

const naiveEntryFromProxyUrl = (proxyURL, meta) => {
    try {
        const u = new URL(proxyURL);
        const host = u.hostname;
        if (!host) return null;
        const port = u.port
            ? parseInt(u.port, 10)
            : u.protocol === "http:"
              ? 80
              : 443;
        const username = decodeURIComponent(u.username || "");
        const password = decodeURIComponent(u.password || "");
        if (!username || !password) return null;
        const extra = {};
        if (meta?.listen != null && String(meta.listen).trim() !== "")
            extra.naive_listen = String(meta.listen).trim();
        if (meta?.log != null && String(meta.log).trim() !== "")
            extra.naive_log = String(meta.log).trim();
        const sni = u.searchParams.get("sni") || u.searchParams.get("host");
        if (sni) extra.sni = sni.trim();
        const ins = (u.searchParams.get("insecure") || "").toLowerCase();
        if (["1", "true", "yes"].includes(ins)) extra.insecure = true;
        return {
            ip: host,
            port,
            type: "NAIVEPROXY",
            name: "NaiveProxy",
            username,
            password,
            country: "\u{1F310}",
            extra: Object.keys(extra).length ? extra : undefined,
        };
    } catch {
        return null;
    }
};

/** Naive client JSON: { listen, proxy: "https://user:pass@host", log } */
const parseNaiveClientJSON = (text) => {
    const t = text.trim();
    if (!t.startsWith("{") || !t.endsWith("}")) return null;
    try {
        const obj = JSON.parse(t);
        if (!obj || typeof obj !== "object" || Array.isArray(obj)) return null;
        if (obj.protocol || obj.type) return null;
        if (obj.outbounds) return null;
        const proxyStr = String(obj.proxy || "").trim();
        if (!proxyStr.startsWith("http://") && !proxyStr.startsWith("https://")) return null;
        const hasListen = Object.prototype.hasOwnProperty.call(obj, "listen");
        const hasLog = Object.prototype.hasOwnProperty.call(obj, "log");
        const onlyProxy = Object.keys(obj).length === 1 && Object.prototype.hasOwnProperty.call(obj, "proxy");
        if (!hasListen && !hasLog && !onlyProxy) return null;
        return naiveEntryFromProxyUrl(proxyStr, obj);
    } catch {
        return null;
    }
};

const parseNaiveURI = (line) => {
    let toParse = line.trim();
    if (toParse.startsWith("naive+https://")) toParse = toParse.slice("naive+".length);
    else if (toParse.startsWith("naive://")) toParse = toParse.replace(/^naive:\/\//, "https://");
    else return null;
    try {
        const u = new URL(toParse);
        const host = u.hostname;
        if (!host) return null;
        const port = u.port ? parseInt(u.port, 10) : 443;
        const username = decodeURIComponent(u.username || "");
        const password = decodeURIComponent(u.password || "");
        if (!username || !password) return null;
        const extra = {};
        const sni =
            u.searchParams.get("sni") ||
            u.searchParams.get("serverName") ||
            u.searchParams.get("server_name") ||
            u.searchParams.get("peer");
        if (sni) extra.sni = sni.trim();
        const ins = (
            u.searchParams.get("insecure") ||
            u.searchParams.get("allowInsecure") ||
            ""
        ).toLowerCase();
        if (["1", "true", "yes"].includes(ins)) extra.insecure = true;
        let name = "NaiveProxy";
        if (u.hash) {
            try {
                name = decodeURIComponent(u.hash.replace(/^#/, ""));
            } catch {
                name = u.hash.replace(/^#/, "");
            }
        }
        return {
            ip: host,
            port,
            type: "NAIVEPROXY",
            name,
            username,
            password,
            country: "\u{1F310}",
            extra: Object.keys(extra).length ? extra : undefined,
        };
    } catch {
        return null;
    }
};

export const parseProxies = (content) => {
    if (!content || typeof content !== "string") return [];

    const wireguardProxy = parseWireGuardConf(content);
    if (wireguardProxy) return [wireguardProxy];

    const naiveFromJson = parseNaiveClientJSON(content);
    if (naiveFromJson) return [naiveFromJson];

    const lines = content
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => line.length > 0);
    if (lines.length === 0) return [];

    
    const firstLine = lines[0].toLowerCase();
    if (firstLine.includes("ip") && firstLine.includes("port")) {
        return parseCSV(lines);
    }

    
    return lines.map((line) => parseLine(line)).filter((p) => p !== null);
};

const parseCSV = (lines) => {
    const headers = lines[0].split(/[;,]/).map((h) => h.trim().toLowerCase());
    const results = [];

    for (let i = 1; i < lines.length; i++) {
        const values = lines[i].split(/[;,]/).map((v) => v.trim());
        if (values.length < 2) continue;

        const proxy = {
            type: "HTTP", 
        };

        headers.forEach((header, index) => {
            const val = values[index] || "";
            if (header === "ip") proxy.ip = val;
            else if (header === "port") proxy.port = parseInt(val, 10);
            else if (header === "login" || header === "username" || header === "user")
                proxy.username = val;
            else if (header === "password" || header === "pass") proxy.password = val;
            else if (header === "type" || header === "protocol")
                proxy.type = val.toUpperCase();
            else if (header === "name") proxy.name = val;
        });

        if (proxy.ip && proxy.port) {
            results.push(proxy);
        }
    }

    return results;
};

const parseLine = (line) => {
    
    if (line.startsWith("ss://")) return parseShadowsocks(line);
    if (line.startsWith("vmess://")) return parseVMess(line);
    if (line.startsWith("vless://")) return parseVLESS(line);
    if (line.startsWith("trojan://")) return parseTrojan(line);
    if (line.startsWith("naive+https://") || line.startsWith("naive://")) return parseNaiveURI(line);
    if (line.startsWith("hy2://")) return parseHysteria2(line);
    if (line.startsWith("hysteria2://")) return parseHysteria2(line);

    // A scheme this parser does not know (awg://, wg://, anything added later)
    // must not reach the bare host:port branches below: they would split
    // "awg://1.2.3.4:51820?..." on ":" and invent an HTTP proxy out of it.
    // Returning null lets AddProxyView fall back to the Go parser, which does
    // understand those schemes.
    if (/^[a-z][a-z0-9+.-]*:\/\//i.test(line)) return null;


    if (line.includes("@")) {
        const [server, auth] = line.split("@");
        const [ip, port] = server.split(":");
        const [login, password] = (auth || "").split(":");
        if (ip && port) {
            return {
                ip,
                port: parseInt(port, 10),
                username: login || "",
                password: password || "",
                type: "HTTP",
                name: `${ip}:${port}`,
            };
        }
    }

    
    const parts = line.split(":");
    if (parts.length >= 2) {
        return {
            ip: parts[0],
            port: parseInt(parts[1], 10),
            username: parts[2] || "",
            password: parts[3] || "",
            type: "HTTP",
            name: `${parts[0]}:${parts[1]}`,
        };
    }

    return null;
};

const safeB64Decode = (str) => {
    try {
        
        const padding = str.length % 4 === 0 ? '' : '='.repeat(4 - (str.length % 4));
        return decodeURIComponent(escape(atob(str + padding)));
    } catch (e) {
        return "";
    }
};

const parseShadowsocks = (uri) => {
    try {
        let remainder = uri.replace("ss://", "");
        let name = "Shadowsocks";
        const hashIdx = remainder.indexOf("#");
        if (hashIdx >= 0) {
            name = decodeURIComponent(remainder.slice(hashIdx + 1)) || "Shadowsocks";
            remainder = remainder.slice(0, hashIdx);
        }

        // SIP002 puts the SIP003 plugin in the query string; the legacy
        // base64 form keeps it right after the blob. Split it off up front:
        // feeding "?plugin=..." straight into base64 decoding breaks the
        // whole link (legacy) or leaks into the port (SIP002).
        let query = "";
        const qIdx = remainder.indexOf("?");
        if (qIdx >= 0) {
            query = remainder.slice(qIdx + 1);
            remainder = remainder.slice(0, qIdx);
        }

        let method = "";
        let password = "";
        let host = "";
        let port = 0;

        if (remainder.includes("@")) {

            const [b64Auth, serverInfo] = remainder.split("@");
            const decodedAuth = safeB64Decode(b64Auth);
            if (decodedAuth) {
                [method, password] = decodedAuth.split(":");
            }
            [host, port] = serverInfo.split(":");
        } else {

            const decoded = safeB64Decode(remainder);
            if (decoded && decoded.includes("@")) {
                const [auth, serverInfo] = decoded.split("@");
                [method, password] = auth.split(":");
                [host, port] = serverInfo.split(":");
            }
        }

        if (host && port) {
            const extra = { method: method || "aes-256-gcm" };
            if (query) {
                const queryParams = new URLSearchParams(query);
                const raw = (queryParams.get("plugin") || "").trim();
                if (raw) {
                    // pluginName (not "name" — that would shadow the display
                    // name parsed above from the URI fragment).
                    let pluginName = raw;
                    let opts = "";
                    const semiIdx = raw.indexOf(";");
                    if (semiIdx >= 0) {
                        pluginName = raw.slice(0, semiIdx).trim();
                        opts = raw.slice(semiIdx + 1).trim();
                    }
                    extra.plugin = pluginName;
                    if (opts) {
                        // The core's SIP003 option parser errors out on a
                        // stray empty segment from consecutive/leading/
                        // trailing semicolons, and that error aborts
                        // outbound creation. Drop empty segments before they
                        // ever reach it.
                        const sanitizedOpts = opts
                            .split(";")
                            .filter((seg) => seg !== "")
                            .join(";");
                        if (sanitizedOpts) extra.plugin_opts = sanitizedOpts;
                    }
                }
            }
            return {
                ip: host,
                port: parseInt(port, 10),
                type: "SS",
                name: name,
                username: "",
                password: password || "",
                extra,
            };
        }
    } catch (e) {
        console.error("SS parse error", e);
    }
    return null;
};

const parseVMess = (uri) => {
    try {
        const b64 = uri.replace("vmess://", "");
        const decoded = safeB64Decode(b64);
        const json = JSON.parse(decoded);

        if (json.add && json.port) {
            const security = json.tls === "tls" ? "tls" : "none";
            const extra = {
                uuid: json.id,
                alterId: json.aid,
                network: json.net || "tcp",
                path: json.path || "",
                host: json.host || "",
                security: security,
                sni: json.sni || "",
                fp: json.fp || "",
                tls: security === "tls",
            };
            // In the vmess:// JSON, "type" is the mKCP header type and
            // "path" carries the mKCP seed — but only when the network is
            // kcp. For ws/tcp these two fields mean something else
            // entirely, so the mapping must stay gated on the network:
            // ws links must keep "path" as a path and must not gain a
            // headerType/seed.
            const net = String(json.net || "").trim().toLowerCase();
            if (net === "kcp" || net === "mkcp") {
                const headerType = String(json.type || "").trim();
                if (headerType) extra.headerType = headerType;
                const seed = String(json.path || "").trim();
                if (seed) extra.seed = seed;
            }
            return {
                ip: json.add,
                port: parseInt(json.port, 10),
                type: "VMESS",
                name: json.ps || "VMess",
                username: "",
                password: "",
                extra,
            };
        }
    } catch (e) {
        console.error("VMESS parse error", e);
    }
    return null;
};


const mergeVLESSURLEmbeddedExtra = (target, raw) => {
    if (raw == null || String(raw).trim() === "") return;
    let inner;
    try {
        inner = JSON.parse(raw);
    } catch {
        try {
            inner = JSON.parse(decodeURIComponent(raw));
        } catch {
            return;
        }
    }
    if (!inner || typeof inner !== "object" || Array.isArray(inner)) return;
    Object.assign(target, inner);
    if (!target.x_padding_bytes && inner.xPaddingBytes != null && inner.xPaddingBytes !== "") {
        target.x_padding_bytes = String(inner.xPaddingBytes);
    }
};

// applyDefaultIfAbsent sets extra[key] = fallback only when the key is not
// already present. Used for fields like "network"/"security" that carry a
// hard-coded default: the default must lose to a value already merged in
// from ?extra={...}, not silently overwrite it.
const applyDefaultIfAbsent = (extra, key, fallback) => {
    if (!Object.prototype.hasOwnProperty.call(extra, key)) extra[key] = fallback;
};

// applyQueryOverrides copies each entry of values into extra, but only when
// the query actually supplied a non-empty value for it. An empty/absent
// query param must not stomp a value ?extra={...} already provided — that
// silent overwrite is the defect class this whole pass exists to close.
const applyQueryOverrides = (extra, values) => {
    Object.keys(values).forEach((key) => {
        const value = values[key];
        if (value) extra[key] = value;
    });
};

// firstNonEmpty returns the first value that is non-empty after trimming,
// or "" when none of them are.
const firstNonEmpty = (...values) => {
    for (const v of values) {
        if (v != null && String(v).trim() !== "") return String(v).trim();
    }
    return "";
};

// applyMKCPQueryParams copies mKCP's query-string knobs into extra. Shared
// between vless:// and trojan:// — outbound.go's kcp/mkcp transport branch
// does not look at the proxy type, so a trojan link needs these knobs
// exactly as much as a vless one.
const applyMKCPQueryParams = (params, extra) => {
    ["seed", "headerType", "uplinkCapacity", "downlinkCapacity", "readBufferSize", "writeBufferSize"].forEach(
        (key) => {
            const v = (params.get(key) || "").trim();
            if (v) extra[key] = v;
        },
    );
    // mtu/tti/congestion must land as native number/boolean: the Go-side
    // outbound builder reads them through positiveIntFromExtra/getBoolField,
    // neither of which parses a quoted "1350"/"true" the way a plain string
    // field would — the value would simply be dropped.
    const mtu = (params.get("mtu") || "").trim();
    if (mtu) {
        const n = Number(mtu);
        if (Number.isInteger(n) && n > 0) extra.mtu = n;
    }
    const tti = (params.get("tti") || "").trim();
    if (tti) {
        const n = Number(tti);
        if (Number.isInteger(n) && n > 0) extra.tti = n;
    }
    const congestion = (params.get("congestion") || "").trim().toLowerCase();
    if (["1", "true", "yes", "on"].includes(congestion)) extra.congestion = true;
    else if (["0", "false", "no", "off"].includes(congestion)) extra.congestion = false;
};

const parseVLESS = (uri) => {
    try {
        const urlStr = uri.replace("vless://", "http://");
        const url = new URL(urlStr);
        const params = url.searchParams;

        const extra = {};
        mergeVLESSURLEmbeddedExtra(extra, params.get("extra"));

        extra.uuid = url.username;
        // A default here must not overwrite what ?extra={...} already
        // provided: fall back to it only when neither the query nor the
        // embedded extra carries the key. This is the fix for the
        // reality-collapses-to-none bug — the old unconditional assignment
        // ran after the embedded-extra merge and beat it with the plain-TLS
        // default every time security= was absent from the query.
        const networkParam = params.get("type");
        if (networkParam) extra.network = networkParam;
        else applyDefaultIfAbsent(extra, "network", "tcp");
        const securityParam = params.get("security");
        if (securityParam) extra.security = securityParam;
        else applyDefaultIfAbsent(extra, "security", "none");

        // Every other query knob only overwrites the embedded-extra value
        // when the query actually carries it non-empty — an absent query
        // param must not stomp a value ?extra={...} already supplied.
        applyQueryOverrides(extra, {
            sni: params.get("sni"),
            fp: params.get("fp"),
            pbk: params.get("pbk"),
            sid: params.get("sid"),
            flow: params.get("flow"),
            path: params.get("path"),
            host: params.get("host"),
            alpn: params.get("alpn"),
            mode: params.get("mode"),
            method: params.get("method"),
        });
        const encryption = params.get("encryption");
        if (encryption) extra.encryption = encryption;

        const grpcServiceName = firstNonEmpty(
            params.get("grpc-service-name"),
            params.get("serviceName"),
            params.get("service_name"),
            params.get("grpc_service_name"),
        );
        if (grpcServiceName) {
            extra["grpc-service-name"] = grpcServiceName;
            extra.serviceName = grpcServiceName;
        }
        const authority = firstNonEmpty(
            params.get("authority"),
            params.get("grpc-authority"),
            params.get("grpc_authority"),
        );
        if (authority) extra.authority = authority;

        applyMKCPQueryParams(params, extra);

        return {
            ip: url.hostname,
            port: parseInt(url.port, 10),
            type: "VLESS",
            name: decodeURIComponent(url.hash.replace("#", "") || "VLESS"),
            username: "",
            password: "",
            extra,
        };
    } catch (e) {
        console.error("VLESS parse error", e);
    }
    return null;
};

const parseTrojan = (uri) => {
    try {
        const urlStr = uri.replace("trojan://", "http://");
        const url = new URL(urlStr);
        const params = url.searchParams;

        const extra = {};
        mergeVLESSURLEmbeddedExtra(extra, params.get("extra"));

        // network decides which alias order the sni fallback below uses
        // (grpc prefers sni over peer, everything else prefers peer), so it
        // must be resolved first — with the same "query wins over embedded,
        // embedded wins over the tcp default" priority as everything else
        // here.
        const networkParam = firstNonEmpty(params.get("network"), params.get("type"));
        if (networkParam) extra.network = networkParam.toLowerCase();
        else applyDefaultIfAbsent(extra, "network", "tcp");
        const network = String(extra.network || "tcp").trim().toLowerCase();
        const isGrpcNetwork = network === "grpc";

        const sni = isGrpcNetwork
            ? firstNonEmpty(
                  params.get("sni"),
                  params.get("peer"),
                  params.get("serverName"),
                  params.get("servername"),
                  params.get("server_name"),
              )
            : firstNonEmpty(
                  params.get("sni"),
                  params.get("serverName"),
                  params.get("servername"),
                  params.get("server_name"),
                  params.get("peer"),
              );

        const securityParam = params.get("security");
        if (securityParam) extra.security = securityParam;
        else applyDefaultIfAbsent(extra, "security", "tls");

        // Every other query knob only overwrites the embedded-extra value
        // when the query actually carries it non-empty — an absent query
        // param must not stomp a value ?extra={...} already supplied.
        applyQueryOverrides(extra, {
            sni,
            fp: params.get("fp"),
            path: params.get("path"),
            host: params.get("host"),
            mode: params.get("mode"),
            method: params.get("method"),
            alpn: params.get("alpn"),
            pbk: params.get("pbk"),
            sid: params.get("sid"),
            spx: params.get("spx"),
            flow: params.get("flow"),
        });

        // insecure is only stored when at least one of its aliases is
        // actually present in the query, regardless of the value it
        // carries — an absent alias must not stomp a value ?extra={...}
        // already supplied. Otherwise allowInsecure=0 would stop being
        // distinguishable from "no alias at all" and clobber the embedded
        // value.
        const insecureAliases = ["insecure", "allowInsecure", "allow_insecure", "skip-cert-verify", "skip_cert_verify"];
        if (insecureAliases.some((key) => params.has(key))) {
            const insecureRaw = firstNonEmpty(
                params.get("insecure"),
                params.get("allowInsecure"),
                params.get("allow_insecure"),
                params.get("skip-cert-verify"),
                params.get("skip_cert_verify"),
            ).toLowerCase();
            extra.insecure = ["1", "true", "yes", "on"].includes(insecureRaw);
        }

        const grpcServiceName = firstNonEmpty(
            params.get("grpc-service-name"),
            params.get("serviceName"),
            params.get("service_name"),
            params.get("grpc_service_name"),
        );
        if (grpcServiceName) {
            extra["grpc-service-name"] = grpcServiceName;
            extra.serviceName = grpcServiceName;
        }
        const authority = firstNonEmpty(
            params.get("authority"),
            params.get("grpc-authority"),
            params.get("grpc_authority"),
        );
        if (authority) extra.authority = authority;

        applyMKCPQueryParams(params, extra);

        return {
            ip: url.hostname,
            port: parseInt(url.port, 10),
            type: "TROJAN",
            name: decodeURIComponent(url.hash.replace("#", "") || "Trojan"),
            username: "",
            password: url.username,
            extra,
        };
    } catch (e) {
        console.error("Trojan parse error", e);
    }
    return null;
};

const parseHysteria2 = (uri) => {
    try {
        const normalized = uri.startsWith("hysteria2://")
            ? uri.replace("hysteria2://", "hy2://")
            : uri;
        const urlStr = normalized.replace("hy2://", "http://");
        const url = new URL(urlStr);
        const params = url.searchParams;
        const insecureRaw = String(params.get("insecure") || "").trim().toLowerCase();
        const insecure = insecureRaw === "1" || insecureRaw === "true" || insecureRaw === "yes" || insecureRaw === "on";

        const extra = {
            password: url.username || "",
            sni: params.get("sni") || "",
            alpn: params.get("alpn") || "",
            insecure,
            obfs_type: params.get("obfs") || "",
            obfs_password: params.get("obfs-password") || "",
        };
        // Port hopping: the Go-side outbound builder turns these into
        // server_ports; this parser only needs to carry the raw values
        // through under the names it reads.
        const mport = firstNonEmpty(params.get("mport"), params.get("ports"));
        if (mport) extra.mport = mport;
        const hopInterval = firstNonEmpty(params.get("hop-interval"), params.get("hopInterval"));
        if (hopInterval) extra.hop_interval = hopInterval;

        return {
            ip: url.hostname,
            port: parseInt(url.port, 10),
            type: "HYSTERIA2",
            name: decodeURIComponent(url.hash.replace("#", "") || "Hysteria2"),
            username: "",
            password: "",
            extra,
        };
    } catch (e) {
        console.error("Hysteria2 parse error", e);
    }
    return null;
};

const parseWireGuardConf = (content) => {
    const lines = String(content || "").split(/\r?\n/);
    let section = "";
    const iface = {};
    const peer = {};

    const readValue = (src) => {
        const cut = src.split("#")[0].split(";")[0];
        return cut.trim();
    };

    for (const raw of lines) {
        const line = raw.trim();
        if (!line) continue;
        if (line.startsWith("[") && line.endsWith("]")) {
            section = line.slice(1, -1).trim().toLowerCase();
            continue;
        }
        const idx = line.indexOf("=");
        if (idx < 0) continue;
        const key = line.slice(0, idx).trim().toLowerCase();
        const val = readValue(line.slice(idx + 1));
        if (!val) continue;
        if (section === "interface") iface[key] = val;
        if (section === "peer") peer[key] = val;
    }

    if (!iface.privatekey || !peer.publickey || !peer.endpoint) return null;
    const endpoint = peer.endpoint;
    const parseEndpoint = (value) => {
        const ep = String(value || "").trim();
        if (!ep) return null;
        if (ep.startsWith("[") && ep.includes("]:")) {
            const idx = ep.lastIndexOf("]:");
            const h = ep.slice(1, idx).trim();
            const p = parseInt(ep.slice(idx + 2).trim(), 10);
            if (!h || !Number.isFinite(p) || p <= 0) return null;
            return { host: h, port: p };
        }
        const idx = ep.lastIndexOf(":");
        if (idx < 0) return null;
        const h = ep.slice(0, idx).trim();
        const p = parseInt(ep.slice(idx + 1).trim(), 10);
        if (!h || !Number.isFinite(p) || p <= 0) return null;
        return { host: h, port: p };
    };
    const parsedEndpoint = parseEndpoint(endpoint);
    if (!parsedEndpoint) return null;
    const host = parsedEndpoint.host;
    const port = parsedEndpoint.port;
    if (!host || !port) return null;

    const splitCSV = (v) =>
        String(v || "")
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean);

    const toInt = (v) => {
        const n = parseInt(String(v || "").trim(), 10);
        return Number.isFinite(n) ? n : 0;
    };
    const amnezia = {};
    // j1-j3 and itime are intentionally absent: the wireguard-go fork behind
    // the engine has no UAPI device key for them, so emitting any of them
    // makes IpcSet fail and the endpoint never starts.
    const amneziaIntKeys = ["jc", "jmin", "jmax", "s1", "s2", "s3", "s4"];
    const amneziaHeaderKeys = ["h1", "h2", "h3", "h4"];
    const amneziaStringKeys = ["i1", "i2", "i3", "i4", "i5"];
    // AmneziaWG 3.0 device knobs. .conf keys arrive lowercased with the
    // separators dropped (HeaderProtectionKey -> headerprotectionkey), so map
    // them back onto their canonical snake_case names.
    const amneziaAWG3Keys = {
        headerprotectionkey: "header_protection_key",
        contentpaddingaddition: "content_padding_addition",
        rekeyaftertime: "rekey_after_time",
        rekeytimeout: "rekey_timeout",
        rejectaftertime: "reject_after_time",
        keepalivetimeout: "keepalive_timeout",
        maxhandshakeattempts: "max_handshake_attempts",
    };
    for (const key of amneziaIntKeys) {
        if (iface[key] != null && String(iface[key]).trim() !== "") {
            const n = toInt(iface[key]);
            if (n > 0) amnezia[key] = n;
        }
    }
    // H1-H4 may be a single uint32 (AWG 1.0) or a "low-high" range string
    // (AWG 2.0). Preserve strings as-is so the Go side can pick a random
    // value within the range at engine build time. Plain numbers are kept
    // as numbers for backward compatibility.
    for (const key of amneziaHeaderKeys) {
        const raw = iface[key];
        if (raw == null) continue;
        const s = String(raw).trim();
        if (!s) continue;
        if (s.includes("-")) {
            amnezia[key] = s;
        } else {
            const n = toInt(s);
            if (n > 0) amnezia[key] = n;
        }
    }
    for (const key of amneziaStringKeys) {
        if (iface[key] != null && String(iface[key]).trim() !== "") {
            amnezia[key] = String(iface[key]).trim();
        }
    }
    for (const [confKey, canonical] of Object.entries(amneziaAWG3Keys)) {
        if (iface[confKey] != null && String(iface[confKey]).trim() !== "") {
            amnezia[canonical] = String(iface[confKey]).trim();
        }
    }
    const hasAmnezia = Object.keys(amnezia).length > 0;

    const extra = {
        private_key: iface.privatekey,
        public_key: peer.publickey,
        allowed_ips: splitCSV(peer.allowedips),
        address: splitCSV(iface.address),
        system: false,
    };
    if (iface.presharedkey) extra.pre_shared_key = iface.presharedkey;
    if (peer.presharedkey) extra.pre_shared_key = peer.presharedkey;

    const keepaliveRaw = peer.persistentkeepalive || iface.persistentkeepalive;
    const keepalive = parseInt(keepaliveRaw || "", 10);
    if (keepalive) extra.persistent_keepalive_interval = keepalive;

    const mtu = parseInt(iface.mtu || "", 10);
    if (mtu) extra.mtu = mtu;
    const listenPort = parseInt(iface.listenport || "", 10);
    if (listenPort) extra.listen_port = listenPort;
    const reserved = splitCSV(peer.reserved).map((x) => parseInt(x, 10)).filter((n) => Number.isFinite(n));
    if (reserved.length) extra.reserved = reserved;
    const dns = splitCSV(iface.dns);
    if (dns.length) {
        extra.dns_servers = dns;
        extra.dns = dns;
    }
    if (hasAmnezia) extra.amnezia = amnezia;

    return {
        ip: host,
        port,
        type: hasAmnezia ? "AMNEZIAWG" : "WIREGUARD",
        name: hasAmnezia ? "AmneziaWG" : "WireGuard",
        username: "",
        password: "",
        extra,
    };
};

/*
 * Протоколы-«эндпоинты»: это не прокси, а сетевой интерфейс, и живут они
 * только через TUN. Режим «прокси» им недоступен, а туннель требует прав
 * администратора — на оба вывода опирается главный экран.
 */
export const ENDPOINT_TYPES = ["WIREGUARD", "AMNEZIAWG"];

export const isEndpointProtocol = (proxy) =>
    ENDPOINT_TYPES.includes(String(proxy?.type || "").toUpperCase());

export const VPN_TYPES = [
    "SS",
    "VMESS",
    "VLESS",
    "TROJAN",
    "WIREGUARD",
    "AMNEZIAWG",
    "HYSTERIA2",
    "NAIVEPROXY",
    "AUTO",
];

const FLAG_EMOJI_PREFIX = /^[\u{1F1E6}-\u{1F1FF}][\u{1F1E6}-\u{1F1FF}]\s*/u;


export const formatProxyDisplayName = (name, countryCode) => {
    if (!name || typeof name !== "string") return name || "";
    let s = name.trim().replace(FLAG_EMOJI_PREFIX, "");
    const cc = (countryCode || "").toString().toLowerCase();
    if (/^[a-z]{2}$/.test(cc)) {
        const re = new RegExp(`^${cc}\\s+`, "i");
        const next = s.replace(re, "").trim();
        if (next) s = next;
    }
    return s.trim() || name;
};


export const mergeSubscriptionRefreshCountries = (
    prevProxies,
    updatedProxies,
    subscriptionURL,
) => {
    const oldSub = prevProxies.filter((p) => p.subscriptionUrl === subscriptionURL);
    const keyOf = (p) =>
        `${p.ip}|${parseInt(p.port, 10) || 0}|${String(p.type || "").toUpperCase()}`;

    const sectionOrdinal = (arr, idx) => {
        let n = 0;
        for (let j = 0; j < idx; j++) {
            if (String(arr[j]?.type || "").toUpperCase() === "SECTION") n++;
        }
        return n;
    };

    const rowKey = (arr, p, idx) => {
        const t = String(p.type || "").toUpperCase();
        if (t === "SECTION") {
            return `section|${sectionOrdinal(arr, idx)}|${String(p.name || "").trim()}`;
        }
        // An AUTO head carries no address: keyOf would hash every head to
        // "|0|AUTO" and collapse a provider's sections into one row. The
        // section name is its identity — it is what the head IS.
        if (t === "AUTO") {
            return `auto|${String(p.name || "").trim()}`;
        }
        return keyOf(p);
    };

    // Use namespace-separated keys to prevent collision between auto members and
    // individual servers that share the same ip:port:type. Members use key|member,
    // individuals use key. Both types get proper ID continuity across refreshes.
    const parseRawExtraInline = (raw) => {
        if (!raw) return {};
        if (raw instanceof Uint8Array || Array.isArray(raw)) {
            try { return JSON.parse(String.fromCharCode(...raw)); } catch { return {}; }
        }
        if (typeof raw === "string") { try { return JSON.parse(raw); } catch { return {}; } }
        return raw || {};
    };
    // Sections routinely list the SAME backends (impio ships one pool per
    // tier over one node set), so ip:port:type alone cannot tell a member of
    // "Авто" from a member of "Авто | Когда не глушат". autoGroup can.
    const memberKey = (p) => `${keyOf(p)}|member|${String(p.autoGroup || "").trim()}`;

    const oldMemberIds = new Set();
    oldSub.forEach((p) => {
        if (p.type?.toUpperCase() === "AUTO") {
            const extra = parseRawExtraInline(p.extra);
            (extra?.members || []).forEach((id) => oldMemberIds.add(String(id)));
        }
    });
    const oldBy = new Map();
    oldSub.forEach((p, idx) => {
        const key = oldMemberIds.has(String(p.id)) ? memberKey(p) : rowKey(oldSub, p, idx);
        oldBy.set(key, p);
    });

    const freshMemberIds = new Set();
    updatedProxies.forEach((p) => {
        if (p.type?.toUpperCase() === "AUTO") {
            const extra = parseRawExtraInline(p.extra);
            (extra?.members || []).forEach((id) => freshMemberIds.add(String(id)));
        }
    });

    // First pass: build merged array with country/id continuity
    const merged = updatedProxies.map((p, idx) => {
        const key = freshMemberIds.has(String(p.id)) ? memberKey(p) : rowKey(updatedProxies, p, idx);
        const old = oldBy.get(key);
        const port = parseInt(p.port, 10) || 0;
        const base = {
            ...p,
            port,
            id: old ? String(old.id) : String(p.id),
        };
        const c = base.country;
        const bad =
            !c || c === "unknown" || c === "\u{1F310}" || c === "Unknown";
        if (
            old &&
            bad &&
            old.country &&
            old.country !== "unknown" &&
            old.country !== "\u{1F310}" &&
            old.country !== "Unknown"
        ) {
            return { ...base, country: old.country };
        }
        return base;
    });

    // Second pass: fix AUTO entry member IDs.
    // The backend assigns fresh time-based IDs each refresh, but the first pass
    // replaces member IDs with old ones for ping continuity. The AUTO entry's
    // extra.members still references fresh backend IDs — remap them to merged IDs.
    const freshToMerged = new Map(
        updatedProxies.map((p, i) => [String(p.id), String(merged[i].id)])
    );
    const parseRawExtra = (raw) => {
        if (!raw) return {};
        if (raw instanceof Uint8Array || Array.isArray(raw)) {
            try { return JSON.parse(String.fromCharCode(...raw)); } catch { return {}; }
        }
        if (typeof raw === "string") { try { return JSON.parse(raw); } catch { return {}; } }
        return raw || {};
    };
    return merged.map((p) => {
        if (p.type?.toUpperCase() !== "AUTO") return p;
        const extra = parseRawExtra(p.extra);
        if (!Array.isArray(extra?.members)) return p;
        const fixedMembers = extra.members.map((id) => freshToMerged.get(String(id)) ?? String(id));
        return { ...p, extra: { ...extra, members: fixedMembers } };
    });
};


export const subscriptionLabelFromURL = (urlStr) => {
    try {
        const u = new URL(urlStr.trim());
        const host = u.hostname.replace(/^www\./i, "");
        const parts = host.split(".");
        if (parts.length >= 2) {
            const n = parts[parts.length - 2];
            if (n) return n.charAt(0).toUpperCase() + n.slice(1).toLowerCase();
        }
        return host || "Subscription";
    } catch {
        return "Subscription";
    }
};

export const isVpnType = (type) => {
    const t = type?.toUpperCase();
    if (t === "SECTION") return false;
    return VPN_TYPES.includes(t);
};

export const isSubscriptionURL = (text) => {
    const trimmed = text.trim();
    return /^https?:\/\/.+/.test(trimmed) && !trimmed.includes("\n");
};

export const isEncryptedSubscription = (text) => {
    const trimmed = text.trim();
    return (
        trimmed.startsWith("RVSUB1:") ||
        /^resultv:(\/\/)?/i.test(trimmed)
    );
};

export const VPN_NETWORK_OPTIONS = ["tcp", "ws", "grpc", "h2", "http", "xhttp", "httpupgrade", "kcp"];

export const parseProxyExtra = (raw) => {
    if (raw == null || raw === "") return {};
    if (typeof raw === "object" && !Array.isArray(raw)) return { ...raw };
    if (typeof raw === "string") {
        try {
            const o = JSON.parse(raw);
            return typeof o === "object" && o !== null && !Array.isArray(o) ? { ...o } : {};
        } catch {
            return {};
        }
    }
    return {};
};

export const normalizeNetworkForSelect = (network) => {
    let n = String(network || "tcp").toLowerCase();
    if (n === "websocket") return "ws";
    if (!VPN_NETWORK_OPTIONS.includes(n)) return "tcp";
    return n;
};

export const normalizeSecurityForSelect = (security) => {
    const s = String(security || "none").toLowerCase();
    if (s === "none" || s === "tls" || s === "reality") return s;
    return "none";
};

const sanitizeAlpnIfNotXhttp = (ex, network) => {
    const net = String(network || "tcp").toLowerCase();
    if (net === "xhttp" || net === "splithttp") return;
    const alpn = ex.alpn;
    if (typeof alpn !== "string" || !alpn.trim()) return;
    const parts = alpn.split(/[,\s]+/).map((s) => s.trim()).filter(Boolean);
    if (parts.length === 0 || parts[0] !== "h3") return;
    const rest = parts.slice(1).filter((p) => p !== "h3");
    const out = [];
    const seen = new Set();
    for (const p of ["h2", "http/1.1", ...rest]) {
        if (p && !seen.has(p)) {
            seen.add(p);
            out.push(p);
        }
    }
    ex.alpn = out.join(",");
};

export const readVpnTransportFieldsFromExtra = (extra) => {
    const ex = parseProxyExtra(extra);
    const p = (v) => (v != null && v !== undefined ? String(v) : "");
    return {
        transPath: p(ex.path || ex["ws-path"]) || "/",
        transHost: p(ex.host || ex["ws-host"]),
        grpcService: p(ex.serviceName || ex.service_name || ex["grpc-service-name"]),
        httpHost: p(ex["http-host"]),
        httpPath: p(ex["http-path"]) || "/",
        xhttpMode: p(ex.mode) || "auto",
    };
};

export const applyVpnTransportFieldsToExtra = (extra, network, fields) => {
    const ex = parseProxyExtra(extra);
    const net = String(network || "tcp").toLowerCase();
    const f = fields || {};
    if (net === "ws" || net === "websocket" || net === "httpupgrade") {
        ex.path = f.transPath ? String(f.transPath) : "/";
        ex.host = f.transHost != null ? String(f.transHost) : "";
    } else if (net === "grpc") {
        ex.serviceName = f.grpcService != null ? String(f.grpcService) : "";
    } else if (net === "http" || net === "h2") {
        ex["http-host"] = f.httpHost != null ? String(f.httpHost) : "";
        ex["http-path"] = f.httpPath ? String(f.httpPath) : "/";
    } else if (net === "xhttp" || net === "splithttp") {
        ex.path = f.transPath ? String(f.transPath) : "/";
        ex.host = f.transHost != null ? String(f.transHost) : "";
        ex.mode = f.xhttpMode ? String(f.xhttpMode) : "auto";
    }
    // "kcp" intentionally has no branch here: there is no form UI for its
    // mKCP knobs (seed/headerType), so this must fall through and leave the
    // extra map untouched rather than guess at defaults that would clobber
    // what the link/subscription already put there.
    return ex;
};

export const sanitizeVpnExtraForEdit = (extra, { type, network, security, uuid, ssMethod }) => {
    const t = String(type || "").toUpperCase();
    const ex = parseProxyExtra(extra);
    const net = String(network || "tcp").toLowerCase();
    const sec = String(security || "none").toLowerCase();

    if (t === "VLESS" || t === "VMESS" || t === "TROJAN") {
        ex.network = net;
    }

    if (t === "VLESS" || t === "VMESS" || t === "TROJAN") {
        ex.security = sec;
        if (t !== "TROJAN") {
            ex.uuid = uuid;
        }
        if (sec === "none") {
            delete ex.tls;
            const f = String(ex.flow || "");
            if (/xtls|vision/i.test(f)) ex.flow = "";
        }
        sanitizeAlpnIfNotXhttp(ex, net);
    }

    if (t === "SS") {
        ex.method = (ssMethod || "").trim() || "aes-256-gcm";
    }

    return ex;
};

export const getProtocolLabel = (proxy) => {
    if (!proxy?.extra || !isVpnType(proxy.type)) return proxy?.type || "";
    if (proxy.type?.toUpperCase() === "AUTO") return "AUTO";
    const extra = typeof proxy.extra === "string" ? JSON.parse(proxy.extra) : proxy.extra;
    const type = proxy.type?.toUpperCase();
    const security = extra.security || "";
    const network = extra.network || "tcp";

    let label = type;
    if (security === "reality") label += " + Reality";
    else if (security === "tls") label += " + TLS";
    if (network === "ws" || network === "websocket") label += " + WS";
    else if (network === "grpc") label += " + gRPC";
    else if (network === "xhttp") label += " + XHTTP";
    else if (network === "h2" || network === "http") label += " + H2";
    return label;
};

const stableHash32 = (s) => {
    const str = String(s || "");
    let h = 2166136261;
    for (let i = 0; i < str.length; i++) {
        h ^= str.charCodeAt(i);
        h = Math.imul(h, 16777619);
    }
    return (h >>> 0);
};

export const rebuildSubscriptionsFromProxies = (proxies) => {
    const list = Array.isArray(proxies) ? proxies : [];
    const byUrl = new Map();
    for (const p of list) {
        const subUrl = (p?.subscriptionUrl || "").toString().trim();
        if (!subUrl) continue;
        if (byUrl.has(subUrl)) continue;
        const provider = (p?.provider || "").toString().trim();
        const name = provider || subscriptionLabelFromURL(subUrl);
        byUrl.set(subUrl, {
            id: String(stableHash32(subUrl)),
            name,
            url: subUrl,
            updatedAt: "",
            trafficUpload: 0,
            trafficDownload: 0,
            trafficTotal: 0,
            expireUnix: 0,
            iconUrl: "",
        });
    }
    return Array.from(byUrl.values());
};
