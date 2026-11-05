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
	coresocks "github.com/sagernet/sing-box/protocol/socks"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/config"
)

const (
	socks5ExitOutboundType = "resultv-socks5-exit"
	socks5ExitSelectorType = "resultv-socks5-exit-selector"
	socks5ExitTag          = "socks5-exit"
	socks5ExitSelector     = "socks5-exit-switch"
)

var errSocks5ExitUnsupported = errors.New("SOCKS5 exit requires a global VLESS tunnel")

func supportsSocks5Exit(profile *config.Socks5ExitConfig, mode ProxyMode, routing RoutingMode, nodeType string) bool {
	return profile != nil && mode == ProxyModeTunnel && routing == ModeGlobal && strings.EqualFold(strings.TrimSpace(nodeType), "vless")
}

func addSocks5Exit(sb *SingBoxConfig, cfg EngineConfig) error {
	if !supportsSocks5Exit(cfg.Socks5Exit, cfg.Mode, cfg.RoutingMode, cfg.Proxy.Type) {
		if cfg.Socks5ExitEnabled {
			return errSocks5ExitUnsupported
		}
		return nil
	}
	p := cfg.Socks5Exit
	exit := SBOutbound{
		Type: socks5ExitOutboundType, Tag: socks5ExitTag, Version: "5", Network: "tcp", Detour: "proxy",
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

func registerSocks5ExitOutbounds(registry *outbound.Registry) {
	outbound.Register[option.SelectorOutboundOptions](registry, socks5ExitSelectorType, newSocks5ExitSelector)
	outbound.Register[option.SOCKSOutboundOptions](registry, socks5ExitOutboundType, newSocks5ExitOutbound)
}

func newSocks5ExitOutbound(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SOCKSOutboundOptions) (adapter.Outbound, error) {
	out, err := coresocks.NewOutbound(ctx, router, logger, tag, options)
	if err != nil {
		return nil, err
	}
	return &tcpOnlySocks5Exit{Outbound: out}, nil
}

// Network filtering in the router can race with selecting a new exit. Reject
// UDP again at the SOCKS5 dial boundary so it never gets an ASSOCIATE request.
type tcpOnlySocks5Exit struct{ adapter.Outbound }

func (o *tcpOnlySocks5Exit) DialContext(ctx context.Context, network string, target M.Socksaddr) (net.Conn, error) {
	if N.NetworkName(network) != N.NetworkTCP {
		return nil, N.ErrUnknownNetwork
	}
	return o.Outbound.DialContext(ctx, network, target)
}

func (o *tcpOnlySocks5Exit) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, N.ErrUnknownNetwork
}

func newSocks5ExitSelector(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SelectorOutboundOptions) (adapter.Outbound, error) {
	// The manager chooses the initial exit for each session. The stock selector
	// gives cached choices priority over Default, even before the TUN can start.
	// Keep the shared cache available to DNS and other outbounds.
	ctx = service.ContextWith[adapter.CacheFile](service.ExtendContext(ctx), nil)
	return group.NewSelector(ctx, router, logger, tag, options)
}

func (e *SingBoxEngine) SetSocks5ExitEnabled(enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.instance == nil || !e.running.Load() {
		return errSocks5ExitUnsupported
	}
	out, _ := e.instance.Outbound().Outbound(socks5ExitSelector)
	selector, ok := out.(*group.Selector)
	if !ok {
		return errSocks5ExitUnsupported
	}
	tag := "proxy"
	if enabled {
		tag = socks5ExitTag
	}
	if !selector.SelectOutbound(tag) {
		return errors.New("SOCKS5 selector could not apply the requested exit")
	}
	e.savedCfg.Socks5ExitEnabled = enabled
	return nil
}
