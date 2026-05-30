# ResultV Android Migration Plan

Migration of the ResultV desktop client (Wails v2 + Go + sing-box) to Android.
Go core stays shared; UI is rebuilt as a native Android app (Kotlin + Compose,
Material 3) that calls into Go via a `gomobile bind` AAR.

---

## Phase 0 — Go core builds for Android — ✅ DONE

- [x] `mobile` build tag on desktop-only files (Wails runtime, systray,
      autostart, killswitch).
- [x] `mobile/` Go package with gomobile-friendly API (string / int64 / bool / []byte).
- [x] Workaround for golang.org/issues/68760 (`pidfd_android.go` + `-checklinkname=0`).
- [x] First successful AAR via `gomobile bind`.

## Phase 1 — Gradle skeleton + libbox AAR loads — ✅ DONE

- [x] `android/` module: AGP 8.13, Kotlin 2.0, Compose, Java 17.
- [x] `app/build.gradle.kts` with `compileSdk=34`, `minSdk=26`, BuildConfig.
- [x] `local.properties` → `BuildConfig.VLESS_URI` for PoC test URI (gitignored).
- [x] `proguard-rules.pro` keeps `libbox.**`, `mobile.**`, `go.**`.
- [x] AAR loads in real APK; `Mobile.version()` and `Mobile.parseProxyURI()`
      verified on Pixel 9 Pro emulator (API 36).
- [x] Bind both `./mobile` + `experimental/libbox` into one AAR (sagernet's
      gomobile fork, blank-import keeps it as a direct go.mod dep).

## Phase 2 — VPN data plane (PoC) — ✅ DONE

- [x] `ResultVpnService` (foreground service, `specialUse` FGS type).
- [x] `BoxModule` singleton: `Libbox.setup`, `CommandServer`, lifecycle.
- [x] `BoxPlatform : PlatformInterface` — `openTun`, `protect`, stubs, custom
      `findConnectionOwner` that throws to dodge a wrapper nil-deref.
- [x] FD lifecycle: service owns `tunPfd`, closes on STOP/onRevoke/onDestroy.
- [x] Android-specific config tweaks in mobile wrapper:
      `strict_route=false`, `auto_route=true`, no `route_exclude_address`,
      `auto_detect_interface=false` (rejected by sing-box on Android),
      `process_path_regex` rules stripped (SELinux denies /proc/net/tcp),
      `dns.local` server replaced with `udp 1.1.1.1` via `direct` outbound
      (no resolv.conf / 127.0.0.1:53 daemon on Android).
- [x] Tested working (emulator): VLESS+REALITY+XHTTP packet-up, stream-up,
      Trojan+REALITY+gRPC.

## Phase 3 — Stability & polish — ⚠️ ONE ITEM LEFT

- [x] `BoxModule.start` off the main thread (single-thread Executor).
- [x] Connection state machine: `Idle / Connecting / Connected / Error` via
      `VpnState: StateFlow`, surfaced with spinner in Compose.
- [x] Notification: title+text reflect state, "Disconnect" action button.
- [x] Optimistic UI on Disconnect (state flips immediately, slow stop runs in BG).
- [x] **Live config reload** — `BoxModule.reload(configJson)` calls
      libbox's `startOrReloadService` on the running CommandServer; engine
      swaps in-place and re-invokes `BoxPlatform.openTun` so per-app routing
      changes also take effect. `ResultVpnService` runs a flow watcher
      (`combine(RoutingRulesRepository, AppRoutingRepository, ProfileRepository)
      .distinctUntilChanged().drop(1).debounce(300)`) that rebuilds the
      sing-box config and calls reload — so domain exclusions, per-app rules,
      and active-profile switches apply without disconnect/reconnect.
- [x] Auto-reconnect on `onRevoke()` via notification — separate channel
      `resultv_vpn_revoke`, action button "Reconnect" launches MainActivity
      with `EXTRA_RECONNECT_AFTER_REVOKE` which auto-triggers `connect()`
      after the user reconfirms the system VPN consent dialog.
- [x] Replace `android.R.drawable.ic_lock_lock` with a real app icon —
      adaptive launcher icon driven by the brand PNG
      (`mipmap-xxxhdpi/ic_launcher_logo.png`, 1080×1080) wrapped in
      `drawable/ic_launcher_foreground.xml` (`<inset 20%>` so the logo
      reads as a shield inside the launcher mask, white background).
      Splash screen: `drawable/splash_background.xml` (logo on
      `#060608`) + `Theme.ResultV.Splash` swapped to `Theme.ResultV` in
      `MainActivity.onCreate` before `super`. Notification small icon
      switched to `R.drawable.ic_notification` but the path didn't
      render at 24dp; left as a flat shield placeholder, low priority.
