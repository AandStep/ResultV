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
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ConnectErrorInvalidConfig = "invalid_config"
	ConnectErrorTunPrivileges = "tun_privileges"
	ConnectErrorEngineStart   = "engine_start_failed"
)

func validateEngineConfig(cfg EngineConfig) (string, error) {
	sb, err := BuildProxyModeConfig(cfg)
	if err != nil {
		return ConnectErrorInvalidConfig, err
	}
	if cfg.Mode == ProxyModeTunnel {
		sb, err = BuildTunnelModeConfig(cfg)
		if err != nil {
			return ConnectErrorInvalidConfig, err
		}
	}

	if err := validateRouteFinalTarget(sb); err != nil {
		return ConnectErrorInvalidConfig, err
	}
	if err := validateDNSConfig(cfg, sb); err != nil {
		return ConnectErrorInvalidConfig, err
	}
	if err := validateProtocolRequiredFields(cfg.Proxy); err != nil {
		return ConnectErrorInvalidConfig, err
	}
	return "", nil
}

func validateRouteFinalTarget(cfg SingBoxConfig) error {
	if cfg.Route == nil {
		return fmt.Errorf("route section is missing")
	}
	final := strings.TrimSpace(cfg.Route.Final)
	if final == "" {
		return fmt.Errorf("route final target is empty")
	}
	for _, o := range cfg.Outbounds {
		if strings.EqualFold(strings.TrimSpace(o.Tag), final) {
			return nil
		}
	}
	for _, e := range cfg.Endpoints {
		if strings.EqualFold(strings.TrimSpace(e.Tag), final) {
			return nil
		}
	}
	return fmt.Errorf("route final target %q is not defined in outbounds or endpoints", final)
}

func validateDNSConfig(engineCfg EngineConfig, cfg SingBoxConfig) error {
	if engineCfg.Mode != ProxyModeTunnel {
		return nil
	}
	if cfg.DNS == nil || len(cfg.DNS.Servers) == 0 {
		return fmt.Errorf("dns servers are not configured for tunnel mode")
	}
	hasHijack := false
	if cfg.Route == nil {
		return fmt.Errorf("route section is missing")
	}
	for _, r := range cfg.Route.Rules {
		if r.Action == "hijack-dns" {
			hasHijack = true
			break
		}
	}
	if !hasHijack {
		return fmt.Errorf("tunnel mode requires hijack-dns route rule")
	}
	return nil
}

func validateProtocolRequiredFields(proxyCfg ProxyConfig) error {
	pt := strings.ToUpper(strings.TrimSpace(proxyCfg.Type))
	extra := parseExtra(proxyCfg)
	switch pt {
	case "WIREGUARD", "AMNEZIAWG":
		if strings.TrimSpace(getStringField(extra, "private_key", getStringField(extra, "privateKey", ""))) == "" {
			return fmt.Errorf("%s requires private_key", strings.ToLower(pt))
		}
		if strings.TrimSpace(getStringField(extra, "public_key", getStringField(extra, "publicKey", ""))) == "" {
			return fmt.Errorf("%s requires public_key", strings.ToLower(pt))
		}
		addrs := stringListFromExtra(extra, "address", "local_address", "localAddress")
		if len(addrs) == 0 {
			return fmt.Errorf("%s requires address", strings.ToLower(pt))
		}
		if _, err := normalizeWireGuardLocalPrefixes(addrs); err != nil {
			return fmt.Errorf("%s address: %w", strings.ToLower(pt), err)
		}
		if len(stringListFromExtra(extra, "allowed_ips", "allowedIps")) == 0 {
			return fmt.Errorf("%s requires allowed_ips", strings.ToLower(pt))
		}
	case "HYSTERIA2":
		if strings.TrimSpace(getStringField(extra, "password", strings.TrimSpace(proxyCfg.Password))) == "" {
			return fmt.Errorf("hysteria2 requires password")
		}
	case "NAIVEPROXY", "NAIVE":
		if strings.TrimSpace(proxyCfg.IP) == "" || proxyCfg.Port <= 0 {
			return fmt.Errorf("naiveproxy requires host and port")
		}
		if strings.TrimSpace(proxyCfg.Username) == "" || strings.TrimSpace(proxyCfg.Password) == "" {
			return fmt.Errorf("naiveproxy requires username and password")
		}
		if getBoolField(extra, "insecure") {
			return fmt.Errorf("naiveproxy: sing-box naive outbound does not support insecure TLS; use a publicly trusted certificate or remove insecure")
		}
	}
	if proxyCfg.Extra != nil && len(proxyCfg.Extra) > 0 {
		var js map[string]interface{}
		if err := json.Unmarshal(proxyCfg.Extra, &js); err != nil {
			return fmt.Errorf("invalid extra json: %w", err)
		}
	}
	return nil
}
