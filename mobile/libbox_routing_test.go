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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const routingBindPayload = `{
  "Name": "Panel Routing",
  "RouteOrder": "block-proxy-direct",
  "DirectSites": ["direct.example"],
  "ProxySites": ["proxy.example"],
  "BlockSites": ["block.example"],
  "LastUpdated": "1788322632"
}`

func routingBindLink() string {
	return "resultv://routing/onadd/" +
		base64.RawURLEncoding.EncodeToString([]byte(routingBindPayload))
}

func TestIsRoutingDeepLinkSplitsKinds(t *testing.T) {
	if !IsRoutingDeepLink(routingBindLink()) {
		t.Error("routing-ссылка не опознана")
	}
	if IsRoutingDeepLink("resultv://import/AAAA") {
		t.Error("ссылка подписки опознана как routing")
	}
}

func TestPreviewRoutingDeepLinkReturnsProfileWithoutID(t *testing.T) {
	out, err := PreviewRoutingDeepLink(routingBindLink())
	if err != nil {
		t.Fatalf("PreviewRoutingDeepLink: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("результат не JSON: %v", err)
	}
	if p["name"] != "Panel Routing" {
		t.Errorf("name = %v", p["name"])
	}
	if p["originName"] != "Panel Routing" {
		t.Errorf("originName = %v", p["originName"])
	}
	if id, ok := p["id"]; ok && id != "" {
		t.Errorf("превью назначило id %v — это дело merge", id)
	}
	if p["routeOrder"] != "block-proxy-direct" {
		t.Errorf("routeOrder = %v", p["routeOrder"])
	}
	if p["source"] != "deeplink" {
		t.Errorf("source = %v", p["source"])
	}
}

// Имена полей — единственное, что связывает Go и хранилище на Kotlin.
// Переименуй любое, и оно молча потеряется на круге.
func TestRoutingProfileJSONFieldNamesAreStable(t *testing.T) {
	out, err := PreviewRoutingDeepLink(routingBindLink())
	if err != nil {
		t.Fatalf("PreviewRoutingDeepLink: %v", err)
	}
	for _, key := range []string{
		"name", "directSites", "proxySites", "blockSites",
		"routeOrder", "originName", "source", "updatedAt",
	} {
		if !strings.Contains(out, `"`+key+`"`) {
			t.Errorf("в JSON нет ключа %q: %s", key, out)
		}
	}
}

func TestPreviewRoutingDeepLinkRejectsJunk(t *testing.T) {
	for _, bad := range []string{
		"resultv://routing/onadd/!!!!",
		"resultv://routing/onadd/" + base64.RawURLEncoding.EncodeToString([]byte(`{"Name":"x"}`)),
		"resultv://import/AAAA",
		"",
	} {
		if _, err := PreviewRoutingDeepLink(bad); err == nil {
			t.Errorf("принята негодная ссылка %q", bad)
		}
	}
}

func TestMergeRoutingProfileAssignsIDAndActivatesFirst(t *testing.T) {
	incoming, err := PreviewRoutingDeepLink(routingBindLink())
	if err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	out, err := MergeRoutingProfile(`{"profiles":[],"activeId":""}`, incoming, false)
	if err != nil {
		t.Fatalf("MergeRoutingProfile: %v", err)
	}
	var res struct {
		Profiles []map[string]any `json:"profiles"`
		ActiveID string           `json:"activeId"`
		Saved    map[string]any   `json:"saved"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("результат не JSON: %v", err)
	}
	if len(res.Profiles) != 1 {
		t.Fatalf("профилей %d, ждали 1", len(res.Profiles))
	}
	id, _ := res.Saved["id"].(string)
	if id == "" {
		t.Fatal("id не назначен")
	}
	if res.ActiveID != id {
		t.Errorf("activeId = %q, ждали %q", res.ActiveID, id)
	}
}

func TestMergeRoutingProfileTolerantToEmptyStore(t *testing.T) {
	incoming, err := PreviewRoutingDeepLink(routingBindLink())
	if err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	for _, store := range []string{"", "   ", "{}", `{"profiles":null,"activeId":""}`} {
		if _, err := MergeRoutingProfile(store, incoming, false); err != nil {
			t.Errorf("пустое хранилище %q отвергнуто: %v", store, err)
		}
	}
}

func TestMergeRoutingProfileRejectsBrokenJSON(t *testing.T) {
	incoming, err := PreviewRoutingDeepLink(routingBindLink())
	if err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	if _, err := MergeRoutingProfile("не json", incoming, false); err == nil {
		t.Error("испорченное хранилище принято — молча начали бы с пустого")
	}
	if _, err := MergeRoutingProfile("{}", "не json", false); err == nil {
		t.Error("испорченный входящий профиль принят")
	}
}

func TestCompileRoutingProfileAndStatus(t *testing.T) {
	dir := t.TempDir()
	profile := `{"id":"bindprof","name":"B","proxySites":["proxy.example"],"directSites":["direct.example"]}`
	out, err := CompileRoutingProfile(profile, dir, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	var rep struct {
		Counts     map[string]int    `json:"counts"`
		Unresolved map[string]string `json:"unresolved"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("отчёт не JSON: %v", err)
	}
	if rep.Counts["proxy"] == 0 || rep.Counts["direct"] == 0 {
		t.Errorf("счётчики пусты: %v", rep.Counts)
	}
	statusJSON, err := RoutingProfileStatus(dir, "bindprof")
	if err != nil {
		t.Fatalf("RoutingProfileStatus: %v", err)
	}
	var st map[string]bool
	if err := json.Unmarshal([]byte(statusJSON), &st); err != nil {
		t.Fatalf("статус не JSON: %v", err)
	}
	if !st["proxy"] || !st["direct"] {
		t.Errorf("статус = %v, ждали proxy и direct готовыми", st)
	}
	if st["block"] {
		t.Error("block отмечен готовым, хотя правил для него не было")
	}
}

func TestCompileRoutingProfileRequiresDataDir(t *testing.T) {
	profile := `{"id":"nodir","name":"N","proxySites":["proxy.example"]}`
	if _, err := CompileRoutingProfile(profile, "", false); err == nil {
		t.Fatal("сборка без dataDir прошла — писать было бы некуда")
	}
}

func TestRemoveRoutingProfileDropsCache(t *testing.T) {
	dir := t.TempDir()
	profile := `{"id":"goneprof","name":"G","proxySites":["proxy.example"]}`
	if _, err := CompileRoutingProfile(profile, dir, false); err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	if err := RemoveRoutingProfile(dir, "goneprof"); err != nil {
		t.Fatalf("RemoveRoutingProfile: %v", err)
	}
	statusJSON, err := RoutingProfileStatus(dir, "goneprof")
	if err != nil {
		t.Fatalf("RoutingProfileStatus: %v", err)
	}
	if strings.Contains(statusJSON, "true") {
		t.Errorf("после удаления статус = %s", statusJSON)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "routing"))
	if err != nil {
		t.Fatalf("читаем каталог кэша: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "prof-goneprof-") {
			t.Errorf("файл пережил удаление: %s", e.Name())
		}
	}
}

func TestExtractSubscriptionRoutingFromJSONBody(t *testing.T) {
	body := `{"routingLists":[{"name":"L","url":"https://panel.example/l.txt","action":"proxy"}]}`
	out, err := ExtractSubscriptionRouting("", body)
	if err != nil {
		t.Fatalf("ExtractSubscriptionRouting: %v", err)
	}
	if !strings.Contains(out, "panel.example/l.txt") {
		t.Errorf("список не извлечён: %s", out)
	}
}

func TestExtractSubscriptionRoutingEmptyIsArrayNotNull(t *testing.T) {
	// Kotlin разбирает это как JSONArray: null там был бы падением, а не
	// пустым списком.
	out, err := ExtractSubscriptionRouting("", "ничего похожего")
	if err != nil {
		t.Fatalf("ExtractSubscriptionRouting: %v", err)
	}
	if out != "[]" {
		t.Errorf("результат = %q, ждали []", out)
	}
}
