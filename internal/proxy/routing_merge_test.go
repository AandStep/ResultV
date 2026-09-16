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

// Правка из редактора приходит с УЖЕ известным id, и она обязана мочь сменить
// имя. Сопоставление по имени издателя, которое бережёт переименование от
// повторной публикации, для неё не годится: оно вернуло бы прежнее имя, и
// переименовать профиль стало бы нечем.
func TestUpsertByIDLetsTheEditorRename(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID:         "edit1",
		Name:       "Старое имя",
		OriginName: "Panel A",
		Source:     "deeplink",
		ProxySites: []string{"old.example"},
	}}
	edited := stored[0]
	edited.Name = "Новое имя"
	edited.ProxySites = []string{"new.example"}

	out, _, saved, err := UpsertRoutingProfile(stored, edited, "edit1", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профилей %d, ждали 1", len(out))
	}
	if saved.Name != "Новое имя" {
		t.Errorf("имя = %q, правка не применилась", saved.Name)
	}
	if saved.ProxySites[0] != "new.example" {
		t.Errorf("правила не обновились: %v", saved.ProxySites)
	}
}

// Происхождение редактору не принадлежит: профиль из подписки остаётся её
// профилем, чтобы следующая синхронизация его узнала. Вместе с ним остаются
// OriginName и ссылки на списки — их в редакторе нет, и потерять их он не
// должен.
func TestUpsertByIDKeepsProvenance(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID:             "sub1",
		Name:           "impVPN",
		OriginName:     "impVPN",
		Source:         "subscription",
		SubscriptionID: "s1",
		ListURLs:       map[string][]string{"proxy": {"https://panel.example/l.txt"}},
		AllowInsecure:  true,
		ProxySites:     []string{"old.example"},
	}}
	edited := config.RoutingProfile{
		ID:         "sub1",
		Name:       "Моё имя",
		Source:     "manual", // редактор не знает происхождения
		ProxySites: []string{"new.example"},
	}

	_, _, saved, err := UpsertRoutingProfile(stored, edited, "sub1", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if saved.Name != "Моё имя" {
		t.Errorf("имя не сменилось: %q", saved.Name)
	}
	if saved.Source != "subscription" || saved.SubscriptionID != "s1" {
		t.Errorf("происхождение переписано: %q / %q", saved.Source, saved.SubscriptionID)
	}
	if saved.OriginName != "impVPN" {
		t.Errorf("OriginName переписан: %q", saved.OriginName)
	}
	if len(saved.ListURLs["proxy"]) != 1 {
		t.Errorf("ссылки на списки потеряны: %v", saved.ListURLs)
	}
	if !saved.AllowInsecure {
		t.Error("согласие на plaintext потеряно")
	}
}

// А вот повторная публикация приходит БЕЗ id — и там прежнее имя обязано
// уцелеть. Это две разные операции, и путать их нельзя.
func TestUpsertWithoutIDStillKeepsUserRename(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "keepme", Name: "Моя маршрутизация", OriginName: "Panel A", Source: "deeplink",
		ProxySites: []string{"old.example"},
	}}
	republished := config.RoutingProfile{
		Name: "Panel A", OriginName: "Panel A", Source: "deeplink",
		ProxySites: []string{"new.example"},
	}
	_, _, saved, err := UpsertRoutingProfile(stored, republished, "keepme", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if saved.Name != "Моя маршрутизация" {
		t.Errorf("переименование затёрто публикацией: %q", saved.Name)
	}
}

