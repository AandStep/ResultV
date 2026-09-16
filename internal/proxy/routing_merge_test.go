// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"fmt"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestUpsertAddsAndAssignsID(t *testing.T) {
	in := config.RoutingProfile{Name: "Panel A", OriginName: "Panel A", Source: "deeplink"}
	out, activeID, saved, err := UpsertRoutingProfile(nil, in, "", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профилей %d, ждали 1", len(out))
	}
	if saved.ID == "" {
		t.Error("id не назначен")
	}
	if !ValidRoutingProfileID(saved.ID) {
		t.Errorf("назначенный id %q не проходит ValidRoutingProfileID", saved.ID)
	}
	// Первый профиль становится активным, даже когда makeActive=false: иначе
	// импорт выглядел бы как «ничего не произошло».
	if activeID != saved.ID {
		t.Errorf("activeID = %q, ждали %q", activeID, saved.ID)
	}
}

func TestUpsertReplacesByOriginNameAndKeepsUserRename(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID:         "keepme",
		Name:       "Моя маршрутизация",
		OriginName: "Panel A",
		Source:     "deeplink",
		ProxySites: []string{"old.example"},
	}}
	in := config.RoutingProfile{
		Name:       "Panel A",
		OriginName: "Panel A",
		Source:     "deeplink",
		ProxySites: []string{"new.example"},
	}
	out, _, saved, err := UpsertRoutingProfile(stored, in, "keepme", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профиль раздвоился: %d записей", len(out))
	}
	if saved.ID != "keepme" {
		t.Errorf("id сменился на %q", saved.ID)
	}
	if saved.Name != "Моя маршрутизация" {
		t.Errorf("переименование затёрто: %q", saved.Name)
	}
	if len(saved.ProxySites) != 1 || saved.ProxySites[0] != "new.example" {
		t.Errorf("правила не обновились: %v", saved.ProxySites)
	}
}

func TestUpsertDoesNotMatchAcrossSources(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "sub1", Name: "Panel A", OriginName: "Panel A",
		Source: "subscription", SubscriptionID: "s1",
	}}
	in := config.RoutingProfile{Name: "Panel A", OriginName: "Panel A", Source: "deeplink"}
	out, _, _, err := UpsertRoutingProfile(stored, in, "sub1", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("профилей %d, ждали 2: подписочный и диплинковый — разные", len(out))
	}
}

func TestUpsertDoesNotMatchAcrossSubscriptions(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "s1", Name: "Panel A", OriginName: "Panel A",
		Source: "subscription", SubscriptionID: "one",
	}}
	in := config.RoutingProfile{
		Name: "Panel A", OriginName: "Panel A",
		Source: "subscription", SubscriptionID: "two",
	}
	out, _, _, err := UpsertRoutingProfile(stored, in, "s1", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("профилей %d, ждали 2: две подписки с одинаковым именем — разные профили", len(out))
	}
}

func TestUpsertFallsBackToNameWhenNoOrigin(t *testing.T) {
	stored := []config.RoutingProfile{{ID: "old", Name: "Panel A", Source: "manual"}}
	in := config.RoutingProfile{Name: "panel a", Source: "manual"}
	out, _, saved, err := UpsertRoutingProfile(stored, in, "", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профилей %d, ждали 1: сопоставление имён не зависит от регистра", len(out))
	}
	if saved.ID != "old" {
		t.Errorf("id сменился на %q", saved.ID)
	}
}

func TestUpsertHonoursMakeActive(t *testing.T) {
	stored := []config.RoutingProfile{{ID: "a", Name: "A", OriginName: "A", Source: "manual"}}
	in := config.RoutingProfile{Name: "B", OriginName: "B", Source: "deeplink"}
	_, activeID, saved, err := UpsertRoutingProfile(stored, in, "a", true)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if activeID != saved.ID {
		t.Errorf("activeID = %q, ждали новый %q", activeID, saved.ID)
	}
}

func TestUpsertLeavesActiveAloneWhenNotAsked(t *testing.T) {
	stored := []config.RoutingProfile{{ID: "a", Name: "A", OriginName: "A", Source: "manual"}}
	in := config.RoutingProfile{Name: "B", OriginName: "B", Source: "deeplink"}
	_, activeID, _, err := UpsertRoutingProfile(stored, in, "a", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if activeID != "a" {
		t.Errorf("activeID = %q, ждали a — фоновое обновление не меняет выбор", activeID)
	}
}

func TestUpsertRefusesPastTheCap(t *testing.T) {
	stored := make([]config.RoutingProfile, MaxRoutingProfiles)
	for i := range stored {
		name := fmt.Sprintf("профиль %d", i)
		stored[i] = config.RoutingProfile{
			ID: NewRoutingProfileID(), Name: name, OriginName: name, Source: "manual",
		}
	}
	in := config.RoutingProfile{Name: "ещё один", OriginName: "ещё один", Source: "deeplink"}
	if _, _, _, err := UpsertRoutingProfile(stored, in, "", false); err == nil {
		t.Fatal("профиль сверх лимита принят")
	}
	// Замена существующего под лимит не попадает: файл не растёт.
	again := stored[0]
	again.ProxySites = []string{"new.example"}
	out, _, _, err := UpsertRoutingProfile(stored, again, "", false)
	if err != nil {
		t.Fatalf("замена на полном хранилище отвергнута: %v", err)
	}
	if len(out) != MaxRoutingProfiles {
		t.Errorf("профилей %d, ждали %d", len(out), MaxRoutingProfiles)
	}
}

func TestUpsertDoesNotAliasCallerSlice(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "a", Name: "A", OriginName: "A", Source: "manual",
		ProxySites: []string{"old.example"},
	}}
	in := config.RoutingProfile{
		Name: "A", OriginName: "A", Source: "manual",
		ProxySites: []string{"new.example"},
	}
	if _, _, _, err := UpsertRoutingProfile(stored, in, "a", false); err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if stored[0].ProxySites[0] != "old.example" {
		t.Error("исходный срез изменён на месте")
	}
}

func TestNewRoutingProfileIDIsUniqueAndSafe(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 256; i++ {
		id := NewRoutingProfileID()
		if !ValidRoutingProfileID(id) {
			t.Fatalf("id %q не годится как имя файла", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("id %q выдан дважды", id)
		}
		seen[id] = struct{}{}
	}
}
