// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestProfileNeedsGeoOnlyWhenReferenced(t *testing.T) {
	plain := config.RoutingProfile{ProxySites: []string{"example.com"}}
	if site, ip := ProfileNeedsGeo(plain); site || ip {
		t.Error("профиль из обычных доменов просит geo-базы")
	}
	withSite := config.RoutingProfile{
		ProxySites: []string{"geosite:whitelist"},
		GeoSiteURL: "https://panel.example/geosite.dat",
	}
	if site, ip := ProfileNeedsGeo(withSite); !site || ip {
		t.Errorf("geosite=%v geoip=%v, ждали true/false", site, ip)
	}
	withIP := config.RoutingProfile{
		BlockIPs: []string{"geoip:cn"},
		GeoIPURL: "https://panel.example/geoip.dat",
	}
	if site, ip := ProfileNeedsGeo(withIP); site || !ip {
		t.Errorf("geosite=%v geoip=%v, ждали false/true", site, ip)
	}
	// Ссылки нет — качать нечего, значит и просить нечего.
	noURL := config.RoutingProfile{ProxySites: []string{"geosite:whitelist"}}
	if site, _ := ProfileNeedsGeo(noURL); site {
		t.Error("geosite запрошен без ссылки на базу")
	}
}

func TestGeoCachePathKeyedByURLNotProfile(t *testing.T) {
	a := GeoCachePath("/data", "geosite", "https://a.example/geosite.dat")
	b := GeoCachePath("/data", "geosite", "https://b.example/geosite.dat")
	if a == b {
		t.Error("разные ссылки дали один путь кэша")
	}
	again := GeoCachePath("/data", "geosite", " https://a.example/geosite.dat ")
	if a != again {
		t.Error("та же ссылка с пробелами дала другой путь")
	}
	// Разные виды баз на одной ссылке не должны делить файл.
	if GeoCachePath("/data", "geoip", "https://a.example/db.dat") ==
		GeoCachePath("/data", "geosite", "https://a.example/db.dat") {
		t.Error("geoip и geosite по одной ссылке пишутся в один файл")
	}
	if base := filepath.Base(filepath.Dir(a)); base != "geo" {
		t.Errorf("кэш geo лежит не в routing/geo: %s", a)
	}
}

