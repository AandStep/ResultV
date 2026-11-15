package proxy

import (
	"context"
	"net"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/dialer"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/protocol/socks/socks5"

	"resultproxy-wails/internal/config"
)

// UDP stays refused until a check of the current profile has seen a reply:
// most SOCKS5 servers sold as residential exits only relay TCP.
type socks5ExitSlot struct {
	outbound.Adapter
	configure func(*config.Socks5ExitConfig) (*socks.Client, error)

	mu     sync.RWMutex
	client *socks.Client
	gen    uint64
	udp    bool
}

func newSocks5ExitOutbound(ctx context.Context, _ adapter.Router, _ log.ContextLogger, tag string, options option.SOCKSOutboundOptions) (adapter.Outbound, error) {
	o := &socks5ExitSlot{
		Adapter: outbound.NewAdapterWithDialerOptions("socks", tag, options.Network.Build(), options.DialerOptions),
	}
	initial := config.Socks5ExitConfig{
		Host: options.Server, Port: int(options.ServerPort), Username: options.Username, Password: options.Password,
	}
	options.Server, options.ServerPort, options.Username, options.Password = "", 0, "", ""
	o.configure = func(profile *config.Socks5ExitConfig) (*socks.Client, error) {
		if profile == nil || profile.Host == "" {
			return nil, nil
		}
		next := options
		next.Server, next.ServerPort = profile.Host, uint16(profile.Port)
		upstream, err := dialer.New(ctx, next.DialerOptions, next.ServerIsDomain())
		if err != nil {
			return nil, err
		}
		method := byte(socks5.AuthTypeNotRequired)
		if profile.Username != "" {
			method = socks5.AuthTypeUsernamePassword
		}
		server := next.ServerOptions.Build()
		return socks.NewClient(socks5MethodDialer{Dialer: upstream, method: method, server: server}, server,
			socks.Version5, profile.Username, profile.Password), nil
	}
	err := o.setProfile(&initial)
	return o, err
}

func (o *socks5ExitSlot) setProfile(profile *config.Socks5ExitConfig) error {
	client, err := o.configure(profile)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.client, o.udp = client, false
	o.gen++
	o.mu.Unlock()
	return nil
}

func (o *socks5ExitSlot) current() (*socks.Client, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.client, o.udp
}

func (o *socks5ExitSlot) generation() uint64 {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.gen
}

func (o *socks5ExitSlot) confirmUDP(gen uint64, ok bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.gen == gen {
		o.udp = ok
	}
}

func (o *socks5ExitSlot) Network() []string {
	if _, udp := o.current(); udp {
		return []string{N.NetworkTCP, N.NetworkUDP}
	}
	return []string{N.NetworkTCP}
}

func (o *socks5ExitSlot) DialContext(ctx context.Context, network string, target M.Socksaddr) (net.Conn, error) {
	client, udp := o.current()
	switch N.NetworkName(network) {
	case N.NetworkTCP:
		if meta := adapter.ContextFrom(ctx); meta != nil && target.IsIP() && M.IsDomainName(meta.Domain) {
			target = M.Socksaddr{Fqdn: meta.Domain, Port: target.Port}
		}
	case N.NetworkUDP:
		if !udp {
			return nil, N.ErrUnknownNetwork
		}
	default:
		return nil, N.ErrUnknownNetwork
	}
	if client == nil {
		return nil, errSocks5ExitUnavailable
	}
	ctx, metadata := adapter.ExtendContext(ctx)
	metadata.Outbound, metadata.Destination = o.Tag(), target
	return client.DialContext(ctx, network, target)
}

func (o *socks5ExitSlot) ListenPacket(ctx context.Context, target M.Socksaddr) (net.PacketConn, error) {
	if _, udp := o.current(); !udp {
		return nil, N.ErrUnknownNetwork
	}
	return o.probeListenPacket(ctx, target)
}

func (o *socks5ExitSlot) probeDial(ctx context.Context, target M.Socksaddr) (net.Conn, error) {
	client, _ := o.current()
	if client == nil {
		return nil, errSocks5ExitUnavailable
	}
	return client.DialContext(ctx, N.NetworkTCP, target)
}

func (o *socks5ExitSlot) probeListenPacket(ctx context.Context, target M.Socksaddr) (net.PacketConn, error) {
	client, _ := o.current()
	if client == nil {
		return nil, errSocks5ExitUnavailable
	}
	return client.ListenPacket(ctx, target)
}