- [ ] Verify `addDisallowedApplication(packageName)` survives package rename
      for release builds.
- [x] Drop hardcoded `log.level=debug`. Now parametrised via
      `EngineConfig.LogLevel` / `BuildOptions.logLevel`; default is `info`,
      release builds stay quiet, the Kotlin side persists the choice in
      `SettingsRepository`.

## Phase 4 — Profile management — ⚠️ TWO ITEMS LEFT (QR / edit-reorder)

- [x] Persist proxy profiles in `profiles.json` under `filesDir`
      (`ProfileRepository`, JSON-backed, reactive `StateFlow`).
- [x] `Profile` carries either source `uri` or full parsed `entryJson`.
- [x] Paste-URI import with validation via `Mobile.parseProxyURI`.
- [x] **From clipboard** + **From file (SAF)** quick-import buttons on Add screen.
- [x] Subscription import (`Mobile.fetchSubscription`):
      - Desktop-parity `User-Agent: ResultV/3.1.1`, stable `x-hwid`.
      - `Happ/1.0` UA fallback when primary lacks Hysteria2 / fails.
      - Provider URL normalisation (`/json` suffix for `my.impio.space`).
      - Auto-bundle: collapses N "<name> Auto" duplicates into one virtual
        AUTO entry whose Extra carries members inline; Connect picks
        `members[0]` (latency-driven selection — TODO).
      - Diagnostic error format with URL, response size, preview, parse counts.
- [x] Profile list with delete + active-radio (Proxies tab).
- [x] Connect path prefers `entryJson` when present, falls back to URI.
- [x] Subscription metadata + lifecycle — full slice.
      Engine: `Mobile.FetchSubscriptionV2` returns
      `{entries, userInfo, title}` carrying `Subscription-Userinfo` /
      `Profile-Title` headers; `FinalizeSubscriptionEntries` (engine)
      turns placeholder-host rows into SECTION labels instead of dropping
      them.
      Kotlin: `Subscription` data class, `SubscriptionRepository`
      JSON-backed with cascade delete (`profiles.json` profiles tagged
      via `Profile.subscriptionId`). `ProxiesScreen` shows each
      subscription as a collapsible card with logo, name, server count,
      traffic-quota progress bar + expiry days, refresh + delete IconButtons.
      Refresh re-fetches and reconciles via `replaceForSubscription`
      while preserving favourite flags by name. impVPN logo override
      driven by `source == "rvsub"` or "impvpn" in name/title.
      Host:port hidden for subscription-tagged profiles — show only
      protocol tag.
- [x] Manual protocol picker on Add screen — third tab "Manual" with a 3×N
      grid of protocol cards (VLESS / VMess / Trojan / Shadowsocks / Hysteria2 /
      WireGuard / AmneziaWG). Per-protocol form (typed fields, choice
      dropdowns, password masking) builds a share-URI, validates via
      `Mobile.parseProxyURI`, then persists as a normal `Profile.fromUri`.
      `ManualPane.kt`, pure Kotlin — no AAR rebuild.
      HTTP/HTTPS skipped (engine has no top-level handler); SOCKS5 deferred
      to a separate slice that requires `Profile.fromEntryJson` plumbing.
- [x] Deep-link import — `resultv://import/<base64>`, `rvsub/`, `crypt4/`,
      `i/`, opaque `resultv:import/...`. Manifest intent-filter on
      `resultv` scheme; `MainActivity.handleDeepLinkIntent` delegates to
      `DeepLinkImporter` which calls `Mobile.decodeDeepLink` (RVSUB1 AES-GCM
      via the existing `subscription_decrypt.go` pipeline) and routes
      single-URL plaintext → `Mobile.fetchSubscriptionV2`, multi-line
      bodies → per-line `Mobile.parseProxyURI` import. Rvsub-flavoured
      links auto-tag the subscription so the impVPN logo lights up.
- [x] QR-code scan import — Google Play Services bundled scanner
      (`play-services-code-scanner:16.1.0`, no `CAMERA` permission needed).
      Third quick-import card on `AddScreen` opens the system scanner UI;
      decoded value is routed via `DeepLinkImporter.importPlain` so
      `resultv://`, `http(s)://` (subscription) and bare share-links all
      flow through the same dispatcher. Manifest carries a
      `com.google.mlkit.vision.DEPENDENCIES = barcode_ui` meta-data tag so
      the scanner module preloads on install.
- [x] Edit / rename / drag-reorder profiles — long-press a row on the
      Proxies screen opens a `ModalBottomSheet` (`ProfileEditSheet`) with
      rename + Move up / Move down + Delete. Reorder swaps with the
      immediate neighbour in the same bucket (same `subscriptionId`, no
      crossing SECTION labels) via `ProfileRepository.move(id, ±1)`; move
      buttons disable at edges and when sort mode ≠ Default (under
      ping/country/etc. the visual order ≠ storage order, so reorder would
      be confusing). Drag-handle reorder (gesture-based) skipped — the
      bottom-sheet move buttons cover the same need without pulling in a
      compose-reorderable dep.