func TestCompileRoutingProfilePlainTokensNoNetwork(t *testing.T) {
	dir := t.TempDir()
	p := config.RoutingProfile{
		ID:          "plainprof",
		Name:        "Plain",
		DirectSites: []string{"direct.example"},
		ProxySites:  []string{"domain:proxy.example"},
		BlockIPs:    []string{"10.0.0.0/8"},
	}
	rep, err := CompileRoutingProfile(context.Background(), p, dir, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	for _, action := range RoutingActions {
		if rep.Counts[action] == 0 {
			t.Errorf("действие %s собралось в ноль", action)
		}
		if !RoutingProfileSRSReady(dir, "plainprof", action) {
			t.Errorf("SRS для %s не готов", action)
		}
	}
	if len(rep.Unresolved) != 0 {
		t.Errorf("непринятые токены на чистом профиле: %v", rep.Unresolved)
	}
}

func TestCompileRoutingProfileReportsUnexpressibleTokens(t *testing.T) {
	dir := t.TempDir()
	p := config.RoutingProfile{
		ID:         "reportprof",
		Name:       "Report",
		ProxySites: []string{"ok.example", `regexp:.*\.example`, "keyword:ads"},
	}
	rep, err := CompileRoutingProfile(context.Background(), p, dir, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	if len(rep.Unresolved) != 2 {
		t.Errorf("непринятых токенов %d, ждали 2: %v", len(rep.Unresolved), rep.Unresolved)
	}
	if rep.Counts["proxy"] == 0 {
		t.Error("выразимая часть профиля потеряна вместе с невыразимой")
	}
}

func TestCompileRoutingProfileFailsWhenNothingCompiles(t *testing.T) {
	dir := t.TempDir()
	p := config.RoutingProfile{
		ID:         "emptyprof",
		Name:       "Empty",
		ProxySites: []string{"geosite:whitelist"}, // базы нет — развернуть нечем
	}
	if _, err := CompileRoutingProfile(context.Background(), p, dir, false); err == nil {
		t.Fatal("профиль, из которого не собралось ни одного правила, принят")
	}
}

func TestCompileRoutingProfileClearsStaleAction(t *testing.T) {
	dir := t.TempDir()
	first := config.RoutingProfile{
		ID: "staleprof", Name: "Stale",
		ProxySites:  []string{"proxy.example"},
		DirectSites: []string{"direct.example"},
	}
	if _, err := CompileRoutingProfile(context.Background(), first, dir, false); err != nil {
		t.Fatalf("первая сборка: %v", err)
	}
	if !RoutingProfileSRSReady(dir, "staleprof", "direct") {
		t.Fatal("подготовка не удалась")
	}
	second := first
	second.DirectSites = nil
	if _, err := CompileRoutingProfile(context.Background(), second, dir, false); err != nil {
		t.Fatalf("вторая сборка: %v", err)
	}
	if RoutingProfileSRSReady(dir, "staleprof", "direct") {
		t.Error("SRS выброшенного действия пережил пересборку — маршрут остался бы жить")
	}
}

func TestCompileRoutingProfileRefusesBadID(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"../escape", "", "a/b"} {
		p := config.RoutingProfile{ID: id, ProxySites: []string{"a.example"}}
		if _, err := CompileRoutingProfile(context.Background(), p, dir, false); err == nil {
			t.Errorf("профиль с id %q собран", id)
		}
	}
}

func TestValidateGeoBlobRejectsNonDatabase(t *testing.T) {
	if err := validateGeoBlob("geosite", []byte("<html>404 Not Found</html>")); err == nil {
		t.Fatal("страница ошибки принята за базу geosite")
	}
	if err := validateGeoBlob("geoip", []byte("nope, not protobuf at all")); err == nil {
		t.Fatal("мусор принят за базу geoip")
	}
	if err := validateGeoBlob("whatever", []byte("x")); err == nil {
		t.Fatal("неизвестный вид базы принят")
	}
}

func TestGeoCacheNotWrittenWhenFetchFails(t *testing.T) {
	dir := t.TempDir()
	// Loopback, а не несуществующее имя: защита из routing_fetch.go отсекает
	// его в Control, до всякого запроса. Несуществующее имя стоило бы трёх
	// повторов по DNS — пятнадцать секунд и зависимость от сети.
	url := "https://127.0.0.1:1/geosite.dat"
	path := GeoCachePath(dir, "geosite", url)
	p := config.RoutingProfile{
		ID: "geoprof", ProxySites: []string{"geosite:whitelist"}, GeoSiteURL: url,
	}
	if _, err := CompileRoutingProfile(context.Background(), p, dir, false); err == nil {
		t.Fatal("сборка прошла без доступной базы")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("неудачная загрузка оставила файл в кэше — следующая сборка приняла бы его за попадание")
	}
}

func TestCompileRoutingProfileExpandsGeoFromCache(t *testing.T) {
	// Кэш заполняется заранее, сеть не нужна: проверяется, что при готовом
	// файле сборка разворачивает категорию, а не ходит за базой снова.
	dir := t.TempDir()
	url := "https://panel.example/geosite.dat"
	blob := geoSiteListMsg(
		geoSiteMsg("WHITELIST", geoDomainMsg(geoDomainDomain, "listed.example", true)),
	)
	path := GeoCachePath(dir, "geosite", url)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("подготовка каталога: %v", err)
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("подготовка кэша: %v", err)
	}

	p := config.RoutingProfile{
		ID: "geocached", Name: "Geo", ProxySites: []string{"geosite:whitelist"},
		GeoSiteURL: url,
	}
	rep, err := CompileRoutingProfile(context.Background(), p, dir, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	if rep.Counts["proxy"] != 1 {
		t.Fatalf("правил proxy %d, ждали 1: %v", rep.Counts["proxy"], rep.Unresolved)
	}
	m, err := LoadRoutingDomainMatcher(RoutingProfileSRSPath(dir, "geocached", "proxy"))
	if err != nil {
		t.Fatalf("LoadRoutingDomainMatcher: %v", err)
	}
	if !m.Match("sub.listed.example") {
		t.Error("развёрнутая категория не ловит поддомен")
	}
	if m.Match("other.example") {
		t.Error("правило поймало то, чего в категории нет")
	}
}
