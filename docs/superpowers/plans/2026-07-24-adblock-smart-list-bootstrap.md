# Proactive List Prefetch — Engine Never Blocks on Connect — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fresh-install Smart connect never fails and never needs a reconnect: ad-block rule_sets are local-only (engine start can't be aborted by a network download), lists are warmed with a bounded pre-connect wait, and anything not ready in time is applied via an in-place reload.

**Architecture:** (1) Go engine emits ad-block rule_sets ONLY for SRS files cached & validated on disk — the fatal sing-box `remote` rule_set is removed; reject rules reference only present tags. (2) Kotlin connect path warms the ad-block + smart caches (bounded) before building the config, then — if a list wasn't ready — finishes warming and reloads in place. (3) A guard stops a transient builtin smart-list fallback from overwriting a good list.

**Tech Stack:** Go (sing-box config gen, `internal/proxy`), Kotlin/Android (`ResultVpnService`, coroutines), JUnit4 JVM unit tests, Go `testing`.

## Global Constraints

- Remove the sing-box `remote` ad-block rule_set branch **entirely** — ad-block rule_sets are `local`-only; SRS files are warmed out-of-band by `DownloadAdBlockRuleSets` (which has the jsDelivr mirror + HTTP/2).
- `CONNECT_LIST_WAIT_MS = 4000L` — the pre-connect list-warm deadline.
- Engine ad-block toggle is `SettingsRepository.state.value.adblock` (lowercase). Smart mode is `RoutingRulesRepository.state.value.mode == RoutingMode.Smart`.
- Commits are **file-scoped** — the working tree carries unrelated in-progress work; `git add <exact paths>` only, never `git add -A`/`.`.
- End every commit message with: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- Go files keep the existing GPLv3 header; new Kotlin test files follow `EngineErrorsTest.kt` (package line only, no license header).

---

## File Structure

- `internal/proxy/adblock_rules.go` — `buildAdBlockRuleSets` becomes local-only; add `availableAdBlockRuleSetTags`; drop the now-unused `adBlockUpdateInterval` const.
- `internal/proxy/engine.go` — the DNS reject (`:620`) and route reject (`:878`) rule_set rules reference `availableAdBlockRuleSetTags(effectiveDataDir(cfg))` and are omitted when empty.
- `internal/proxy/adblock_rules_test.go` — rewrite the corrupt→remote test to corrupt→dropped; add an `availableAdBlockRuleSetTags` test.
- `internal/proxy/engine_adblock_test.go` — rewrite the two remote-fallback tests to omitted; seed SRS in the four reject-rule tests; add a no-cache-omits-reject test; add a `seedAdBlockSRS` helper.
- `android/app/src/main/java/com/resultv/android/vpn/SmartListRepository.kt` — add pure `shouldReplaceSmartSnapshot`; use it in `refresh()`.
- `android/app/src/test/java/com/resultv/android/vpn/SmartListSnapshotTest.kt` — new JVM test for the guard.
- `android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt` — restructure the connect branch: bounded pre-connect warm + fallback reload; add `CONNECT_LIST_WAIT_MS`, `warmListsBeforeConnect`, `scheduleListReadyReload`.

---

## Task 1: Ad-block rule_sets are local-only (Go)

**Files:**
- Modify: `internal/proxy/adblock_rules.go` (`buildAdBlockRuleSets`, add `availableAdBlockRuleSetTags`, remove `adBlockUpdateInterval`)
- Test: `internal/proxy/adblock_rules_test.go`, `internal/proxy/engine_adblock_test.go`

**Interfaces:**
- Produces: `func buildAdBlockRuleSets(dataDir string) []SBRouteRuleSet` — now returns only `local` rule_sets for present+valid files (possibly empty; never `remote`).
- Produces: `func availableAdBlockRuleSetTags(dataDir string) []string` — tags with a usable local SRS, in `defaultAdBlockRuleSets` order.

- [ ] **Step 1: Rewrite the two remote-fallback tests in `engine_adblock_test.go` to expect omission (failing first).**

Replace `TestBuildAdBlockRuleSets_LocalWhenCachedElseRemoteViaDirect` (currently ~line 169) and `TestBuildAdBlockRuleSets_TruncatedCacheStaysRemote` (~line 199) with:

