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

package proxy

// Routing profiles delivered by a deep link.
//
// A panel publishes a profile as an https link that redirects to a client's own
// scheme, carrying the rules as base64url'd JSON:
//
//	https://panel.example/routing/resultv/whitelist
//	  -> 302 Location: resultv://routing/onadd/<base64url(JSON)>
//
// The payload shape is the one panels already emit for other clients, field
// names and all — see docs/ROUTING-DEEPLINK.md. Matching it exactly is the whole
// point: a panel that already publishes such links only has to add our scheme
// to the list, not build anything new. So the fields are read as they arrive,
// including the ones that arrive as strings ("true", "1788322632") rather than
// as the booleans and numbers they describe.
//
// The payload is NOT encrypted, unlike a resultv:// subscription link. A routing
// profile is public — a list of what goes where — and requiring our RVSUB1 key
// would mean no third-party panel could ever publish one.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	urlpkg "net/url"
	"strconv"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
)

// Kinds of resultv:// link, as reported by DeepLinkKind.
const (
	DeepLinkKindSubscription = "subscription"
	DeepLinkKindRouting      = "routing"
)

// routingDeepLinkPaths are the prefixes that mark a routing link. "onadd" is
// the form panels already emit; the bare "routing/" form is accepted so a
// hand-written link is not rejected over a missing path segment.
var routingDeepLinkPaths = []string{"routing/onadd/", "routing/add/", "routing/"}

// Caps on a decoded profile. A deep link is attacker-reachable — anyone can
// send one — and everything here lands in the config file, so a payload that
// would bloat it past usefulness is refused rather than trimmed silently.
const (
	MaxRoutingProfileTokens   = 20000
	MaxRoutingProfileNameLen  = 200
	MaxRoutingDeepLinkPayload = 4 << 20 // 4 MiB of base64
)

// ErrNotRoutingDeepLink is returned when the URL is a resultv:// link of some
// other kind. Callers use it to fall through to the subscription path.
var ErrNotRoutingDeepLink = errors.New("not a routing deep link")

// routingProfilePayload mirrors the JSON panels publish. Field names follow the
// wire exactly, including its inconsistent casing (Geoipurl, DirectIp): these
// are matched case-insensitively by encoding/json, but spelling them as they
// arrive keeps the mapping checkable against a captured payload.
type routingProfilePayload struct {
	Name string `json:"Name"`

	DirectSites []string `json:"DirectSites"`
	DirectIP    []string `json:"DirectIp"`
	ProxySites  []string `json:"ProxySites"`
	ProxyIP     []string `json:"ProxyIp"`
	BlockSites  []string `json:"BlockSites"`
	BlockIP     []string `json:"BlockIp"`

	RouteOrder     string `json:"RouteOrder"`
	DomainStrategy string `json:"DomainStrategy"`

	GeoIPURL   string `json:"Geoipurl"`
	GeoSiteURL string `json:"Geositeurl"`

	// Arrives as a string holding a unix timestamp.
	LastUpdated string `json:"LastUpdated"`
}

// IsRoutingDeepLink reports whether rawURL is a resultv:// routing link.
func IsRoutingDeepLink(rawURL string) bool {
	_, ok := routingDeepLinkBody(rawURL)
	return ok
}

// DeepLinkKind classifies a resultv:// link so the caller knows which importer
// to hand it to. Anything that is not a routing link is a subscription link:
// that was the only kind before profiles existed, and links already in the wild
// carry no marker saying so.
func DeepLinkKind(rawURL string) string {
	if IsRoutingDeepLink(rawURL) {
		return DeepLinkKindRouting
	}
	return DeepLinkKindSubscription
}

// routingDeepLinkBody strips the scheme and the routing path prefix, returning
// the base64 body.
func routingDeepLinkBody(rawURL string) (string, bool) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimRight(rawURL, "/\x00\r\n\t ")
	if !IsDeepLink(rawURL) {
		return "", false
	}
	low := strings.ToLower(rawURL)
	var body string
	switch {
	case strings.HasPrefix(low, DeepLinkScheme):
		body = rawURL[len(DeepLinkScheme):]
	case strings.HasPrefix(low, deepLinkSchemeOpaque):
		body = rawURL[len(deepLinkSchemeOpaque):]
	default:
		return "", false
	}
	body = strings.TrimLeft(body, "/")
	lower := strings.ToLower(body)
	for _, prefix := range routingDeepLinkPaths {
		if strings.HasPrefix(lower, prefix) {
			return body[len(prefix):], true
		}
	}
	return "", false
}

// DecodeRoutingDeepLink turns a resultv://routing/onadd/<base64> link into a
// profile. The returned profile has no ID — the app layer assigns one, since
// only it knows what is already stored.
func DecodeRoutingDeepLink(rawURL string) (config.RoutingProfile, error) {
	body, ok := routingDeepLinkBody(rawURL)
	if !ok {
		return config.RoutingProfile{}, ErrNotRoutingDeepLink
	}
	blob, err := decodeRoutingPayload(body)
	if err != nil {
		return config.RoutingProfile{}, err
	}
	return ParseRoutingProfileJSON(blob)
}

