/*
 * Copyright (C) 2026 ResultProxy
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

/**
 * Парсит строку или содержимое файла с прокси.
 * Поддерживает форматы:
 * 1. CSV с заголовками (ip,port,login,password)
 * 2. ip:port:login:password
 * 3. ip:port@login:password
 * 4. URI: ss://, vmess://, vless://, trojan://
 */
export const parseProxies = (content) => {
    if (!content || typeof content !== "string") return [];

    const lines = content
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => line.length > 0);
    if (lines.length === 0) return [];

    // Проверка на CSV с заголовками
    const firstLine = lines[0].toLowerCase();
    if (firstLine.includes("ip") && firstLine.includes("port")) {
        return parseCSV(lines);
    }

    // Парсинг TXT форматов и URI
    return lines.map((line) => parseLine(line)).filter((p) => p !== null);
};

const parseCSV = (lines) => {
    const headers = lines[0].split(/[;,]/).map((h) => h.trim().toLowerCase());
    const results = [];

    for (let i = 1; i < lines.length; i++) {
        const values = lines[i].split(/[;,]/).map((v) => v.trim());
        if (values.length < 2) continue;

        const proxy = {
            type: "HTTP", // Значение по умолчанию
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
    // В первую очередь проверяем URI
    if (line.startsWith("ss://")) return parseShadowsocks(line);
    if (line.startsWith("vmess://")) return parseVMess(line);
    if (line.startsWith("vless://")) return parseVLESS(line);
    if (line.startsWith("trojan://")) return parseTrojan(line);

    // Формат ip:port@login:password
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

    // Формат ip:port:login:password или ip:port
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
        // Добавляем паддинг если нужно
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
            // SIP002 формат (начинается с base64-кодированного метода и пароля)
            const [b64Auth, serverInfo] = mainPart.split("@");
            const decodedAuth = safeB64Decode(b64Auth);
            if (decodedAuth) {
                [method, password] = decodedAuth.split(":");
            }
            [host, port] = serverInfo.split(":");
        } else {
            // Устаревший формат (base64 кодируется вся строка method:pass@host:port)
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

/** Merge Xray `extra` query JSON into target; mirrors Go mergeVLESSURLEmbeddedExtra. */
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

        return {
            ip: url.hostname,
            port: parseInt(url.port, 10),
            type: "TROJAN",
            name: decodeURIComponent(url.hash.replace("#", "") || "Trojan"),
            username: "",
            password: url.username,
            extra: {
                security: params.get("security") || "tls",
                sni: params.get("sni") || "",
                fp: params.get("fp") || "",
                network: params.get("type") || "tcp",
                path: params.get("path") || "",
                host: params.get("host") || "",
                alpn: params.get("alpn") || "",
            },
        };
    } catch (e) {
        console.error("Trojan parse error", e);
    }
    return null;
};

export const VPN_TYPES = ["SS", "VMESS", "VLESS", "TROJAN"];

const FLAG_EMOJI_PREFIX = /^[\u{1F1E6}-\u{1F1FF}][\u{1F1E6}-\u{1F1FF}]\s*/u;

/** Убирает дублирование: эмодзи-флаг и префикс кода страны, если он совпадает с proxy.country (например «FI Finland» → «Finland»). */
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

/** После refresh подписки сохраняем country с прежних узлов (совпадение ip|port|type). */
export const mergeSubscriptionRefreshCountries = (
    prevProxies,
    updatedProxies,
    subscriptionURL,
) => {
    const oldSub = prevProxies.filter((p) => p.subscriptionUrl === subscriptionURL);
    const keyOf = (p) =>
        `${p.ip}|${p.port}|${String(p.type || "").toUpperCase()}`;
    const oldBy = new Map(oldSub.map((p) => [keyOf(p), p]));
    return updatedProxies.map((p) => {
        const old = oldBy.get(keyOf(p));
        const c = p.country;
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
            return { ...p, country: old.country };
        }
        return p;
    });
};

/** Имя группы подписки из URL (как extractProviderName в Go). */
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

export const getProtocolLabel = (proxy) => {
    if (!proxy?.extra || !isVpnType(proxy.type)) return proxy?.type || "";
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