- [x] Latency probe + auto-pick on AUTO profile —
      `PingRepository` probes every member, sorts by reachable + latency,
      and `MainActivity.startAutoConnect` walks the sorted list, waiting up
      to 15s per attempt and falling through to the next member on Error /
      timeout. Always-on path still uses engine `members[0]` (no UI to
      drive the failover loop without `lifecycleScope`).
- [x] Favourites: `Profile.isFavorite` persisted in `profiles.json`,
      `ProfileRepository.toggleFavorite(id)`, star toggle in `ServerRow`.
      Home dropdown shows pinned "FAVORITES" section on top; Proxies tab
      sorts favourites first via stable `sortedByDescending { isFavorite }`.
- [x] Subscription title `base64:` prefix decoded —
      `decodePanelTitle` tries `Base64.DEFAULT` then falls back to
      `Base64.URL_SAFE` (xray panels emit standard `+/` alphabet, the
      naive OR-of-flags rejects them). `Subscription.displayName` and
      `subscriptionUsesImpLogo` both go through the decoder, so panels
      that wrap "🚀 impVPN Auto" in base64 render correctly and light
      up the brand logo.
- [x] impVPN brand logo — `res/drawable/imp_logo.png` (copied from
      desktop `ResultV-dev/frontend/src/assets/implogo.png`). Replaces
      the rocket-emoji placeholder in `SubscriptionLogo`.
- [x] Subscription header buttons grouped — refresh/delete/expand
      now share a single compact pill (36dp IconButtons, 18dp icons,
      rounded `Color.White.copy(alpha=0.04f)` background) instead of
      floating with 48dp Material defaults.

## Phase 5 — Routing & per-app rules — ✅ DONE

- [x] `RoutingRulesRepository` — Global / Smart mode + domain exclusions list.
- [x] `AppRoutingRepository` — All / AllowList / DisallowList + selected packages.
- [x] Rules screen redesigned to mirror desktop:
      - Mode cards with green border highlight (Global active, Smart "coming soon").
      - **Domain exclusions** card — input + Add, chip list with X, Quick-add
        chips for `*.ru` `*.рф` `*.su` `*.by` `*.kz`.
      - **Per-app routing** as a section below — segmented All/Allow/Block.
- [x] Per-app routing applied in `BoxPlatform.openTun`
      (addAllowedApplication / addDisallowedApplication, mutually exclusive).
- [x] Domain exclusions wired to sing-box config —
      `Mobile.BuildSingBoxConfig{,FromEntry}` takes an `excludedDomains`
      comma-separated string. `splitDomainPatterns` classifies entries
      (`*.ru` → `domain_suffix=.ru`, `yandex.ru` → exact `domain`) and the
      builder appends a route rule routing matches to `direct`. **Must be
      AFTER the `Action: sniff` rule** — sing-box only knows the IP at TUN
      ingress; SNI/Host (and therefore the domain matcher) only become
      available after sniff runs. Live-reload via `BoxModule.reload`
      means edits apply without disconnect/reconnect (Phase 3).
- [x] **Smart (Antizapret)** — desktop-parity HTTP fetch instead of
      sing-box's remote `route.rule_set` (the old
      `runetfreedom/russia-blocked-geosite/...` URL started 404-ing in
      2026 and the rule_set fetch is synchronous on engine start →
      VPN never came up). New flow:
      - Engine: `EngineConfig.SmartBlockedDomains []string` replaces the
        old `SmartRulesetURL`. `buildRoute` emits a local route rule
        `{domain_suffix: [.x.com, .instagram.com, …], outbound: proxy}`
        built via `splitSmartDomains` (every base domain becomes a
        suffix-match so subdomains are covered).
      - `Mobile.FetchSmartList(country, dataDir)` gomobile binding
        wraps the existing `proxy.HTTPBlockedListProvider` +
        `ResolveBlockedDomains` (citizenlab + itdoginfo sources for RU,
        citizenlab global+country for the rest). Caches in
        `dataDir/smart-blocked.json` with 24h TTL.
      - Kotlin `SmartListRepository` — singleton with `init`,
        `ensureLoaded` (TTL-respecting), `refresh` (force), `refreshAsync`
        / `ensureLoadedAsync`. Persists snapshot to
        `filesDir/smart-list.meta.json`. Default country pinned to `ru`.
      - `MainActivity.onCreate` calls `ensureLoadedAsync()` at app start
        (matches desktop preload behaviour). Rules screen Smart toggle
        triggers `refreshAsync()`. `connect()` blocks on
        `ensureLoaded()` when Smart is on and the list is empty.
      - `BuildOptionsBuilder.currentOptionsJson` emits
        `smartBlockedDomainsList` (newline-separated) which gomobile
        sends to `splitSmartList` → `EngineConfig.SmartBlockedDomains`.
