# Smart per-app allowlist membership — Implementation Plan (Plan 1 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In Smart mode, put an app into the VPN tunnel only if it is associated with blocked resources (matched against the domain blocklist) or is a browser; keep every other app fully out of the tunnel so VPN-hostile apps (Госуслуги, banks) never detect the VPN — all automatically, no manual input, no tunnel restarts.

**Architecture:** Smart mode switches the OS tunnel membership from a denylist (`addDisallowedApplication`) to an **allowlist** (`addAllowedApplication`). The allowlist is `(auto-matched ∪ browsers ∪ intoVpn) − outOfVpn − ownPackage`, computed once at tunnel establishment. Auto-matching is a pure Kotlin function comparing each installed app's package segments + human label + a small alias table against brand labels extracted from the domain blocklist. Global mode is untouched.

**Tech Stack:** Kotlin, Android `VpnService.Builder`, Jetpack Compose, JUnit4 (JVM unit tests, no Robolectric), gomobile `libbox`.

**Companion plan:** Plan 2 (`2026-07-23-smart-ip-routing.md`, to be written) adds IP/CIDR routing to the smart list. It is independent of this plan and can ship separately.

## Global Constraints

- **Pure-logic files must be Android-free.** `SmartAppMatcher.kt` and `AppTunnelMembership.kt` must import no `android.*` — the unit-test source set has no Robolectric (see the note in `AppRules.kt`). Only `AppInventory.kt` and UI touch `Context`/`PackageManager`.
- **Do not change Global-mode behaviour.** Global stays denylist (`addDisallowedApplication` for `outOfVpn` + own package), `final=proxy`, tabs `[из VPN, запретить]`. Byte-for-byte where possible.
- **`addAllowedApplication` and `addDisallowedApplication` are mutually exclusive** on one `VpnService.Builder` — the routing mode selects exactly one.
- **Own package is never in the tunnel** (it dials the proxy). In allowlist mode this means: never add it.
- **`outOfVpn` now applies in both modes** — it is the shared "never tunnel this app" list.
- **minSdk is 26.** `addAllowedApplication`/`addDisallowedApplication` work on all supported levels; `intoVpn` force-proxy and `Block` package_name rules still need API 29 (`findConnectionOwner`), unchanged.
- **User-facing strings** go in both `android/app/src/main/res/values/strings.xml` and `.../values-ru/strings.xml`.
- **Follow existing patterns:** pure data + transitions like `AppRules.kt`; repositories own `Context`/IO; `org.json` for JVM-testable JSON.

**Android unit test command (first run downloads Gradle; allow several minutes):**
```bash
cd android && ./gradlew :app:testDebugUnitTest --tests "com.resultv.android.vpn.<ClassName>"
```

---

### Task 1: SmartAppMatcher (pure brand-matching)

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/vpn/SmartAppMatcher.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/SmartAppMatcherTest.kt`

**Interfaces:**
- Produces:
  - `data class AppMeta(val packageName: String, val label: String)`
  - `SmartAppMatcher.brandsFrom(domains: Collection<String>): Set<String>`
  - `SmartAppMatcher.matchedPackages(apps: List<AppMeta>, blockedDomains: Collection<String>, aliases: Map<String, String> = SmartAppMatcher.DEFAULT_ALIASES): Set<String>`
  - `SmartAppMatcher.DEFAULT_ALIASES: Map<String, String>`

- [ ] **Step 1: Write the failing test**

Create `android/app/src/test/java/com/resultv/android/vpn/SmartAppMatcherTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SmartAppMatcherTest {

    private val blocked = setOf(
        "instagram.com", "youtube.com", "tiktok.com", "x.com", "t.me",
    )

    @Test fun brandsFrom_takesSecondLevelLabelMinLen3() {
        val brands = SmartAppMatcher.brandsFrom(
            listOf("youtube.com", "www.instagram.com", "t.me", ".x.com", "")
        )
        assertTrue("youtube" in brands)
        assertTrue("instagram" in brands)
        assertFalse("t" in brands)   // too short → alias territory
        assertFalse("x" in brands)   // too short → alias territory
        assertFalse("com" in brands)
    }

    @Test fun matches_byPackageSegment() {
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("com.google.android.youtube", "YouTube")), blocked
        )
        assertEquals(setOf("com.google.android.youtube"), out)
    }

    @Test fun matches_byLabel_whenPackageObfuscated() {
        // TikTok's package carries no brand token; its label does.
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("com.zhiliaoapp.musically", "TikTok")), blocked
        )
        assertTrue("com.zhiliaoapp.musically" in out)
    }

    @Test fun matches_byAlias_forRebrandsAndShortNames() {
        // com.twitter.android ↔ x.com, org.telegram.messenger ↔ t.me:
        // neither package nor label yields a ≥3-char brand in the list.
        val apps = listOf(
            AppMeta("com.twitter.android", "X"),
            AppMeta("org.telegram.messenger", "Telegram"),
        )
        val out = SmartAppMatcher.matchedPackages(apps, blocked)
        assertTrue("com.twitter.android" in out)
        assertTrue("org.telegram.messenger" in out)
    }

    @Test fun noMatch_forUnrelatedApp() {
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("ru.gosuslugi.app", "Госуслуги")), blocked
        )
        assertTrue(out.isEmpty())
    }

    @Test fun structuralSegmentsDoNotMatch() {
        // "android"/"com" must never be treated as brands even if a blocked
        // domain happened to be e.g. "android.com".
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("com.example.weather", "Weather")),
            setOf("com.com", "android.com"),
        )
        assertTrue(out.isEmpty())
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests "com.resultv.android.vpn.SmartAppMatcherTest"`
Expected: FAIL — `SmartAppMatcher` / `AppMeta` unresolved (compilation error).

- [ ] **Step 3: Write minimal implementation**

Create `android/app/src/main/java/com/resultv/android/vpn/SmartAppMatcher.kt`:

```kotlin
package com.resultv.android.vpn

