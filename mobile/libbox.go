// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package mobile is the gomobile-bind entrypoint for the Android client.
// All exported symbols become Java/Kotlin bindings via gomobile, so they
// must use only basic types (string, int64, bool, []byte) or
// JSON-serialised payloads. Maps, interface{}, and func parameters are
// not allowed in the public API.
package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/sagernet/gomobile" // ensure sagernet's gomobile/bind stays in go.mod for AAR builds
	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// Version returns the wrapper version. Bump on every breaking change to
// the Kotlin-facing API.
func Version() string {
	return "0.3.0-engine-batch"
}

// PingResult is the JSON-encoded reply from Ping. Mirrors the desktop's
// PingResultDTO so callers can build a uniform display.
//
//	Reachable=true  + LatencyMs=N  → "N ms"
//	Reachable=true  + LatencyMs=-1 → "Online" (UDP, no echo)
//	Reachable=false                → reason carries the failure kind
//
// Reason values: "timeout", "connection_refused", "network_unreachable",
// "no_route_to_host", "connection_closed", "error".
//
// CheckType is "tcp", "udp", "tcp_fallback" (Hysteria2 fallback path).
type PingResult struct {
	Reachable bool   `json:"reachable"`
	LatencyMs int64  `json:"latencyMs"`
	Reason    string `json:"reason,omitempty"`
	CheckType string `json:"checkType,omitempty"`
}

// DeepLinkResult is the JSON-encoded reply from DecodeDeepLink.
//
//	Plain    — the decrypted payload. Either a single line (http(s) URL
//	           or a single proxy URI) that the UI hands off to the
//	           normal subscription/profile import pipeline, or a
//	           multi-line text body to be parsed line-by-line.
//	Source   — provenance tag for UI affordances. "rvsub" when the URL
//	           used the resultv://rvsub/<...> path (forces impVPN logo);
//	           "" for everything else.
type DeepLinkResult struct {
	Plain  string `json:"plain"`
	Source string `json:"source,omitempty"`
}

// DecodeDeepLink unwraps a resultv:// (or resultv:) URL into a
// subscription payload that Kotlin can hand off to the standard import
// path. Returns JSON-encoded DeepLinkResult; on error the Go-side error
// is propagated as-is.
//
// See internal/proxy/deeplink.go for the supported URL shapes.
func DecodeDeepLink(rawURL string) (string, error) {
	plain, err := proxy.DecodeDeepLink(rawURL)
	if err != nil {
		return "", err
	}
	source := ""
	if proxy.DeepLinkUsesRvsubPath(rawURL) {
		source = "rvsub"
	}
	res := DeepLinkResult{Plain: plain, Source: source}
	b, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("marshaling deep link result: %w", err)
	}
	return string(b), nil
}

// IsDeepLink reports whether the URL uses the resultv:// scheme. Cheap
// pre-check before calling DecodeDeepLink.
func IsDeepLink(rawURL string) bool {
	return proxy.IsDeepLink(rawURL)
}

// Ping probes a proxy server with a protocol-appropriate check and
// returns the result as a JSON-encoded PingResult. proxyType is the
// uppercase tag from ProxyEntry.Type ("VLESS", "HYSTERIA2", "WIREGUARD",
// "AMNEZIAWG", …); empty string falls back to plain TCP connect.
//
// This is the binding Kotlin uses to drive ServerRow's latency reading
// and "sort by ping" without needing a long-lived libbox connection.
func Ping(ip string, port int, proxyType string) (string, error) {
	if strings.TrimSpace(ip) == "" {
		return "", fmt.Errorf("ping: ip is empty")
	}
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("ping: port %d out of range", port)
	}

	ptUpper := strings.ToUpper(strings.TrimSpace(proxyType))
	var (
		latency   int64
		reachable bool
		reason    string
		checkType string
	)
	switch ptUpper {
	case "HYSTERIA2":
		latency, reachable, reason, checkType = proxy.PingHysteria2QUIC(ip, port)
	case "WIREGUARD", "AMNEZIAWG":
		latency, reachable, reason = proxy.PingWireGuard(ip, port)
		checkType = "udp"
	default:
		latency, reachable, reason = proxy.PingProxy(ip, port)
		checkType = "tcp"
	}

	res := PingResult{
		Reachable: reachable,
		LatencyMs: latency,
		Reason:    reason,
		CheckType: checkType,
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("marshaling ping result: %w", err)
	}
	return string(b), nil
}