- [x] Bypass-LAN toggle — `EngineConfig.BypassLAN` injects a
      pre-sniff route rule sending RFC1918 / 169.254.0.0/16 /
      224.0.0.0/4 / fc00::/7 / fe80::/10 / ff00::/8 to `direct`.
      Persisted on Kotlin side as `SettingsState.bypassLan` (default on).
- [x] IPv6 toggle — `EngineConfig.IPv6` adds an ULA `/126` to the TUN
      address list and flips `dns.strategy` to `prefer_ipv4`. Off by
      default; Settings → IPv6 wires it through.
- [x] Custom DNS via UI — Kotlin side wired: `SettingsRepository`
      persists `dnsPreset` / `dnsCustom`, `resolveDnsServers()` resolves
      to a comma-separated list, plumbed through both
      `MainActivity.connect()` and `ResultVpnService.triggerReload()`.
      Engine already accepts the `dnsServers` argument on
      `Mobile.buildSingBoxConfig*` — no AAR rebuild needed.

## Phase 6 — Quality of life — ⚠️ TWO ITEMS LEFT (banner / battery stats)

- [x] Material 3 design system: `Brand` palette, `ResultVTheme` with all
      M3 slots filled (no purple leaks).
- [x] `Scaffold` + `CenterAlignedTopAppBar` + `NavigationBar` (5-tab),
      edge-to-edge via `enableEdgeToEdge()` so app draws under status bar
      while M3 Scaffold's `WindowInsets.systemBars` keeps the bottom nav
      bar above the gesture pill.
- [x] Reusable Compose components: `PowerButton`, `ServerRow`, `StatusHeader`,
      `Sparkline`, `Section` wrapper. Unified radii (20dp main cards,
      pill chips). PowerButton uses a radial-gradient halo (no blurred
      opaque disc) for a soft desktop-style glow on connected/error/connecting.
- [x] ServerRow shows numeric ping (`xx ms`, colour-coded < 80 / 80–200 /
      > 200) — placeholder values from `mockLatencyMs(profile.id)` until
      the real probe lands.
- [x] Home statistics: download/upload cards (placeholder values from
      `TrafficStats` — real numbers need libbox `CommandClient` subscription).
- [x] Settings stub:
      - Language switcher with DropdownMenu (EN / RU / ES / DE / FR / ZH) — UI only.
      - General toggles (Kill Switch, Adblock, IPv6) — disabled placeholders.
      - DNS presets via FilterChip (Auto / Google / Cloudflare / Quad9) — UI only.
- [x] Real traffic stats wired from libbox `CommandClient`.
      `TrafficWatcher` (Kotlin singleton) subscribes to `Libbox.CommandStatus`
      at 1Hz, parses `StatusMessage.{uplink,downlink,uplinkTotal,downlinkTotal}`,
      pushes into `TrafficStats.publish` so HomeScreen cards animate.
      Engine side: `SBExperimental.ClashAPI = &SBClashAPI{}` (no
      external controller — clash_api is enabled purely to flip on the
      in-process TrafficManager that libbox reads).
- [x] Real localization — **complete pass**. `locale/LocaleManager.kt`
      persists via SharedPreferences and applies via
      `Activity.attachBaseContext` + `recreate()`. `values/strings.xml` +
      `values-ru/strings.xml` cover all user-visible text: tabs/top bar,
      Home, Settings, Status header, ServerRow, VPN service notifications,
      Add screen (paste/clipboard/file/manual + per-protocol forms),
      Proxies screen, Rules screen, revoke + tile notifications,
      PowerButton contentDescription.
- [x] `vpn/SettingsRepository.kt` — persistent UI settings store
      (DNS preset/custom, kill-switch, adblock, ipv6 — placeholders
      until engine-side wiring lands in next AAR-rebuild session).
- [x] Quick Settings tile — `ResultVTileService` registered in manifest
      with `BIND_QUICK_SETTINGS_TILE`. Mirrors `VpnState.status` to tile
      state (Active/Inactive/Unavailable), one-tap connect/disconnect.
      If consent is needed or device is locked, bounces to MainActivity.
- [x] Always-on VPN compatibility — `ResultVpnService.onStartCommand`
      now lazily inits all repos and, when no `EXTRA_CONFIG_JSON` is
      provided (the OS path for always-on), rebuilds the config from
      `ProfileRepository.active` + `SettingsRepository.resolveDnsServers()`
      + `RoutingRulesRepository.domainExclusions`.
