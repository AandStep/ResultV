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
	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil, embedded, true); err != nil {
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
	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil, nil, false); err != nil {
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
		"sub1", provided, []string{"https://off.test/l.lst"}, nil, false); err != nil {
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
		"sub1", provided, []string{"https://x.test/l.lst"}, nil, true); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := a.config.GetConfig().RoutingRules.Profiles; len(got) != 0 {
		t.Fatalf("профиль не должен был появиться: %+v", got)
	}
}

func TestSubscriptionRoutingUnknownSubscriptionIsNoop(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	provided := []config.RoutingList{{URL: "https://x.test/l.lst", Action: "proxy"}}
	if err := a.syncSubscriptionRoutingProfile("ghost", provided, nil, nil, true); err != nil {
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
		map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"old.test"}}}, true); err != nil {
		t.Fatal(err)
	}
	first := a.config.GetConfig().RoutingRules.Profiles[0].ID

	if err := a.syncSubscriptionRoutingProfile("sub1", provided, nil,
		map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"new.test"}}}, false); err != nil {
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
		map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"gov.test"}}}, true); err != nil {
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
