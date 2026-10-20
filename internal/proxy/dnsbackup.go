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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sagernet/sing-box/adapter"
	sbdns "github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/logger"
)

const (
	configuredDNSCheckDelay   = 3 * time.Second
	configuredDNSCheckTimeout = 3 * time.Second
)

// watchConfiguredDNS says once per session that the configured resolvers are
// silent and the public backups are answering instead. Lookups fall back on
// their own; this only makes it visible in the log.
func watchConfiguredDNS(boxCtx context.Context, cfg EngineConfig, log *logger.Logger) {
	if cfg.Mode != ProxyModeTunnel || log == nil || boxCtx == nil {
		return
	}
	resolvers, backups := tunnelDNSPlan(cfg)
	if len(backups) == 0 {
		return
	}
	tag := configuredDNSGroupTag(resolvers)
	go func() {
		for attempt := 0; attempt < 2; attempt++ {
			select {
			case <-boxCtx.Done():
				return
			case <-time.After(configuredDNSCheckDelay):
			}
			if configuredDNSAnswers(boxCtx, tag) {
				return
			}
		}
		if boxCtx.Err() != nil {
			return
		}
		log.Warning(fmt.Sprintf("[DNS] DNS из конфига (%s) не отвечает через узел — имена резолвит запасной (%s)",
			tunnelResolverAddrs(resolvers), tunnelResolverAddrs(backups)))
	}()
}

// configuredDNSAnswers asks the configured group alone for a name no cache
// holds. Any rcode, NXDOMAIN included, means the resolver is alive.
func configuredDNSAnswers(boxCtx context.Context, tag string) bool {
	manager := service.FromContext[adapter.DNSTransportManager](boxCtx)
	router := service.FromContext[adapter.DNSRouter](boxCtx)
	if manager == nil || router == nil {
		return true
	}
	transport, ok := manager.Transport(tag)
	if !ok {
		return true
	}
	ctx, cancel := context.WithTimeout(boxCtx, configuredDNSCheckTimeout)
	defer cancel()
	name := fmt.Sprintf("rv%d.dnscheck.google.com", time.Now().UnixNano())
	_, err := router.Lookup(ctx, name, adapter.DNSQueryOptions{Transport: transport})
	var rcode sbdns.RcodeError
	return err == nil || errors.As(err, &rcode)
}

func tunnelResolverAddrs(resolvers []tunnelResolver) string {
	addrs := make([]string, 0, len(resolvers))
	for _, r := range resolvers {
		addrs = append(addrs, r.server)
	}
	return strings.Join(addrs, ", ")
}
