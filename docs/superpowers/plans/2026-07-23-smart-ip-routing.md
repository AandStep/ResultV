# Smart-list IP/CIDR routing — Implementation Plan (Plan 2 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route destination IPs/CIDRs from the smart blocklist through the proxy in Smart mode, so apps that connect by IP (QUIC / no SNI) still reach blocked resources — closing the gap where the domain matcher, which needs a sniffed host, can't help.

**Architecture:** Carry blocked IPs in a **separate** end-to-end channel (never through `normalizeRule`, which strips CIDR masks at the first `/`). A dedicated provider fetch classifies IP/CIDR entries; they ride their own cache field, their own `FetchSmartList` JSON field, their own `BuildOptions` transport string, and their own `EngineConfig` slice; `buildRoute` emits one `ip_cidr → proxy` rule that fires **before** `sniff`.

**Tech Stack:** Go (`internal/proxy`, `mobile`), sing-box route rules, gomobile, Kotlin.

**Companion plan:** Plan 1 (`2026-07-23-smart-per-app-allowlist.md`). Independent — either can land first. Both touch `SmartListRepository.kt` and `BuildOptions.kt`; the edits are in different regions and do not conflict.

## Global Constraints

- **IPs must never pass through `normalizeRule`/`normalizeDomains`** (`internal/proxy/router.go:251-253` truncates at `/`, destroying CIDR masks). IPs get their own `normalizeIPs`.
- **Do not change the `BlockedListProvider` interface** (a `fakeBlockedProvider` mock in `blocked_updater_test.go` implements it). IP fetching is a method on the concrete `*HTTPBlockedListProvider`, called directly by `FetchSmartList`.
- **Backward compatible caches:** a `smart-blocked.json` without an `ips` field must load with empty IPs, no error. Desktop's `ResolveBlockedDomains` path leaves IPs empty — it is not in scope here.
- **The IP rule routes to `proxy` and must fire before `sniff`** but **after** the proxy-server-IP and bypass-LAN `direct` rules (first match wins in sing-box).
- **Smart `final=direct` must trigger on a non-empty IP list too**, not only domains — otherwise an IP-only smart list leaves `final=proxy` and tunnels everything.
- **gomobile transport is JSON strings**, `\n`-separated lists (reuse `splitSmartList`).

**Go test command:**
```bash
cd /c/ResultV && go test ./internal/proxy/... ./mobile/...
```

---

### Task 1: Provider — IP cache field, normaliser, and dedicated IP fetch

**Files:**
- Modify: `internal/proxy/blocked_provider.go` (`BlockedDomainsCache` ~138-143; `LoadBlockedDomainsCache` ~145-159; `SaveBlockedDomainsCache` ~161-177; add `normalizeIPs`, `ipSourceTemplates`, `FetchBlockedIPs`, `parseIPPayload`)
- Test: `internal/proxy/blocked_ips_test.go` (new)

**Interfaces:**
- Produces:
  - `BlockedDomainsCache.IPs []string` (json `ips`)
  - `normalizeIPs(in []string) []string`
  - `(*HTTPBlockedListProvider).FetchBlockedIPs(ctx context.Context, country string) ([]string, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/proxy/blocked_ips_test.go`:

```go
package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeIPs(t *testing.T) {
	got := normalizeIPs([]string{
		"1.2.3.4", "1.2.3.0/24", "2001:db8::1", "  5.6.7.8  ",
		"1.2.3.4", // dup of the /32 form
		"not-an-ip", "example.com", "",
	})
	want := map[string]bool{
		"1.2.3.4/32": true, "1.2.3.0/24": true,
		"2001:db8::1/128": true, "5.6.7.8/32": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d entries", got, len(want))
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected entry %q in %v", g, got)
		}
	}
}

func TestCacheRoundTrip_IPs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smart-blocked.json")
	if err := SaveBlockedDomainsCache(path, BlockedDomainsCache{
		Country: "ru", Source: "remote",
		Domains: []string{"instagram.com"},
		IPs:     []string{"1.2.3.0/24", "5.6.7.8"},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	c, err := LoadBlockedDomainsCache(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(c.IPs) != 2 {
		t.Fatalf("want 2 ips, got %v", c.IPs)
	}
	seen := map[string]bool{}
	for _, ip := range c.IPs {
		seen[ip] = true
	}
	if !seen["1.2.3.0/24"] || !seen["5.6.7.8/32"] {
		t.Fatalf("ips not normalized/preserved: %v", c.IPs)
	}
}

func TestLoadCache_NoIPsField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smart-blocked.json")
	// Legacy cache: domains only, no "ips" key.
	if err := os.WriteFile(path, []byte(
		`{"country":"ru","source":"remote","domains":["x.com"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadBlockedDomainsCache(path)
	if err != nil {
		t.Fatalf("legacy cache must load, got err: %v", err)
	}
	if len(c.IPs) != 0 {
		t.Fatalf("legacy cache must have empty ips, got %v", c.IPs)
	}
}

func TestFetchBlockedIPs_ParsesAndClassifies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.2.3.0/24\n5.6.7.8\n# comment\nnot-an-ip\n"))
	}))
	defer srv.Close()

	t.Setenv("RESULTPROXY_BLOCKED_IP_SOURCES", srv.URL)
	p := NewHTTPBlockedListProvider()
	ips, err := p.FetchBlockedIPs(context.Background(), "ru")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	seen := map[string]bool{}
	for _, ip := range ips {
		seen[ip] = true
	}
	if !seen["1.2.3.0/24"] || !seen["5.6.7.8/32"] {
		t.Fatalf("expected classified CIDRs, got %v", ips)
	}
	if seen["not-an-ip"] {
		t.Fatalf("junk leaked into ips: %v", ips)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/ResultV && go test ./internal/proxy/ -run 'IPs|BlockedIPs|NoIPsField' -v`
Expected: FAIL — `normalizeIPs`, `FetchBlockedIPs`, and `BlockedDomainsCache.IPs` undefined.

- [ ] **Step 3: Add the `IPs` field to the cache struct**

In `internal/proxy/blocked_provider.go`, change `BlockedDomainsCache` (lines 138-143) to:

```go
type BlockedDomainsCache struct {
	Country   string   `json:"country"`
	UpdatedAt int64    `json:"updatedAt"`
	Source    string   `json:"source"`
	Domains   []string `json:"domains"`
	IPs       []string `json:"ips,omitempty"`
}
```

- [ ] **Step 4: Normalise IPs on load and save**

In `LoadBlockedDomainsCache`, after `out.Domains = normalizeDomains(out.Domains)` (line 154), add:

```go
	out.IPs = normalizeIPs(out.IPs)
```

In `SaveBlockedDomainsCache`, after `cache.Domains = normalizeDomains(cache.Domains)` (line 162), add:

```go
	cache.IPs = normalizeIPs(cache.IPs)
```

- [ ] **Step 5: Add `normalizeIPs` + IP fetch (append to blocked_provider.go)**

Ensure `net` is imported (it is used elsewhere in the package; `blocked_provider.go`'s import block does not yet list it — add `"net"`). Then append:

```go
// normalizeIPs canonicalises blocked-IP entries WITHOUT the domain pipeline,
// which would strip CIDR masks (normalizeRule cuts at the first '/'). A bare
// IP becomes a /32 (v4) or /128 (v6); a CIDR is kept verbatim; anything that
// is neither is dropped. Order-preserving, deduplicated.
func normalizeIPs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		var canon string
		if _, _, err := net.ParseCIDR(s); err == nil {
			canon = s
		} else if ip := net.ParseIP(s); ip != nil {
			if ip.To4() != nil {
				canon = s + "/32"
			} else {
				canon = s + "/128"
			}
		} else {
			continue
		}
		if _, dup := seen[canon]; dup {
			continue
		}
		seen[canon] = struct{}{}
		out = append(out, canon)
	}
	return out
}