// Профиль подписки опознаётся по её id, а не по имени издателя.
//
// На Android имя издателя не стабильно: панель отдаёт Profile-Title как
// `base64:<UTF-8>`, Go кладёт его в профиль как есть, а раскодирует Kotlin —
// и сохранённое «base64:…» перестаёт совпадать с пришедшим «🚀 impVPN».
// Сравнивай слияние имена — и синхронизация завела бы второй профиль ровно
// тогда, когда панель переименовалась.
func TestUpsertMatchesSubscriptionByIDNotHandle(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "sub-prof", Name: "base64:8J+agCBpbX", OriginName: "base64:8J+agCBpbX",
		Source: "subscription", SubscriptionID: "one",
		ListURLs: map[string][]string{"proxy": {"https://panel.example/old.txt"}},
	}}
	in := config.RoutingProfile{
		Name: "🚀 impVPN", OriginName: "🚀 impVPN",
		Source: "subscription", SubscriptionID: "one",
		ListURLs: map[string][]string{"proxy": {"https://panel.example/new.txt"}},
	}
	out, _, saved, err := UpsertRoutingProfile(stored, in, "sub-prof", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профиль раздвоился: %d записей", len(out))
	}
	if saved.ID != "sub-prof" {
		t.Errorf("id сменился на %q", saved.ID)
	}
	// Имя издателя обязано обновиться: иначе «base64:…» осталось бы в
	// хранилище навсегда и вечно выглядело переименованием.
	if saved.OriginName != "🚀 impVPN" {
		t.Errorf("имя издателя = %q, ждали свежее", saved.OriginName)
	}
	// Ссылки на списки принадлежат подписке, а не сохранённой копии: панель
	// их меняет, и приложение обязано качать новые.
	if got := saved.ListURLs["proxy"]; len(got) != 1 || got[0] != "https://panel.example/new.txt" {
		t.Errorf("ссылки proxy = %v, ждали свежие", got)
	}
}

// Пользователь не переименовывал — имя берётся свежее. Сохранённое имя,
// совпадающее с именем издателя, это имя издателя, а не выбор пользователя.
func TestUpsertTakesFreshNameWhenNeverRenamed(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "sub-prof", Name: "base64:8J+agCBpbX", OriginName: "base64:8J+agCBpbX",
		Source: "subscription", SubscriptionID: "one",
	}}
	in := config.RoutingProfile{
		Name: "🚀 impVPN", OriginName: "🚀 impVPN",
		Source: "subscription", SubscriptionID: "one",
	}
	_, _, saved, err := UpsertRoutingProfile(stored, in, "sub-prof", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if saved.Name != "🚀 impVPN" {
		t.Errorf("имя = %q, ждали свежее от панели", saved.Name)
	}
}

// А переименовал — имя его, и синхронизация подписки его не трогает.
func TestUpsertKeepsSubscriptionRenameAcrossSync(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "sub-prof", Name: "Моя маршрутизация", OriginName: "impVPN Базовый",
		Source: "subscription", SubscriptionID: "one",
	}}
	in := config.RoutingProfile{
		Name: "impVPN Премиум", OriginName: "impVPN Премиум",
		Source: "subscription", SubscriptionID: "one",
	}
	_, _, saved, err := UpsertRoutingProfile(stored, in, "sub-prof", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if saved.Name != "Моя маршрутизация" {
		t.Errorf("переименование затёрто синхронизацией: %q", saved.Name)
	}
	// Имя издателя при этом обновляется: оно опознавательный знак, а не
	// то, что видит пользователь.
	if saved.OriginName != "impVPN Премиум" {
		t.Errorf("имя издателя = %q, ждали свежее", saved.OriginName)
	}
}

// Пустое имя в пришедшем профиле не должно стирать видимое имя.
func TestUpsertDoesNotBlankNameOnRepublish(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "sub-prof", Name: "impVPN", OriginName: "impVPN",
		Source: "subscription", SubscriptionID: "one",
	}}
	in := config.RoutingProfile{
		Name: "", OriginName: "", Source: "subscription", SubscriptionID: "one",
	}
	_, _, saved, err := UpsertRoutingProfile(stored, in, "sub-prof", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if saved.Name != "impVPN" {
		t.Errorf("имя стёрто: %q", saved.Name)
	}
}
