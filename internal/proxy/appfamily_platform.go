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
	"context"
	"net/netip"
	"os"
	"sync"
	"syscall"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/process"
	sblog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
	N "github.com/sagernet/sing/common/network"
)

// familyPlatform is how the app families reach the core. The router asks a
// platform interface who owns a connection whenever one is registered and
// claims the job (route/router.go, newPlatformSearcher) — the hook the mobile
// clients use. This one finds the owner exactly as the built-in searcher does
// and adds the family tag; every other platform service stays off, so the core
// behaves as it does with no platform at all.
type familyPlatform struct {
	resolver *familyResolver
}

var _ adapter.PlatformInterface = (*familyPlatform)(nil)

var (
	ownerSearcherOnce sync.Once
	ownerSearcher     process.Searcher
	ownerSearcherErr  error
)

// sharedOwnerSearcher is the core's own searcher, built once: on Linux it holds
// netlink sockets that nothing would close on an engine reload.
func sharedOwnerSearcher() (process.Searcher, error) {
	ownerSearcherOnce.Do(func() {
		ownerSearcher, ownerSearcherErr = process.NewSearcher(process.Config{
			Logger: sblog.NewNOPFactory().Logger(),
		})
	})
	return ownerSearcher, ownerSearcherErr
}

func newFamilyPlatform(resolver *familyResolver) *familyPlatform {
	return &familyPlatform{resolver: resolver}
}

func (p *familyPlatform) UsePlatformConnectionOwnerFinder() bool { return true }

func (p *familyPlatform) FindConnectionOwner(request *adapter.FindConnectionOwnerRequest) (*adapter.ConnectionOwner, error) {
	searcher, err := sharedOwnerSearcher()
	if err != nil {
		return nil, err
	}
	var network string
	switch request.IpProtocol {
	case syscall.IPPROTO_TCP:
		network = N.NetworkTCP
	case syscall.IPPROTO_UDP:
		network = N.NetworkUDP
	default:
		return nil, process.ErrNotFound
	}
	source, err := requestAddrPort(request.SourceAddress, request.SourcePort)
	if err != nil {
		return nil, err
	}
	destination, _ := requestAddrPort(request.DestinationAddress, request.DestinationPort)

	owner, err := searcher.FindProcessInfo(context.Background(), network, source, destination)
	if owner == nil {
		return nil, err
	}
	family := p.resolver.family(owner.ProcessID, owner.ProcessPath)
	if family != familyNone {
		owner.AndroidPackageNames = []string{family.tag()}
		return owner, nil
	}
	// The searcher reports a PID it could not name as an error; keep that.
	return owner, err
}

func requestAddrPort(address string, port int32) (netip.AddrPort, error) {
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(addr, uint16(port)), nil
}

func (p *familyPlatform) Initialize(adapter.NetworkManager) error                { return nil }
func (p *familyPlatform) UsePlatformAutoDetectInterfaceControl() bool            { return false }
func (p *familyPlatform) AutoDetectInterfaceControl(int) error                   { return os.ErrInvalid }
func (p *familyPlatform) BindInterfaceControl(int, string) error                 { return os.ErrInvalid }
func (p *familyPlatform) UsePlatformInterface() bool                             { return false }
func (p *familyPlatform) ProcessPlatformOptions(option.TunPlatformOptions) error { return nil }
func (p *familyPlatform) UsePlatformDefaultInterfaceMonitor() bool               { return false }
func (p *familyPlatform) UsePlatformNetworkInterfaces() bool                     { return false }
func (p *familyPlatform) UnderNetworkExtension() bool                            { return false }
func (p *familyPlatform) NetworkExtensionIncludeAllNetworks() bool               { return false }
func (p *familyPlatform) ClearDNSCache()                                         {}
func (p *familyPlatform) RequestPermissionForWIFIState() error                   { return os.ErrInvalid }
func (p *familyPlatform) UsePlatformWIFIMonitor() bool                           { return false }
func (p *familyPlatform) UsePlatformNotification() bool                          { return false }
func (p *familyPlatform) MyInterfaceAddress() []netip.Addr                       { return nil }
func (p *familyPlatform) UsePlatformNeighborResolver() bool                      { return false }
func (p *familyPlatform) UsePlatformShell() bool                                 { return false }
func (p *familyPlatform) CheckPlatformShell() error                              { return os.ErrInvalid }
func (p *familyPlatform) LookupSFTPServer() (string, error)                      { return "", os.ErrInvalid }
func (p *familyPlatform) ReadSystemSSHHostKey() ([]byte, error)                  { return nil, os.ErrInvalid }
func (p *familyPlatform) TailscaleHostname() string                              { return "" }
func (p *familyPlatform) UsePlatformBridge() bool                                { return false }
func (p *familyPlatform) CancelNotification(string, int32) error                 { return os.ErrInvalid }

func (p *familyPlatform) OpenInterface(*tun.Options, option.TunPlatformOptions) (tun.Tun, error) {
	return nil, os.ErrInvalid
}

func (p *familyPlatform) CreateDefaultInterfaceMonitor(logger.Logger) tun.DefaultInterfaceMonitor {
	return nil
}

func (p *familyPlatform) NetworkInterfaces() ([]adapter.NetworkInterface, error) {
	return nil, os.ErrInvalid
}

func (p *familyPlatform) ReadWIFIState(context.Context) adapter.WIFIState {
	return adapter.WIFIState{}
}

func (p *familyPlatform) SendNotification(*adapter.Notification) error { return os.ErrInvalid }

func (p *familyPlatform) StartNeighborMonitor(adapter.NeighborUpdateListener) error {
	return os.ErrInvalid
}

func (p *familyPlatform) CloseNeighborMonitor(adapter.NeighborUpdateListener) error {
	return nil
}

func (p *familyPlatform) OpenShellSession(*adapter.PlatformUser, string, []string, string, int32, int32) (adapter.ShellSession, error) {
	return nil, os.ErrInvalid
}

func (p *familyPlatform) LookupUser(string) (*adapter.PlatformUser, error) {
	return nil, os.ErrInvalid
}

func (p *familyPlatform) CreateBridge(adapter.BridgeOptions) (adapter.BridgeSession, error) {
	return nil, os.ErrInvalid
}
