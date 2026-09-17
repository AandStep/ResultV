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
	"net/http"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

func pingDispatchEntry(t *testing.T, proxyType string) string {
	t.Helper()
	entry := config.ProxyEntry{
		IP: "203.0.113.7", Port: 443, Type: proxyType,
		Password: "11111111-1111-1111-1111-111111111111",
		Extra:    []byte(`{"security":"tls","sni":"example.com","type":"tcp"}`),
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Тип решает, чем мерить, и метод HTTP обязан соответствовать имени типа —
// иначе настройка «HEAD» тихо скачивала бы тело.
func TestPingNodeDispatchesHTTPTypes(t *testing.T) {
	orig := pingThroughNodeProbe
	defer func() { pingThroughNodeProbe = orig }()

	var gotMethod, gotURL string
	pingThroughNodeProbe = func(_ context.Context, _ ProxyConfig, method, testURL string) (int64, bool, string) {
		gotMethod, gotURL = method, testURL
		return 42, true, ""
	}

	for _, tc := range []struct {
		pingType   string
		wantMethod string
	}{
		{config.PingTypeHTTPGet, http.MethodGet},
		{config.PingTypeHTTPHead, http.MethodHead},
	} {
		ms, ok, reason, checkType := PingNode(
			pingDispatchEntry(t, "VLESS"),
			PingOptions{Type: tc.pingType, TestURL: config.DefaultPingTestURL, Timeout: time.Second},
			true,
		)
		if !ok || ms != 42 || reason != "" {
			t.Errorf("%s: (%d, %v, %q)", tc.pingType, ms, ok, reason)
		}
		if checkType != tc.pingType {
			t.Errorf("%s: checkType = %q", tc.pingType, checkType)
		}
		if gotMethod != tc.wantMethod {
			t.Errorf("%s: метод = %q, ожидался %q", tc.pingType, gotMethod, tc.wantMethod)
		}
		if gotURL != config.DefaultPingTestURL {
			t.Errorf("%s: адрес = %q", tc.pingType, gotURL)
		}
	}
}

// Негодный тестовый адрес до движка доходить не должен: поднимать sing-box,
// чтобы узнать то, что видно из строки, — чистая трата телефона.
func TestPingNodeRejectsBadTestURLWithoutEngine(t *testing.T) {
	orig := pingThroughNodeProbe
	defer func() { pingThroughNodeProbe = orig }()
	called := false
	pingThroughNodeProbe = func(context.Context, ProxyConfig, string, string) (int64, bool, string) {
		called = true
		return 0, true, ""
	}

	_, ok, reason, _ := PingNode(
		pingDispatchEntry(t, "VLESS"),
		PingOptions{Type: config.PingTypeHTTPGet, TestURL: "http://insecure", Timeout: time.Second},
		true,
	)
	if ok || reason != "bad_test_url" {
		t.Errorf("получено (%v, %q)", ok, reason)
	}
	if called {
		t.Error("движок поднимался ради заведомо негодного адреса")
	}
}

// ICMP мерит путь и потому применим к любому протоколу; отказ называется
// своим именем, а не «таймаутом» — иначе человек пойдёт искать вину узла.
func TestPingNodeICMPTypeAndItsRefusal(t *testing.T) {
	orig := pingICMPProbe
	defer func() { pingICMPProbe = orig }()

	var gotTimeout time.Duration
	pingICMPProbe = func(_, _ string, timeout time.Duration) (int64, bool) {
		gotTimeout = timeout
		return 17, true
	}
	ms, ok, reason, checkType := PingNode(
		pingDispatchEntry(t, "AMNEZIAWG"),
		PingOptions{Type: config.PingTypeICMP, Timeout: 4 * time.Second},
		true,
	)
	if !ok || ms != 17 || reason != "" || checkType != config.PingTypeICMP {
		t.Errorf("(%d, %v, %q, %q)", ms, ok, reason, checkType)
	}
	if gotTimeout != 4*time.Second {
		t.Errorf("бюджет не доехал: %v", gotTimeout)
	}

	pingICMPProbe = func(string, string, time.Duration) (int64, bool) { return 0, false }
	if _, ok, reason, _ := PingNode(
		pingDispatchEntry(t, "VLESS"),
		PingOptions{Type: config.PingTypeICMP, Timeout: time.Second},
		true,
	); ok || reason != "icmp_unavailable" {
		t.Errorf("отказ ICMP = (%v, %q)", ok, reason)
	}
}

// «Авто» — это сегодняшнее поведение: проба выбирается по протоколу.
func TestPingNodeAutoKeepsProtocolProbes(t *testing.T) {
	origTCP, origH2 := pingTCPProbe, pingHysteria2Probe
	defer func() { pingTCPProbe, pingHysteria2Probe = origTCP, origH2 }()
	pingTCPProbe = func(string, int) (int64, bool, string) { return 11, true, "" }
	pingHysteria2Probe = func(string, int) (int64, bool, string, string) { return 22, true, "", "quic" }

	if ms, _, _, checkType := PingNode(pingDispatchEntry(t, "VLESS"), PingOptions{}, true); ms != 11 || checkType != "tcp" {
		t.Errorf("VLESS: %d, %q", ms, checkType)
	}
	if ms, _, _, checkType := PingNode(pingDispatchEntry(t, "HYSTERIA2"), PingOptions{}, true); ms != 22 || checkType != "quic" {
		t.Errorf("HYSTERIA2: %d, %q", ms, checkType)
	}
	// Неизвестный тип читается как «авто», а не как ошибка: настройка могла
	// прийти из более новой сборки.
	if ms, _, _, _ := PingNode(pingDispatchEntry(t, "VLESS"), PingOptions{Type: "нечто"}, true); ms != 11 {
		t.Errorf("неизвестный тип не упал в авто: %d", ms)
	}
}

// Гейт keyed-пробы WG остаётся за вызывающим: при поднятом туннеле она била бы
// по живой сессии.
func TestPingNodeKeyedWGGate(t *testing.T) {
	orig := pingWireGuardProbe
	defer func() { pingWireGuardProbe = orig }()
	pingWireGuardProbe = func(string, int) (int64, bool, string) { return 33, true, "" }

	ms, ok, _, checkType := PingNode(pingDispatchEntry(t, "AMNEZIAWG"), PingOptions{}, false)
	if !ok || ms != 33 || checkType != "wg_liveness" {
		t.Errorf("при запрещённой keyed-пробе получено (%d, %v, %q)", ms, ok, checkType)
	}
}

// Очередь за движком не должна считаться частью замера. Иначе на списке из
// тридцати узлов двенадцать из шестнадцати работников получают «Таймаут», ни
// разу не сходив в сеть, — ровно это и случилось на телефоне, пока место в
// семафоре бралось внутри бюджета.
func TestPingNodeDoesNotSpendBudgetQueueing(t *testing.T) {
	origProbe := pingThroughNodeProbe
	defer func() { pingThroughNodeProbe = origProbe }()
	pingThroughNodeProbe = func(context.Context, ProxyConfig, string, string) (int64, bool, string) {
		return 55, true, ""
	}

	// Занять все места и освободить одно позже, чем «бюджет» одного замера.
	for i := 0; i < pingEngineMaxConcurrency; i++ {
		pingEngineSem <- struct{}{}
	}
	freed := make(chan struct{})
	go func() {
		time.Sleep(250 * time.Millisecond)
		<-pingEngineSem
		close(freed)
	}()
	defer func() {
		<-freed
		for len(pingEngineSem) > 0 {
			<-pingEngineSem
		}
	}()

	ms, ok, reason, _ := PingNode(
		pingDispatchEntry(t, "VLESS"),
		PingOptions{Type: config.PingTypeHTTPGet, TestURL: config.DefaultPingTestURL, Timeout: 100 * time.Millisecond},
		true,
	)
	if !ok || ms != 55 || reason != "" {
		t.Errorf("ожидание очереди съело бюджет: (%d, %v, %q)", ms, ok, reason)
	}
}