// ipSourceTemplates resolves the blocked-IP list source(s). Mirrors
// sourceTemplates for domains but for IP/CIDR lists. Env override:
// RESULTPROXY_BLOCKED_IP_SOURCES (comma-separated).
func (p *HTTPBlockedListProvider) ipSourceTemplates(country string) []string {
	if override := strings.TrimSpace(os.Getenv("RESULTPROXY_BLOCKED_IP_SOURCES")); override != "" {
		var out []string
		for _, part := range strings.Split(override, ",") {
			if s := strings.TrimSpace(part); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if strings.ToLower(strings.TrimSpace(country)) == "ru" {
		// Re-filter IP list (CIDR ranges of RU-blocked resources). If the URL
		// 404s the fetch degrades to the cached ips, never breaking connect.
		return []string{"https://raw.githubusercontent.com/1andrevich/Re-filter-lists/main/ipsum.lst"}
	}
	return nil
}

// FetchBlockedIPs downloads and classifies the blocked-IP/CIDR list for a
// country. Concrete-only (not on BlockedListProvider) so the interface and its
// mocks stay unchanged. Empty result is not an error to the caller — it means
// "no IP routing", handled by degrading to cache upstream.
func (p *HTTPBlockedListProvider) FetchBlockedIPs(ctx context.Context, country string) ([]string, error) {
	templates := p.ipSourceTemplates(country)
	if len(templates) == 0 {
		return nil, fmt.Errorf("no ip sources for %q", country)
	}
	merged := make([]string, 0, 1024)
	var lastErr error
	for _, u := range templates {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := p.client().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("ip list http %d: %s", resp.StatusCode, u)
			resp.Body.Close()
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		merged = append(merged, parseIPPayload(body)...)
	}
	merged = normalizeIPs(merged)
	if len(merged) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("empty ip list")
	}
	return merged, nil
}

// parseIPPayload extracts IP/CIDR tokens from a newline list, one per line,
// tolerating '#' comments and inline trailing comments.
func parseIPPayload(raw []byte) []string {
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if i := strings.IndexByte(s, '#'); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
		if f := strings.Fields(s); len(f) > 0 {
			s = f[0]
		}
		out = append(out, s)
	}
	return out
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /c/ResultV && go test ./internal/proxy/ -run 'IPs|BlockedIPs|NoIPsField' -v`
Expected: PASS (4 tests).

- [ ] **Step 7: Commit**

```bash
git add internal/proxy/blocked_provider.go internal/proxy/blocked_ips_test.go
git commit -m "feat(proxy): blocked-IP cache field, normaliser, and dedicated fetch"
```

---

### Task 2: Engine — SmartBlockedIPs + ip_cidr proxy rule before sniff

**Files:**
- Modify: `internal/proxy/engine.go` (`EngineConfig` after line 97; `buildRoute` `final` at 781-783; insert rule after the bypass-LAN block ~830, before `sniff` at 832)
- Test: `internal/proxy/engine_smart_test.go` (append)

**Interfaces:**
- Consumes: `SBRouteRule{Action, IPCidr, Outbound}` (existing).
- Produces: `EngineConfig.SmartBlockedIPs []string`.

- [ ] **Step 1: Write the failing test**

Append to `internal/proxy/engine_smart_test.go`:

```go
// TestBuildRoute_SmartMode_IPsProxiedBeforeSniff pins that blocked IPs are
// routed to the proxy by a pure ip_cidr rule that fires BEFORE sniff (so
// no-SNI / QUIC connections match), and AFTER the proxy-server-IP and
// bypass-LAN direct rules (which must still win).
func TestBuildRoute_SmartMode_IPsProxiedBeforeSniff(t *testing.T) {
	route := buildRoute(EngineConfig{
		Mode:            ProxyModeTunnel,
		SmartMode:       true,
		SmartBlockedIPs: []string{"1.2.3.0/24"},
		BypassLAN:       true,
		Proxy:           ProxyConfig{IP: "9.9.9.9", Port: 443, Type: "vless"},
	})

	sniffIdx, ipProxyIdx, serverDirectIdx, lanDirectIdx := -1, -1, -1, -1
	for i, r := range route.Rules {
		switch {
		case r.Action == "sniff":
			sniffIdx = i
		case r.Outbound == "proxy" && len(r.IPCidr) == 1 && r.IPCidr[0] == "1.2.3.0/24":
			ipProxyIdx = i
		case r.Outbound == "direct" && contains(r.IPCidr, "9.9.9.9/32"):
			serverDirectIdx = i
		case r.Outbound == "direct" && len(r.IPCidr) > 1: // lanBypassCIDRs()
			lanDirectIdx = i
		}
	}
	if ipProxyIdx < 0 {
		t.Fatalf("no smart ip_cidr proxy rule, rules=%+v", route.Rules)
	}
	if sniffIdx < 0 || ipProxyIdx > sniffIdx {
		t.Fatalf("ip_cidr proxy rule (%d) must come before sniff (%d)", ipProxyIdx, sniffIdx)
	}
	if serverDirectIdx < 0 || serverDirectIdx > ipProxyIdx {
		t.Fatalf("server-IP direct rule (%d) must precede ip proxy rule (%d)", serverDirectIdx, ipProxyIdx)
	}
	if lanDirectIdx < 0 || lanDirectIdx > ipProxyIdx {
		t.Fatalf("bypass-LAN rule (%d) must precede ip proxy rule (%d)", lanDirectIdx, ipProxyIdx)
	}
	if route.Final != "direct" {
		t.Fatalf("smart final must be direct with an IP list, got %q", route.Final)
	}
}

// TestBuildRoute_SmartMode_NoIPs_NoIPRule: empty IP list emits no ip proxy rule.
func TestBuildRoute_SmartMode_NoIPs_NoIPRule(t *testing.T) {
	route := buildRoute(EngineConfig{
		Mode: ProxyModeTunnel, SmartMode: true,
		SmartBlockedDomains: []string{"x.com"},
	})
	for _, r := range route.Rules {
		if r.Outbound == "proxy" && len(r.IPCidr) > 0 {
			t.Fatalf("unexpected ip_cidr proxy rule: %+v", r)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/ResultV && go test ./internal/proxy/ -run 'SmartMode_IPs|SmartMode_NoIPs' -v`
Expected: FAIL — `SmartBlockedIPs` undefined (compile error).

- [ ] **Step 3: Add the config field**

In `internal/proxy/engine.go`, after the `SmartBlockedDomains []string` field (line 97), add:

```go
	// SmartBlockedIPs is the resolved list of destination IP/CIDRs that
	// should route through the proxy when [SmartMode] is on. Carried
	// separately from domains because CIDR masks don't survive the domain
	// normaliser. Matched pre-sniff, so it catches no-SNI / QUIC flows.
	SmartBlockedIPs []string
```

- [ ] **Step 4: Include IPs in the Smart `final=direct` trigger**

In `buildRoute`, change the condition at lines 781-783 from:

```go
	if cfg.SmartMode && len(cfg.SmartBlockedDomains) > 0 {
		final = "direct"
	}
```

to:

```go
	if cfg.SmartMode && (len(cfg.SmartBlockedDomains) > 0 || len(cfg.SmartBlockedIPs) > 0) {
		final = "direct"
	}
```

- [ ] **Step 5: Emit the ip_cidr proxy rule before sniff**

In `buildRoute`, immediately BEFORE the `sniff` rule append (currently line 832, `rules = append(rules, SBRouteRule{ Action: "sniff" })`), insert:

```go
	// Smart mode: destination IPs/CIDRs in SmartBlockedIPs go through the
	// proxy. Pure IP match — fires BEFORE sniff, catching apps that connect
	// by IP (QUIC / no SNI) whose blocked host never surfaces as a domain.
	// Placed after the proxy-server-IP and bypass-LAN direct rules so those
	// still win (sing-box takes the first match).
	if cfg.SmartMode && len(cfg.SmartBlockedIPs) > 0 {
		rules = append(rules, SBRouteRule{
			Action:   "route",
			IPCidr:   cfg.SmartBlockedIPs,
			Outbound: "proxy",
		})
	}

```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /c/ResultV && go test ./internal/proxy/ -run 'SmartMode' -v`
Expected: PASS (including the two new tests and the existing Smart tests).

- [ ] **Step 7: Commit**

```bash
git add internal/proxy/engine.go internal/proxy/engine_smart_test.go
git commit -m "feat(proxy): route smart-list IPs to proxy via pre-sniff ip_cidr rule"
```

---

### Task 3: libbox — transport field, config wiring, FetchSmartList IPs

**Files:**
- Modify: `mobile/libbox.go` (`BuildOptions` after `SmartBlockedDomainsList` ~656; `buildSingBoxConfigFromEntry` cfg ~1113; `FetchSmartList` ~940-953; add `resolveSmartIPs` helper)
- Test: `mobile/libbox_smart_ips_test.go` (new)

**Interfaces:**
- Consumes: `splitSmartList` (existing, `mobile/libbox.go:872`), `proxy.HTTPBlockedListProvider.FetchBlockedIPs`, `proxy.LoadBlockedDomainsCache`, `proxy.SaveBlockedDomainsCache` (Task 1), `EngineConfig.SmartBlockedIPs` (Task 2).
- Produces: `BuildOptions.SmartBlockedIPsList string` (json `smartBlockedIPsList`).

- [ ] **Step 1: Write the failing test**

Create `mobile/libbox_smart_ips_test.go`:

```go
package mobile

import (
	"encoding/json"
	"testing"
)

const smartURI = "vless://11111111-1111-1111-1111-111111111111@9.9.9.9:443?security=tls&type=tcp#s"

func TestBuild_SmartIPs_EmitsIPCidrProxyRule(t *testing.T) {
	out, err := BuildSingBoxConfigV2(smartURI, t.TempDir(), encodeOptions(BuildOptions{
		SmartMode:               true,
		SmartBlockedIPsList:     "1.2.3.0/24\n5.6.7.8/32",
		SmartBlockedDomainsList: "x.com",
	}))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	route, _ := m["route"].(map[string]any)
	rules, _ := route["rules"].([]any)
	found := false
	for _, r := range rules {
		rr, _ := r.(map[string]any)
		if rr["action"] == "route" && rr["outbound"] == "proxy" {
			if ips, ok := rr["ip_cidr"].([]any); ok && len(ips) == 2 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected an ip_cidr→proxy rule with 2 entries, rules=%+v", rules)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/ResultV && go test ./mobile/ -run 'SmartIPs' -v`
Expected: FAIL — `BuildOptions.SmartBlockedIPsList` undefined.

- [ ] **Step 3: Add the transport field**

In `mobile/libbox.go`, in `BuildOptions`, directly after the `SmartBlockedDomainsList string` field (line 656), add:

```go
	// SmartBlockedIPsList is the `\n`-separated list of blocked IP/CIDRs
	// routed through the proxy in Smart mode. Separate from the domain list
	// because CIDR masks don't survive the domain normaliser. Fetched by
	// FetchSmartList (the `ips` field) alongside the domains.
	SmartBlockedIPsList string `json:"smartBlockedIPsList,omitempty"`
```

- [ ] **Step 4: Wire IPs into the engine config**

In `buildSingBoxConfigFromEntry`, next to the `SmartBlockedDomains:` assignment (line 1113), add:

```go
		SmartBlockedIPs:     splitSmartList(opts.SmartBlockedIPsList),
```

- [ ] **Step 5: Fetch + cache IPs in FetchSmartList**

In `mobile/libbox.go`, add this helper (near `FetchSmartList`):

```go
// resolveSmartIPs fetches the blocked-IP list for a country best-effort and
// merges it into the existing cache (read-modify-write, so the domain payload
// already written is preserved). On fetch failure it degrades to the cached
// IPs. Returns nil when there is nothing to route.
func resolveSmartIPs(provider *proxy.HTTPBlockedListProvider, cachePath, country string) []string {
	cc := strings.ToLower(strings.TrimSpace(country))
	if cc != "" {
		if ips, err := provider.FetchBlockedIPs(context.Background(), cc); err == nil && len(ips) > 0 {
			if cache, lerr := proxy.LoadBlockedDomainsCache(cachePath); lerr == nil {
				cache.IPs = ips
				_ = proxy.SaveBlockedDomainsCache(cachePath, cache)
			}
			return ips
		}
	}
	if cache, err := proxy.LoadBlockedDomainsCache(cachePath); err == nil {
		return cache.IPs
	}
	return nil
}
```

Then in `FetchSmartList`, where the `out` map is built (lines 940-948), add the `ips` field. Change:

```go
	out := map[string]interface{}{
		"domains": result.Domains,
		"country": result.Country,
		"source":  result.Source,
		"error":   "",
	}
```

to:

```go
	out := map[string]interface{}{
		"domains": result.Domains,
		"ips":     resolveSmartIPs(provider, cachePath, result.Country),
		"country": result.Country,
		"source":  result.Source,
		"error":   "",
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /c/ResultV && go test ./mobile/ -run 'SmartIPs' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add mobile/libbox.go mobile/libbox_smart_ips_test.go
git commit -m "feat(mobile): carry smart-list IPs through libbox config + FetchSmartList"
```

---

### Task 4: Kotlin — SmartListRepository IPs + BuildOptions wiring

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/SmartListRepository.kt` (`Snapshot` ~40-50; `parseSnapshot` ~161-173; `loadMeta` ~175-196; `saveMeta` ~198-216; add `toEngineIPList`)
- Modify: `android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt:41` (add `smartBlockedIPsList`)

**Interfaces:**
- Consumes: `Mobile.fetchSmartList` JSON now carries `ips`; `BuildOptions.smartBlockedIPsList` consumed by libbox (Task 3).
- Produces: `SmartListRepository.toEngineIPList(): String`.

**Context:** No unit test — this is JSON plumbing verified by compile + the Go tests above and on-device (Task 5). Follow the existing `domains` handling exactly, in parallel.

- [ ] **Step 1: Add `ips` to the Snapshot**

In `SmartListRepository.kt`, change the `Snapshot` data class (lines 40-46) to add an `ips` field:

```kotlin
    data class Snapshot(
        val domains: List<String> = emptyList(),
        val ips: List<String> = emptyList(),
        val country: String = "",
        val source: String = "",
        val fetchedAt: Long = 0L,
        val lastError: String = "",
    ) {
```

- [ ] **Step 2: Parse `ips` from the engine response**

In `parseSnapshot` (lines 161-173), after the `domains` list is built, add an `ips` parse and include it in the returned `Snapshot`. Replace the function body with:

```kotlin
    private fun parseSnapshot(rawJson: String): Snapshot {
        val o = JSONObject(rawJson)
        val arr = o.optJSONArray("domains")
        val list = if (arr == null) emptyList()
        else (0 until arr.length()).mapNotNull { arr.optString(it).ifBlank { null } }
        val ipArr = o.optJSONArray("ips")
        val ips = if (ipArr == null) emptyList()
        else (0 until ipArr.length()).mapNotNull { ipArr.optString(it).ifBlank { null } }
        return Snapshot(
            domains = list,
            ips = ips,
            country = o.optString("country").ifBlank { country },
            source = o.optString("source"),
            fetchedAt = System.currentTimeMillis(),
            lastError = o.optString("error"),
        )
    }
```

- [ ] **Step 3: Persist + restore `ips` in the meta file**

In `loadMeta` (lines 175-196), after building the domains `list`, add ips parsing and include in the `Snapshot`. Replace the `try` block body's `Snapshot(...)` construction and preceding parse with:

```kotlin
            val arr = o.optJSONArray("domains")
            val list = if (arr == null) emptyList()
            else (0 until arr.length()).mapNotNull { arr.optString(it).ifBlank { null } }
            val ipArr = o.optJSONArray("ips")
            val ips = if (ipArr == null) emptyList()
            else (0 until ipArr.length()).mapNotNull { ipArr.optString(it).ifBlank { null } }
            Snapshot(
                domains = list,
                ips = ips,
                country = country,
                source = o.optString("source"),
                fetchedAt = o.optLong("fetchedAt", 0L),
                lastError = o.optString("lastError"),
            )
```

In `saveMeta` (lines 198-216), add an `ips` array to the persisted JSON. After the `domains` array is added to `o` (`.put("domains", arr)`), add:

```kotlin
            val ipArr = org.json.JSONArray()
            s.ips.forEach { ipArr.put(it) }
            o.put("ips", ipArr)
```

(Place the `ipArr` build+put right after the existing `.put("domains", arr)` in the `JSONObject` construction — restructure to statement form if needed so both arrays are added.)

- [ ] **Step 4: Add `toEngineIPList()`**

In `SmartListRepository.kt`, right after `toEngineList()` (ends ~line 157) and the `currentDomains()` added in Plan 1 (if present), add:

```kotlin
    /** Engine wire format for blocked IPs: newline-separated, one CIDR per line. */
    fun toEngineIPList(): String {
        val s = _state.value
        if (s.ips.isEmpty()) return ""
        return s.ips.joinToString("\n")
    }
```

- [ ] **Step 5: Pass the IP list in BuildOptions**

In `BuildOptions.kt`, after the `.put("smartBlockedDomainsList", ...)` line (line 41), add:

```kotlin
            .put("smartBlockedIPsList", if (smartMode) SmartListRepository.toEngineIPList() else "")
```

- [ ] **Step 6: Verify it compiles**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: BUILD SUCCESSFUL.

- [ ] **Step 7: Commit**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/SmartListRepository.kt \
        android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt
git commit -m "feat(android): carry smart-list IPs from FetchSmartList into engine options"
```

---

### Task 5: Full sweep + on-device verification

**Files:** none (verification only).

- [ ] **Step 1: Go test + build sweep**

Run: `cd /c/ResultV && go test ./internal/proxy/... ./mobile/... && go build ./...`
Expected: all tests PASS, build clean.

- [ ] **Step 2: Android build**

Run: `cd android && ./gradlew :app:assembleDebug`
Expected: BUILD SUCCESSFUL.

- [ ] **Step 3: On-device — IP routing**

Install, switch to Smart, connect. Open an app/service known to be reachable only by a blocked IP range (no SNI). Expected: it connects through the proxy. Confirm in logcat the config dump contains an `ip_cidr` rule with `outbound: proxy`, and that `FetchSmartList` returned a non-empty `ips` count.

- [ ] **Step 4: Regression — domains still route**

Confirm a normal blocked site (via SNI) still loads in Smart (domain rule unaffected), and Global mode is unchanged.

---

## Self-Review

**Spec coverage** (against `2026-07-23-smart-per-app-vpn-routing-design.md` §7 + §11 IP items):
- §7 source → engine pipeline: Task 1 (source + classify + cache) → Task 3 (FetchSmartList `ips`, transport) → Task 2 (`ip_cidr → proxy` before sniff) → Task 4 (Kotlin repo + BuildOptions). ✓
- §7 "IPs carried separately (CIDR survives)" → Task 1 `normalizeIPs` bypasses `normalizeRule`; separate cache/transport/config fields throughout. ✓
- §7 rule placement (after server-IP/bypass-LAN, before sniff) → Task 2 Step 5 + test ordering assertions. ✓
- §9/§10 legacy cache without `ips` loads empty → Task 1 `TestLoadCache_NoIPsField`. ✓
- Smart `final=direct` on IP-only list → Task 2 Step 4 + test. ✓
- §11 Go tests (engine ip rule, provider classify, cache round-trip, libbox split) → Tasks 1–3; on-device → Task 5. ✓
- **Out of scope (Plan 1):** allowlist membership, matcher, UI.

**Placeholder scan:** No TBD/TODO; the Re-filter IP URL is a concrete default with graceful degradation, flagged in §12 of the spec for verification — not a code placeholder. Every code step is complete; every command has expected output. ✓

**Type consistency:** `BlockedDomainsCache.IPs` (Task 1) read by `resolveSmartIPs` (Task 3). `FetchBlockedIPs(ctx, country) ([]string, error)` (Task 1) called in `resolveSmartIPs` (Task 3). `EngineConfig.SmartBlockedIPs` (Task 2) set from `splitSmartList(opts.SmartBlockedIPsList)` (Task 3). `BuildOptions.SmartBlockedIPsList` json tag `smartBlockedIPsList` (Task 3) matches the Kotlin `.put("smartBlockedIPsList", …)` (Task 4). `FetchSmartList` JSON `ips` (Task 3) parsed by `SmartListRepository.parseSnapshot` `optJSONArray("ips")` (Task 4). ✓
