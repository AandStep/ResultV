// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"resultproxy-wails/internal/logger"
)

type stubOutbound struct {
	adapter.Outbound
	tag string
}

func (s stubOutbound) Tag() string  { return s.tag }
func (s stubOutbound) Type() string { return s.tag }

func trackerForTest() *trafficTracker {
	return &trafficTracker{
		upload:   new(atomic.Int64),
		download: new(atomic.Int64),
		log:      logger.New(),
	}
}

// Узел WireGuard в туннельном режиме на 1.14 вообще не доходит до
// RoutedConnection: TUN спрашивает роутер о каждом новом потоке, эндпоинт
// отвечает PreMatchFlow для любой сети и реализует tun.Port, поэтому пакеты
// идут L3 и никогда не становятся net.Conn. Без RoutedFlow индикатор скорости
// и traffic veto в KillSwitchWatchdog видят ноль всю сессию.
func TestRoutedFlowCountsBytesForForwardedFlows(t *testing.T) {
	tr := trackerForTest()
	flow := tr.RoutedFlow(context.Background(), adapter.InboundContext{
		Network:     N.NetworkTCP,
		Source:      M.ParseSocksaddr("10.0.0.2:51820"),
		Destination: M.ParseSocksaddr("203.0.113.7:443"),
	}, nil, stubOutbound{tag: "proxy"})
	if flow == nil {
		t.Fatal("RoutedFlow вернул nil для TCP-потока — сессия WireGuard была бы невидима для индикатора скорости")
	}
	flow.CountForward(1500)
	flow.CountReverse(9000)
	if got := tr.upload.Load(); got != 1500 {
		t.Fatalf("upload = %d, ожидалось 1500", got)
	}
	if got := tr.download.Load(); got != 9000 {
		t.Fatalf("download = %d, ожидалось 9000", got)
	}
}

// ICMP — единственный поток, о котором ядро спрашивает и на обычном узле:
// `direct` реализует FlowOutbound исключительно ради него. В эти счётчики
// пинги не попадали и до 1.14, и начинать не должны.
func TestRoutedFlowIgnoresICMP(t *testing.T) {
	tr := trackerForTest()
	if flow := tr.RoutedFlow(context.Background(), adapter.InboundContext{
		Network:     N.NetworkICMP,
		Source:      M.ParseSocksaddr("10.0.0.2:0"),
		Destination: M.ParseSocksaddr("1.1.1.1:0"),
	}, nil, stubOutbound{tag: "direct"}); flow != nil {
		t.Fatal("RoutedFlow не должен участвовать в учёте ICMP")
	}
}
