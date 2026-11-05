package proxy

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing/service"
)

const socks5ExitSelectorType = "resultv-socks5-exit-selector"

const (
	socks5ExitTag      = "socks5-exit"
	socks5ExitSelector = "socks5-exit-switch"
)

var errSocks5ExitUnsupported = errors.New("SOCKS5 exit requires a global VLESS tunnel")

func addSocks5Exit(sb *SingBoxConfig, cfg EngineConfig) error {
	if cfg.Socks5Exit == nil || cfg.Mode != ProxyModeTunnel || cfg.RoutingMode != ModeGlobal || !strings.EqualFold(cfg.Proxy.Type, "vless") {
		if cfg.Socks5ExitEnabled {
			return errSocks5ExitUnsupported
		}
		return nil
	}
	p := cfg.Socks5Exit
	exit := SBOutbound{
		Type: "socks", Tag: socks5ExitTag, Version: "5", Network: "tcp", Detour: "proxy",
		Server: p.Host, ServerPort: p.Port, Username: p.Username, Password: p.Password,
	}
	if net.ParseIP(p.Host) == nil {
		exit.DomainResolver = firstDetourServerTag(sb.DNS.Servers, "proxy")
	}
	initial := "proxy"
	if cfg.Socks5ExitEnabled {
		initial = socks5ExitTag
	}
	sb.Outbounds = append(sb.Outbounds, exit, SBOutbound{
		Type: socks5ExitSelectorType, Tag: socks5ExitSelector, Outbounds: []string{"proxy", socks5ExitTag},
		Default: initial, InterruptExistConnections: true,
	})
	// Place this after explicit actions, immediately before the ordinary
	// global route final. The health and update inbounds keep using proxy.
	sb.Route.Rules = append(sb.Route.Rules, SBRouteRule{
		Inbound: []string{"tun-in"}, DomainSuffix: p.Domains, Action: "route", Outbound: socks5ExitSelector,
	})
	return nil
}

func registerSocks5ExitSelector(registry *outbound.Registry) {
	outbound.Register[option.SelectorOutboundOptions](registry, socks5ExitSelectorType, newSocks5ExitSelector)
}

func newSocks5ExitSelector(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SelectorOutboundOptions) (adapter.Outbound, error) {
	// The manager chooses the initial exit for each session. The stock selector
	// gives cached choices priority over Default, even before the TUN can start.
	// Keep the shared cache available to DNS and other outbounds.
	ctx = service.ContextWith[adapter.CacheFile](service.ExtendContext(ctx), nil)
	return group.NewSelector(ctx, router, logger, tag, options)
}
