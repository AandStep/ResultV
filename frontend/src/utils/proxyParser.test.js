import assert from "node:assert/strict";
import test from "node:test";
import { mergeImportedProxies, mergeSubscriptionRefreshCountries, parseProxies } from "./proxyParser.js";

test("importing or reimporting a subscription keeps other subscriptions and their AUTO members", () => {
    const ownURL = "https://own.example/sub";
    const newURL = "https://other.example/sub";
    const member = { ip: "192.0.2.10", port: 443, type: "VLESS" };
    const own = [
        { id: "own-auto", ip: "", port: 0, type: "AUTO", extra: { members: ["own-member"] }, subscriptionUrl: ownURL },
        { id: "own-member", ...member, subscriptionUrl: ownURL },
    ];
    const manual = { id: "manual", ...member };
    const imported = [
        { id: "new-auto", ip: "", port: 0, type: "AUTO", extra: { members: ["new-member"] }, subscriptionUrl: newURL },
        { id: "new-member", ...member, subscriptionUrl: newURL },
    ];
    const stale = { id: "old-server", ip: "192.0.2.11", port: 443, type: "VLESS", subscriptionUrl: newURL };

    assert.deepEqual(mergeImportedProxies([...own, manual], imported), [...own, manual, ...imported]);
    assert.deepEqual(mergeImportedProxies([...own, manual, stale], imported), [...own, manual, ...imported]);
});

test("importing a manual VPN server preserves matching subscription entries", () => {
    const manual = { id: "manual", ip: "192.0.2.10", port: 443, type: "VLESS" };
    const subscriber = { ...manual, id: "subscriber", subscriptionUrl: "https://own.example/sub" };
    const replacement = { ...manual, id: "replacement" };
    assert.deepEqual(mergeImportedProxies([manual, subscriber], [replacement]), [subscriber, replacement]);
});

test("refresh keeps distinct IDs for servers that share an endpoint", () => {
    const subscriptionUrl = "https://other.example/sub";
    const endpoint = { ip: "192.0.2.10", port: 443, type: "VLESS", subscriptionUrl };
    const first = { ...endpoint, id: "old-a", name: "Германия | №1" };
    const second = { ...endpoint, id: "old-b", name: "Германия | №2" };
    const updated = [{ ...second, id: "new-b" }, { ...first, id: "new-a" }];

    assert.deepEqual(
        mergeSubscriptionRefreshCountries([first, second], updated, subscriptionUrl).map((p) => p.id),
        ["old-b", "old-a"],
    );
    assert.deepEqual(
        mergeSubscriptionRefreshCountries([first, { ...second, id: first.id }], updated, subscriptionUrl).map((p) => p.id),
        ["old-a", "new-a"],
    );
});

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