// DetectCountry resolves a proxy server host (IP literal or hostname) to its
// lowercase ISO-3166 alpha-2 country code via the project-controlled GeoIP
// API, caching results on disk under dataDir (country.cache.json). Mirrors
// the desktop App.DetectCountry flow so the Android UI can show server flags
// for entries whose name/host carries no country hint.
//
// Returns an error for private/loopback hosts (caller should skip those) and
// when the API is unreachable; callers treat an error as "no flag".
func DetectCountry(host, dataDir string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("detect country: host is empty")
	}
	if isPrivateOrLoopbackHost(host) {
		return "", fmt.Errorf("detect country: %q is private/loopback", host)
	}
	cc := proxy.NewCountryClient(dataDir)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return cc.LookupCountryByIP(ctx, host)
}

// isPrivateOrLoopbackHost reports whether a host is a non-routable address
// the country API can't resolve (RFC1918, loopback, link-local, localhost).
// Hostnames that aren't IP literals are treated as routable.
func isPrivateOrLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// ParseProxyURI parses a single proxy URI (vless://, vmess://, ss://,
// trojan://, hy2://, wg://, awg://) and returns a JSON-serialised
// ProxyEntry. Useful for displaying parsed fields; the connection path
// uses BuildSingBoxConfig instead.
func ParseProxyURI(uri string) (string, error) {
	entry, err := proxy.ParseProxyURI(uri)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FetchSubscription downloads a subscription URL and parses its body
// into a JSON array of ProxyEntry objects. Handles base64-encoded,
// line-separated, and JSON subscription formats (see
// proxy.ParseSubscriptionBody).
//
// Uses the default ResultV/<ver> User-Agent. Users who need to
// impersonate another client (e.g. a panel that only returns the rich
// config to a specific UA) can override it via Settings → Subscriptions
// → User Agent — see FetchSubscriptionV3.
//
// Returns the JSON array as a string so gomobile can shuttle it across
// the JNI boundary; Kotlin re-parses with org.json.
//
// Legacy entrypoint: returns only the entry list. Use FetchSubscriptionV2
// to also receive Subscription-Userinfo / Profile-Title metadata.
func FetchSubscription(subURL, dataDir string) (string, error) {
	out, err := fetchSubscription(subURL, dataDir, SubscriptionFetchOptions{})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(out.Entries)
	if err != nil {
		return "", fmt.Errorf("marshaling: %w", err)
	}
	return string(data), nil
}

// SubscriptionResponse is the JSON shape returned by FetchSubscriptionV2.
// Mirrors the desktop's Subscription model: parsed entries + the
// SIP008-style metadata headers the panel exposed.
//
// SupportURL is the panel-supplied `Support-Url` header, surfaced by the
// mobile edit sheet's Links section. Empty when the panel doesn't ship one.
type SubscriptionResponse struct {
	Entries    json.RawMessage `json:"entries"`
	UserInfo   string          `json:"userInfo,omitempty"` // "upload=…; download=…; total=…; expire=…"
	Title      string          `json:"title,omitempty"`
	SupportURL string          `json:"supportUrl,omitempty"`
}

// FetchSubscriptionV2 returns the entries together with subscription
// metadata. Kotlin parses the outer JSON to extract `userInfo` (used by
// the new traffic/expiry surface) and `title` (default subscription name
// when the user hasn't typed one).
func FetchSubscriptionV2(subURL, dataDir string) (string, error) {
	return FetchSubscriptionV3(subURL, dataDir, "")
}

// SubscriptionFetchOptions carries Settings-driven knobs for the
// subscription fetch path. Encoded as JSON so gomobile can shuttle the
// payload across the JNI boundary (struct binding isn't supported).
//
// All fields are optional — empty / zero values fall back to the same
// defaults FetchSubscriptionV2 uses.
type SubscriptionFetchOptions struct {
	// UserAgent overrides the default ResultV/<ver> UA.
	UserAgent string `json:"userAgent,omitempty"`
	// SendHWID controls the x-hwid header. Pointer so the absence of the
	// field defaults to true (matches desktop EffectiveSubscriptionSendHWID).
	SendHWID *bool `json:"sendHwid,omitempty"`
	// DeviceOS / OSVersion / Model fill the x-device-os / x-ver-os /
	// x-device-model headers that Happ / Remnawave panels read for the
	// device-list / HWID-limit UI. Empty means "don't send".
	DeviceOS  string `json:"deviceOs,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
	Model     string `json:"model,omitempty"`
	// HWID is a stable OS-level device identifier (Android:
	// Settings.Secure.ANDROID_ID) the caller supplies as the x-hwid source.
	// Unlike the dataDir fallback file, it survives app reinstalls — without
	// it every reinstall mints a fresh random fingerprint and the panel
	// registers a duplicate device, burning the provider's HWID-limit slots.
	// Empty falls back to config.StableHardwareID(dataDir).
	HWID string `json:"hwid,omitempty"`
}

// FetchSubscriptionV3 is the options-aware variant of V2. Kotlin should
// build the options JSON from SettingsRepository state so user-configured
// UA / HWID / device-tag preferences flow through to the fetcher.
//
// Empty / malformed [optionsJson] falls back to the V2 defaults.
func FetchSubscriptionV3(subURL, dataDir, optionsJson string) (string, error) {
	opts := decodeSubscriptionFetchOptions(optionsJson)
	out, err := fetchSubscription(subURL, dataDir, opts)
	if err != nil {
		return "", err
	}
	entries, err := json.Marshal(out.Entries)
	if err != nil {
		return "", fmt.Errorf("marshaling entries: %w", err)
	}
	resp := SubscriptionResponse{
		Entries:    entries,
		UserInfo:   out.UserInfo,
		Title:      out.Title,
		SupportURL: out.SupportURL,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("marshaling response: %w", err)
	}
	return string(data), nil
}

func decodeSubscriptionFetchOptions(optionsJson string) SubscriptionFetchOptions {
	var o SubscriptionFetchOptions
	if strings.TrimSpace(optionsJson) == "" {
		return o
	}
	if err := json.Unmarshal([]byte(optionsJson), &o); err != nil {
		return SubscriptionFetchOptions{}
	}
	return o
}

// fetchSubscription is the shared fetch path used by V1/V2/V3. Returns
// parsed entries + the metadata captured from the HTTP response (userinfo,
// title, support URL).
func fetchSubscription(subURL, dataDir string, opts SubscriptionFetchOptions) (subscriptionResult, error) {
	subURL = strings.TrimSpace(subURL)
	if subURL == "" {
		return subscriptionResult{}, fmt.Errorf("subscription URL is empty")
	}
	subURL = normalizeSubscriptionURL(subURL)

	sendHWID := true
	if opts.SendHWID != nil {
		sendHWID = *opts.SendHWID
	}
	hwid := ""
	if sendHWID {
		// Prefer the caller-supplied stable OS identifier (mobile passes
		// Android's ANDROID_ID) so the fingerprint survives reinstalls. Only
		// fall back to the dataDir-derived id when none was provided — that
		// file is wiped on reinstall and would mint a duplicate device.
		if src := strings.TrimSpace(opts.HWID); src != "" {
			hwid = config.HashHWIDSource(src)
		} else if h, err := config.StableHardwareID(dataDir); err == nil {
			hwid = h
		}
		// HWID isn't strictly required — some panels just won't gate the
		// real config without it. Continue and let the fetch decide.
	}

	// User-Agent: Settings override (Subscriptions → User Agent) wins,
	// otherwise match desktop's `ResultV/<ver>` so panels that gate on
	// the string see the same identity across platforms. No more Happ/1.0
	// fallback — users who need to impersonate Happ can do so explicitly
	// via the UA override.
	userAgent := strings.TrimSpace(opts.UserAgent)
	if userAgent == "" {
		userAgent = "ResultV/3.1.1"
	}
	headers := subscriptionDeviceHeaders(opts)
	primaryRes, primaryErr := fetchSubscriptionWithUA(subURL, userAgent, hwid, headers)

	if primaryErr != nil {
		return subscriptionResult{}, primaryErr
	}
	entries := primaryRes.Entries
	rawCount := len(entries)
	// Convert 0.0.0.0 placeholder rows into SECTION labels (matches the
	// desktop pipeline). impVPN-style helper rows like "👇 выберите конфиг
	// ниже" stay in list order but are non-clickable.
	entries = proxy.FinalizeSubscriptionEntries(entries)
	// A subscription needs at least one real proxy to be useful. SECTION
	// rows alone don't count.
	hasReal := false
	for _, e := range entries {
		if !strings.EqualFold(strings.TrimSpace(e.Type), "SECTION") {
			hasReal = true
			break
		}
	}
	if !hasReal {
		return subscriptionResult{}, fmt.Errorf(
			"no valid proxies in subscription (parsed=%d, all sections, url=%s, diag=%s)",
			rawCount, subURL, primaryRes.Diag,
		)
	}

	// Mirror desktop: collapse N "<name> Auto" copies into one virtual AUTO
	// entry whose Extra carries the member entries inline. mobile picks
	// members[0] at connect time (latency-based selection comes later).
	visible := buildAutoAwareEntries(entries)

	return subscriptionResult{
		Entries:    visible,
		UserInfo:   primaryRes.UserInfo,
		Title:      primaryRes.Title,
		SupportURL: primaryRes.SupportURL,
	}, nil
}

type subscriptionResult struct {
	Entries    []config.ProxyEntry
	UserInfo   string
	Title      string
	SupportURL string
}

// buildAutoAwareEntries reproduces app.go's auto-bundling and extends it
// with multi-group support: subscriptions that ship several "<region>
// Auto" clusters (one per country) emit one AUTO virtual entry per
// cluster instead of being silently flattened.
//
// Resolution order:
//  1. SplitAutoEntriesMulti — partitions auto entries by flag emoji and
//     creates one AutoGroup per partition with ≥2 members that share a
//     recoverable group name.
//  2. ExtractAutoGroupName on the whole list — fallback for the
//     no-individual case where every entry shares a single name (e.g.
//     "🇨🇦 impVPN Auto" repeated N times with no other entries).
//
// AUTO entries land first in the output (preserving the order in which
// each group first appeared), individuals afterwards.
func buildAutoAwareEntries(entries []config.ProxyEntry) []config.ProxyEntry {
	if len(entries) <= 1 {
		return entries
	}

	groups, individuals := proxy.SplitAutoEntriesMulti(entries)
	if len(groups) > 0 {
		out := make([]config.ProxyEntry, 0, len(groups)+len(individuals))
		for _, g := range groups {
			out = append(out, config.ProxyEntry{
				Name:  g.Name,
				Type:  "AUTO",
				Extra: marshalAutoMembers(g.Members),
			})
		}
		out = append(out, individuals...)
		return out
	}

	if sharedName, ok := proxy.ExtractAutoGroupName(entries); ok && len(entries) > 1 {
		auto := config.ProxyEntry{
			Name:  sharedName,
			Type:  "AUTO",
			Extra: marshalAutoMembers(entries),
		}
		return []config.ProxyEntry{auto}
	}

	return entries
}

func marshalAutoMembers(members []config.ProxyEntry) json.RawMessage {
	data, err := json.Marshal(map[string]interface{}{"members": members})
	if err != nil {
		return json.RawMessage(`{"members":[]}`)
	}
	return data
}

// subscriptionFetchResult bundles the parsed entries with the
// metadata headers we surface to the UI (subscription-userinfo for
// traffic quota, profile-title for default subscription name, support-url
// for the per-sub help link in the edit sheet).
type subscriptionFetchResult struct {
	Entries    []config.ProxyEntry
	UserInfo   string
	Title      string
	SupportURL string
	Diag       string
}

// subscriptionDeviceHeaders pulls the user-tag headers out of the fetch
// options. Mirrors the desktop trio (x-device-os / x-ver-os /
// x-device-model) — panels read these for the device-list UI and (in
// some Remnawave deployments) the HWID-limit decision.
func subscriptionDeviceHeaders(opts SubscriptionFetchOptions) map[string]string {
	out := map[string]string{}
	if s := strings.TrimSpace(opts.DeviceOS); s != "" {
		out["x-device-os"] = s
	}
	if s := strings.TrimSpace(opts.OSVersion); s != "" {
		out["x-ver-os"] = s
	}
	if s := strings.TrimSpace(opts.Model); s != "" {
		out["x-device-model"] = s
	}
	return out
}

// fetchSubscriptionWithUA returns the parsed result + diag string for
// telemetry. diag is a short human-readable summary used when the
// caller wants to surface why a fetch produced zero usable entries.
func fetchSubscriptionWithUA(subURL, userAgent, hwid string, extraHeaders map[string]string) (subscriptionFetchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return subscriptionFetchResult{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if hwid != "" {
		req.Header.Set("x-hwid", hwid)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return subscriptionFetchResult{}, fmt.Errorf("fetching subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return subscriptionFetchResult{}, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return subscriptionFetchResult{}, fmt.Errorf("reading body: %w", err)
	}

	preview := string(body)
	if len(preview) > 80 {
		preview = preview[:80]
	}
	preview = strings.ReplaceAll(strings.ReplaceAll(preview, "\n", " "), "\r", "")

	entries, err := proxy.ParseSubscriptionBody(string(body))
	diag := fmt.Sprintf("ua=%q bytes=%d preview=%q parsed=%d", userAgent, len(body), preview, len(entries))
	if err != nil {
		return subscriptionFetchResult{Diag: diag}, fmt.Errorf("parsing subscription (%s): %w", diag, err)
	}
	// SIP008 / clash convention: traffic quota and expiry come in via
	// `Subscription-Userinfo`, friendly name in `Profile-Title`,
	// per-sub help link in `Support-Url`. All optional — providers that
	// don't set them get empty strings here and the UI hides the
	// corresponding affordance instead of falling back to a hardcoded value.
	return subscriptionFetchResult{
		Entries:    entries,
		UserInfo:   resp.Header.Get("Subscription-Userinfo"),
		Title:      resp.Header.Get("Profile-Title"),
		SupportURL: strings.TrimSpace(resp.Header.Get("Support-Url")),
		Diag:       diag,
	}, nil
}

// normalizeSubscriptionURL applies provider-specific URL tweaks. impio.space
// only returns the rich Xray JSON config when the path ends in /json — without
// the suffix it serves a thin Shadowsocks-only list.
func normalizeSubscriptionURL(subURL string) string {
	if u, err := url.Parse(subURL); err == nil && u.Host == "my.impio.space" {
		if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/json") {
			u.Path = strings.TrimRight(u.Path, "/") + "/json"
			return u.String()
		}
	}
	return subURL
}

// BuildOptions controls optional knobs on the mobile config builder.
// Mirrors the user-facing Settings: DNS preset, IPv6 dual-stack,
// bypass-LAN, log verbosity.
//
// gomobile cannot bind structs across the JNI boundary, so callers
// stringify their options as JSON and pass via the V2 entry point.
type BuildOptions struct {
	DNSServers      string `json:"dnsServers"`
	ExcludedDomains string `json:"excludedDomains"`
	IPv6            bool   `json:"ipv6"`
	BypassLAN       bool   `json:"bypassLAN"`
	LogLevel        string `json:"logLevel"`
	// SmartMode enables Antizapret-style routing: only listed domains
	// go through the proxy; everything else (foreign sites, LAN, system
	// traffic) stays direct. The list is fetched by Mobile.FetchSmartList
	// and passed back via SmartBlockedDomains as a `\n`-separated string
	// (gomobile can't bind []string across JNI cleanly).
	SmartMode               bool   `json:"smartMode"`
	SmartBlockedDomainsList string `json:"smartBlockedDomainsList,omitempty"`
	// AdBlock enables DNS+route reject of ad/tracker domains via sing-box
	// rule_set (see internal/proxy/adblock_rules.go). Lists are cached by
	// FetchAdBlockLists; a missing list falls back to remote download and
	// degrades to "no blocking" rather than breaking the connection.
	AdBlock bool `json:"adblock,omitempty"`
	// YouTubeUnblock enables the YouTube geo-split (see
	// internal/proxy/youtube_rules.go): video via proxy, player/API direct so
	// Google sees the user's Russian IP and serves no ads.
	YouTubeUnblock bool `json:"youtubeUnblock,omitempty"`
}

// BuildSingBoxConfig converts a proxy URI directly into a sing-box JSON
// config suitable for libbox.CommandServer.StartOrReloadService on
// Android. The TUN inbound is left in auto_route mode so libbox calls
// the platform-side OpenTun() callback (which the VpnService implements).
//
// dataDir must be a writable, app-private path — typically
// context.filesDir on Android — used by sing-box for its cache_file.
//
// dnsServers is a comma-separated list ("8.8.8.8,1.1.1.1"); pass "" for
// built-in defaults.
//
// excludedDomains is a comma-separated list of host patterns that should
// bypass the proxy (route via `direct`). Patterns starting with `*.`
// become domain_suffix matches (`*.ru` → `.ru`); bare hostnames
// (`yandex.ru`) become exact domain matches. Pass "" for none.
//
// Legacy entrypoint — toggles default to IPv4-only, no bypass-LAN,
// info-level logging. Use BuildSingBoxConfigV2 to pass the new flags.
func BuildSingBoxConfig(uri string, dataDir string, dnsServers string, excludedDomains string) (string, error) {
	return BuildSingBoxConfigV2(uri, dataDir, encodeOptions(BuildOptions{
		DNSServers:      dnsServers,
		ExcludedDomains: excludedDomains,
	}))
}

// BuildSingBoxConfigFromEntry is the entry-based counterpart used by
// subscription imports, where entries arrive as parsed ProxyEntry JSON
// (no source URI to round-trip through). entryJson is the same JSON that
// FetchSubscription puts in its result array.
//
// AUTO envelopes (Type=="AUTO" with Extra.members) are unwrapped to the
// first member; sing-box can't multiplex these natively in our PoC
// pipeline, and a latency-driven probe will live in Connect later.
//
// See BuildSingBoxConfig for excludedDomains semantics.
func BuildSingBoxConfigFromEntry(entryJson, dataDir, dnsServers, excludedDomains string) (string, error) {
	return BuildSingBoxConfigFromEntryV2(entryJson, dataDir, encodeOptions(BuildOptions{
		DNSServers:      dnsServers,
		ExcludedDomains: excludedDomains,
	}))
}

// BuildSingBoxConfigV2 is the JSON-options variant. Pass `optionsJson`
// as a stringified BuildOptions ({"dnsServers":"...","ipv6":true,...}).
// Empty / malformed JSON falls back to all-defaults.
func BuildSingBoxConfigV2(uri, dataDir, optionsJson string) (string, error) {
	entry, err := proxy.ParseProxyURI(uri)
	if err != nil {
		return "", fmt.Errorf("parsing URI: %w", err)
	}
	return buildSingBoxConfigFromEntry(entry, dataDir, decodeOptions(optionsJson))
}

// BuildSingBoxConfigFromEntryV2 is the JSON-options variant of
// BuildSingBoxConfigFromEntry.
func BuildSingBoxConfigFromEntryV2(entryJson, dataDir, optionsJson string) (string, error) {
	var entry config.ProxyEntry
	if err := json.Unmarshal([]byte(entryJson), &entry); err != nil {
		return "", fmt.Errorf("parsing entry JSON: %w", err)
	}
	if strings.EqualFold(entry.Type, "AUTO") {
		members, err := decodeAutoMembers(entry.Extra)
		if err != nil {
			return "", fmt.Errorf("auto members: %w", err)
		}
		if len(members) == 0 {
			return "", fmt.Errorf("auto profile has no members")
		}
		entry = members[0]
	}
	return buildSingBoxConfigFromEntry(entry, dataDir, decodeOptions(optionsJson))
}

func encodeOptions(o BuildOptions) string {
	b, err := json.Marshal(o)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func decodeOptions(optionsJson string) BuildOptions {
	var o BuildOptions
	if strings.TrimSpace(optionsJson) == "" {
		return o
	}
	if err := json.Unmarshal([]byte(optionsJson), &o); err != nil {
		return BuildOptions{}
	}
	return o
}

// splitSmartList parses the newline-separated blocked-domain list sent by
// the Kotlin side (SmartListRepository persists it as plain text — gomobile
// can't shuttle []string cleanly across JNI). Blank lines and `#` comments
// are tolerated so the list can be edited by hand if needed.
func splitSmartList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, s)
	}
	return out
}

