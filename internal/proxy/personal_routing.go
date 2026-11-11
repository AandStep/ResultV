package proxy

import (
	"net/netip"
	"slices"
)

// PersonalRouting carries the additional lists alongside the legacy fields.
// The engine selects these by mode, including when rebuilding a live session.
type PersonalRouting struct {
	SmartVPNSites    []string
	SmartDirectSites []string
	SmartDirectApps  []string
	GlobalVPNSites   []string
	GlobalVPNApps    []string
}

func (p PersonalRouting) forMode(mode RoutingMode) PersonalRouting {
	switch mode {
	case ModeSmart:
		return PersonalRouting{SmartVPNSites: p.SmartVPNSites, SmartDirectSites: p.SmartDirectSites, SmartDirectApps: p.SmartDirectApps}
	case ModeGlobal, ModeWhitelist:
		return PersonalRouting{GlobalVPNSites: p.GlobalVPNSites, GlobalVPNApps: p.GlobalVPNApps}
	default:
		return PersonalRouting{}
	}
}

func (cfg EngineConfig) directApps() []string {
	if cfg.RoutingMode == ModeSmart {
		return cfg.PersonalRouting.SmartDirectApps
	}
	return cfg.AppWhitelist
}

func (cfg EngineConfig) directSites() []string {
	if cfg.RoutingMode == ModeSmart {
		return cfg.PersonalRouting.SmartDirectSites
	}
	return cfg.Whitelist
}

func (cfg EngineConfig) personalSiteRoutes() []SBRouteRule {
	vpnSites := cfg.PersonalRouting.GlobalVPNSites
	if cfg.RoutingMode == ModeSmart {
		vpnSites = cfg.PersonalRouting.SmartVPNSites
	}
	type siteRoute struct {
		match       SBRouteRule
		specificity int
	}
	var sites []siteRoute
	for _, direction := range []struct {
		outbound string
		entries  []string
	}{{"proxy", vpnSites}, {"direct", cfg.directSites()}} {
		for _, value := range direction.entries {
			rule := SBRouteRule{Action: "route", Outbound: direction.outbound}
			if cidrs := normalizeCIDRs([]string{value}); len(cidrs) > 0 {
				prefix := netip.MustParsePrefix(cidrs[0])
				rule.IPCidr = cidrs
				sites = append(sites, siteRoute{rule, prefix.Bits()})
			} else if domain := normalizeRule(value); domain != "" {
				rule.DomainSuffix = []string{domain}
				sites = append(sites, siteRoute{rule, len(domain)})
			}
		}
	}
	// A child domain or narrower IP prefix wins. Stable sorting gives VPN
	// priority for identical entries in imported configurations.
	slices.SortStableFunc(sites, func(a, b siteRoute) int { return b.specificity - a.specificity })
	var rules []SBRouteRule
	for _, site := range sites {
		rules = append(rules, site.match)
	}
	return rules
}

// Preserve block rules for personal destinations even if a profile evaluates
// its direct/proxy rules first. Other traffic keeps the profile's own order.
func (cfg EngineConfig) personalProfileBlocks(sites []SBRouteRule) []SBRouteRule {
	if cfg.RoutingMode == ModeSmart {
		return nil
	}
	var blocks []string
	for _, list := range cfg.RoutingLists {
		if list.Action == "block" && routingListCacheReady(list.Path) {
			blocks = append(blocks, list.Tag)
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	var rules []SBRouteRule
	if len(appFamilyVPNList(cfg)) > 0 {
		sites = append(slices.Clone(sites), SBRouteRule{PackageName: []string{familyTagVPN}})
	}
	for _, site := range sites {
		site.Action, site.Outbound = "", ""
		rules = append(rules, SBRouteRule{
			Type: "logical", Mode: "and", Action: "reject",
			Rules: []SBRouteRule{site, {RuleSet: blocks}},
		})
	}
	return rules
}

// If any VPN exception could match an OS bypass, let the engine decide the
// direction. The system proxy cannot express an exception to its bypass list.
func (cfg EngineConfig) systemProxyBypass() []string {
	if cfg.RoutingMode == ModeSmart || len(cfg.PersonalRouting.GlobalVPNSites) > 0 {
		return nil
	}
	for _, list := range cfg.RoutingLists {
		if list.Action == "block" && routingListCacheReady(list.Path) {
			return nil
		}
	}
	return cfg.Whitelist
}
