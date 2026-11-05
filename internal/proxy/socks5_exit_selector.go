package proxy

import (
	"context"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing/service"
)

const socks5ExitSelectorType = "resultv-socks5-exit-selector"

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
