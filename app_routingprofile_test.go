// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"encoding/base64"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
)

// A payload shaped exactly like the one a live panel publishes.
const testProfileJSON = `{"Name":"impVPN Routing Whitelist",` +
	`"RouteOrder":"block-proxy-direct","DomainStrategy":"IPIfNonMatch",` +
	`"Geoipurl":"https://panel.example/geo/geoip.dat",` +
	`"Geositeurl":"https://panel.example/geo/geosite.dat",` +
	`"DirectSites":["geosite:private","geosite:whitelist"],` +
	`"DirectIp":["geoip:private","geoip:whitelist"],` +
	`"ProxySites":[],"ProxyIp":[],` +
	`"BlockSites":["geosite:win-spy","geosite:torrent","geosite:category-ads"],` +
	`"BlockIp":[]}`

func testRoutingLink(t *testing.T, payload string) string {
	t.Helper()
	return "resultv://routing/onadd/" + base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func TestImportRoutingDeepLinkStoresAndActivates(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	got, err := a.ImportRoutingDeepLink(testRoutingLink(t, testProfileJSON), true)
	if err != nil {
		t.Fatalf("ImportRoutingDeepLink: %v", err)
	}
	if got.ID == "" {
		t.Fatal("stored profile has no id")
	}
	if got.Name != "impVPN Routing Whitelist" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Source != "deeplink" {
		t.Errorf("Source = %q, want deeplink", got.Source)
	}

	rr := a.config.GetConfig().RoutingRules
	if len(rr.Profiles) != 1 {
		t.Fatalf("stored %d profiles, want 1", len(rr.Profiles))
	}
	if rr.ActiveProfileID != got.ID {
		t.Errorf("ActiveProfileID = %q, want %q", rr.ActiveProfileID, got.ID)
	}

	active, ok := a.ActiveRoutingProfile()
	if !ok || active.ID != got.ID {
		t.Fatalf("ActiveRoutingProfile = %+v, ok=%v", active, ok)
	}
}

func TestImportRoutingDeepLinkTwiceUpdatesInPlace(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	link := testRoutingLink(t, testProfileJSON)

	first, err := a.ImportRoutingDeepLink(link, true)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	// Same profile republished with one more block rule — panels keep the link
	// stable and change what it carries.
	updated := strings.Replace(testProfileJSON,
		`"BlockIp":[]`, `"BlockIp":["1.2.3.0/24"]`, 1)
	second, err := a.ImportRoutingDeepLink(testRoutingLink(t, updated), true)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("re-import made a new profile (%q vs %q)", second.ID, first.ID)
	}
	rr := a.config.GetConfig().RoutingRules
	if len(rr.Profiles) != 1 {
		t.Fatalf("stored %d profiles, want the same one updated", len(rr.Profiles))
	}
	if rr.Profiles[0].RuleCount("block") != 4 {
		t.Errorf("block count = %d, want the new rule to have landed", rr.Profiles[0].RuleCount("block"))
	}
}

func TestImportRoutingDeepLinkKeepsUserRename(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	link := testRoutingLink(t, testProfileJSON)

	saved, err := a.ImportRoutingDeepLink(link, true)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	saved.Name = "Мой профиль"
	if _, err := a.SaveRoutingProfile(saved); err != nil {
		t.Fatalf("rename: %v", err)
	}
	// Re-opening the link must not undo the rename.
	if _, err := a.ImportRoutingDeepLink(link, true); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	rr := a.config.GetConfig().RoutingRules
	if len(rr.Profiles) != 1 {
		t.Fatalf("stored %d profiles, want 1", len(rr.Profiles))
	}
	if rr.Profiles[0].Name != "Мой профиль" {
		t.Errorf("Name = %q, want the rename kept", rr.Profiles[0].Name)
	}
}

func TestPreviewRoutingDeepLinkStoresNothing(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.PreviewRoutingDeepLink(testRoutingLink(t, testProfileJSON))
	if err != nil {
		t.Fatalf("PreviewRoutingDeepLink: %v", err)
	}
	if p.Name == "" {
		t.Error("preview returned nothing to show")
	}
	// The user has not agreed to anything yet.
	if len(a.config.GetConfig().RoutingRules.Profiles) != 0 {
		t.Fatal("preview stored the profile")
	}
}