- [x] Home dropdown UX polish —
      - Refresh-ping (left) + sort (right) toolbar placed under the
        PowerButton, always visible (not nested inside the dropdown).
      - Active-server card + expandable list back to one unified Card.
      - `ProfileSortMenu` wraps IconButton + DropdownMenu in a `Box` so
        the popup anchors under the button instead of jumping to the
        popup window's top-left.
      - `Mobile.ping` wired via `PingRepository` (60s ticker +
        on-demand refresh + AUTO member probing). `mockLatencyMs`
        gone. AUTO connect path orders members by reachable+latency
        and falls through on Error (`MainActivity.startAutoConnect`).
- [x] Connection-stats banner (uplink/downlink/duration) on Home —
      `VpnStatus.Connected(connectedAt: Long)` carries the wall-clock
      moment the tunnel came up. `HomeScreen.ConnectionStatsBanner` sits
      between PowerButton and the refresh-ping toolbar (only when
      connected), shows uptime + down + up in a compact 3-cell pill, and
      reads `TrafficStats.snapshot` for live speeds. The existing
      DOWNLOAD/UPLOAD cards stay — banner is a glanceable summary, the
      cards are the detailed view.
- [ ] Battery / data usage stats from libbox `CommandClient`.
- [ ] Material You dynamic color — **dropped** at user request.

## Phase 7 — Release engineering — ⚠️ ONE ITEM LEFT (distribution)

- [x] Multi-ABI release build: `splits.abi` block in `app/build.gradle.kts`
      produces per-ABI APKs for `arm64-v8a` + `armeabi-v7a` + universal.
      Per-ABI `versionCode` offsets for Play Store compatibility.
- [x] App signing config: `keystore.properties` (gitignored) with
      `keystore.properties.example` template. `signingConfigs.release`
      reads credentials; falls back to debug signing when the file is
      absent (CI without secrets, fresh clone).
- [x] R8 minification + ProGuard rules audit: `isMinifyEnabled=true`,
      `isShrinkResources=true` on release. `proguard-rules.pro` keeps
      `mobile.**`, `libbox.**`, `go.**`, `org.golang.app.**`,
      Compose `@Composable` metadata, ML Kit / Code Scanner classes.
- [x] AAR rebuild script: `scripts/build-android-aar.sh` +
      `scripts/build-android-aar.ps1`. `--with-naive` / `-WithNaive`
      flag for future NaiveProxy support.
- [x] CI: `build-android` job in `.github/workflows/release.yml`.
      Builds AAR from source via gomobile, decodes base64 keystore
      from secrets, assembles release APKs, attaches to GitHub Release.
- [x] Version bump: `versionCode=10000`, `versionName="1.0.0"`
      (from `0.2.0-poc` / `versionCode=1`).
- [ ] Play Store listing OR direct-download APK + auto-update flow.

---

## Protocol coverage matrix

| Protocol | Emulator (AVD) | Real device | Notes |
|---|---|---|---|
| VLESS + REALITY + XHTTP (packet-up) | ✅ | ⏳ untested | First PoC |
| VLESS + REALITY + XHTTP (stream-up) | ✅ | ⏳ untested | impVPN subscription |
| Trojan + REALITY + gRPC | ✅ | ⏳ untested | impVPN subscription |
| VMess | ⏳ untested | ⏳ untested | Parser & build supports it |
| Shadowsocks | ⏳ untested | ⏳ untested | Same |
| Hysteria2 | ❌ AVD UDP/QUIC NAT issues | ⏳ untested | Tunnel up, QUIC handshake times out — emulator-only fault expected |
| WireGuard | 🔒 blocked on UI | 🔒 blocked on UI | `.conf` file import not implemented yet |
| AmneziaWG | 🔒 blocked on UI | 🔒 blocked on UI | Same |

A full QA pass on a real device is the next milestone — Phase 5 wiring
and real traffic stats are in, but everything in the matrix is still
emulator-only.

---

## What's left from the original plan

The list below is what's *actually* unchecked above, grouped by partition so
you can plan AAR rebuilds intelligently. Pure-Kotlin items are cheap; Go
items must be batched together to amortise the gomobile rebuild cost.

### A. Pure-Kotlin (no AAR rebuild)

~~A1. Real ping wiring~~ — ✅ DONE. `PingRepository` (singleton,
    `Mobile.ping` via gomobile binding) keyed by `profile.id`, with a
    60s polling ticker started in `MainActivity.onCreate`. AUTO entries
    expand into per-member targets; the repo publishes both the best
    reachable sample per profile (`results`) and raw per-target samples
    (`targetResults`) used by the AUTO connect path. 5s freshness cache
    absorbs duplicate probes from sort recomposition + manual refresh.

