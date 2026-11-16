// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
)

const subTestEntry = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=none&type=tcp#n\n"

func TestFetchSubscriptionCarriesPublishedRoutingProfile(t *testing.T) {
	const payload = `{"Name":"Provider routing","DirectSites":["direct.example"],"ProxySites":["proxy.example"],"BlockSites":["block.example"],"Geoipurl":"https://panel.example/geoip.dat"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Routing", "happ://routing/onadd/"+base64.RawURLEncoding.EncodeToString([]byte(payload)))
		w.Header().Set("Profile-Title", "Subscription title")
		w.Write([]byte(subTestEntry))
	}))
	defer server.Close()

	raw, err := FetchSubscriptionV3(server.URL, t.TempDir(), `{"sendHwid":false}`)
	if err != nil {
		t.Fatal(err)
	}
	var response SubscriptionResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatal(err)
	}
	if response.Routing == "" {
		t.Fatal("the published Routing header was lost")
	}
	var profile config.RoutingProfile
	if err := json.Unmarshal([]byte(response.Routing), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Name != "Provider routing" || profile.OriginName != profile.Name || profile.Source != "subscription" ||
		profile.RuleCount("direct") != 1 || profile.RuleCount("proxy") != 1 || profile.RuleCount("block") != 1 ||
		profile.GeoIPURL != "https://panel.example/geoip.dat" {
		t.Fatalf("unexpected provider profile: %+v", profile)
	}
}

// Linked lists reach Kotlin in the same routing field as published profiles.
func TestFetchSubscriptionCarriesRoutingFromHeader(t *testing.T) {
	decl := `[{"name":"L","url":"https://panel.example/l.txt","action":"proxy"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Routing-Lists", base64.StdEncoding.EncodeToString([]byte(decl)))
		w.Header().Set("Profile-Title", "impVPN")
		w.Write([]byte(subTestEntry))
	}))
	defer srv.Close()

	out, err := FetchSubscriptionV3(srv.URL, t.TempDir(), "{}")
	if err != nil {
		t.Fatalf("FetchSubscriptionV3: %v", err)
	}
	var res struct {
		Entries []map[string]any `json:"entries"`
		Routing string           `json:"routing"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Fatal("серверы потеряны")
	}
	if res.Routing == "" {
		t.Fatal("ключа routing нет")
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(res.Routing), &p); err != nil {
		t.Fatalf("routing не JSON: %v", err)
	}
	if p["source"] != "subscription" {
		t.Errorf("source = %v", p["source"])
	}
	if p["name"] != "impVPN" {
		t.Errorf("имя = %v, ждали заголовок Profile-Title", p["name"])
	}
	if !strings.Contains(res.Routing, "panel.example/l.txt") {
		t.Errorf("ссылка на список потеряна: %s", res.Routing)
	}
}

// Embedded Xray rules reach Kotlin even when the server sends no header.
func TestFetchSubscriptionCarriesRoutingFromBody(t *testing.T) {
	body := `{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"1.2.3.4",` +
		`"port":443,"users":[{"id":"11111111-1111-1111-1111-111111111111"}]}]},` +
		`"streamSettings":{"network":"tcp","security":"none"},"tag":"proxy"}],` +
		`"routing":{"rules":[{"type":"field","outboundTag":"direct","domain":["direct.example"]}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Profile-Title", "impVPN")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	out, err := FetchSubscriptionV3(srv.URL, t.TempDir(), "{}")
	if err != nil {
		t.Fatalf("FetchSubscriptionV3: %v", err)
	}
	var res struct {
		Routing string `json:"routing"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if !strings.Contains(res.Routing, "direct.example") {
		t.Errorf("встроенные правила потеряны: %s", res.Routing)
	}
}

// Подписка без маршрутизации не должна давать пустой профиль: он занял бы
// строку в списке, предложил себя включить и ничего бы не маршрутизировал.
func TestFetchSubscriptionWithoutRoutingLeavesKeyEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(subTestEntry))
	}))
	defer srv.Close()

	out, err := FetchSubscriptionV3(srv.URL, t.TempDir(), "{}")
	if err != nil {
		t.Fatalf("FetchSubscriptionV3: %v", err)
	}
	var res struct {
		Routing string `json:"routing"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if res.Routing != "" {
		t.Errorf("routing = %q, ждали пусто", res.Routing)
	}
}
