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

export const parseProxies = (content) => {
    if (!content || typeof content !== "string") return [];

    const wireguardProxy = parseWireGuardConf(content);
    if (wireguardProxy) return [wireguardProxy];


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
    if (line.startsWith("hy2://")) return parseHysteria2(line);
    if (line.startsWith("hysteria2://")) return parseHysteria2(line);

    
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
        const urlPart = uri.replace("ss://", "");
        let mainPart = urlPart.split("#")[0];
        const name = decodeURIComponent(urlPart.split("#")[1] || "Shadowsocks");

        let method = "";
        let password = "";
        let host = "";
        let port = 0;

        if (mainPart.includes("@")) {
            
            const [b64Auth, serverInfo] = mainPart.split("@");
            const decodedAuth = safeB64Decode(b64Auth);
            if (decodedAuth) {
                [method, password] = decodedAuth.split(":");
            }
            [host, port] = serverInfo.split(":");
        } else {
            
            const decoded = safeB64Decode(mainPart);
            if (decoded && decoded.includes("@")) {
                const [auth, serverInfo] = decoded.split("@");
                [method, password] = auth.split(":");
                [host, port] = serverInfo.split(":");
            }
        }

        if (host && port) {
            return {
                ip: host,
                port: parseInt(port, 10),
                type: "SS",
                name: name,
                username: "",
                password: password || "",
                extra: { method: method || "aes-256-gcm" },
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
            return {
                ip: json.add,
                port: parseInt(json.port, 10),
                type: "VMESS",
                name: json.ps || "VMess",
                username: "",
                password: "",
                extra: {
                    uuid: json.id,
                    alterId: json.aid,
                    network: json.net || "tcp",
                    path: json.path || "",
                    host: json.host || "",
                    security: security,
                    sni: json.sni || "",
                    fp: json.fp || "",
                    tls: security === "tls",
                },
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

const parseVLESS = (uri) => {
    try {
        const urlStr = uri.replace("vless://", "http://");
        const url = new URL(urlStr);
        const params = url.searchParams;

        const extra = {};
        mergeVLESSURLEmbeddedExtra(extra, params.get("extra"));

        extra.uuid = url.username;
        extra.network = params.get("type") || "tcp";
        extra.security = params.get("security") || "none";
        extra.sni = params.get("sni") || "";
        extra.fp = params.get("fp") || "";
        extra.pbk = params.get("pbk") || "";
        extra.sid = params.get("sid") || "";
        extra.flow = params.get("flow") || "";
        extra.path = params.get("path") || "";
        extra.host = params.get("host") || "";
        extra.alpn = params.get("alpn") || "";
        extra.mode = params.get("mode") || "";
        extra.method = params.get("method") || "";

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
        const network = String(params.get("network") || params.get("type") || "tcp").toLowerCase();
        const sni = params.get("sni") ||
            params.get("serverName") ||
            params.get("servername") ||
            params.get("server_name") ||
            params.get("peer") ||
            "";
        const insecureRaw = String(
            params.get("insecure") ||
            params.get("allowInsecure") ||
            params.get("allow_insecure") ||
            params.get("skip-cert-verify") ||
            params.get("skip_cert_verify") ||
            ""
        ).trim().toLowerCase();
        const insecure = insecureRaw === "1" || insecureRaw === "true" || insecureRaw === "yes" || insecureRaw === "on";
        const grpcServiceName =
            params.get("grpc-service-name") ||
            params.get("serviceName") ||
            params.get("service_name") ||
            params.get("grpc_service_name") ||
            "";
        const authority =
            params.get("authority") ||
            params.get("grpc-authority") ||
            params.get("grpc_authority") ||
            "";

        return {
            ip: url.hostname,
            port: parseInt(url.port, 10),
            type: "TROJAN",
            name: decodeURIComponent(url.hash.replace("#", "") || "Trojan"),
            username: "",
            password: url.username,
            extra: {
                security: params.get("security") || "tls",
                sni,
                fp: params.get("fp") || "",
                network,
                path: params.get("path") || "",
                host: params.get("host") || "",
                alpn: params.get("alpn") || "",
                insecure,
                peer: params.get("peer") || "",
                "grpc-service-name": grpcServiceName,
                serviceName: grpcServiceName,
                authority,
                pbk: params.get("pbk") || "",
                sid: params.get("sid") || "",
                spx: params.get("spx") || "",
                flow: params.get("flow") || "",
            },
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

        return {
            ip: url.hostname,
            port: parseInt(url.port, 10),
            type: "HYSTERIA2",
            name: decodeURIComponent(url.hash.replace("#", "") || "Hysteria2"),
            username: "",
            password: "",
            extra: {
                password: url.username || "",
                sni: params.get("sni") || "",
                alpn: params.get("alpn") || "",
                insecure,
                obfs_type: params.get("obfs") || "",
                obfs_password: params.get("obfs-password") || "",
            },
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
    const amneziaIntKeys = ["jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "itime"];
    const amneziaStringKeys = ["i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3"];
    for (const key of amneziaIntKeys) {
        if (iface[key] != null && String(iface[key]).trim() !== "") {
            const n = toInt(iface[key]);
            if (n > 0) amnezia[key] = n;
        }
    }
    for (const key of amneziaStringKeys) {
        if (iface[key] != null && String(iface[key]).trim() !== "") {
            amnezia[key] = String(iface[key]).trim();
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

export const VPN_TYPES = [
    "SS",
    "VMESS",
    "VLESS",
    "TROJAN",
    "WIREGUARD",
    "AMNEZIAWG",
    "HYSTERIA2",
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
    const memberKey = (p) => `${keyOf(p)}|member`;

    const oldMemberIds = new Set();
    oldSub.forEach((p) => {
        if (p.type?.toUpperCase() === "AUTO") {
            const extra = parseRawExtraInline(p.extra);
            (extra?.members || []).forEach((id) => oldMemberIds.add(String(id)));
        }
    });
    const oldBy = new Map();
    oldSub.forEach((p) => {
        const key = oldMemberIds.has(String(p.id)) ? memberKey(p) : keyOf(p);
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
    const merged = updatedProxies.map((p) => {
        const key = freshMemberIds.has(String(p.id)) ? memberKey(p) : keyOf(p);
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

export const isVpnType = (type) => VPN_TYPES.includes(type?.toUpperCase());

export const isSubscriptionURL = (text) => {
    const trimmed = text.trim();
    return /^https?:\/\/.+/.test(trimmed) && !trimmed.includes("\n");
};

export const isEncryptedSubscription = (text) => {
    return text.trim().startsWith("RVSUB1:");
};

export const VPN_NETWORK_OPTIONS = ["tcp", "ws", "grpc", "h2", "http", "xhttp"];

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
    if (net === "ws" || net === "websocket") {
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