// FetchSmartList resolves the user's country and downloads the
// Antizapret-style blocked-domain list for that country. The list is
// cached in dataDir/smart-blocked.json (managed by the existing
// blocked_provider) and returned as a JSON-encoded result the Kotlin side
// can render:
//
//	{
//	  "domains":   ["instagram.com", "x.com", …],   // normalised, dedup'd
//	  "country":   "ru",                              // resolved alpha-2
//	  "source":    "remote" | "cache" | "builtin",
//	  "error":     ""                                 // non-empty on soft failure
//	}
//
// country may be empty to force auto-detection via the embedded provider;
// pass a 2-letter ISO code (e.g. "ru") to skip auto-detect (faster, no
// IP-geolocation request).
func FetchSmartList(country string, dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required for smart-list cache")
	}
	provider := proxy.NewHTTPBlockedListProvider()
	cachePath := filepath.Join(dataDir, "smart-blocked.json")

	cc := strings.ToLower(strings.TrimSpace(country))
	var result proxy.BlockedDomainsResolveResult
	if cc != "" {
		// Caller pinned country (typical: Settings → Smart mode region picker).
		// Skip the provider's country resolution to avoid the ip-api leak.
		domains, fetchErr := provider.FetchBlockedDomains(context.Background(), cc)
		if fetchErr == nil && len(domains) > 0 {
			cache := proxy.BlockedDomainsCache{
				Country:   cc,
				UpdatedAt: time.Now().Unix(),
				Source:    "remote",
				Domains:   domains,
			}
			_ = proxy.SaveBlockedDomainsCache(cachePath, cache)
			result = proxy.BlockedDomainsResolveResult{
				Domains: domains,
				Country: cc,
				Source:  "remote",
			}
		} else {
			// Fall through to cache / builtin via the standard resolver.
			result = proxy.ResolveBlockedDomains(context.Background(), nil, cachePath)
			if fetchErr != nil {
				result.Err = fetchErr
			}
		}
	} else {
		result = proxy.ResolveBlockedDomains(context.Background(), provider, cachePath)
	}

	out := map[string]interface{}{
		"domains": result.Domains,
		"country": result.Country,
		"source":  result.Source,
		"error":   "",
	}
	if result.Err != nil {
		out["error"] = result.Err.Error()
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshaling smart list: %w", err)
	}
	return string(data), nil
}