/** Package + human label of one installed app. Android-free so the matcher
 *  is JVM-testable (the unit-test source set has no Robolectric). */
data class AppMeta(val packageName: String, val label: String)

/**
 * Decides which installed apps are "associated with a blocked resource" and
 * should therefore ride the VPN in Smart mode. Pure: no Context, no I/O.
 *
 * Matching is deliberately heuristic — the analogue of the domain blocklist
 * at package granularity, where no canonical list exists. Recall boosters:
 * package segments, human label, and a small alias table for rebrands /
 * short names the ≥3-char brand rule can't reach. Misses are recoverable via
 * the manual "в VPN" list; false positives only over-tunnel an app.
 */
object SmartAppMatcher {

    /** Rebrands / short names where neither package nor label yields a
     *  ≥3-char brand present in the list. Package → a domain in the blocklist. */
    val DEFAULT_ALIASES: Map<String, String> = mapOf(
        "com.twitter.android" to "x.com",
        "com.zhiliaoapp.musically" to "tiktok.com",
        "com.zhiliaoapp.musically.go" to "tiktok.com",
        "org.telegram.messenger" to "t.me",
        "org.telegram.messenger.web" to "t.me",
        "org.thunderdog.challegram" to "t.me",
    )

    // Structural / vendor package segments that are never brands.
    private val STOP_SEGMENTS = setOf(
        "com", "org", "net", "io", "co", "app", "apps", "android",
        "mobile", "client", "free", "pro", "www", "the",
    )

    /** Second-level labels (≥3 chars) of the blocklist, e.g. youtube.com → "youtube". */
    fun brandsFrom(domains: Collection<String>): Set<String> {
        val out = HashSet<String>()
        for (raw in domains) {
            val host = raw.trim().lowercase().trimStart('.')
            if (host.isEmpty()) continue
            val labels = host.split('.')
            if (labels.size < 2) continue
            val sld = labels[labels.size - 2]
            if (sld.length >= 3) out.add(sld)
        }
        return out
    }

    fun matchedPackages(
        apps: List<AppMeta>,
        blockedDomains: Collection<String>,
        aliases: Map<String, String> = DEFAULT_ALIASES,
    ): Set<String> {
        val brands = brandsFrom(blockedDomains)
        val blockedSet = blockedDomains.mapTo(HashSet()) { it.trim().lowercase().trimStart('.') }
        val out = HashSet<String>()
        for (app in apps) {
            if (matches(app, brands, blockedSet, aliases)) out.add(app.packageName)
        }
        return out
    }

    private fun matches(
        app: AppMeta,
        brands: Set<String>,
        blockedDomains: Set<String>,
        aliases: Map<String, String>,
    ): Boolean {
        aliases[app.packageName]?.let { domain ->
            if (domain.trim().lowercase().trimStart('.') in blockedDomains) return true
        }
        for (tok in candidateTokens(app)) {
            if (tok in brands) return true
        }
        return false
    }

