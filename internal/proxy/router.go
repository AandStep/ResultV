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

import (
	"bufio"
	"net"
	"os"
	"strings"
	"sync"
)


type RoutingMode string

const (
	ModeGlobal    RoutingMode = "global"    
	ModeWhitelist RoutingMode = "whitelist" 
	ModeSmart     RoutingMode = "smart"     
)


type WhitelistResult struct {
	IsWhitelisted bool     
	MatchingRules []string 
}



type Router struct {
	mu             sync.RWMutex
	blockedDomains []string
	blockedCIDRs   []string
}


func NewRouter() *Router {
	return &Router{
		blockedDomains: defaultBlockedDomains(),
		blockedCIDRs:   defaultBlockedCIDRs(),
	}
}


func (r *Router) LoadBlockedLists(paths ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range paths {
		domains := loadDomainFile(p)
		for _, d := range normalizeDomains(domains) {
			if !r.containsBlocked(d) {
				r.blockedDomains = append(r.blockedDomains, d)
			}
		}
	}
}

func (r *Router) SetBlockedDomains(domains []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockedDomains = normalizeDomains(domains)
}

// SetBlockedCIDRs replaces the IP-subnet block-list (Telegram MTProto
// data-center ranges and any other IP-only blocked services). Invalid entries
// are dropped; bare IPs are widened to host CIDRs.
func (r *Router) SetBlockedCIDRs(cidrs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockedCIDRs = normalizeCIDRs(cidrs)
}

// GetBlockedCIDRs returns a copy of the IP-subnet block-list. Consumed by the
// engine in Smart mode to route those ranges through the proxy (domain rules
// can't catch IP-only protocols like Telegram's MTProto).
func (r *Router) GetBlockedCIDRs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.blockedCIDRs))
	copy(out, r.blockedCIDRs)
	return out
}













func (r *Router) IsWhitelisted(hostname string, whitelist []string) WhitelistResult {
	if hostname == "" || len(whitelist) == 0 {
		return WhitelistResult{IsWhitelisted: false, MatchingRules: nil}
	}

	h := normalizeRule(hostname)
	if h == "" {
		return WhitelistResult{IsWhitelisted: false, MatchingRules: nil}
	}

	
	seen := make(map[string]struct{}, len(whitelist))
	normalized := make([]string, 0, len(whitelist))
	for _, rule := range whitelist {
		n := normalizeRule(rule)
		if n == "" {
			continue
		}
		if _, exists := seen[n]; !exists {
			seen[n] = struct{}{}
			normalized = append(normalized, n)
		}
	}

	
	var matching []string
	for _, rule := range normalized {
		if h == rule || strings.HasSuffix(h, "."+rule) {
			matching = append(matching, rule)
		}
	}

	if len(matching) == 0 {
		return WhitelistResult{IsWhitelisted: false, MatchingRules: nil}
	}

	
	return WhitelistResult{
		IsWhitelisted: len(matching)%2 == 1,
		MatchingRules: matching,
	}
}



func (r *Router) ShouldProxy(hostname string, mode RoutingMode, whitelist []string) (useProxy bool, reason string) {
	result := r.IsWhitelisted(hostname, whitelist)

	switch mode {
	case ModeSmart:
		if !result.IsWhitelisted && len(result.MatchingRules) > 0 {
			return true, "Nested exception: [" + strings.Join(result.MatchingRules, ", ") + "]"
		}
		blocked := r.IsBlockedDomain(hostname)
		if result.IsWhitelisted {
			if blocked {
				return true, "Blocked resource in Smart mode"
			}
			return false, "Bypass (Whitelisted)"
		}
		if blocked {
			return true, "Blocked resource (No match)"
		}
		return false, "Direct (Not blocked)"

	case ModeWhitelist:
		fallthrough
	default: 
		if result.IsWhitelisted {
			return false, "Whitelisted (Bypass)"
		}
		return true, "Not whitelisted (Proxy)"
	}
}