```go
func TestBuildAdBlockRuleSets_LocalWhenCachedElseOmitted(t *testing.T) {
	dir := t.TempDir()

	// No cached files yet → nothing is emitted (a remote rule_set would be
	// fatal on a cold start; the SRS is warmed out-of-band and applied on
	// the next reload).
	if got := buildAdBlockRuleSets(dir); len(got) != 0 {
		t.Fatalf("expected no rule_sets with an empty cache, got %+v", got)
	}

	// A valid cached SRS flips that list to a local rule_set.
	sub := filepath.Join(dir, adBlockRuleSetsSubdir)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, defaultAdBlockRuleSets[0].fileName), validSRSBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	got := buildAdBlockRuleSets(dir)
	if len(got) != 1 {
		t.Fatalf("expected exactly the one cached rule_set, got %+v", got)
	}
	if got[0].Type != "local" || got[0].Path == "" {
		t.Fatalf("expected a local rule_set for the cached file, got %+v", got[0])
	}
}

func TestBuildAdBlockRuleSets_TruncatedCacheOmitted(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, adBlockRuleSetsSubdir)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	// A short / half-written file must NOT be referenced as local (that would
	// fail sing-box startup) and must NOT emit a remote fallback either — it is
	// simply skipped this session.
	if err := os.WriteFile(filepath.Join(sub, defaultAdBlockRuleSets[0].fileName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := buildAdBlockRuleSets(dir); len(got) != 0 {
		t.Fatalf("truncated cache must be omitted, got %+v", got)
	}
}
```

- [ ] **Step 2: Rewrite the corrupt-local test in `adblock_rules_test.go` (failing first).**

Replace `TestBuildAdBlockRuleSets_CorruptLocalFallsBackToRemote` (~line 156) with:

```go
// TestBuildAdBlockRuleSets_CorruptLocalDropped guards a corrupt SRS cached from
// before validation existed: it must NOT be referenced as a local rule-set
// (which fails sing-box startup), must NOT emit a remote fallback, and must be
// removed so a fresh download can replace it.
func TestBuildAdBlockRuleSets_CorruptLocalDropped(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(adBlockRuleSetsDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	// A corrupt file that clears the size floor but fails SRS parsing.
	corruptPath := filepath.Join(adBlockRuleSetsDir(dir), defaultAdBlockRuleSets[0].fileName)
	if err := os.WriteFile(corruptPath, bytes.Repeat([]byte("x"), minLocalSRSBytes+16), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := buildAdBlockRuleSets(dir); len(got) != 0 {
		t.Fatalf("corrupt local SRS must be dropped, got %+v", got)
	}
	if _, err := os.Stat(corruptPath); !os.IsNotExist(err) {
		t.Fatalf("corrupt local SRS not removed (self-heal); stat err=%v", err)
	}
}
```

- [ ] **Step 3: Add an `availableAdBlockRuleSetTags` test in `adblock_rules_test.go` (failing first).**

Append:

```go
func TestAvailableAdBlockRuleSetTags(t *testing.T) {
	dir := t.TempDir()
	if got := availableAdBlockRuleSetTags(dir); len(got) != 0 {
		t.Fatalf("expected no tags with an empty cache, got %v", got)
	}
	if err := os.MkdirAll(adBlockRuleSetsDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	// Cache only the first list → only its tag is available.
	if err := os.WriteFile(filepath.Join(adBlockRuleSetsDir(dir), defaultAdBlockRuleSets[0].fileName), validSRSBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	got := availableAdBlockRuleSetTags(dir)
	if len(got) != 1 || got[0] != defaultAdBlockRuleSets[0].tag {
		t.Fatalf("expected only %q, got %v", defaultAdBlockRuleSets[0].tag, got)
	}
}
```

- [ ] **Step 4: Run the tests to confirm they fail.**

Run: `go test ./internal/proxy/ -run 'TestBuildAdBlockRuleSets_LocalWhenCachedElseOmitted|TestBuildAdBlockRuleSets_TruncatedCacheOmitted|TestBuildAdBlockRuleSets_CorruptLocalDropped|TestAvailableAdBlockRuleSetTags' -v`
Expected: compile error or FAIL — `availableAdBlockRuleSetTags` undefined and `buildAdBlockRuleSets` still returns `remote`.