~~A2. Sort by ping + provider grouping on Home~~ — ✅ DONE. Shared
    `ProfileSortMode` enum + `sortProfiles` helper + `ProfileSortMenu`
    composable in `ui/components/ProfileSort.kt`. Modes: Default / Ping /
    Country / Type / Name. Favourites pin first in every mode.
    `ProxiesScreen` shows the menu beside the count header and applies
    per-subscription too (SECTION rows still anchor the row blocks).
    `HomeScreen` dropdown gets its own menu. Subscription grouping on
    the Home dropdown is already implicit via favourites partition —
    explicit per-provider headers there would just blow the 6-row cap.

~~A3. AUTO latency probe~~ — ✅ DONE. See Phase 4 entry above:
    `PingRepository.probeAndSort` returns members ordered by reachable
    + latency; `MainActivity.startAutoConnect` walks them with a 15s
    per-attempt deadline. `BuildOptionsBuilder.buildConfigFromEntry`
    lets the failover loop swap the engine's `members[0]` default for
    the chosen target without mutating the user-visible Profile.

~~A4. QR-code scan import~~ — ✅ DONE. Implemented via Google Play
    Services `play-services-code-scanner:16.1.0` (no `CAMERA`
    permission, system-provided UI). QR-card on `AddScreen` calls
    `GmsBarcodeScanning.getClient(...).startScan()`; decoded payload
    is fed to `DeepLinkImporter.importPlain(ctx, raw)` which handles
    `resultv://` deep-links, `http(s)://` subscription URLs, and
    bare share-links through one dispatch path.

~~A5. Edit / rename / drag-reorder profiles~~ — ✅ DONE. Long-press a
    row on Proxies opens `ProfileEditSheet` (M3 `ModalBottomSheet`)
    with rename + Move up / Move down + Delete. Reorder uses
    `ProfileRepository.move(id, ±1)` swapping with the immediate
    neighbour in the same bucket (`subscriptionId` match, no crossing
    SECTION labels). Move buttons disable at edges and outside
    Default sort. Drag-handle gesture skipped — Move buttons cover
    the same need without a new dep.

A6. **Verify `addDisallowedApplication(packageName)` survives package
    rename for release builds** — currently we pass `service.packageName`
    which is the *applicationId*, not the manifest package. With ProGuard
    on the release build the manifest package may get rewritten;
    sanity-test once the keystore lands (Phase 7).

~~A7. Connection-stats banner on Home~~ — ✅ DONE.
    `VpnStatus.Connected(connectedAt: Long)` is now a data class
    carrying the wall-clock moment the tunnel came up.
    `ConnectionStatsBanner` (3-cell pill: UPTIME · DOWN · UP) sits
    between PowerButton and the refresh-ping toolbar when connected,
    reading live speeds from `TrafficStats.snapshot` and ticking the
    duration once per second via a `LaunchedEffect(connectedAt)`
    loop.

~~A8. impVPN logo as real drawable~~ — ✅ DONE.
    `res/drawable/imp_logo.png` copied from `ResultV-dev/frontend/src/
    assets/implogo.png`; `SubscriptionLogo` uses
    `painterResource(R.drawable.imp_logo)` and the rocket-emoji
    fallback is gone.

### B. Go + AAR rebuild (batch these)

⚠️ B1. **NaiveProxy support — parser only.** Go side is fully in:
    `parseNaiveURI`, `tryParseNaiveClientConfigMap`,
    `naiveProxyEntryFromProxyURL`, the `case "naive"` arm in
    `parseJSONOutbound`, the `case "NAIVEPROXY"` outbound builder, and
    validation. `ManualPane` gains a Naive card (Name / Host / Port /
    Username / Password / SNI → `naive+https://user:pass@host:port`).
    The AAR is **not** rebuilt with `with_naive_outbound` yet —
    sagernet's precompiled `libcronet.a` doesn't link under NDK 27.x
    ld.lld (see Known issues). Naive URIs parse and persist, but
    sing-box will reject them at engine-start until the tag flips on.

