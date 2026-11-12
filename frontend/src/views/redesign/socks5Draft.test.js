import assert from "node:assert/strict";
import test from "node:test";
import { addApp, addSite, buildUpdate, isDraftDirty, profileDraft } from "./socks5Draft.js";

test("sites are lower-cased, trimmed and not repeated", () => {
  assert.deepEqual(addSite([], " Example.COM "), ["example.com"]);
  assert.deepEqual(addSite(["example.com"], "EXAMPLE.com"), ["example.com"]);
  assert.deepEqual(addSite([], "   "), []);
});

test("apps match the executable name on every platform", () => {
  assert.deepEqual(addApp([], "tool", true), ["tool.exe"]);
  assert.deepEqual(addApp(["Tool.exe"], "tool.exe", true), ["Tool.exe"]);
  assert.deepEqual(addApp([], "firefox", false), ["firefox"]);
  assert.deepEqual(addApp([], "Google Chrome.app", false), ["Google Chrome"]);
});

test("a change of the lists alone makes the draft dirty", () => {
  const saved = profileDraft({ host: "192.0.2.1", port: 1080, sites: ["example.com"] });
  assert.equal(isDraftDirty(saved, saved), false);
  assert.equal(isDraftDirty({ ...saved, apps: ["tool.exe"] }, saved), true);
  assert.equal(isDraftDirty({ ...saved, sites: [] }, saved), true);
});

test("the update keeps the stored password until it is edited", () => {
  const draft = profileDraft({ host: " 192.0.2.1 ", port: 1080, username: "u", password: "p", sites: ["a.test"], apps: [] });
  assert.deepEqual(buildUpdate(draft), {
    host: "192.0.2.1", port: 1080, username: "u", password: null, sites: ["a.test"], apps: [],
  });
  assert.equal(buildUpdate({ ...draft, password: "new", passwordEdited: true }).password, "new");
});
