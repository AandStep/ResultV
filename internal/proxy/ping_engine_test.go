// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
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
	cfg, err := BuildPingProbeConfig(pingProbeNode(), 34567, nil)
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
		if _, err := BuildPingProbeConfig(node, 34567, nil); !errors.Is(err, errPingProbeUnsupported) {
			t.Errorf("%s: ожидался errPingProbeUnsupported, получено %v", pt, err)
		}
	}
}

// Узел, адресованный именем, нуждается в резолвере внутри пробы — иначе
// аутбаунд не знает, куда идти.
func TestPingProbeConfigResolvesNamedNode(t *testing.T) {
	node := pingProbeNode()
	node.IP = "node.example.com"
	cfg, err := BuildPingProbeConfig(node, 34567, nil)
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

// Разрешённый адрес узла приходит в движок статической записью: своего
// резолвера у пробы нет — платформенного интерфейса libbox у неё тоже.
func TestPingProbeConfigPinsResolvedNode(t *testing.T) {
	node := pingProbeNode()
	node.IP = "node.example.com"
	cfg, err := BuildPingProbeConfig(node, 34567, []string{"198.51.100.4"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DNS == nil || len(cfg.DNS.Servers) != 2 {
		t.Fatalf("ожидались hosts + local, получено %+v", cfg.DNS)
	}
	hosts := cfg.DNS.Servers[0]
	if hosts.Type != "hosts" || hosts.Predefined["node.example.com"][0] != "198.51.100.4" {
		t.Errorf("статическая запись = %+v", hosts)
	}
	if len(cfg.DNS.Rules) != 1 || cfg.DNS.Rules[0].Server != hosts.Tag {
		t.Errorf("правило не указывает на hosts: %+v", cfg.DNS.Rules)
	}
	// Правил мало: в 1.14 аутбаунд с названным резолвером идёт прямо в него и
	// правил не смотрит вовсе. Пока это был "local", на телефоне каждый
	// замер умирал на ::1:53 «connection refused», не дойдя до узла.
	for _, o := range cfg.Outbounds {
		if o.Tag == "proxy" && o.DomainResolver != hosts.Tag {
			t.Errorf("аутбаунд смотрит в %q, а не в статическую запись", o.DomainResolver)
		}
	}
	assertCoreAcceptsConfig(t, cfg)
}

// У узла с литеральным адресом резолвить нечего, и лишний DNS-блок был бы
// ещё одной движущейся частью в замере.
func TestPingProbeConfigSkipsDNSForLiteralNode(t *testing.T) {
	cfg, err := BuildPingProbeConfig(pingProbeNode(), 34567, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DNS != nil {
		t.Errorf("у узла с литеральным адресом DNS-блок лишний: %+v", cfg.DNS)
	}
}

// Любой статус — успех, и это намеренно: запрос идёт по HTTPS через CONNECT,
// сертификат проверяется внутри процесса, поэтому сам факт ответа доказывает,
// что байты дошли до настоящего хоста. Отдельно стоит 407: это отказ прокси, а
// не ответ сайта.
func TestClassifyPingFetch(t *testing.T) {
	if ok, reason := classifyPingFetch(&http.Response{StatusCode: 204}, nil); !ok || reason != "" {
		t.Errorf("204 = (%v, %q), ожидался успех", ok, reason)
	}
	if ok, _ := classifyPingFetch(&http.Response{StatusCode: 500}, nil); !ok {
		t.Error("500 тоже доказывает, что байты дошли")
	}
	if ok, reason := classifyPingFetch(&http.Response{StatusCode: 407}, nil); ok || reason != "proxy_auth_required" {
		t.Errorf("407 = (%v, %q)", ok, reason)
	}
	if ok, reason := classifyPingFetch(nil, errors.New("i/o timeout")); ok || reason != "timeout" {
		t.Errorf("таймаут = (%v, %q)", ok, reason)
	}
	if ok, reason := classifyPingFetch(nil, nil); ok || reason == "" {
		t.Errorf("пустой ответ без ошибки должен быть назван: (%v, %q)", ok, reason)
	}
}

// Узел без аутбаунда обязан ответить именем причины, а не поднимать движок.
func TestPingThroughNodeRefusesWireGuard(t *testing.T) {
	node := ProxyConfig{Type: "AMNEZIAWG", IP: "203.0.113.7", Port: 51820,
		Extra: []byte(`{"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",` +
			`"public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",` +
			`"address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"]}`)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, ok, reason := pingThroughNode(ctx, node, http.MethodHead, config.DefaultPingTestURL)
	if ok || reason != "unsupported_for_protocol" {
		t.Errorf("получено (%v, %q)", ok, reason)
	}
}

// Негодный тестовый адрес — это ошибка запроса, а не узла.
func TestPingThroughNodeRejectsBadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, ok, reason := pingThroughNode(ctx, pingProbeNode(), http.MethodGet, "https://exa mple.com/ping")
	if ok || reason != "bad_test_url" {
		t.Errorf("получено (%v, %q)", ok, reason)
	}
}

// Потолок одновременных движков — не украшение: список пингуется по 16
// параллельно, а движок дороже сокета на порядки.
func TestPingEngineConcurrencyIsCapped(t *testing.T) {
	if cap(pingEngineSem) != pingEngineMaxConcurrency {
		t.Errorf("ёмкость семафора %d, ожидалась %d", cap(pingEngineSem), pingEngineMaxConcurrency)
	}
	if pingEngineMaxConcurrency >= 16 {
		t.Errorf("потолок %d не ниже параллелизма списка (16) — значит не ограничивает", pingEngineMaxConcurrency)
	}
}

// Свежее ядро рвёт первое hy2-соединение стартовым ResetNetwork («network
// changed» через пару миллисекунд), и без повтора hy2-узел в пинге всегда
// выглядел мёртвым.
func TestFetchPingRetriesInstantFailureOnce(t *testing.T) {
	calls := 0
	ms, ok, reason := fetchPing(context.Background(), func() (time.Duration, bool, string) {
		calls++
		if calls == 1 {
			return 2 * time.Millisecond, false, "connection_closed"
		}
		return 180 * time.Millisecond, true, ""
	})
	if !ok || calls != 2 || ms != 180 {
		t.Fatalf("want retry and second latency: ok=%v calls=%d ms=%d reason=%q", ok, calls, ms, reason)
	}
}

func TestFetchPingGivesUpAfterBudget(t *testing.T) {
	calls := 0
	_, ok, reason := fetchPing(context.Background(), func() (time.Duration, bool, string) {
		calls++
		return 3 * time.Second, false, "timeout"
	})
	if ok || calls != pingAttempts || reason != "timeout" {
		t.Fatalf("dead node: ok=%v calls=%d reason=%q", ok, calls, reason)
	}
}

func TestFetchPingStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, ok, _ := fetchPing(ctx, func() (time.Duration, bool, string) {
		calls++
		cancel()
		return time.Second, false, "timeout"
	})
	if ok || calls != 1 {
		t.Fatalf("no retry past the ping timeout: ok=%v calls=%d", ok, calls)
	}
}

func TestBuildPingProbeConfigShortensHysteria2Handshake(t *testing.T) {
	extra, _ := json.Marshal(map[string]interface{}{"password": "x"})
	cfg, err := BuildPingProbeConfig(ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "hysteria2", Extra: extra}, 14999, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range cfg.Outbounds {
		if o.Tag == "proxy" && (o.TLS == nil || o.TLS.HandshakeTimeout != pingHysteria2HandshakeTimeout) {
			t.Fatalf("ping engine hy2 handshake_timeout = %+v", o.TLS)
		}
	}
}