- [ ] **Step 5: Make `buildAdBlockRuleSets` local-only and add `availableAdBlockRuleSetTags`.**

In `internal/proxy/adblock_rules.go`, replace the body of `buildAdBlockRuleSets` (the loop) and its doc comment, and add the new helper. New `buildAdBlockRuleSets`:

```go
// buildAdBlockRuleSets returns the sing-box rule_set definitions for the ad
// lists — ONLY for SRS files already cached locally and validated
// (localAdBlockSRSUsable). A list with no usable cache is omitted entirely.
//
// There is deliberately no `remote` rule_set: sing-box must download a remote
// rule_set synchronously on a cold start (no cache_file yet) and its failure
// ABORTS engine startup — exactly the fresh-install "initialize rule-set" crash
// this replaces. The SRS is instead warmed out-of-band by DownloadAdBlockRuleSets
// (jsDelivr mirror + HTTP/2, no QUIC) and applied on the next connect/reload.
// Reject rules must reference availableAdBlockRuleSetTags so they never point at
// a tag with no definition (which also fails startup).
func buildAdBlockRuleSets(dataDir string) []SBRouteRuleSet {
	out := make([]SBRouteRuleSet, 0, len(defaultAdBlockRuleSets))
	for _, src := range defaultAdBlockRuleSets {
		localPath := filepath.Join(adBlockRuleSetsDir(dataDir), src.fileName)
		if localAdBlockSRSUsable(localPath) {
			out = append(out, SBRouteRuleSet{
				Type:   "local",
				Tag:    src.tag,
				Format: "binary",
				Path:   localPath,
			})
		}
	}
	return out
}

// availableAdBlockRuleSetTags returns the tags whose SRS is cached and usable
// locally, in defaultAdBlockRuleSets order. Reject rules reference ONLY these:
// a rule_set tag with no matching definition fails sing-box startup. Empty when
// nothing is cached — the caller then omits the rule_set-based reject rule.
func availableAdBlockRuleSetTags(dataDir string) []string {
	tags := make([]string, 0, len(defaultAdBlockRuleSets))
	for _, src := range defaultAdBlockRuleSets {
		localPath := filepath.Join(adBlockRuleSetsDir(dataDir), src.fileName)
		if localAdBlockSRSUsable(localPath) {
			tags = append(tags, src.tag)
		}
	}
	return tags
}
```

Then delete the now-unused `adBlockUpdateInterval` const (the `const (...)` block line `adBlockUpdateInterval = "24h"`). Leave `adBlockRuleSetTags()` (still referenced by tests as the full set).

- [ ] **Step 6: Confirm nothing else references the removed symbol.**

Run: `grep -rn "adBlockUpdateInterval" internal/`
Expected: no matches.

- [ ] **Step 7: Run the full proxy test package.**

Run: `go test ./internal/proxy/ -v -run AdBlock`
Expected: the four rewritten/added tests PASS. NOTE: the four reject-rule tests in `engine_adblock_test.go` will now FAIL — they are fixed in Task 2. If you want a green bar before Task 2, run only: `go test ./internal/proxy/ -run 'TestBuildAdBlockRuleSets|TestAvailableAdBlockRuleSetTags|TestDefaultAdBlockRuleSets_HaveMirrors|TestBuildAdBlockRuleSets_ValidLocalUsedLocally' -v` → PASS.

- [ ] **Step 8: Commit.**

