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
	"testing"
)

func TestValidateEngineConfig_WireGuardMissingKey(t *testing.T) {
	extra := map[string]interface{}{
		"public_key":  "pub",
		"address":     []string{"10.0.0.2/32"},
		"allowed_ips": []string{"0.0.0.0/0"},
	}
	raw, _ := json.Marshal(extra)
	code, err := validateEngineConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			Type:  "WIREGUARD",
			IP:    "127.0.0.1",
			Port:  51820,
			Extra: raw,
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if code != ConnectErrorInvalidConfig {
		t.Fatalf("unexpected code: %s", code)
	}
}

func TestValidateEngineConfig_Hysteria2RequiresPassword(t *testing.T) {
	raw := json.RawMessage(`{"sni":"example.com"}`)
	code, err := validateEngineConfig(EngineConfig{
		Mode: ProxyModeProxy,
		Proxy: ProxyConfig{
			Type:  "HYSTERIA2",
			IP:    "example.com",
			Port:  443,
			Extra: raw,
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if code != ConnectErrorInvalidConfig {
		t.Fatalf("unexpected code: %s", code)
	}
}

func TestValidateEngineConfig_WireGuardBareLocalIPv4Normalized(t *testing.T) {
	extra := map[string]interface{}{
		"private_key": "priv",
		"public_key":  "pub",
		"address":     []string{"172.16.0.2"},
		"allowed_ips": []string{"0.0.0.0/0"},
	}
	raw, _ := json.Marshal(extra)
	code, err := validateEngineConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			Type:  "WIREGUARD",
			IP:    "127.0.0.1",
			Port:  51820,
			Extra: raw,
		},
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if code != "" {
		t.Fatalf("unexpected code: %s", code)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "WIREGUARD", IP: "127.0.0.1", Port: 51820, Extra: raw},
	})
	if len(cfg.Endpoints) != 1 || len(cfg.Endpoints[0].Address) != 1 || cfg.Endpoints[0].Address[0] != "172.16.0.2/32" {
		t.Fatalf("endpoint address: %+v", cfg.Endpoints)
	}
}

func TestValidateEngineConfig_WireGuardInvalidLocalAddress(t *testing.T) {
	extra := map[string]interface{}{
		"private_key": "priv",
		"public_key":  "pub",
		"address":     []string{"not-an-ip"},
		"allowed_ips": []string{"0.0.0.0/0"},
	}
	raw, _ := json.Marshal(extra)
	code, err := validateEngineConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			Type:  "WIREGUARD",
			IP:    "127.0.0.1",
			Port:  51820,
			Extra: raw,
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if code != ConnectErrorInvalidConfig {
		t.Fatalf("unexpected code: %s", code)
	}
}

func TestValidateEngineConfig_WireGuardValidConfig(t *testing.T) {
	extra := map[string]interface{}{
		"private_key": "priv",
		"public_key":  "pub",
		"address":     []string{"10.0.0.2/32"},
		"allowed_ips": []string{"0.0.0.0/0"},
	}
	raw, _ := json.Marshal(extra)
	code, err := validateEngineConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			Type:  "WIREGUARD",
			IP:    "127.0.0.1",
			Port:  51820,
			Extra: raw,
		},
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if code != "" {
		t.Fatalf("unexpected code: %s", code)
	}
}

// Заголовок авто-группы — не сервер, и подключаться к нему нечем.
//
// Регрессия по логам 2026-09-05: подбор не нашёл ни одного узла, вызывающий
// откатился на саму AUTO-запись, у неё нет ни адреса, ни порта, ни протокола —
// и buildProxyOutbound собрал из неё http-аутбаунд с пустым сервером. Ядро
// поднялось, приложение отрапортовало «Подключено (AUTO)», кнопка загорелась
// зелёным, а каждое соединение падало с «invalid address». Отказ обязан быть
// отказом, а не зелёной кнопкой поверх мёртвого туннеля.
func TestValidateEngineConfig_RejectsGroupHeadAndAddresslessEntry(t *testing.T) {
	cases := []struct {
		name  string
		proxy ProxyConfig
	}{
		{"заголовок авто-группы", ProxyConfig{Type: "AUTO", IP: "", Port: 0}},
		{"заголовок авто-группы с адресом", ProxyConfig{Type: "AUTO", IP: "1.2.3.4", Port: 443}},
		{"разделитель подписки", ProxyConfig{Type: "SECTION", IP: "", Port: 0}},
		{"узел без адреса", ProxyConfig{Type: "VLESS", IP: "", Port: 443}},
		{"узел без порта", ProxyConfig{Type: "VLESS", IP: "1.2.3.4", Port: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, err := validateEngineConfig(EngineConfig{Mode: ProxyModeProxy, Proxy: tc.proxy})
			if err == nil {
				t.Fatal("ожидали отказ валидации, а не рабочую конфигурацию")
			}
			if code != ConnectErrorInvalidConfig {
				t.Fatalf("ожидали код %s, получили %s", ConnectErrorInvalidConfig, code)
			}
		})
	}
}
