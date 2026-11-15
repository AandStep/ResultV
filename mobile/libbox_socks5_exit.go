package mobile

import (
	"encoding/json"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

const socks5ExitTag = "socks5-exit"
const socks5ExitUDPTag = "socks5-exit-udp"
const smartRaceExitInboundPort = 18132
const smartRaceExitInboundTag = "smart-race-socks5-in"
const smartRelayExitOutboundTag = "smart-relay-socks5"

// NormalizeSocks5Exit validates the same profile shape used on desktop.
func NormalizeSocks5Exit(profileJSON string) (string, error) {
	var profile config.Socks5ExitConfig
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return "", err
	}
	normalized, err := config.NormalizeSocks5ExitConfig(profile)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(normalized)
	return string(out), err
}

func applySocks5Exit(sb *proxy.SingBoxConfig, profile *config.Socks5ExitConfig) error {
	if profile == nil || !profile.Enabled || sb.Route == nil {
		return nil
	}
	exit, err := config.NormalizeSocks5ExitConfig(*profile)
	if err != nil {
		return err
	}

	resolver := "local"
	if sb.DNS != nil {
		for _, server := range sb.DNS.Servers {
			if server.Type == "fallback" {
				resolver = server.Tag
				break
			}
		}
	}
	sb.Outbounds = append(sb.Outbounds, proxy.SBOutbound{
		Type: "socks", Tag: socks5ExitTag, Detour: "proxy", Version: "5",
		Server: exit.Host, ServerPort: exit.Port, DomainResolver: resolver,
		Username: exit.Username, Password: exit.Password,
	}, proxy.SBOutbound{
		Type: "selector", Tag: socks5ExitUDPTag, Outbounds: []string{"proxy", socks5ExitTag},
	})

	var matches []proxy.SBRouteRule
	for _, site := range exit.Sites {
		if rule := siteRouteMatcher(site); siteSpecificity(rule) != 0 {
			matches = append(matches, rule)
		}
	}
	if len(exit.Apps) > 0 {
		matches = append(matches, proxy.SBRouteRule{PackageName: exit.Apps})
	}
	if len(matches) == 0 && len(exit.Sites)+len(exit.Apps) > 0 {
		return nil
	}

	var routed []proxy.SBRouteRule
	adaptiveApps := false
	for _, rule := range sb.Route.Rules {
		if rule.Outbound == smartRelayOutboundTag && len(exit.Apps) > 0 {
			adaptiveApps = true
			routed = append(routed, proxy.SBRouteRule{
				Network: []string{"tcp"}, PackageName: exit.Apps,
				Action: "route", Outbound: smartRelayExitOutboundTag,
			})
		}
		if rule.Outbound == "proxy" && rule.OverrideAddress == "" && rule.OverridePort == 0 {
			originalMatch := rule
			originalMatch.Action, originalMatch.Outbound = "", ""
			routed = append(routed, socks5ExitRules(&originalMatch, matches)...)
		}
		routed = append(routed, rule)
	}
	if sb.Route.Final == "proxy" {
		routed = append(routed, socks5ExitRules(nil, matches)...)
	}
	if adaptiveApps {
		sb.Inbounds = append(sb.Inbounds, proxy.SBInbound{
			Type: "mixed", Tag: smartRaceExitInboundTag, Listen: "127.0.0.1", ListenPort: smartRaceExitInboundPort,
		})
		sb.Outbounds = append(sb.Outbounds, proxy.SBOutbound{
			Type: "http", Tag: smartRelayExitOutboundTag, Server: "127.0.0.1", ServerPort: SmartRelayPort,
			Username: proxy.SmartRelayExitUsername, Password: proxy.SmartRelayExitPassword,
		})
		routed = append([]proxy.SBRouteRule{
			{Inbound: []string{smartRaceExitInboundTag}, Action: "route", Outbound: socks5ExitTag},
		}, routed...)
	}
	sb.Route.Rules = routed
	return nil
}

func socks5ExitRules(original *proxy.SBRouteRule, destinations []proxy.SBRouteRule) []proxy.SBRouteRule {
	var rules []proxy.SBRouteRule
	for _, route := range []struct{ network, outbound string }{
		{"tcp", socks5ExitTag},
		{"udp", socks5ExitUDPTag},
	} {
		condition := proxy.SBRouteRule{Network: []string{route.network}}
		parts := []proxy.SBRouteRule{condition}
		if original != nil {
			parts = append(parts, *original)
		}
		if len(destinations) == 1 {
			parts = append(parts, destinations[0])
		} else if len(destinations) > 1 {
			parts = append(parts, proxy.SBRouteRule{Type: "logical", Mode: "or", Rules: destinations})
		}
		result := parts[0]
		if len(parts) > 1 {
			result = proxy.SBRouteRule{Type: "logical", Mode: "and", Rules: parts}
		}
		result.Action, result.Outbound = "route", route.outbound
		rules = append(rules, result)
	}
	return rules
}
