package proxy

func socks5ExitListActive(cfg EngineConfig) bool {
	return cfg.Socks5Exit.IsActive() && cfg.Socks5Exit.HasLists()
}

func socks5ExitApps(cfg EngineConfig) []string {
	if cfg.Mode != ProxyModeTunnel || !cfg.Socks5Exit.IsActive() {
		return nil
	}
	return cfg.Socks5Exit.Apps
}

func socks5ExitSiteMatchers(cfg EngineConfig) []SBRouteRule {
	if !socks5ExitListActive(cfg) {
		return nil
	}
	var domains, cidrs []string
	for _, value := range cfg.Socks5Exit.Sites {
		if prefixes := normalizeCIDRs([]string{value}); len(prefixes) > 0 {
			cidrs = append(cidrs, prefixes...)
		} else if domain := normalizeRule(value); domain != "" {
			domains = append(domains, domain)
		}
	}
	var out []SBRouteRule
	if len(domains) > 0 {
		out = append(out, SBRouteRule{DomainSuffix: domains})
	}
	if len(cidrs) > 0 {
		out = append(out, SBRouteRule{IPCidr: cidrs})
	}
	if len(socks5ExitApps(cfg)) > 0 {
		out = append(out, SBRouteRule{PackageName: []string{familyTagSocks}})
	}
	return out
}

func socks5ExitListRules(cfg EngineConfig) []SBRouteRule {
	rules := socks5ExitSiteMatchers(cfg)
	for i := range rules {
		rules[i].Action, rules[i].Outbound = "route", socks5ExitSelector
	}
	return rules
}

func socks5ExitDNSRules(cfg EngineConfig, server string) []SBDNSRule {
	var rules []SBDNSRule
	for _, match := range socks5ExitSiteMatchers(cfg) {
		switch {
		case len(match.DomainSuffix) > 0:
			rules = append(rules, SBDNSRule{DomainSuffix: match.DomainSuffix, Server: server})
		case len(match.PackageName) > 0:
			rules = append(rules, SBDNSRule{PackageName: match.PackageName, Server: server})
		}
	}
	return rules
}
