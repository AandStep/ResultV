import assert from "node:assert/strict";
import test from "node:test";
import { parseProxies } from "./proxyParser.js";

test("imports an authenticated SOCKS5 URL alongside a bare proxy address", () => {
    const proxies = parseProxies("socks5://test%40user:p%3Aa%25ss@192.0.2.10:8000\n192.0.2.11:3128");

    assert.deepEqual(proxies, [
        {
            ip: "192.0.2.10",
            port: 8000,
            username: "test@user",
            password: "p:a%ss",
            type: "SOCKS5",
            name: "192.0.2.10:8000",
        },
        {
            ip: "192.0.2.11",
            port: 3128,
            username: "",
            password: "",
            type: "HTTP",
            name: "192.0.2.11:3128",
        },
    ]);
});

test("imports an unauthenticated SOCKS5 URL with an IPv6 host", () => {
    assert.deepEqual(parseProxies("SOCKS5://[2001:db8::1]:1080"), [{
        ip: "2001:db8::1",
        port: 1080,
        username: "",
        password: "",
        type: "SOCKS5",
        name: "[2001:db8::1]:1080",
    }]);
});

test("skips SOCKS5 URLs with invalid credentials or ports", () => {
    assert.deepEqual(parseProxies([
        "socks5://test:%ZZ@192.0.2.10:8000",
        "socks5://192.0.2.10:0",
        "socks5://192.0.2.10:65536",
        "socks5://192.0.2.10:",
    ].join("\n")), []);
});
