// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

// Маршрутизация подписки как ОДИН профиль.
//
// Раньше подписка приносила набор отдельных списков, по одному на действие.
// Пользователь так о ней не думает: маршрутизация от провайдера и по ссылке —
// одно и то же, набор правил, который либо в силе, либо нет, и в силе ровно
// один такой набор. Тесты ниже держат именно это.

import (
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

func TestSubscriptionRoutingBecomesOneProfile(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)

	provided := []config.RoutingList{
		{Name: "embedded-direct", URL: "embedded:direct", Action: "direct"},
		{Name: "embedded-block", URL: "embedded:block", Action: "block"},
	}
	embedded := map[string]proxy.ParsedRoutingList{
		"direct": {Domains: []string{"gov.test", "nalog.test"}, CIDRs: []string{"10.0.0.0/8"}},
		"block":  {Domains: []string{"ads.test"}},
	}
	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil, embedded, subscriptionRoutingHeader{}, true); err != nil {
		t.Fatalf("sync: %v", err)
	}

	rr := a.config.GetConfig().RoutingRules
	if len(rr.Profiles) != 1 {
		t.Fatalf("подписка должна дать РОВНО один профиль, получилось %d", len(rr.Profiles))
	}
	p := rr.Profiles[0]
	if p.Source != "subscription" || p.SubscriptionID != "sub1" {
		t.Errorf("происхождение профиля потеряно: source=%q sub=%q", p.Source, p.SubscriptionID)
	}
	// Все действия внутри одного профиля, а не по профилю на действие.
	if p.RuleCount("direct") != 3 || p.RuleCount("block") != 1 {
		t.Errorf("правила разъехались: direct=%d block=%d", p.RuleCount("direct"), p.RuleCount("block"))
	}
	if rr.ActiveProfileID != p.ID {
		t.Error("профиль, на который пользователь только что согласился, должен стать активным")
	}
	for _, rl := range rr.RoutingLists {
		if rl.SubscriptionID == "sub1" {
			t.Errorf("подписка снова завела список вместо профиля: %+v", rl)
		}
	}
}

func TestSubscriptionRoutingLinkedListStaysALink(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)
	// Адрес намеренно loopback: SSRF-страж отбивает такой запрос сразу, без
	// разрешения имени и трёх ретраев. Тест про то, что попало в конфиг, а не
	// про закачку — с внешним адресом он стоил бы 36 секунд ожидания.
	provided := []config.RoutingList{
		{Name: "Provider list", URL: "https://127.0.0.1:9/l.lst", Action: "proxy"},
	}
	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil, nil, subscriptionRoutingHeader{}, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	profiles := a.config.GetConfig().RoutingRules.Profiles
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(profiles))
	}
	// Список, на который провайдер только сослался, остаётся ссылкой: втянуть
	// 74k записей в конфиг значило бы раздуть его до неюзабельного.
	got := profiles[0].ListURLs["proxy"]
	if len(got) != 1 || got[0] != "https://127.0.0.1:9/l.lst" {
		t.Errorf("ссылка на список не сохранена: %+v", profiles[0].ListURLs)
	}
	if len(profiles[0].ProxySites) != 0 {
		t.Error("ссылка не должна была развернуться в правила прямо в конфиге")
	}
}

func TestSubscriptionRoutingRespectsTombstonesAndDisabled(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, []string{"https://dead.test/l.lst"})
	provided := []config.RoutingList{
		{URL: "https://dead.test/l.lst", Action: "proxy"},
		{URL: "https://off.test/l.lst", Action: "proxy"},
		{URL: "https://127.0.0.1:9/on.lst", Action: "direct"},
	}
	if err := a.syncSubscriptionRoutingProfile(
		"sub1", provided, []string{"https://off.test/l.lst"}, nil, subscriptionRoutingHeader{}, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	profiles := a.config.GetConfig().RoutingRules.Profiles
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(profiles))
	}
	p := profiles[0]
	if len(p.ListURLs["proxy"]) != 0 {
		t.Errorf("удалённый пользователем и выключенный списки не должны вернуться: %+v", p.ListURLs)
	}
	if len(p.ListURLs["direct"]) != 1 {
		t.Errorf("оставленный список потерян: %+v", p.ListURLs)
	}
}

