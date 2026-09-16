// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/base64"
	"testing"
)

func TestBuildSubscriptionRoutingProfileFromHeader(t *testing.T) {
	decl := `[{"name":"L","url":"https://panel.example/l.txt","action":"proxy"}]`
	header := base64.StdEncoding.EncodeToString([]byte(decl))

	p, ok := BuildSubscriptionRoutingProfile("sub1", "impVPN", false, header, "")
	if !ok {
		t.Fatal("профиль не собран из заголовка")
	}
	if p.Source != "subscription" || p.SubscriptionID != "sub1" {
		t.Errorf("происхождение = %q / %q", p.Source, p.SubscriptionID)
	}
	// Имя издателя — имя подписки: по нему повторная синхронизация узнаёт свой
	// профиль, а пользовательское переименование ей не мешает.
	if p.Name != "impVPN" || p.OriginName != "impVPN" {
		t.Errorf("имена = %q / %q", p.Name, p.OriginName)
	}
	if got := p.ListURLs["proxy"]; len(got) != 1 || got[0] != "https://panel.example/l.txt" {
		t.Errorf("ссылки proxy = %v", got)
	}
	// Ссылка считается за одно правило, пока её не скачали.
	if p.RuleCount("proxy") != 1 {
		t.Errorf("счётчик proxy = %d", p.RuleCount("proxy"))
	}
}

func TestBuildSubscriptionRoutingProfileFromEmbeddedXray(t *testing.T) {
	body := `{"routing":{"rules":[
		{"type":"field","outboundTag":"direct","domain":["direct.example"]},
		{"type":"field","outboundTag":"proxy","domain":["proxy.example"],"ip":["10.0.0.0/8"]},
		{"type":"field","outboundTag":"block","domain":["ads.example"]}
	]}}`
	p, ok := BuildSubscriptionRoutingProfile("sub1", "impVPN", false, "", body)
	if !ok {
		t.Fatal("профиль не собран из встроенных правил")
	}
	// Встроенные правила становятся токенами: в хранимый файл они влезают, а
	// ссылки на них нет — качать нечего.
	if len(p.DirectSites) == 0 || len(p.ProxySites) == 0 || len(p.BlockSites) == 0 {
		t.Errorf("токены потеряны: %v / %v / %v", p.DirectSites, p.ProxySites, p.BlockSites)
	}
	if len(p.ProxyIPs) == 0 {
		t.Errorf("подсети proxy потеряны: %v", p.ProxyIPs)
	}
	if len(p.ListURLs) != 0 {
		t.Errorf("встроенные правила стали ссылками: %v", p.ListURLs)
	}
}

func TestBuildSubscriptionRoutingProfileMergesBothSources(t *testing.T) {
	decl := `[{"name":"L","url":"https://panel.example/l.txt","action":"block"}]`
	header := base64.StdEncoding.EncodeToString([]byte(decl))
	body := `{"routing":{"rules":[{"type":"field","outboundTag":"direct","domain":["direct.example"]}]}}`

	p, ok := BuildSubscriptionRoutingProfile("sub1", "impVPN", true, header, body)
	if !ok {
		t.Fatal("профиль не собран")
	}
	if len(p.DirectSites) == 0 {
		t.Error("встроенная часть потеряна")
	}
	if len(p.ListURLs["block"]) != 1 {
		t.Error("объявленная ссылка потеряна")
	}
	// Согласие на plaintext спускается вниз к загрузкам этих ссылок.
	if !p.AllowInsecure {
		t.Error("allowInsecure не передан профилю")
	}
}

func TestBuildSubscriptionRoutingProfileEmptyWhenNothingDeclared(t *testing.T) {
	for _, tc := range []struct{ header, body string }{
		{"", ""},
		{"", "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443"},
		{"не base64", "{}"},
	} {
		if _, ok := BuildSubscriptionRoutingProfile("sub1", "S", false, tc.header, tc.body); ok {
			t.Errorf("пустая подписка дала профиль: header=%q", tc.header)
		}
	}
}

// Подписка без имени не должна давать профиль без опознавательного знака: по
// OriginName его находит следующая синхронизация.
func TestBuildSubscriptionRoutingProfileFallsBackToSubID(t *testing.T) {
	body := `{"routing":{"rules":[{"type":"field","outboundTag":"direct","domain":["a.example"]}]}}`
	p, ok := BuildSubscriptionRoutingProfile("sub1", "   ", false, "", body)
	if !ok {
		t.Fatal("профиль не собран")
	}
	if p.Name == "" || p.OriginName == "" {
		t.Errorf("имена пусты: %q / %q", p.Name, p.OriginName)
	}
}

// Профиль подписки обязан пройти тот же путь, что и любой другой: слияние по
// OriginName и сборку. Значит его id должен быть пригоден как имя файла, когда
// его назначит UpsertRoutingProfile.
func TestBuildSubscriptionRoutingProfileSurvivesUpsert(t *testing.T) {
	body := `{"routing":{"rules":[{"type":"field","outboundTag":"proxy","domain":["a.example"]}]}}`
	p, ok := BuildSubscriptionRoutingProfile("sub1", "impVPN", false, "", body)
	if !ok {
		t.Fatal("профиль не собран")
	}
	out, activeID, saved, err := UpsertRoutingProfile(nil, p, "", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 || !ValidRoutingProfileID(saved.ID) {
		t.Fatalf("id = %q, профилей %d", saved.ID, len(out))
	}
	if activeID != saved.ID {
		t.Errorf("первый профиль не стал активным: %q", activeID)
	}
	// Повторная синхронизация той же подписки обновляет, а не раздваивает.
	again, _ := BuildSubscriptionRoutingProfile("sub1", "impVPN", false, "", body)
	out2, _, _, err := UpsertRoutingProfile(out, again, activeID, false)
	if err != nil {
		t.Fatalf("повторная синхронизация: %v", err)
	}
	if len(out2) != 1 {
		t.Errorf("профиль раздвоился: %d записей", len(out2))
	}
}