func (r *Router) IsBlockedDomain(hostname string) bool {
	if hostname == "" {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	h := strings.ToLower(hostname)
	for _, d := range r.blockedDomains {
		if strings.Contains(h, d) {
			return true
		}
	}
	return false
}




func (r *Router) GetSafeOSWhitelist(whitelist []string) []string {
	if len(whitelist) == 0 {
		return nil
	}

	
	seen := make(map[string]struct{}, len(whitelist))
	normalized := make([]string, 0, len(whitelist))
	for _, rule := range whitelist {
		n := normalizeRule(rule)
		if n == "" {
			continue
		}
		if _, exists := seen[n]; !exists {
			seen[n] = struct{}{}
			normalized = append(normalized, n)
		}
	}

	var safe []string
	for _, rule := range normalized {
		
		result := r.IsWhitelisted(rule, whitelist)
		if !result.IsWhitelisted {
			continue
		}

		
		hasChild := false
		for _, other := range normalized {
			if other != rule && strings.HasSuffix(other, "."+rule) {
				hasChild = true
				break
			}
		}
		if !hasChild {
			safe = append(safe, rule)
		}
	}
	return safe
}


func (r *Router) GetBlockedDomains() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, len(r.blockedDomains))
	copy(result, r.blockedDomains)
	return result
}





func normalizeRule(rule string) string {
	if rule == "" {
		return ""
	}
	r := strings.TrimSpace(rule)
	r = strings.ToLower(r)

	
	if idx := strings.Index(r, "://"); idx >= 0 {
		r = r[idx+3:]
	}
	
	if idx := strings.IndexByte(r, '/'); idx >= 0 {
		r = r[:idx]
	}
	
	r = strings.TrimLeft(r, "*.")
	
	r = strings.TrimRight(r, "*.")

	return r
}

func (r *Router) containsBlocked(domain string) bool {
	for _, d := range r.blockedDomains {
		if d == domain {
			return true
		}
	}
	return false
}

// normalizeCIDRs validates and de-duplicates a list of CIDR strings. Bare IPs
// are widened to host CIDRs (/32 or /128); blanks, comments and unparseable
// entries are dropped. Output is canonicalised via net.IPNet.String().
func normalizeCIDRs(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if !strings.Contains(s, "/") {
			ip := net.ParseIP(s)
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		_, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			continue
		}
		n := ipNet.String()
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// defaultBlockedCIDRs are the Telegram MTProto data-center subnets. Telegram's
// native clients dial these IPs directly — no domain, no TLS SNI — so the
// domain-suffix block-list can never catch them; Smart mode needs an ip_cidr
// rule. Sourced from itdoginfo/allow-domains (Subnets/IPv4+IPv6/telegram.lst);
// kept as a static fallback for when the remote list and cache are unavailable.
func defaultBlockedCIDRs() []string {
	return []string{
		// IPv4
		"5.28.192.0/18",
		"91.105.192.0/23",
		"91.108.4.0/22",
		"91.108.8.0/21",
		"91.108.16.0/21",
		"91.108.56.0/22",
		"95.161.64.0/20",
		"149.154.160.0/20",
		"185.76.151.0/24",
		"194.221.0.0/16",
		// IPv6
		"2001:67c:4e8::/48",
		"2001:b28:f23c::/47",
		"2001:b28:f23f::/48",
		"2a0a:f280::/32",
	}
}

func defaultBlockedDomains() []string {
	return []string{
		"instagram.com",
		"facebook.com",
		"twitter.com",
		"x.com",
		"t.me",
		"discord.com",
		"netflix.com",
	}
}

func loadDomainFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var domains []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.ToLower(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			domains = append(domains, line)
		}
	}
	return domains
}

func normalizeDomains(domains []string) []string {
	if len(domains) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		n := normalizeRule(d)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