func TestSubscriptionRoutingAllDisabledLeavesNoProfile(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)
	provided := []config.RoutingList{{URL: "https://x.test/l.lst", Action: "proxy"}}
	// Пользователь отказался от маршрутизации подписки — пустого профиля
	// заводить не надо, показывать в окне было бы нечего.
	if err := a.syncSubscriptionRoutingProfile(
		"sub1", provided, []string{"https://x.test/l.lst"}, nil, subscriptionRoutingHeader{}, true); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := a.config.GetConfig().RoutingRules.Profiles; len(got) != 0 {
		t.Fatalf("профиль не должен был появиться: %+v", got)
	}
}

func TestSubscriptionRoutingUnknownSubscriptionIsNoop(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	provided := []config.RoutingList{{URL: "https://x.test/l.lst", Action: "proxy"}}
	if err := a.syncSubscriptionRoutingProfile("ghost", provided, nil, nil, subscriptionRoutingHeader{}, true); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := a.config.GetConfig().RoutingRules.Profiles; len(got) != 0 {
		t.Fatalf("неизвестная подписка не должна ничего создавать: %+v", got)
	}
}

func TestSubscriptionRoutingContentUpdateReflected(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)
	provided := []config.RoutingList{{Name: "embedded-direct", URL: "embedded:direct", Action: "direct"}}

	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil,
		map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"old.test"}}}, subscriptionRoutingHeader{}, true); err != nil {
		t.Fatal(err)
	}
	first := a.config.GetConfig().RoutingRules.Profiles[0].ID

	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil,
		map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"new.test"}}}, subscriptionRoutingHeader{}, false); err != nil {
		t.Fatal(err)
	}
	profiles := a.config.GetConfig().RoutingRules.Profiles
	if len(profiles) != 1 || profiles[0].ID != first {
		t.Fatalf("повторный синк должен обновить тот же профиль, а не завести второй: %+v", profiles)
	}
	// Провайдер сменил состав — в профиле должно быть новое, не старое.
	joined := strings.Join(profiles[0].DirectSites, ",")
	if strings.Contains(joined, "old.test") || !strings.Contains(joined, "new.test") {
		t.Errorf("состав профиля не обновился: %v", profiles[0].DirectSites)
	}
}

func TestSubscriptionRoutingReplacesLegacyLists(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	// Так выглядит конфиг, доставшийся от прошлой версии: маршрутизация
	// подписки лежит отдельными списками.
	seedSubWithLists(t, a, "sub1", []config.RoutingList{
		{ID: "old1", SubscriptionID: "sub1", URL: "embedded:direct", Action: "direct", Enabled: true},
		{ID: "user", SubscriptionID: "", URL: "https://user.test/l.lst", Action: "direct", Enabled: true},
	}, nil)

	provided := []config.RoutingList{{Name: "embedded-direct", URL: "embedded:direct", Action: "direct"}}
	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil,
		map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"gov.test"}}}, subscriptionRoutingHeader{}, true); err != nil {
		t.Fatal(err)
	}
	rr := a.config.GetConfig().RoutingRules
	for _, rl := range rr.RoutingLists {
		if rl.SubscriptionID == "sub1" {
			t.Errorf("старый список подписки остался рядом с профилем — правила применялись бы дважды: %+v", rl)
		}
	}
	// Свой список пользователя к подписке отношения не имеет и должен уцелеть.
	found := false
	for _, rl := range rr.RoutingLists {
		if rl.ID == "user" {
			found = true
		}
	}
	if !found {
		t.Error("список самого пользователя не должен был пострадать")
	}
	if len(rr.Profiles) != 1 {
		t.Fatalf("профиль не создан: %+v", rr.Profiles)
	}
}

func headerProfileFixture() *config.RoutingProfile {
	return &config.RoutingProfile{
		Name:        "Provider",
		OriginName:  "Provider",
		Source:      "subscription",
		DirectSites: []string{"geosite:private", "gov.test"},
		DirectIPs:   []string{"geoip:private"},
		ProxySites:  []string{"geosite:youtube"},
		BlockSites:  []string{"geosite:category-ads"},
		RouteOrder:  "block-proxy-direct",
		GeoSiteURL:  "https://127.0.0.1:9/geosite.dat",
		GeoIPURL:    "https://127.0.0.1:9/geoip.dat",
		UpdatedAt:   1700000000,
	}
}