    private fun candidateTokens(app: AppMeta): Set<String> {
        val out = HashSet<String>()
        for (seg in app.packageName.lowercase().split('.')) {
            if (seg.length >= 3 && seg !in STOP_SEGMENTS) out.add(seg)
        }
        for (word in app.label.lowercase().split(Regex("[^a-z0-9]+"))) {
            if (word.length >= 3 && word !in STOP_SEGMENTS) out.add(word)
        }
        return out
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests "com.resultv.android.vpn.SmartAppMatcherTest"`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/SmartAppMatcher.kt \
        android/app/src/test/java/com/resultv/android/vpn/SmartAppMatcherTest.kt
git commit -m "feat(android): SmartAppMatcher — match installed apps to blocklist brands"
```

---

### Task 2: AppTunnelMembership (pure allowlist composition + toggle)

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/vpn/AppTunnelMembership.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/AppTunnelMembershipTest.kt`

**Interfaces:**
- Consumes: `AppRulesState` (from `AppRules.kt`: `outOfVpn`, `intoVpn`, `blocked`, `withAction`, `withoutAction`), `RuleAction`.
- Produces:
  - `AppTunnelMembership.smartAllowlist(matched: Set<String>, browsers: Set<String>, intoVpn: Set<String>, outOfVpn: Set<String>, ownPackage: String): Set<String>`
  - `AppTunnelMembership.isAuto(pkg: String, matched: Set<String>, browsers: Set<String>): Boolean`
  - `AppTunnelMembership.isInSmart(pkg: String, matched: Set<String>, browsers: Set<String>, rules: AppRulesState): Boolean`
  - `AppTunnelMembership.setSmartMembership(rules: AppRulesState, pkg: String, wantIn: Boolean, isAuto: Boolean): AppRulesState`

- [ ] **Step 1: Write the failing test**

Create `android/app/src/test/java/com/resultv/android/vpn/AppTunnelMembershipTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppTunnelMembershipTest {

    private val own = "com.resultv.android"

    @Test fun allowlist_unionMinusExclusionsMinusOwn() {
        val allow = AppTunnelMembership.smartAllowlist(
            matched = setOf("a", "b"),
            browsers = setOf("chrome", own),   // own must be dropped even if a "browser"
            intoVpn = setOf("c"),
            outOfVpn = setOf("b"),               // exclusion wins over match
            ownPackage = own,
        )
        assertEquals(setOf("a", "chrome", "c"), allow)
    }

    @Test fun include_manualApp_clearsExclusionAndAddsIntoVpn() {
        val before = AppRulesState(outOfVpn = setOf("m"))
        val after = AppTunnelMembership.setSmartMembership(before, "m", wantIn = true, isAuto = false)
        assertTrue("m" in after.intoVpn)
        assertFalse("m" in after.outOfVpn)
    }

    @Test fun include_autoApp_onlyClearsExclusion() {
        val before = AppRulesState(outOfVpn = setOf("auto"))
        val after = AppTunnelMembership.setSmartMembership(before, "auto", wantIn = true, isAuto = true)
        assertFalse("auto" in after.outOfVpn)
        assertFalse("auto" in after.intoVpn) // stays auto, no manual entry
    }

    @Test fun exclude_autoApp_addsToOutOfVpn() {
        val before = AppRulesState()
        val after = AppTunnelMembership.setSmartMembership(before, "auto", wantIn = false, isAuto = true)
        assertTrue("auto" in after.outOfVpn)
    }

    @Test fun exclude_manualApp_removesIntoVpn() {
        val before = AppRulesState(intoVpn = setOf("m"))
        val after = AppTunnelMembership.setSmartMembership(before, "m", wantIn = false, isAuto = false)
        assertFalse("m" in after.intoVpn)
        assertFalse("m" in after.outOfVpn)
    }

    @Test fun isInSmart_reflectsAutoUnionManualMinusExclusion() {
        val rules = AppRulesState(intoVpn = setOf("m"), outOfVpn = setOf("x"))
        assertTrue(AppTunnelMembership.isInSmart("m", emptySet(), emptySet(), rules))
        assertTrue(AppTunnelMembership.isInSmart("auto", setOf("auto"), emptySet(), rules))
        assertFalse(AppTunnelMembership.isInSmart("x", setOf("x"), emptySet(), rules)) // excluded wins
        assertFalse(AppTunnelMembership.isInSmart("none", emptySet(), emptySet(), rules))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests "com.resultv.android.vpn.AppTunnelMembershipTest"`
Expected: FAIL — `AppTunnelMembership` unresolved.

- [ ] **Step 3: Write minimal implementation**

Create `android/app/src/main/java/com/resultv/android/vpn/AppTunnelMembership.kt`:

```kotlin
package com.resultv.android.vpn

/**
 * Pure composition of the Smart-mode tunnel allowlist and the per-app
 * include/exclude transitions the UI drives. No Context, no I/O.
 *
 * Membership layer only: which apps enter the TUN. Routing (which of their
 * traffic hits the proxy) is the engine's job, unchanged.
 */
object AppTunnelMembership {

    /** Packages to hand VpnService.Builder.addAllowedApplication in Smart. */
    fun smartAllowlist(
        matched: Set<String>,
        browsers: Set<String>,
        intoVpn: Set<String>,
        outOfVpn: Set<String>,
        ownPackage: String,
    ): Set<String> {
        val allow = HashSet<String>(matched.size + browsers.size + intoVpn.size)
        allow.addAll(matched)
        allow.addAll(browsers)
        allow.addAll(intoVpn)
        allow.removeAll(outOfVpn)
        allow.remove(ownPackage)
        return allow
    }

    fun isAuto(pkg: String, matched: Set<String>, browsers: Set<String>): Boolean =
        pkg in matched || pkg in browsers

    /** Effective "rides the VPN in Smart" state for a single app. */
    fun isInSmart(
        pkg: String,
        matched: Set<String>,
        browsers: Set<String>,
        rules: AppRulesState,
    ): Boolean {
        if (pkg in rules.outOfVpn) return false
        return isAuto(pkg, matched, browsers) || pkg in rules.intoVpn
    }

    /**
     * Toggle one app's Smart membership. Reuses the AppRulesState transitions
     * so the block/route invariants hold:
     * - wantIn : clear any exclusion; if not auto, record a manual include.
     * - wantOut: clear any manual include; if auto, record a manual exclusion.
     */
    fun setSmartMembership(
        rules: AppRulesState,
        pkg: String,
        wantIn: Boolean,
        isAuto: Boolean,
    ): AppRulesState = if (wantIn) {
        val cleared = rules.withoutAction(pkg, RuleAction.OutOfVpn)
        if (isAuto) cleared else cleared.withAction(pkg, RuleAction.IntoVpn)
    } else {
        val cleared = rules.withoutAction(pkg, RuleAction.IntoVpn)
        if (isAuto) cleared.withAction(pkg, RuleAction.OutOfVpn) else cleared
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests "com.resultv.android.vpn.AppTunnelMembershipTest"`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/AppTunnelMembership.kt \
        android/app/src/test/java/com/resultv/android/vpn/AppTunnelMembershipTest.kt
git commit -m "feat(android): AppTunnelMembership — Smart allowlist composition + toggle"
```

---

### Task 3: Android glue — inventory, domain accessor, repo transition, RuleAction doc

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/vpn/AppInventory.kt`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/SmartListRepository.kt` (add `currentDomains()` near `toEngineList()`, ~line 157)
- Modify: `android/app/src/main/java/com/resultv/android/vpn/AppRouting.kt` (add `setSmartMembership`, `allowedPackages`; ~after line 52)
- Modify: `android/app/src/main/java/com/resultv/android/vpn/RuleAction.kt` (doc only)

**Interfaces:**
- Consumes: `AppMeta` (Task 1), `AppTunnelMembership`, `SmartAppMatcher` (Tasks 1–2), `AppRulesState`.
- Produces:
  - `AppInventory.installedApps(ctx: Context): List<AppMeta>`
  - `AppInventory.browserPackages(ctx: Context): Set<String>`
  - `SmartListRepository.currentDomains(): List<String>`
  - `AppRoutingRepository.setSmartMembership(pkg: String, wantIn: Boolean, isAuto: Boolean)`
  - `AppRoutingRepository.smartAllowlist(matched: Set<String>, browsers: Set<String>): Set<String>`

- [ ] **Step 1: Create AppInventory.kt**

This is a thin `Context`/`PackageManager` adapter (not unit-tested — no Robolectric). It reuses the same catalogue shape `RulesScreen.loadInstalledApps` already builds, minus the icon.

Create `android/app/src/main/java/com/resultv/android/vpn/AppInventory.kt`:

```kotlin
package com.resultv.android.vpn

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.util.Log

private const val TAG = "ResultV/AppInventory"

/**
 * Thin PackageManager adapter for tunnel-membership computation. Kept Context-
 * bound and free of routing logic so SmartAppMatcher / AppTunnelMembership stay
 * pure and JVM-testable.
 */
object AppInventory {

    /** Installed apps as (package, label). Excludes our own package. */
    fun installedApps(ctx: Context): List<AppMeta> {
        val pm = ctx.packageManager
        val own = ctx.packageName
        return pm.getInstalledApplications(PackageManager.GET_META_DATA)
            .asSequence()
            .filter { it.packageName != own }
            .map { AppMeta(it.packageName, pm.getApplicationLabel(it).toString()) }
            .toList()
    }

    /**
     * Packages that can handle a plain http(s) VIEW intent — i.e. browsers.
     * They must ride the VPN so arbitrary blocked sites open. Best-effort:
     * a query failure yields an empty set (matcher/manual list still apply).
     */
    fun browserPackages(ctx: Context): Set<String> {
        val intent = Intent(Intent.ACTION_VIEW, Uri.parse("http://example.com"))
            .addCategory(Intent.CATEGORY_BROWSABLE)
        return try {
            ctx.packageManager
                .queryIntentActivities(intent, PackageManager.MATCH_ALL)
                .mapNotNull { it.activityInfo?.packageName }
                .filter { it != ctx.packageName }
                .toSet()
        } catch (t: Throwable) {
            Log.w(TAG, "browser query failed", t)
            emptySet()
        }
    }
}
```

- [ ] **Step 2: Add `currentDomains()` to SmartListRepository**

In `SmartListRepository.kt`, immediately after the `toEngineList()` function (ends ~line 157), add:

```kotlin
    /** Snapshot of blocked domains, for per-app brand matching (Smart membership). */
    fun currentDomains(): List<String> = _state.value.domains
```

- [ ] **Step 3: Add repository transitions to AppRouting.kt**

In `AppRoutingRepository` (`AppRouting.kt`), after `clearAction` (~line 52), add:

```kotlin
    /** UI toggle for Smart per-app membership (auto ∪ manual − exclusions). */
    @Synchronized
    fun setSmartMembership(pkg: String, wantIn: Boolean, isAuto: Boolean) {
        if (pkg == ownPackage) return
        mutate { AppTunnelMembership.setSmartMembership(it, pkg, wantIn, isAuto) }
    }

    /** Packages handed to VpnService.Builder.addAllowedApplication (Smart only). */
    fun smartAllowlist(matched: Set<String>, browsers: Set<String>): Set<String> =
        AppTunnelMembership.smartAllowlist(
            matched = matched,
            browsers = browsers,
            intoVpn = _state.value.intoVpn,
            outOfVpn = _state.value.outOfVpn,
            ownPackage = ownPackage,
        )
```

- [ ] **Step 4: Update RuleAction doc**

In `RuleAction.kt`, replace the doc comment above the enum with:

```kotlin
/**
 * What a rule does with a package's or a domain's traffic.
 *
 * [OutOfVpn] excludes an app from the tunnel in BOTH modes (the shared "never
 * tunnel this app" list): in Global it bypasses the proxy, in Smart it also
 * removes the app from the allowlist so VPN-hostile apps can't detect the VPN.
 * [IntoVpn] is Smart-only (force into the tunnel/proxy). [Block] applies in
 * both — see AppRulesState for the invariant that follows from this.
 */
```

- [ ] **Step 5: Verify it compiles**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: BUILD SUCCESSFUL (no new tests here — this is glue consumed by Tasks 4–5).

- [ ] **Step 6: Commit**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/AppInventory.kt \
        android/app/src/main/java/com/resultv/android/vpn/SmartListRepository.kt \
        android/app/src/main/java/com/resultv/android/vpn/AppRouting.kt \
        android/app/src/main/java/com/resultv/android/vpn/RuleAction.kt
git commit -m "feat(android): app inventory + Smart membership repo transitions"
```

---

### Task 4: Apply the allowlist in BoxPlatform.openTun (Smart branch + fallback)

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/BoxModule.kt:299-307` (`applyAppRouting`)

**Interfaces:**
- Consumes: `AppInventory` (Task 3), `SmartAppMatcher` (Task 1), `AppRoutingRepository.smartAllowlist` (Task 3), `RoutingRulesRepository.state`, `SmartListRepository.currentDomains` (Task 3).
- Produces: no new symbols — behavioural change to `openTun`.

**Context:** `applyAppRouting(builder)` runs inside `BoxPlatform.openTun` with `service` (a `Context`). Global path is unchanged (denylist). Smart path computes the allowlist and calls `addAllowedApplication`. If the allowlist is empty (no browser, no match — e.g. blocklist not downloaded yet), fall back to the denylist path so Smart never becomes a no-op tunnel.

- [ ] **Step 1: Replace `applyAppRouting`**

In `BoxModule.kt`, replace the whole `applyAppRouting` function (lines 299-307) with:

```kotlin
    private fun applyAppRouting(builder: VpnService.Builder) {
        val ownPkg = service.packageName
        val mode = RoutingRulesRepository.state.value.mode
        if (mode == RoutingMode.Smart && applySmartAllowlist(builder, ownPkg)) return

        // Global (or Smart fallback): denylist. Own package + "out of VPN" apps.
        tryDisallow(builder, ownPkg)
        for (pkg in AppRoutingRepository.disallowedPackages(mode)) {
            if (pkg == ownPkg) continue
            tryDisallow(builder, pkg)
        }
    }

    /**
     * Smart membership: put ONLY blocked-associated apps, browsers and the
     * manual "в VPN" list into the tunnel (minus "out of VPN"). Everything else
     * stays out, so VPN-hostile apps (gov, banks) never see tun0.
     *
     * Returns false when the computed allowlist is empty (blocklist not yet
     * downloaded, no browser) — the caller then falls back to the denylist so
     * the tunnel still carries traffic.
     */
    private fun applySmartAllowlist(builder: VpnService.Builder, ownPkg: String): Boolean {
        val apps = AppInventory.installedApps(service)
        val browsers = AppInventory.browserPackages(service)
        val matched = SmartAppMatcher.matchedPackages(apps, SmartListRepository.currentDomains())
        val allow = AppRoutingRepository.smartAllowlist(matched = matched, browsers = browsers)
        if (allow.isEmpty()) {
            Log.w(TAG, "Smart allowlist empty — falling back to denylist")
            return false
        }
        var added = 0
        for (pkg in allow) {
            if (pkg == ownPkg) continue
            try {
                builder.addAllowedApplication(pkg)
                added++
            } catch (t: Throwable) {
                // App uninstalled between enumeration and establish, etc.
                Log.w(TAG, "addAllowedApplication($pkg) failed", t)
                AppLog.warning(R.string.log_app_route_failed, pkg)
            }
        }
        if (added == 0) {
            Log.w(TAG, "Smart allowlist added 0 apps — falling back to denylist")
            return false
        }
        Log.i(TAG, "Smart allowlist: $added app(s) in tunnel")
        AppLog.info(R.string.log_smart_allowlist, added, source = EngineLog.ENGINE)
        return true
    }
```

- [ ] **Step 2: Add the log string (both locales)**

In `android/app/src/main/res/values/strings.xml`, add:

```xml
    <string name="log_smart_allowlist">Smart: %1$d app(s) routed through VPN</string>
```

In `android/app/src/main/res/values-ru/strings.xml`, add:

```xml
    <string name="log_smart_allowlist">Smart: приложений через VPN — %1$d</string>
```

- [ ] **Step 3: Verify it compiles**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: BUILD SUCCESSFUL.

- [ ] **Step 4: Full unit-test sweep (guards no regression in pure logic)**

Run: `cd android && ./gradlew :app:testDebugUnitTest`
Expected: PASS (existing suites + Tasks 1–2).

- [ ] **Step 5: Commit**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/BoxModule.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml
git commit -m "feat(android): Smart mode uses per-app allowlist in openTun"
```

---

### Task 5: RulesScreen — Smart per-app auto-state + override toggle

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RulesScreen.kt` (`PerAppRoutingSection` ~516-641, `AppRow` ~643-689)
- Modify: `android/app/src/main/res/values/strings.xml`, `.../values-ru/strings.xml` (badge + Smart hint strings)

**Interfaces:**
- Consumes: `AppInventory` (Task 3), `SmartAppMatcher` (Task 1), `AppTunnelMembership` (Task 2), `AppRoutingRepository.setSmartMembership` (Task 3), `SmartListRepository`.
- Produces: no new public symbols — UI behaviour for Smart mode.

**Context:** In Smart, the "в VPN" tab must show each app's *effective* membership (auto ∪ manual − exclusions), badge auto-included apps, and toggle via `setSmartMembership`. The `Block` tab and all of Global stay exactly as they are. The whole app catalogue is already loaded once (`apps`); we compute the auto set the same way.

- [ ] **Step 1: Compute the auto set inside PerAppRoutingSection**

In `PerAppRoutingSection`, right after the block that loads `apps` (the `LaunchedEffect(Unit) { ... loadInstalledApps ... }`, ends ~line 558), add a derived-state block that computes the auto-included set for Smart:

```kotlin
    val smartSnapshot by SmartListRepository.state.collectAsStateWithLifecycle()
    var autoIn by remember { mutableStateOf<Set<String>>(emptySet()) }
    LaunchedEffect(mode, apps, smartSnapshot.domains) {
        autoIn = if (mode != RoutingMode.Smart || apps.isEmpty()) {
            emptySet()
        } else withContext(Dispatchers.IO) {
            val meta = apps.map { AppMeta(it.packageName, it.label) }
            val matched = SmartAppMatcher.matchedPackages(meta, smartSnapshot.domains)
            val browsers = AppInventory.browserPackages(ctx)
            matched + browsers
        }
    }
```

(Add imports as needed: `com.resultv.android.vpn.AppInventory`, `com.resultv.android.vpn.AppMeta`, `com.resultv.android.vpn.AppTunnelMembership`, `com.resultv.android.vpn.SmartAppMatcher`, `com.resultv.android.vpn.SmartListRepository`.)

- [ ] **Step 2: Branch the row rendering for the Smart "в VPN" tab**

Replace the `items(filtered, ...) { app -> ... }` body (lines ~623-637) with:

```kotlin
                items(filtered, key = { it.packageName }) { app ->
                    val smartMembershipTab = mode == RoutingMode.Smart && tab == RuleAction.IntoVpn
                    if (smartMembershipTab) {
                        val auto = app.packageName in autoIn
                        val inVpn = AppTunnelMembership.isInSmart(
                            app.packageName, autoIn, emptySet(), appRules
                        )
                        val blocked = app.packageName in appRules.blocked
                        AppRow(
                            app = app,
                            checked = inVpn,
                            blockedElsewhere = blocked,
                            autoBadge = auto && inVpn,
                            onToggle = {
                                AppRoutingRepository.setSmartMembership(
                                    app.packageName, wantIn = !inVpn, isAuto = auto
                                )
                            },
                        )
                    } else {
                        val effective = appRules.actionOf(app.packageName, mode)
                        AppRow(
                            app = app,
                            checked = effective == tab,
                            blockedElsewhere = effective == RuleAction.Block && tab != RuleAction.Block,
                            autoBadge = false,
                            onToggle = {
                                if (effective == tab) AppRoutingRepository.clearAction(app.packageName, tab)
                                else AppRoutingRepository.setAction(app.packageName, tab)
                            },
                        )
                    }
                }
```

(Note: `autoIn` is used as the `matched` argument and `emptySet()` as `browsers` in `isInSmart` because browsers are already folded into `autoIn` in Step 1.)

- [ ] **Step 3: Add the `autoBadge` parameter to AppRow**

In `AppRow` (signature ~643-649), add the parameter and render the badge. Replace the signature and the `Column` label block:

```kotlin
@Composable
private fun AppRow(
    app: InstalledApp,
    checked: Boolean,
    blockedElsewhere: Boolean,
    autoBadge: Boolean,
    onToggle: () -> Unit,
) {
```

Inside the `Column(modifier = Modifier.padding(start = 12.dp).weight(1f))`, directly under the `Text(app.label, ...)` line, add:

```kotlin
            if (autoBadge) {
                Text(
                    "✓ " + stringResource(R.string.rules_badge_auto_vpn),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.GreenLight,
                )
            }
```

- [ ] **Step 4: Add the strings (both locales)**

In `android/app/src/main/res/values/strings.xml`:

```xml
    <string name="rules_badge_auto_vpn">auto: through VPN</string>
```

In `android/app/src/main/res/values-ru/strings.xml`:

```xml
    <string name="rules_badge_auto_vpn">авто: через VPN</string>
```

- [ ] **Step 5: Verify it compiles**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: BUILD SUCCESSFUL.

- [ ] **Step 6: Assemble the debug APK (catches Compose/resource wiring)**

Run: `cd android && ./gradlew :app:assembleDebug`
Expected: BUILD SUCCESSFUL.

- [ ] **Step 7: Commit**

```bash
git add android/app/src/main/java/com/resultv/android/ui/screens/RulesScreen.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml
git commit -m "feat(android): Smart per-app UI — auto-state badge + membership override"
```

---

### Task 6: Manual on-device verification

**Files:** none (verification only).

No automated test can cover VpnService membership + real VPN-detecting apps, so verify on a device/emulator (API 29+ for full behaviour; the allowlist itself works from API 26).

- [ ] **Step 1: Build & install**

Run: `cd android && ./gradlew :app:installDebug`

- [ ] **Step 2: Smart + a VPN-hostile app**

Switch to Smart, connect, open Госуслуги (or a bank app). Expected: it does not warn about VPN and works normally (it is NOT in the allowlist → outside the tunnel). Confirm in logcat: `Smart allowlist: N app(s) in tunnel` and that the app's package is absent.

- [ ] **Step 3: Smart + blocked apps**

Open Instagram / YouTube / TikTok. Expected: they load blocked content through the VPN (auto-matched — TikTok via label). Confirm each shows "авто: через VPN" (checked) in Rules → apps → «в VPN».

- [ ] **Step 4: Smart + browser + blocked site**

Open a blocked site in Chrome. Expected: it loads (browser auto-included).

- [ ] **Step 5: Manual override both directions**

Manually check a deliberately-unmatched app (e.g. a niche app that needs a blocked resource) → it starts working through VPN after reconnect. Uncheck an auto-included browser → it drops out of the tunnel after reconnect.

- [ ] **Step 6: Global unchanged**

Switch to Global. Confirm the app list still shows `[из VPN, запретить]` and "из VPN" excludes as before.

---

## Self-Review

**Spec coverage** (against `2026-07-23-smart-per-app-vpn-routing-design.md`):
- §3 allowlist formula → Task 2 (`smartAllowlist`) + Task 4 (application). ✓
- §3 empty-allowlist fallback → Task 4 `applySmartAllowlist` returns false → denylist. ✓
- §3 Global unchanged → Task 4 keeps the denylist path. ✓
- §4 `SmartAppMatcher` → Task 1; `AppTunnelMembership` → Task 2; browser detection → Task 3 (`AppInventory.browserPackages`); alias table → Task 1 (`DEFAULT_ALIASES`); `SmartListRepository` domain accessor → Task 3; `RuleAction` doc → Task 3; `BoxModule` → Task 4; UI → Task 5. ✓
- §6 matcher (package + label + alias, ≥3 threshold, stop-segments) → Task 1. ✓
- §8 UI auto-state badge + two-way override, no third tab → Task 5. ✓
- §9 API<29 (allowlist works) → allowlist uses `addAllowedApplication` (all levels); documented in Global Constraints. ✓
- §9 empty blocklist on first connect → Task 4 fallback + Task 5 recomputes on `smartSnapshot.domains` change. ✓
- §10 no data migration (reuse `outOfVpn`/`intoVpn`) → Tasks 2–3 use existing `AppRulesState`. ✓
- §11 Kotlin tests → Tasks 1–2; on-device → Task 6. ✓
- **Out of scope for this plan (Plan 2):** §7 IP pipeline, engine `ip_cidr` rule, provider IP source. Not covered here by design.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every command has expected output. ✓

**Type consistency:** `AppMeta(packageName,label)` consistent across Tasks 1/3/5. `matchedPackages(apps, blockedDomains, aliases=DEFAULT_ALIASES)` called with 2 args in Tasks 4/5 (default alias) — matches signature. `smartAllowlist(matched,browsers,intoVpn,outOfVpn,ownPackage)` (Task 2) wrapped by `AppRoutingRepository.smartAllowlist(matched,browsers)` (Task 3), called in Task 4. `isInSmart(pkg,matched,browsers,rules)` (Task 2) called in Task 5 with `browsers=emptySet()` because browsers are pre-folded into `autoIn`. `setSmartMembership(rules,pkg,wantIn,isAuto)` (Task 2) → `AppRoutingRepository.setSmartMembership(pkg,wantIn,isAuto)` (Task 3) → called in Task 5. ✓