```bash
git add internal/proxy/adblock_rules.go internal/proxy/adblock_rules_test.go internal/proxy/engine_adblock_test.go
git commit -m "fix(proxy): ad-block rule_sets are local-only — no fatal cold-start remote fetch

A remote ad-block rule_set must be downloaded synchronously on a cold start
(no cache_file yet) and its failure aborts engine startup — the fresh-install
'initialize rule-set' crash. Emit rule_sets only for locally-cached, validated
SRS; warm them out-of-band via the mirror-capable downloader instead.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Reject rules reference only present tags (Go)

**Files:**
- Modify: `internal/proxy/engine.go` (DNS reject `~:620`, route reject `~:878`)
- Test: `internal/proxy/engine_adblock_test.go`

**Interfaces:**
- Consumes: `availableAdBlockRuleSetTags(dataDir string) []string` (Task 1).

- [ ] **Step 1: Add the `seedAdBlockSRS` helper and fix the four reject-rule tests + add the no-cache test (failing first).**

In `internal/proxy/engine_adblock_test.go`, add the helper (top of file, after `sameStringSet`):

```go
// seedAdBlockSRS writes a valid SRS for every default ad list into dataDir so
// buildRoute/buildDNS emit their local rule_sets and the reject rules reference
// the full tag set.
func seedAdBlockSRS(t *testing.T, dataDir string) {
	t.Helper()
	dir := adBlockRuleSetsDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, src := range defaultAdBlockRuleSets {
		if err := os.WriteFile(filepath.Join(dir, src.fileName), validSRSBytes(t), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
```

Change `TestBuildRoute_AdBlock_AddsRejectAfterSniffAndDefinesRuleSets` to seed the SRS. Replace its first two lines:

```go
func TestBuildRoute_AdBlock_AddsRejectAfterSniffAndDefinesRuleSets(t *testing.T) {
	dir := t.TempDir()
	seedAdBlockSRS(t, dir)
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, DataDir: dir}
	route := buildRoute(cfg)
```
(the rest of the test body is unchanged.)

Change `TestBuildDNS_AdBlock_AddsRejectRule` to pin a seeded DataDir. Replace its `cfg :=` line:

```go
func TestBuildDNS_AdBlock_AddsRejectRule(t *testing.T) {
	dir := t.TempDir()
	seedAdBlockSRS(t, dir)
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, DataDir: dir, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
```
(rest unchanged.)

Change `TestBuildDNS_AdBlock_BypassesConnectivityDomainsBeforeReject` the same way:

```go
func TestBuildDNS_AdBlock_BypassesConnectivityDomainsBeforeReject(t *testing.T) {
	dir := t.TempDir()
	seedAdBlockSRS(t, dir)
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, DataDir: dir, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
```
(rest unchanged.)

Change `TestBuildRoute_AdBlock_BypassesConnectivityDomainsBeforeReject` first line:

```go
func TestBuildRoute_AdBlock_BypassesConnectivityDomainsBeforeReject(t *testing.T) {
	dir := t.TempDir()
	seedAdBlockSRS(t, dir)
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, DataDir: dir})
```
(rest unchanged.)

Add a new test asserting the no-cache omission (place after `TestBuildRoute_AdBlockOff_NoRejectRules`):

```go
// TestBuildRoute_AdBlock_NoCache_OmitsRuleSetReject: with ad-block on but no SRS
// cached, the rule_set-based reject rule (and its rule_set defs) must be absent
// — otherwise the config references an undefined rule_set and startup fails.
// The static domain-based ad reject (extraAdDeliveryDomains) still applies.
func TestBuildRoute_AdBlock_NoCache_OmitsRuleSetReject(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, DataDir: t.TempDir()})
	if len(route.RuleSet) != 0 {
		t.Fatalf("expected no rule_set defs with an empty cache, got %+v", route.RuleSet)
	}
	staticReject := false
	for _, r := range route.Rules {
		if r.Action == "reject" && len(r.RuleSet) > 0 {
			t.Fatalf("did not expect a rule_set reject with an empty cache, rule=%+v", r)
		}
		if r.Action == "reject" && len(r.Domain) > 0 && r.Domain[0] == extraAdDeliveryDomains[0] {
			staticReject = true
		}
	}
	if !staticReject {
		t.Fatal("expected the static ad-delivery reject rule to remain")
	}
}
```

- [ ] **Step 2: Run the reject-rule tests to confirm they fail.**

Run: `go test ./internal/proxy/ -run 'TestBuildRoute_AdBlock_AddsRejectAfterSniffAndDefinesRuleSets|TestBuildDNS_AdBlock_AddsRejectRule|TestBuildDNS_AdBlock_BypassesConnectivityDomainsBeforeReject|TestBuildRoute_AdBlock_BypassesConnectivityDomainsBeforeReject|TestBuildRoute_AdBlock_NoCache_OmitsRuleSetReject' -v`
Expected: FAIL — the new no-cache test fails (engine.go still emits a reject rule referencing empty tags via `adBlockRuleSetTags()`), and the seeded tests may pass by luck but the no-cache one pins the behavior.

- [ ] **Step 3: Wire the DNS reject rule to available tags.**

In `internal/proxy/engine.go`, replace the DNS reject block (currently `~:620-623`):

```go
			if tags := availableAdBlockRuleSetTags(effectiveDataDir(cfg)); len(tags) > 0 {
				dns.Rules = append(dns.Rules, SBDNSRule{
					RuleSet: tags,
					Action:  "reject",
				})
			}
```

- [ ] **Step 4: Wire the route reject rule to available tags.**

In `internal/proxy/engine.go`, replace the route reject block (currently `~:878-881`):

```go
		if tags := availableAdBlockRuleSetTags(effectiveDataDir(cfg)); len(tags) > 0 {
			rules = append(rules, SBRouteRule{
				RuleSet: tags,
				Action:  "reject",
			})
		}
```

- [ ] **Step 5: Run the whole proxy package.**

Run: `go test ./internal/proxy/ -v`
Expected: PASS (all, including Task 1's tests).

- [ ] **Step 6: Commit.**

```bash
git add internal/proxy/engine.go internal/proxy/engine_adblock_test.go
git commit -m "fix(proxy): ad-block reject rules reference only cached rule_set tags

Pair with local-only rule_sets: a reject rule pointing at a rule_set tag that
has no definition fails sing-box startup. Reference availableAdBlockRuleSetTags
and omit the rule_set reject entirely when nothing is cached; the static
domain-based ad reject still applies.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Smart-list builtin fallback can't overwrite a good list (Kotlin)

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/SmartListRepository.kt`
- Create: `android/app/src/test/java/com/resultv/android/vpn/SmartListSnapshotTest.kt`

**Interfaces:**
- Produces: `internal fun shouldReplaceSmartSnapshot(curSource: String, curEmpty: Boolean, nextSource: String): Boolean` (file-level, in `SmartListRepository.kt`).

- [ ] **Step 1: Write the failing JVM test.**

Create `android/app/src/test/java/com/resultv/android/vpn/SmartListSnapshotTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SmartListSnapshotTest {

    @Test fun builtinDoesNotOverwriteRealList() {
        // A transient antizapret failure yields the small builtin list; it must
        // not replace an already-loaded remote/cache list (Problem #2: collapses
        // the Smart allowlist and kills traffic).
        assertFalse(shouldReplaceSmartSnapshot(curSource = "remote", curEmpty = false, nextSource = "builtin"))
        assertFalse(shouldReplaceSmartSnapshot(curSource = "cache", curEmpty = false, nextSource = "builtin"))
    }

    @Test fun builtinAcceptedWhenNoRealListYet() {
        // Cold start: some blocking beats none.
        assertTrue(shouldReplaceSmartSnapshot(curSource = "", curEmpty = true, nextSource = "builtin"))
        assertTrue(shouldReplaceSmartSnapshot(curSource = "builtin", curEmpty = false, nextSource = "builtin"))
    }

    @Test fun realListAlwaysReplaces() {
        assertTrue(shouldReplaceSmartSnapshot(curSource = "cache", curEmpty = false, nextSource = "remote"))
        assertTrue(shouldReplaceSmartSnapshot(curSource = "remote", curEmpty = false, nextSource = "cache"))
        assertTrue(shouldReplaceSmartSnapshot(curSource = "builtin", curEmpty = false, nextSource = "remote"))
    }
}
```

- [ ] **Step 2: Run it to confirm it fails.**

Run (from `android/`): `./gradlew :app:testDebugUnitTest --tests "com.resultv.android.vpn.SmartListSnapshotTest"`
Expected: FAIL — `shouldReplaceSmartSnapshot` unresolved (compile error).

- [ ] **Step 3: Add the pure helper.**

In `SmartListRepository.kt`, add at file level (after the imports, before `object SmartListRepository`):

```kotlin
/**
 * Whether a freshly-fetched smart-list result (source [nextSource]) should
 * replace the current snapshot (source [curSource], [curEmpty]).
 *
 * Guards Problem #2: a transient failure to reach the antizapret list makes the
 * engine fall back to the small builtin list (source "builtin"). Applying it as
 * the Smart routing list collapses the per-app allowlist and kills traffic — so
 * a builtin result must NOT overwrite an already-loaded real ("remote"/"cache")
 * list; it is treated as "not ready yet". On a cold start (no real list yet)
 * builtin IS accepted — some blocking beats none.
 */
internal fun shouldReplaceSmartSnapshot(
    curSource: String,
    curEmpty: Boolean,
    nextSource: String,
): Boolean {
    if (nextSource == "builtin" && !curEmpty && curSource != "builtin") return false
    return true
}
```

- [ ] **Step 4: Run the test to confirm it passes.**

Run (from `android/`): `./gradlew :app:testDebugUnitTest --tests "com.resultv.android.vpn.SmartListSnapshotTest"`
Expected: PASS.

- [ ] **Step 5: Use the guard in `refresh()`.**

In `SmartListRepository.refresh()`, after the `parsed == null` block and before `if (parsed.country.isNotBlank())`, insert:

```kotlin
        val cur = _state.value
        if (!shouldReplaceSmartSnapshot(cur.source, cur.isEmpty, parsed.source)) {
            Log.i(TAG, "ignoring builtin smart-list (${parsed.domains.size}); keeping ${cur.source} (${cur.domains.size})")
            return@withLock cur
        }
```

- [ ] **Step 6: Run the app's unit tests to confirm nothing regressed.**

Run (from `android/`): `./gradlew :app:testDebugUnitTest`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/SmartListRepository.kt android/app/src/test/java/com/resultv/android/vpn/SmartListSnapshotTest.kt
git commit -m "fix(android): don't let a builtin smart-list fallback overwrite a good list

A transient antizapret fetch failure returns the small builtin list; applying
it collapses the Smart per-app allowlist and kills traffic. Keep the loaded
remote/cache list instead; accept builtin only when there is no real list yet.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Bounded pre-connect list warm + fallback reload (Kotlin)

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt`

**Interfaces:**
- Consumes: `AdBlockRepository.ensureLoaded()`, `SmartListRepository.ensureLoaded()` (suspend, coalescing), `BuildOptionsBuilder.buildConfig`, `BoxModule.start/reload/isRunning`.
- Depends on Tasks 1–2 (engine start is non-fatal without cached lists) and Task 3 (a warm returns a real smart list, not builtin).

This task is service orchestration (no pure unit under test); verify by compile + the manual fresh-install protocol in Step 6.

- [ ] **Step 1: Add imports and the constant.**

In `ResultVpnService.kt`, add to the coroutine imports:

```kotlin
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
```

Add near the other top-level `private const val` declarations:

```kotlin
// How long Connect waits for the ad-block SRS + smart-list caches to warm
// before building the config. The engine never blocks on a network download
// (rule_sets are local-only); this only decides whether the very first connect
// already has the lists or picks them up a moment later via reload. The
// app-start prefetch usually makes this near-instant.
private const val CONNECT_LIST_WAIT_MS = 4000L
```

- [ ] **Step 2: Replace the connect (`else ->`) branch body.**

Replace the entire `else -> { ... }` block in `onStartCommand` (currently lines ~112–186, from `else -> {` through its `return START_STICKY`) with:

```kotlin
            else -> {
                // Need an active profile to connect. Cheap check up front so we
                // can promise foreground immediately, then do the (possibly slow)
                // list warm-up + config build off the main thread.
                if (ProfileRepository.state.value.active == null) {
                    Log.e(TAG, "no active profile — stopping")
                    stopSelf()
                    return START_NOT_STICKY
                }
                // A reload is an internal stop+start of a fresh service instance —
                // triggerReload already logged "Applying changes…"; suppress the
                // duplicate connect/connected pair. The flag rides on the restart
                // intent because the new instance can't see the old's fields.
                val isReload = intent?.getBooleanExtra(EXTRA_IS_RELOAD, false) ?: false
                VpnState.set(VpnStatus.Connecting)
                startForeground(NOTIFICATION_ID, buildNotification(VpnStatus.Connecting))
                if (!isReload) {
                    val serverName = ProfileRepository.state.value.active?.name ?: ""
                    AppLog.info(getString(R.string.log_connecting, serverName))
                }
                val connectedMsg = getString(R.string.log_connected)
                scope.launch {
                    // Warm the ad-block SRS + smart-list caches so the config we
                    // build references LOCAL files. Bounded so a slow/blocked
                    // source never holds the tunnel back. Skipped on reloads
                    // (lists already loaded; a reload must stay fast).
                    val listsReady = if (isReload) true else warmListsBeforeConnect()
                    val tCfg = System.currentTimeMillis()
                    val config = buildConfigFromActiveProfile()
                    Log.i(TAG, "connect timing: buildConfig=${System.currentTimeMillis() - tCfg}ms, size=${config?.length ?: 0}")
                    if (config.isNullOrEmpty()) {
                        Log.e(TAG, "no config available — stopping")
                        withContext(Dispatchers.Main) { stopSelf() }
                        return@launch
                    }
                    worker.execute {
                        val t0 = System.currentTimeMillis()
                        try {
                            // openTun during start() must see filterProxyRunning=false
                            // (browser ad-block attaches after, off the critical path).
                            BoxModule.filterProxyRunning = false
                            BoxModule.start(this@ResultVpnService, config)
                            val connectedAt = System.currentTimeMillis()
                            Log.i(TAG, "connect timing: BoxModule.start=${connectedAt - t0}ms")
                            val connected = VpnStatus.Connected(connectedAt)
                            VpnState.set(connected)
                            if (!isReload) {
                                AppLog.success(connectedMsg)
                                AppLog.info(
                                    R.string.log_connect_timing,
                                    connectedAt - t0,
                                    source = AppLog.resolve(R.string.log_source_proxy),
                                )
                            }
                            renotify(buildNotification(connected))
                            startReloadWatcher()
                            startKillSwitchWatchdog()
                            TrafficWatcher.start()
                            ConnectionWatcher.start()
                            attachBrowserAdBlockAsync()
                            // Fallback: connected before the lists finished warming.
                            // Finish warming them and apply on the fly — no reconnect.
                            if (!listsReady) scheduleListReadyReload()
                        } catch (t: Throwable) {
                            Log.e(TAG, "BoxModule.start failed", t)
                            val msg = t.message ?: t.javaClass.simpleName
                            VpnState.set(VpnStatus.Error(msg))
                            AppLog.error(getString(R.string.log_conn_failed, msg))
                            closeTun()
                            stopForeground(STOP_FOREGROUND_REMOVE)
                            stopSelf()
                        }
                    }
                }
                return START_STICKY
            }
```

- [ ] **Step 3: Add `warmListsBeforeConnect`.**

Add as a private member of `ResultVpnService` (near `buildConfigFromActiveProfile`):

```kotlin
    /**
     * Warm the ad-block + smart-list caches before building the connect config,
     * bounded by CONNECT_LIST_WAIT_MS so a blocked/slow source never holds the
     * tunnel back. Returns true if the enabled lists finished in time.
     * ensureLoaded() coalesces with the app-start prefetch, so this is usually
     * near-instant. Only warms a list whose feature is on.
     */
    private suspend fun warmListsBeforeConnect(): Boolean =
        withTimeoutOrNull(CONNECT_LIST_WAIT_MS) {
            if (SettingsRepository.state.value.adblock) AdBlockRepository.ensureLoaded()
            if (RoutingRulesRepository.state.value.mode == RoutingMode.Smart) {
                SmartListRepository.ensureLoaded()
            }
            true
        } ?: false
```

- [ ] **Step 4: Add `scheduleListReadyReload`.**

Add as a private member of `ResultVpnService`:

```kotlin
    /**
     * Fallback for a connect that came up before the lists finished downloading
     * (CONNECT_LIST_WAIT_MS elapsed). Finish warming them WITHOUT a deadline,
     * then rebuild the config and reload in place so ad-block + the full smart
     * list activate — no user reconnect. Reloads at most once, and only if a
     * list actually became available. The existing reloadWatcher does not observe
     * the list repos, so this cannot double-fire with it.
     */
    private fun scheduleListReadyReload() {
        scope.launch {
            val ab = if (SettingsRepository.state.value.adblock) AdBlockRepository.ensureLoaded() else null
            val sl = if (RoutingRulesRepository.state.value.mode == RoutingMode.Smart) {
                SmartListRepository.ensureLoaded()
            } else null
            val gotAdBlock = ab?.hasLists == true
            val gotSmart = sl != null && !sl.isEmpty && sl.source != "builtin"
            if (!BoxModule.isRunning || (!gotAdBlock && !gotSmart)) return@launch
            val active = ProfileRepository.state.value.active ?: return@launch
            val cfg = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath) ?: return@launch
            Log.i(TAG, "lists ready post-connect — reloading (adblock=$gotAdBlock smart=$gotSmart)")
            worker.execute { BoxModule.reload(cfg) }
        }
    }
```

- [ ] **Step 5: Confirm imports resolve and the app compiles.**

Run (from `android/`): `./gradlew :app:compileDebugKotlin`
Expected: BUILD SUCCESSFUL. If `RoutingMode` is unresolved, add `import` for it (same package `com.resultv.android.vpn`, so no import needed — verify it's in that package; if not, add the correct import).

- [ ] **Step 6: Manual verification (fresh-install protocol).**

Build & install a debug APK, then simulate a fresh install and confirm the three original symptoms are gone:

```bash
# from android/
./gradlew :app:installDebug
adb shell pm clear com.resultv.android   # wipe app data = fresh install state
```

Then on device:
1. Open the app, add the subscription, **immediately** switch to Smart and Connect.
   - Expect: connects with NO `Сбой подключения: initialize rule-set` error (Problem #1 fixed). If lists weren't cached in time, connection still comes up and a single `Конфигурация движка перезагружена на лету` follows within a few seconds.
2. With Smart connected, confirm traffic flows through the tunnel for a blocked app (e.g. a `... via proxy` CONN line appears) and the log does not show the smart list collapsing to a small count (`записей: 1579`) replacing the full one (Problem #2 fixed).
3. Toggle Global↔Smart a couple of times: no reconnect should ever be required to get working Smart traffic.

Capture the in-app log (export) and verify: no `initialize rule-set` failure, at most one post-connect reload, and the smart count stays at the full value.

- [ ] **Step 7: Commit.**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt
git commit -m "fix(android): warm lists before connect with a bounded wait + fallback reload

Fresh-install connect now warms the ad-block SRS + smart-list caches (bounded by
CONNECT_LIST_WAIT_MS) before building the config, so the first connect already
has local lists. If a list isn't ready in time the tunnel still comes up (engine
is non-fatal without them) and the lists are applied via an in-place reload — no
user reconnect.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Компонент 1 (local-only, available-tags) → Tasks 1 & 2. ✓
- Компонент 2 (bounded pre-connect wait + fallback) → Task 4 Steps 2–3. ✓
- Компонент 3 (auto-reload when ready) → Task 4 Step 4 (`scheduleListReadyReload`). ✓
- Компонент 4 (prefetch already exists; builtin guard) → Task 3 + existing `MainActivity` prefetch (unchanged, noted). ✓
- Error handling (both mirrors down → no ad-block, non-fatal; antizapret down → empty smart list keeps global via `engine.go:781`; in-place reload safe) → covered by Tasks 1/2 semantics + Task 4. ✓
- Testing (Go rule_set/tag/reject tests; Kotlin guard test) → Tasks 1–3. ✓

**Placeholder scan:** none — every step has concrete code/commands.

**Type consistency:** `availableAdBlockRuleSetTags(dataDir string) []string` defined in Task 1, consumed identically in Task 2. `shouldReplaceSmartSnapshot(curSource, curEmpty, nextSource)` defined and consumed with the same signature in Task 3. `CONNECT_LIST_WAIT_MS`, `warmListsBeforeConnect(): Boolean`, `scheduleListReadyReload()` consistent within Task 4. `Snapshot` fields used (`source`, `isEmpty`, `hasLists`, `domains`) match `AdBlockRepository`/`SmartListRepository` definitions.

**Note on existing tests:** `TestBuildAdBlockRuleSets_ValidLocalUsedLocally` and `TestDefaultAdBlockRuleSets_HaveMirrors` stay valid (valid-local → local; the URL mirrors are still used by the out-of-band downloader) and are intentionally not modified.