func TestRoutingProfileCRUD(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	p, err := a.SaveRoutingProfile(config.RoutingProfile{
		Name:        "Ручной",
		DirectSites: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Source != "manual" {
		t.Errorf("Source = %q, want manual", p.Source)
	}

	p.BlockSites = []string{"ads.example"}
	if _, err := a.SaveRoutingProfile(p); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := a.config.GetConfig().RoutingRules.Profiles[0].RuleCount("block"); got != 1 {
		t.Errorf("block count = %d after update", got)
	}

	if err := a.SetActiveRoutingProfile(p.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if err := a.DeleteRoutingProfile(p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rr := a.config.GetConfig().RoutingRules
	if len(rr.Profiles) != 0 {
		t.Errorf("profiles left after delete: %d", len(rr.Profiles))
	}
	// Deleting the profile in force must not silently promote another one.
	if rr.ActiveProfileID != "" {
		t.Errorf("ActiveProfileID = %q, want empty after deleting the active one", rr.ActiveProfileID)
	}
}

func TestRoutingProfileRejectsEmptyAndUnknown(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	if _, err := a.SaveRoutingProfile(config.RoutingProfile{Name: "  "}); err == nil {
		t.Error("a nameless profile was accepted")
	}
	if _, err := a.SaveRoutingProfile(config.RoutingProfile{Name: "X"}); err == nil {
		t.Error("a profile with no rules was accepted")
	}
	if _, err := a.SaveRoutingProfile(config.RoutingProfile{
		ID: "nope", Name: "X", DirectSites: []string{"a.test"},
	}); err == nil {
		t.Error("editing a profile that does not exist was accepted")
	}
	if err := a.DeleteRoutingProfile("nope"); err == nil {
		t.Error("deleting a profile that does not exist was accepted")
	}
	if err := a.SetActiveRoutingProfile("nope"); err == nil {
		t.Error("activating a profile that does not exist was accepted")
	}
}

func TestSetActiveRoutingProfileEmptyTurnsOff(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.SaveRoutingProfile(config.RoutingProfile{
		Name: "X", DirectSites: []string{"a.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetActiveRoutingProfile(p.ID); err != nil {
		t.Fatal(err)
	}
	// "" is a legitimate choice: profiles off, none deleted.
	if err := a.SetActiveRoutingProfile(""); err != nil {
		t.Fatalf("turning profiles off: %v", err)
	}
	if _, ok := a.ActiveRoutingProfile(); ok {
		t.Error("a profile is still in force")
	}
	if len(a.config.GetConfig().RoutingRules.Profiles) != 1 {
		t.Error("turning profiles off deleted one")
	}
}

func TestSaveRoutingProfileKeepsProvenance(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	imported, err := a.ImportRoutingDeepLink(testRoutingLink(t, testProfileJSON), false)
	if err != nil {
		t.Fatal(err)
	}
	// The editor must not be able to launder a delivered profile into a manual
	// one — a later subscription sync still has to recognise it.
	imported.Source = "manual"
	imported.SubscriptionID = "forged"
	saved, err := a.SaveRoutingProfile(imported)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Source != "deeplink" || saved.SubscriptionID != "" {
		t.Errorf("provenance changed: source=%q sub=%q", saved.Source, saved.SubscriptionID)
	}
}

func TestGetRoutingProfilesCopiesOutOfTheLiveConfig(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if _, err := a.SaveRoutingProfile(config.RoutingProfile{
		Name: "X", DirectSites: []string{"a.test"},
	}); err != nil {
		t.Fatal(err)
	}
	got := a.GetRoutingProfiles()
	profiles, _ := got["profiles"].([]config.RoutingProfile)
	if len(profiles) != 1 {
		t.Fatalf("got %d profiles", len(profiles))
	}
	// GetConfig hands back slices aliasing the live cache; the getter must not
	// let a caller edit the stored config by writing to what it returned.
	profiles[0].Name = "перезаписано"
	if a.config.GetConfig().RoutingRules.Profiles[0].Name != "X" {
		t.Fatal("the returned slice aliases the live config")
	}
}
