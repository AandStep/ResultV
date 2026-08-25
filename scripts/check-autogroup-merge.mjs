// Regression check for subscription-refresh ID continuity when a provider
// ships more than one AUTO section.
//
// Two hazards, both invisible while there is only ever one AUTO group:
//   1. an AUTO head has ip="" and port=0, so every head hashes to "|0|AUTO";
//   2. two sections list the SAME backends, so their members hash to the same
//      ip|port|type|member key.
// Either one makes two distinct rows inherit a single old ID.
//
// Run: node scripts/check-autogroup-merge.mjs
import assert from "node:assert/strict";
import { mergeSubscriptionRefreshCountries } from "../frontend/src/utils/proxyParser.js";

const SUB = "https://example.test/sub";
const A = "🚀 impVPN Auto";
const B = "⚡ Авто | ✅ Когда не глушат интернет";

const row = (id, over) => ({
    id,
    ip: "",
    port: 0,
    type: "VLESS",
    name: "",
    country: "us",
    subscriptionUrl: SUB,
    ...over,
});

const build = (suffix) => [
    row(`head-a-${suffix}`, { type: "AUTO", name: A, extra: { members: [`m-a-${suffix}`] } }),
    row(`head-b-${suffix}`, { type: "AUTO", name: B, extra: { members: [`m-b-${suffix}`] } }),
    row(`m-a-${suffix}`, { ip: "n1.example", port: 443, name: "🇺🇸 VLESS #1", autoGroup: A }),
    row(`m-b-${suffix}`, { ip: "n1.example", port: 443, name: "🇺🇸 VLESS #1", autoGroup: B }),
    row(`srv-${suffix}`, { ip: "n1.example", port: 443, name: "🇺🇸 США | VLESS TCP | №1" }),
];

const merged = mergeSubscriptionRefreshCountries(build("old"), build("new"), SUB);

const ids = merged.map((p) => String(p.id));
assert.equal(new Set(ids).size, ids.length, `duplicate ids after merge: ${ids.join(", ")}`);

const byName = (name, group) =>
    merged.find((p) => p.name === name && (group === undefined || p.autoGroup === group));

assert.equal(byName(A).id, "head-a-old", "AUTO head A must keep its old id");
assert.equal(byName(B).id, "head-b-old", "AUTO head B must keep its old id");
assert.equal(byName("🇺🇸 VLESS #1", A).id, "m-a-old", "member of A must keep its old id");
assert.equal(byName("🇺🇸 VLESS #1", B).id, "m-b-old", "member of B must keep its old id");
assert.equal(byName("🇺🇸 США | VLESS TCP | №1").id, "srv-old", "plain server must keep its old id");

// The heads' member lists must be remapped onto the merged ids, not left
// pointing at the fresh backend ids.
for (const [head, want] of [[A, "m-a-old"], [B, "m-b-old"]]) {
    assert.deepEqual(byName(head).extra.members, [want], `members of ${head}`);
}

console.log("OK: autogroup merge keys are collision-free");
