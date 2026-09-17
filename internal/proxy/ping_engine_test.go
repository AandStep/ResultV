// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"errors"
	"testing"
)

func pingProbeNode() ProxyConfig {
	return ProxyConfig{
		Type: "VLESS", IP: "203.0.113.7", Port: 443,
		Password: "11111111-1111-1111-1111-111111111111",
		Extra:    []byte(`{"security":"tls","sni":"example.com","type":"tcp"}`),
	}
}

// Конфиг пробы — это петлевой инбаунд, узел и ничего больше: всё лишнее в нём
// мерилось бы вместе с узлом.
func TestPingProbeConfigShape(t *testing.T) {
	cfg, err := BuildPingProbeConfig(pingProbeNode(), 34567)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatalf("инбаундов %d, ожидался один", len(cfg.Inbounds))
	}
	in := cfg.Inbounds[0]
	if in.Type != "mixed" || in.Listen != "127.0.0.1" || in.ListenPort != 34567 {
		t.Errorf("инбаунд = %+v", in)
	}
	if in.Tag != pingProbeInboundTag {
		t.Errorf("тег инбаунда = %q", in.Tag)
	}
	if cfg.Route == nil || cfg.Route.Final != "proxy" {
		t.Errorf("route.final = %+v, ожидался proxy", cfg.Route)
	}
	if cfg.Log == nil || cfg.Log.Level != "error" {
		t.Errorf("уровень лога = %+v: обход списка поднимает десятки таких движков", cfg.Log)
	}
	var hasProxy bool
	for _, o := range cfg.Outbounds {
		if o.Tag == "proxy" {
			hasProxy = true
		}
	}
	if !hasProxy {
		t.Error("в конфиге нет аутбаунда proxy")
	}
	assertCoreAcceptsConfig(t, cfg)
}

// WireGuard и AmneziaWG аутбаунда не имеют вовсе — они эндпоинты, и вести
// через них HTTP нечем. Отказ должен быть назван, а не превращён в таймаут.
func TestPingProbeConfigRefusesWireGuard(t *testing.T) {
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		node := ProxyConfig{Type: pt, IP: "203.0.113.7", Port: 51820,
			Extra: []byte(`{"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",` +
				`"public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",` +
				`"address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"]}`)}
		if _, err := BuildPingProbeConfig(node, 34567); !errors.Is(err, errPingProbeUnsupported) {
			t.Errorf("%s: ожидался errPingProbeUnsupported, получено %v", pt, err)
		}
	}
}

// Узел, адресованный именем, нуждается в резолвере внутри пробы — иначе
// аутбаунд не знает, куда идти.
func TestPingProbeConfigResolvesNamedNode(t *testing.T) {
	node := pingProbeNode()
	node.IP = "node.example.com"
	cfg, err := BuildPingProbeConfig(node, 34567)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DNS == nil || len(cfg.DNS.Servers) == 0 {
		t.Fatal("у узла с именем должен быть DNS-блок")
	}
	var resolverTagged bool
	for _, o := range cfg.Outbounds {
		if o.Tag == "proxy" && o.DomainResolver != "" {
			resolverTagged = true
		}
	}
	if !resolverTagged {
		t.Error("аутбаунду узла-имени не назван резолвер")
	}
	assertCoreAcceptsConfig(t, cfg)
}

// У узла с литеральным адресом резолвить нечего, и лишний DNS-блок был бы
// ещё одной движущейся частью в замере.
func TestPingProbeConfigSkipsDNSForLiteralNode(t *testing.T) {
	cfg, err := BuildPingProbeConfig(pingProbeNode(), 34567)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DNS != nil {
		t.Errorf("у узла с литеральным адресом DNS-блок лишний: %+v", cfg.DNS)
	}
}