func TestSubscriptionRoutingHeaderProfileWins(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)

	res := subscriptionFetch{
		Embedded: map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"embedded.test"}}},
		Routing:  subscriptionRoutingHeader{Profile: headerProfileFixture()},
	}
	provided := subscriptionRoutingDeclarations(res)
	if len(provided) != 1 || provided[0].URL != routingHeaderURL {
		t.Fatalf("при профиле в заголовке встроенные правила не объявляются: %+v", provided)
	}
	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil, res.Embedded, res.Routing, true); err != nil {
		t.Fatalf("sync: %v", err)
	}

	rr := a.config.GetConfig().RoutingRules
	if len(rr.Profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(rr.Profiles))
	}
	p := rr.Profiles[0]
	if p.Name != "Provider" || p.OriginName != "Provider" || p.SubscriptionID != "sub1" || p.Source != "subscription" {
		t.Errorf("identity: %+v", p)
	}
	if p.GeoSiteURL != "https://127.0.0.1:9/geosite.dat" || p.GeoIPURL != "https://127.0.0.1:9/geoip.dat" {
		t.Errorf("geo URLs lost: %q %q", p.GeoSiteURL, p.GeoIPURL)
	}
	if p.RuleCount("direct") != 3 || p.RuleCount("proxy") != 1 || p.RuleCount("block") != 1 {
		t.Errorf("rules: direct=%d proxy=%d block=%d", p.RuleCount("direct"), p.RuleCount("proxy"), p.RuleCount("block"))
	}
	for _, tok := range p.DirectSites {
		if tok == "embedded.test" {
			t.Error("встроенные правила не должны смешиваться с профилем из заголовка")
		}
	}
	if rr.ActiveProfileID != p.ID {
		t.Error("принятый при импорте профиль должен стать активным")
	}
}

func TestSubscriptionRoutingHeaderRespectsImportOptOut(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)
	res := subscriptionFetch{Routing: subscriptionRoutingHeader{Profile: headerProfileFixture()}}
	if err := a.syncSubscriptionRoutingProfile(
		"sub1", subscriptionRoutingDeclarations(res), []string{routingHeaderURL}, nil, res.Routing, true); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := a.config.GetConfig().RoutingRules.Profiles; len(got) != 0 {
		t.Fatalf("пользователь отказался от маршрутизации подписки, профиль не нужен: %+v", got)
	}
}

func TestSubscriptionRoutingHeaderOffRemovesProfile(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)
	on := subscriptionFetch{Routing: subscriptionRoutingHeader{Profile: headerProfileFixture()}}
	if err := a.syncSubscriptionRoutingProfile("sub1", subscriptionRoutingDeclarations(on), nil, nil, on.Routing, true); err != nil {
		t.Fatal(err)
	}
	off := subscriptionFetch{Routing: subscriptionRoutingHeader{Off: true}}
	if err := a.syncSubscriptionRoutingProfile("sub1", subscriptionRoutingDeclarations(off), nil, nil, off.Routing, false); err != nil {
		t.Fatal(err)
	}
	if got := a.config.GetConfig().RoutingRules.Profiles; len(got) != 0 {
		t.Fatalf("провайдер снял маршрутизацию, профиль должен уйти: %+v", got)
	}
}

func TestSubscriptionRoutingHeaderReplacesEarlierProfile(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)
	if err := a.syncSubscriptionRoutingProfile("sub1",
		[]config.RoutingList{{URL: "embedded:direct", Action: "direct"}}, nil,
		map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"gov.test"}}},
		subscriptionRoutingHeader{}, true); err != nil {
		t.Fatal(err)
	}
	res := subscriptionFetch{Routing: subscriptionRoutingHeader{Profile: headerProfileFixture()}}
	if err := a.syncSubscriptionRoutingProfile("sub1", subscriptionRoutingDeclarations(res), nil, nil, res.Routing, false); err != nil {
		t.Fatal(err)
	}
	profiles := a.config.GetConfig().RoutingRules.Profiles
	if len(profiles) != 1 {
		t.Fatalf("у подписки должен остаться один профиль, а не два: %+v", profiles)
	}
	if profiles[0].Name != "Provider" {
		t.Errorf("непереименованный профиль должен взять имя от провайдера, got %q", profiles[0].Name)
	}
}