// FetchAdBlockLists downloads the ad-blocking rule-set SRS files into
// dataDir/adblock so the engine can reference them locally (offline, instant)
// on the next connect instead of having sing-box fetch them remotely. Returns
// JSON the Kotlin side can render:
//
//	{ "ready": 2, "total": 2, "error": "" }
//
// Safe to call repeatedly: writes are atomic, a failed download leaves any
// previously cached list intact, and the engine falls back to a remote
// rule_set for whatever isn't cached — so ad-block never blocks a connection
// from coming up.
func FetchAdBlockLists(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required for adblock cache")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res := proxy.DownloadAdBlockRuleSets(ctx, dataDir)
	out := map[string]interface{}{
		"ready": res.Ready,
		"total": res.Total,
		"error": "",
	}
	if res.Err != nil {
		out["error"] = res.Err.Error()
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshaling adblock result: %w", err)
	}
	return string(data), nil
}

// splitDomainPatterns parses a comma-separated list of host patterns into
// sing-box `domain` (exact) and `domain_suffix` (substring suffix) buckets.
//
//	"*.ru, yandex.ru, *.рф" → exact=[yandex.ru]  suffix=[.ru .рф]
//
// `*.foo` strips the asterisk and keeps the leading dot so it matches only
// subdomains of foo, not the bare TLD label inside unrelated hostnames.
// Whitespace and empty entries are tolerated; duplicates are kept (sing-box
// dedups internally when matching).
func splitDomainPatterns(csv string) (exact []string, suffix []string) {
	for _, raw := range strings.Split(csv, ",") {
		p := strings.ToLower(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		switch {
		case strings.HasPrefix(p, "*."):
			suffix = append(suffix, strings.TrimPrefix(p, "*"))
		case strings.HasPrefix(p, "."):
			suffix = append(suffix, p)
		default:
			exact = append(exact, p)
		}
	}
	return exact, suffix
}

func decodeAutoMembers(extra json.RawMessage) ([]config.ProxyEntry, error) {
	if len(extra) == 0 {
		return nil, nil
	}
	var wrap struct {
		Members []config.ProxyEntry `json:"members"`
	}
	if err := json.Unmarshal(extra, &wrap); err != nil {
		return nil, err
	}
	return wrap.Members, nil
}

func buildSingBoxConfigFromEntry(entry config.ProxyEntry, dataDir string, opts BuildOptions) (string, error) {
	if dataDir == "" {
		return "", fmt.Errorf("dataDir is required on mobile (pass context.filesDir)")
	}

	cfg := proxy.EngineConfig{
		Proxy: proxy.ProxyConfig{
			IP:       entry.IP,
			Port:     entry.Port,
			Type:     entry.Type,
			Username: entry.Username,
			Password: entry.Password,
			URI:      entry.URI,
			Extra:    entry.Extra,
		},
		Mode:                proxy.ProxyModeTunnel,
		DataDir:             dataDir,
		TunIPv4:             "172.19.0.1/30",
		IsAndroid:           true,
		IPv6:                opts.IPv6,
		BypassLAN:           opts.BypassLAN,
		LogLevel:            opts.LogLevel,
		SmartMode:           opts.SmartMode,
		SmartBlockedDomains: splitSmartList(opts.SmartBlockedDomainsList),
		AdBlock:             opts.AdBlock,
		YouTubeUnblock:      opts.YouTubeUnblock,
		// Desktop's DNSLeakProtection (toggles strict_route) is forced off
		// on Android — VpnService can't manipulate routes outside its TUN,
		// and AutoRoute already catches all egress traffic, so DNS bypass
		// isn't possible the way it is on desktop.
	}
	if opts.DNSServers != "" {
		for _, p := range strings.Split(opts.DNSServers, ",") {
			if s := strings.TrimSpace(p); s != "" {
				cfg.DNSServers = append(cfg.DNSServers, s)
			}
		}
	}

	sb := proxy.BuildTunnelModeConfig(cfg)

	// Android-specific overrides: VpnService can't manipulate iptables
	// or perform privileged route operations. strict_route MUST be off
	// regardless of protocol. auto_route stays on — that's the signal
	// libbox uses to call PlatformInterface.OpenTun() instead of opening
	// a tun device itself. route_exclude_address is unnecessary since
	// VpnService.protect(fd) handles server-bypass at the socket level.
	//
	// Additionally, strip any process_path_regex rules: on Android,
	// SELinux (tclass=file, proc_net_tcp_udp) prevents untrusted_app from
	// reading /proc/net/tcp, so sing-box can never resolve a process path.
	// Our own process is already excluded via addDisallowedApplication in
	// BoxPlatform.openTun, so the self-bypass rule is redundant anyway.
	for i := range sb.Inbounds {
		if sb.Inbounds[i].Type == "tun" {
			sb.Inbounds[i].StrictRoute = false
			sb.Inbounds[i].AutoRoute = true
			sb.Inbounds[i].RouteExcludeAddress = nil
		}
	}
	if sb.Route != nil {
		var filteredRules []proxy.SBRouteRule
		for _, r := range sb.Route.Rules {
			if len(r.ProcessPathRegex) == 0 {
				filteredRules = append(filteredRules, r)
			}
		}
		sb.Route.Rules = filteredRules
		// `auto_detect_interface` is rejected by sing-box's NewNetworkManager
		// on Android (route/network.go:63 — only Linux/macOS/Windows). When
		// libbox is used WITH a PlatformInterface, sing-box discovers the
		// underlying interface via our CreateDefaultInterfaceMonitor and
		// per-socket `protect(fd)` automatically; we must NOT set the flag.
		sb.Route.AutoDetect = false

		// Domain exclusions: route matching hosts to `direct`, bypassing
		// the proxy. MUST be appended AFTER the `sniff` action rule —
		// sing-box only knows the destination IP at TUN ingress; the
		// domain is populated from TLS SNI / HTTP Host once `sniff` runs,
		// after which `domain` / `domain_suffix` matchers can fire.
		// Putting this rule before sniff means it never matches.
		if opts.ExcludedDomains != "" {
			seen := make(map[string]struct{})
			var normalized []string
			for _, part := range strings.Split(opts.ExcludedDomains, ",") {
				n := strings.TrimSpace(part)
				n = strings.ToLower(n)
				n = strings.TrimLeft(n, "*.")
				n = strings.TrimRight(n, "*.")
				if n == "" {
					continue
				}
				if _, ok := seen[n]; !ok {
					seen[n] = struct{}{}
					normalized = append(normalized, n)
				}
			}

			if len(normalized) > 0 {
				ordered := append([]string(nil), normalized...)
				sort.SliceStable(ordered, func(i, j int) bool {
					di := strings.Count(ordered[i], ".")
					dj := strings.Count(ordered[j], ".")
					if di != dj {
						return di > dj
					}
					if len(ordered[i]) != len(ordered[j]) {
						return len(ordered[i]) > len(ordered[j])
					}
					return ordered[i] < ordered[j]
				})

				isExcluded := func(host string, all []string) bool {
					matchCount := 0
					for _, rule := range all {
						if host == rule || strings.HasSuffix(host, "."+rule) {
							matchCount++
						}
					}
					return matchCount > 0 && matchCount%2 == 1
				}

				for _, suffix := range ordered {
					outbound := "proxy"
					if isExcluded(suffix, normalized) {
						outbound = "direct"
					}
					sb.Route.Rules = append(sb.Route.Rules, proxy.SBRouteRule{
						Action:       "route",
						DomainSuffix: []string{suffix},
						Outbound:     outbound,
					})
				}
			}
		}
	}

	// `type: local` resolves through the system resolver, which on Android
	// means /etc/resolv.conf (absent) or 127.0.0.1:53 (no daemon) — every
	// proxy-server hostname lookup fails with "connection refused" and the
	// VLESS dial never starts. Replace the local server with a real public
	// DNS dialled via `direct` (Android's normal network, works even
	// before the tunnel comes up).
	//
	// MUST always be direct/UDP regardless of DNSLeakProtection: the
	// "local" tag is the bootstrap resolver — engine.go adds a DNS rule
	// routing the proxy server's own hostname to "local". Sending that
	// query through the proxy creates a chicken-and-egg loop (proxy can't
	// dial because we can't resolve its hostname; we can't resolve
	// because the proxy isn't up). User DNS leak protection is already
	// covered by the custom-* servers, which use `detour: proxy`.
	if sb.DNS != nil {
		for i := range sb.DNS.Servers {
			if sb.DNS.Servers[i].Type == "local" {
				sb.DNS.Servers[i] = proxy.SBDNSServer{
					Type:   "udp",
					Tag:    sb.DNS.Servers[i].Tag,
					Server: "1.1.1.1",
					Detour: "direct",
				}
			}
		}
	}
	// Log level is set by EngineConfig.LogLevel above — callers may pass
	// "debug" while iterating on a new protocol; default is "error" so
	// release builds don't spam logcat.

	out, err := json.MarshalIndent(sb, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}
	return string(out), nil
}
