// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Diagnostic companion to TestDumpLinks (internal/proxy/linkdump_test.go).
//
// Adding a node by pasting a link goes through the frontend's own JS parser,
// not the Go one — the Go parser only sees subscriptions. This script prints
// what that JS path salvages from a link, so the two halves can be compared:
// a key the Go dump shows but this one does not is a field lost on the manual
// "add by link" path.
//
// Usage:
//   node scripts/dump-link-js.mjs "vless://..."
//   node scripts/dump-link-js.mjs path\to\links.txt

import { readFileSync } from "node:fs";
import { parseProxies } from "../frontend/src/utils/proxyParser.js";

const arg = process.argv[2];
if (!arg) {
    console.error("usage: node scripts/dump-link-js.mjs <link|file>");
    process.exit(2);
}

let text = arg;
try {
    text = readFileSync(arg, "utf8");
} catch {
    // not a file — treat the argument as the link itself
}

const parsed = parseProxies(text);
if (parsed.length === 0) {
    console.error("no proxies parsed — the JS parser rejected this input entirely");
    process.exit(1);
}

for (const [i, p] of parsed.entries()) {
    const extra = typeof p.extra === "string" ? JSON.parse(p.extra || "{}") : p.extra || {};
    console.log(`[${i + 1}] ${p.type} ${p.ip}:${p.port} (${p.name})`);
    console.log(`[${i + 1}] EXTRA: ${JSON.stringify(extra, null, 2)}`);
    for (const [k, v] of Object.entries(extra)) {
        if (v === "" || v == null) {
            console.log(`[${i + 1}] note: "${k}" is present but empty`);
        }
    }
}
