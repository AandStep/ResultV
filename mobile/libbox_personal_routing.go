package mobile

import (
	"net/netip"
	"slices"
	"strings"

	"resultproxy-wails/internal/proxy"
)

func personalSiteRules(opts BuildOptions) []proxy.SBRouteRule {
	var routes []proxy.SBRouteRule
	for _, direction := range []struct{ csv, outbound string }{
		{opts.IntoVpnDomains, "proxy"},
		{opts.ExcludedDomains, "direct"},
	} {
		for _, site := range splitRuleCSV(direction.csv) {
			rule := siteRouteMatcher(site)
			if siteSpecificity(rule) == 0 {
				continue
			}
			rule.Action, rule.Outbound = "route", direction.outbound
			routes = append(routes, rule)
		}
	}
	slices.SortStableFunc(routes, func(a, b proxy.SBRouteRule) int {
		return siteSpecificity(b) - siteSpecificity(a)
	})
	return routes
}

func siteRouteMatcher(site string) proxy.SBRouteRule {
	if p, err := netip.ParsePrefix(site); err == nil {
		return proxy.SBRouteRule{IPCidr: []string{p.Masked().String()}}
	}
	if addr, err := netip.ParseAddr(site); err == nil && addr.Zone() == "" {
		return proxy.SBRouteRule{IPCidr: []string{netip.PrefixFrom(addr, addr.BitLen()).String()}}
	}
	if site = strings.Trim(strings.ToLower(site), "*."); site != "" && !strings.ContainsAny(site, "/: *") {
		return proxy.SBRouteRule{DomainSuffix: []string{site}}
	}
	return proxy.SBRouteRule{}
}

func siteSpecificity(rule proxy.SBRouteRule) int {
	if len(rule.DomainSuffix) > 0 {
		return len(rule.DomainSuffix[0])
	}
	if len(rule.IPCidr) > 0 {
		return netip.MustParsePrefix(rule.IPCidr[0]).Bits() + 1
	}
	return 0
}

func personalProfileBlocks(opts BuildOptions, blockSet string) []proxy.SBRouteRule {
	matches := personalSiteRules(opts)
	if apps := splitRuleCSV(opts.IntoVpnApps); len(apps) > 0 {
		matches = append(matches, proxy.SBRouteRule{PackageName: apps})
	}
	for i, match := range matches {
		match.Action, match.Outbound = "", ""
		matches[i] = proxy.SBRouteRule{
			Type: "logical", Mode: "and", Action: "reject",
			Rules: []proxy.SBRouteRule{match, {RuleSet: []string{blockSet}}},
		}
	}
	return matches
}