func decodeRoutingPayload(body string) ([]byte, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("routing link carries no payload")
	}
	if len(body) > MaxRoutingDeepLinkPayload {
		return nil, fmt.Errorf("routing link payload is too large (%d bytes)", len(body))
	}
	// Percent-encoding survives some browsers and shells.
	if decoded, derr := urlpkg.QueryUnescape(body); derr == nil && decoded != "" {
		body = decoded
	}
	body = sanitizeBase64(body)
	if body == "" {
		return nil, errors.New("routing link payload contains no base64 characters")
	}
	// Panels emit base64url without padding; accept every spelling rather than
	// bouncing a link over a couple of '=' characters.
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.StdEncoding,
	} {
		if blob, derr := enc.DecodeString(body); derr == nil && len(blob) > 0 {
			return blob, nil
		}
	}
	return nil, errors.New("routing link payload is not valid base64")
}

// ParseRoutingProfileJSON reads the published JSON into a profile.
func ParseRoutingProfileJSON(blob []byte) (config.RoutingProfile, error) {
	var raw routingProfilePayload
	if err := json.Unmarshal(blob, &raw); err != nil {
		return config.RoutingProfile{}, fmt.Errorf("routing profile is not valid JSON: %w", err)
	}

	p := config.RoutingProfile{
		Name:           trimTo(strings.TrimSpace(raw.Name), MaxRoutingProfileNameLen),
		DirectSites:    cleanTokens(raw.DirectSites),
		DirectIPs:      cleanTokens(raw.DirectIP),
		ProxySites:     cleanTokens(raw.ProxySites),
		ProxyIPs:       cleanTokens(raw.ProxyIP),
		BlockSites:     cleanTokens(raw.BlockSites),
		BlockIPs:       cleanTokens(raw.BlockIP),
		RouteOrder:     normalizeRouteOrder(raw.RouteOrder),
		DomainStrategy: strings.TrimSpace(raw.DomainStrategy),
		GeoIPURL:       safeGeoURL(raw.GeoIPURL),
		GeoSiteURL:     safeGeoURL(raw.GeoSiteURL),
		Source:         "deeplink",
		UpdatedAt:      time.Now().Unix(),
	}

	total := p.RuleCount("direct") + p.RuleCount("proxy") + p.RuleCount("block")
	if total == 0 {
		return config.RoutingProfile{}, errors.New("routing profile carries no rules")
	}
	if total > MaxRoutingProfileTokens {
		return config.RoutingProfile{}, fmt.Errorf(
			"routing profile carries %d rules, more than the %d allowed",
			total, MaxRoutingProfileTokens)
	}
	if p.Name == "" {
		p.Name = "Routing profile"
	}
	// The publisher's own name, kept apart from the one the user may rename.
	p.OriginName = p.Name
	// LastUpdated is the panel's own stamp; it says when the rules were built,
	// not when we imported them, so it only replaces ours when it parses.
	if ts, err := strconv.ParseInt(strings.TrimSpace(raw.LastUpdated), 10, 64); err == nil && ts > 0 {
		p.UpdatedAt = ts
	}
	return p, nil
}

// RoutingProfileTokens returns the tokens of one action, sites and IPs together
// — the form ResolveGeoTokens expects.
func RoutingProfileTokens(p config.RoutingProfile, action string) []string {
	switch action {
	case "direct":
		return append(append([]string{}, p.DirectSites...), p.DirectIPs...)
	case "proxy":
		return append(append([]string{}, p.ProxySites...), p.ProxyIPs...)
	case "block":
		return append(append([]string{}, p.BlockSites...), p.BlockIPs...)
	}
	return nil
}

// validRouteOrders are the ways the three actions can be ordered. The order
// decides which rule wins when several match, so an unrecognised value is
// dropped rather than guessed at.
var validRouteOrders = map[string]struct{}{
	"block-proxy-direct": {}, "block-direct-proxy": {},
	"proxy-block-direct": {}, "proxy-direct-block": {},
	"direct-block-proxy": {}, "direct-proxy-block": {},
}

func normalizeRouteOrder(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	if _, ok := validRouteOrders[v]; ok {
		return v
	}
	return ""
}

// safeGeoURL keeps only http(s) URLs. The value is fetched later, so a
// "file:///" or "javascript:" spelling from a hostile link must not reach the
// fetcher at all.
func safeGeoURL(s string) string {
	v := strings.TrimSpace(s)
	if v == "" {
		return ""
	}
	low := strings.ToLower(v)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return ""
	}
	return v
}

func cleanTokens(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if len(t) > 512 { // no legitimate rule token is this long
			continue
		}
		key := strings.ToLower(t)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func trimTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max])
}
