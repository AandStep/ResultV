import assert from "node:assert/strict";
import test from "node:test";
import { isProxyListed } from "./proxyPresence.js";

const sub = "https://example.com/s/abc";
const list = [
    { id: "1", type: "VLESS", ip: "198.51.100.1", port: 443, subscriptionUrl: sub },
    { id: "2", type: "AUTO", ip: "", port: 0, name: "⚡ Авто", subscriptionUrl: sub },
    { id: "3", type: "SECTION", ip: "", port: 0, name: "Подписка закончилась", subscriptionUrl: sub },
];

test("a server kept under its ID is listed", () => {
    assert.equal(isProxyListed(list, { id: "1", type: "VLESS", ip: "198.51.100.1", port: 443 }), true);
});

test("a server re-imported under a new ID is still listed by address", () => {
    assert.equal(isProxyListed(list, { id: "old", type: "vless", ip: "198.51.100.1", port: "443" }), true);
});

test("an auto group re-imported under a new ID is matched by name and subscription", () => {
    assert.equal(isProxyListed(list, { id: "old", type: "AUTO", name: "⚡ Авто", subscriptionUrl: sub }), true);
    assert.equal(isProxyListed(list, { id: "old", type: "AUTO", name: "⚡ Авто", subscriptionUrl: "https://other.example/s/x" }), false);
});

test("a deleted server is not listed", () => {
    assert.equal(isProxyListed(list, { id: "9", type: "VLESS", ip: "203.0.113.5", port: 443 }), false);
    assert.equal(isProxyListed([], { id: "1", type: "VLESS", ip: "198.51.100.1", port: 443 }), false);
});

test("section rows never stand in for a server", () => {
    assert.equal(isProxyListed(list, { id: "x", type: "SECTION", ip: "", port: 0 }), false);
    assert.equal(isProxyListed(list, { id: "x", type: "VLESS", ip: "", port: 0 }), false);
});