~~B2. Security / TLS extras~~ — ✅ DONE. `resolvedFingerprint` lands
    with a mobile-friendly default — there's no Edge WebView2 on
    Android so the empty case falls through to `"chrome"` (safest
    mainstream fingerprint that still passes Reality's masquerade).
    Three call sites in `applyTLSAndTransport` (reality / tls /
    `extra.tls=true`) switched from `fingerprintFromExtra` to
    `resolvedFingerprint`. `applyTLSExtras`,
    `normalizeRealityShortID`, `resolvePacketEncoding`,
    `fingerprintFromExtra` were already in sync (file diff against
    ResultV-dev was clean).

~~B3. Old-protocol parser fixes~~ — ✅ DONE. File diff against
    ResultV-dev showed Hysteria2 password chain
    (`password → auth → auth_str → userpass`), ALPN helpers
    (`xhttpPreferH2ALPN`, `defaultALPNForNetwork`), AmneziaWG
    `getQueryParamCI`, and VLESS `mergeVLESSURLEmbeddedExtra` already
    present from prior ports. The only real delta in this slice was
    the `parseJSONOutbound` `case "naive"` (folded into B1 above).

B4. **Battery / data-usage stats** — extend `Mobile` binding to expose
    cumulative byte totals + interval breakdown for OS-level battery
    accounting (`NetworkStatsManager.queryDetailsForUid`). Surface on
    Settings → About. Most of the data is already in `TrafficStats`
    but needs persistence across service restarts.

### C. Phase 7 — Release engineering

~~C1. Multi-ABI release build~~ — ✅ DONE. `splits.abi` block in
    `app/build.gradle.kts` with arm64-v8a + armeabi-v7a + universal.
    Per-ABI versionCode offsets (armeabi-v7a +1, arm64-v8a +2).

~~C2. Release signing config~~ — ✅ DONE. `keystore.properties`
    (gitignored) + `keystore.properties.example` template.
    `signingConfigs.release` reads from the file; absent file =
    debug signing fallback.

~~C3. R8 minification + ProGuard rules audit~~ — ✅ DONE.
    `isMinifyEnabled=true`, `isShrinkResources=true` on release.
    Added `-keep class libbox.** { *; }`,
    `-keep class org.golang.app.** { *; }`, Compose @Composable
    keeper, ML Kit / Code Scanner keeps.

~~C4. AAR rebuild script~~ — ✅ DONE.
    `scripts/build-android-aar.sh` + `scripts/build-android-aar.ps1`.
    `--with-naive` / `-WithNaive` flag gates the NaiveProxy tag.

~~C5. CI — GitHub Actions~~ — ✅ DONE. `build-android` job in
    `release.yml`: Go + Java 17 + Android SDK setup, gomobile init,
    AAR build, base64-decoded keystore from secrets, assembleRelease,
    APK upload as release asset.

~~C6. Version bump~~ — ✅ DONE. `versionCode=10000`,
    `versionName="1.0.0"`.

C7. **Distribution** — Play Store listing OR direct-download APK on
    `result-proxy.ru` with an auto-update flow (poll the same updater
    endpoint the desktop uses; `internal/updater` is desktop-only today
    so we'd need a `mobile_updater.go`).

---

## Pull from the new PC version (`ResultV-dev`)

These are improvements landed on the new ПК desktop branch after the
mobile port forked. Items B1–B3 above already cover the engine-side
audit; this section is the user-facing UI parity that depends on them.

### Already landed in B1–B3 (engine-side)
- NaiveProxy parser + outbound (B1)
- TLS extras / REALITY normalisation / packet encoding (B2)
- Hysteria2 / ALPN / AmneziaWG / VLESS parser fixes (B3)

### Still to port on the Kotlin side

~~P1. Subscription header pretty-print~~ — ✅ DONE. Paired-unit format
    (`18.4 / 50 GB` collapses to a single suffix when both values share
    one) via `formatBytesPair`; days-left switched to Android plurals
    (`values{,-ru}/plurals.xml` — proper one/few/many/other RU forms);
    absolute expire date (`до DD.MM.YY HH:MM` / `until …`) added as a
    second line under days-left.

~~P2. Subscription editor~~ — ✅ DONE. Pencil-edit `CircleActionChip`
    next to refresh/delete in `SubscriptionHeader`. Tap opens
    `SubscriptionUrlEditDialog` (AlertDialog + OutlinedTextField,
    requires `http(s)://` and a changed value to enable Save).
    Save calls `SubscriptionRepository.update(id) { it.copy(url=…) }`
    then re-runs `refreshSubscription` so the panel swap takes effect
    immediately.

~~P3. Domain exclusions UX polish~~ — ✅ DONE. (a) Inline shadow
    warning under the input — when the typed pattern is already
    covered by an existing entry (`*.ru` covers `yandex.ru`) the
    redundancy is flagged in `Brand.Warning`; the symmetric "this
    pattern will shadow X, Y" warning fires when the user is about to
    add a broader pattern. (b) Persisted `RoutingRulesState.domainHistory`
    MRU list (24 entries) drives a "Recently used" chip row below the
    active exclusions, so re-adding a removed pattern is one tap.
    Shadow logic lives in `domainPatternShadows(pattern, candidate)` on
    `RoutingRules.kt` so the rule is testable and reusable.

~~P4. Selectable protocol filter on ProxiesScreen~~ — ✅ DONE. New
    `ProtocolFilterChips` composable (multi-select FilterChip row)
    placed under the ping/sort toolbar on Proxies. `profileProtocol(p)`
    on `HomeScreen.kt` canonicalises `entryJson.type` / URI scheme so
    `SS` ↔ `SHADOWSOCKS`, `WG` ↔ `WIREGUARD`, `AWG` ↔ `AMNEZIAWG`
    collapse to one bucket each; AUTO entries are excluded (they mix
    inner protocols). Empty selection = "no filter"; active filter
    drops SECTION labels and hides subscription cards with no
    matches. Chip row hides itself when fewer than two protocols are
    in the list.

~~P5. Multi-AUTO virtual entries~~ — ✅ DONE. New
    `proxy.SplitAutoEntriesMulti` in `internal/proxy/uriparser.go`
    partitions auto-named entries by their leading flag emoji,
    then runs `ExtractAutoGroupName` per partition; each partition
    of ≥2 entries with a recoverable shared name becomes one
    AutoGroup. `buildAutoAwareEntries` in `mobile/libbox.go` walks
    the returned groups and emits one virtual AUTO per cluster.
    The "no individuals + single shared name" fallback path stays
    intact for the simple single-group case. Tests in
    `internal/proxy/lcp_test.go` cover single-flag, two-flag, mixed,
    and degenerate (1-entry-per-flag) shapes.

---

## Known issues / tech debt

- `findConnectionOwner` throws on every connection — spammy but harmless.
  The wrapper's nil-deref guard is what's actually saving us.
- `usePlatformAutoDetectInterfaceControl=true` is set but **never fires** in
  observed logs. Bypass works only because `addDisallowedApplication(ownPkg)`
  keeps our UID off the tunnel. Revisit if always-on VPN needs platform protect.
- Gomobile uses sagernet's fork pinned at v0.1.12.
- **NaiveProxy outbound is parser-only on mobile right now.** Toggling
  `with_naive_outbound` on the gomobile bind cmd pulls in
  `github.com/sagernet/cronet-go`, whose precompiled `libcronet.a`
  ships R_AARCH64_AUTH_ABS64 relocations (315) that NDK 27.x's
  ld.lld rejects with `unknown relocation`. The parser, ManualPane
  card, validation, and outbound builder code are committed, but
  the AAR is currently built without the tag — sing-box will refuse
  to instantiate `"type":"naive"` outbounds at runtime until either
  (a) an older NDK (25 or 26) is wired into Android Studio's SDK
  Manager and gomobile is pointed at it, or (b) sagernet ships a
  cronet-go binary rebuilt with the new ABI. Flip the tag back on
  in the cheatsheet once that's resolved.
- `internal/godebug.defaultGODEBUG=multipathtcp=0` not yet wired into ldflags.
  Harmless on user devices but produces a Go runtime warning in logcat.
- TUN stack `gvisor` (sing-box default). `system` stack broke our env, kept gvisor.
- DoT (TCP/853) traffic from system DNS hits in-tunnel server (172.19.0.2)
  and gets forwarded to proxy uselessly — wastes a few seconds at start. Cosmetic.
- Connect-by-URI keeps the original URI; subscription-imported profiles
  use `entryJson`. Both paths converge through `buildSingBoxConfigFromEntry`.
- Smart-mode list fetch goes through citizenlab + itdoginfo over plain
  HTTPS, with country resolution leaking through geojs.io (the desktop
  ResultV-dev switched to a project-owned API — that hop hasn't been
  ported yet). On a captive-portal network the fetch can stall the
  first connect for up to 10s before timing out → engine starts with
  an empty domain list (effectively Global → direct). The 24h cache in
  `dataDir/smart-blocked.json` saves subsequent launches.
- Smart-mode country picker is missing. `SmartListRepository.country`
  is hardcoded to `"ru"`; users outside RU get a Russia-flavoured
  blocked list that won't match their needs. Trivial to expose in
  Settings once we know how to UX the country list.
- LSP errors visible on Windows host for `resultProxyDataDir` and
  `tunnelProbeDomains` — both are `//go:build mobile` symbols in
  `mobile_stubs.go`. gomobile compiles for `GOOS=android` where the
  stubs apply, so AAR builds succeed; only the IDE analyser is confused.

## Build cheatsheet

```bash
# from repo root
gomobile bind \
  -target=android -androidapi=26 \
  -tags="mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard" \
  -ldflags="-checklinkname=0" \
  -o android/libs/libbox.aar \
  ./mobile github.com/sagernet/sing-box/experimental/libbox
```

`with_naive_outbound` is **not** in the active tag list — NDK 27.x
ld.lld can't link sagernet's precompiled `libcronet.a` (see Known
issues). Add the tag back to the cmd once the toolchain is sorted.

Then in Android Studio: Sync → Run on Pixel 9 Pro emulator (API 36).
Pure-Kotlin changes don't need a rebuild — just Sync.
