// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"resultproxy-wails/internal/proxy"
)

const routingTestURI = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&type=tcp#routing"

// compileProfileForRules собирает SRS всех трёх действий, чтобы правилам было
// на что ссылаться: эмиссия пропускает действие без готового файла.
func compileProfileForRules(t *testing.T, dir, id string) {
	t.Helper()
	profile := `{"id":"` + id + `","name":"R",` +
		`"directSites":["direct.example"],` +
		`"proxySites":["proxy.example"],` +
		`"blockSites":["block.example"]}`
	if _, err := CompileRoutingProfile(profile, dir, false); err != nil {
		t.Fatalf("подготовка SRS: %v", err)
	}
}

func buildConfigWithProfile(t *testing.T, dir string, opts BuildOptions) map[string]any {
	t.Helper()
	out, err := BuildSingBoxConfigV2(routingTestURI, dir, encodeOptions(opts))
	if err != nil {
		t.Fatalf("BuildSingBoxConfigV2: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("конфиг не JSON: %v", err)
	}
	return cfg
}

func routeRules(t *testing.T, cfg map[string]any) []map[string]any {
	t.Helper()
	route, ok := cfg["route"].(map[string]any)
	if !ok {
		t.Fatal("в конфиге нет route")
	}
	raw, _ := route["rules"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func ruleSetTags(t *testing.T, cfg map[string]any) []string {
	t.Helper()
	route, _ := cfg["route"].(map[string]any)
	raw, _ := route["rule_set"].([]any)
	var out []string
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			if tag, ok := m["tag"].(string); ok {
				out = append(out, tag)
			}
		}
	}
	return out
}

// indexOfProfileRule возвращает позицию первого правила, ссылающегося на тег,
// и -1 если такого нет.
func indexOfProfileRule(rules []map[string]any, tag string) int {
	for i, r := range rules {
		set, _ := r["rule_set"].([]any)
		for _, s := range set {
			if s == tag {
				return i
			}
		}
	}
	return -1
}

func TestProfileRulesEmittedInGlobal(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "ruleprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{RoutingProfileID: "ruleprof"})

	tags := ruleSetTags(t, cfg)
	for _, action := range proxy.RoutingActions {
		want := proxy.RoutingProfileRuleSetTag("ruleprof", action)
		found := false
		for _, tag := range tags {
			if tag == want {
				found = true
			}
		}
		if !found {
			t.Errorf("rule_set %q не зарегистрирован: %v", want, tags)
		}
	}

	rules := routeRules(t, cfg)
	for _, action := range proxy.RoutingActions {
		tag := proxy.RoutingProfileRuleSetTag("ruleprof", action)
		if indexOfProfileRule(rules, tag) < 0 {
			t.Errorf("нет правила для %q", tag)
		}
	}
}

func TestProfileRulesAbsentWithoutProfileID(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "unused")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{})
	for _, tag := range ruleSetTags(t, cfg) {
		if strings.HasPrefix(tag, "prof-") {
			t.Errorf("rule_set %q зарегистрирован без выбранного профиля", tag)
		}
	}
}

func TestProfileRulesIgnoredInSmart(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "smartprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "smartprof",
		SmartMode:        true,
	})
	rules := routeRules(t, cfg)
	for _, action := range proxy.RoutingActions {
		tag := proxy.RoutingProfileRuleSetTag("smartprof", action)
		if indexOfProfileRule(rules, tag) >= 0 {
			t.Errorf("правило %q попало в Smart — профиль там не действует", tag)
		}
	}
	for _, tag := range ruleSetTags(t, cfg) {
		if strings.HasPrefix(tag, "prof-") {
			t.Errorf("rule_set %q зарегистрирован в Smart", tag)
		}
	}
}

func TestProfileRulesSkipActionsWithoutSRS(t *testing.T) {
	dir := t.TempDir()
	// Только proxy — у двух других действий правил нет, файлов не будет.
	profile := `{"id":"partial","name":"P","proxySites":["proxy.example"]}`
	if _, err := CompileRoutingProfile(profile, dir, false); err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	cfg := buildConfigWithProfile(t, dir, BuildOptions{RoutingProfileID: "partial"})
	for _, tag := range ruleSetTags(t, cfg) {
		if tag == proxy.RoutingProfileRuleSetTag("partial", "direct") ||
			tag == proxy.RoutingProfileRuleSetTag("partial", "block") {
			t.Errorf("зарегистрирован rule_set без файла: %q", tag)
		}
	}
	rules := routeRules(t, cfg)
	if indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("partial", "proxy")) < 0 {
		t.Error("правило proxy потеряно")
	}
}

func TestProfileRulesComeAfterExcludedDomains(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "orderprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "orderprof",
		ExcludedDomains:  "bank.example",
	})
	rules := routeRules(t, cfg)

	excluded := -1
	for i, r := range rules {
		suffixes, _ := r["domain_suffix"].([]any)
		for _, s := range suffixes {
			if s == "bank.example" {
				excluded = i
			}
		}
	}
	if excluded < 0 {
		t.Fatal("правило исключения не найдено")
	}
	profileAt := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("orderprof", "proxy"))
	if profileAt < 0 {
		t.Fatal("правило профиля не найдено")
	}
	if profileAt < excluded {
		t.Errorf("правило профиля (%d) стоит выше исключения (%d) — "+
			"вручную добавленный домен должен быть сильнее", profileAt, excluded)
	}
}

func TestProfileRuleOrderHonoursRouteOrder(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "roprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "roprof",
		RoutingOrder:     "direct-proxy-block",
	})
	rules := routeRules(t, cfg)
	d := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("roprof", "direct"))
	p := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("roprof", "proxy"))
	b := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("roprof", "block"))
	if d < 0 || p < 0 || b < 0 {
		t.Fatalf("не все правила на месте: direct=%d proxy=%d block=%d", d, p, b)
	}
	if !(d < p && p < b) {
		t.Errorf("порядок direct=%d proxy=%d block=%d, ждали direct<proxy<block", d, p, b)
	}
}

func TestProfileRuleOrderFallsBackOnGarbage(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "badorder")
	for _, bad := range []string{"proxy-proxy-direct", "whatever", "block-proxy", ""} {
		cfg := buildConfigWithProfile(t, dir, BuildOptions{
			RoutingProfileID: "badorder",
			RoutingOrder:     bad,
		})
		rules := routeRules(t, cfg)
		b := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("badorder", "block"))
		p := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("badorder", "proxy"))
		d := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("badorder", "direct"))
		if !(b < p && p < d) {
			t.Errorf("порядок %q дал block=%d proxy=%d direct=%d, ждали умолчание block<proxy<direct",
				bad, b, p, d)
		}
	}
}

func TestProfileBlockRuleRejects(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "blockprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{RoutingProfileID: "blockprof"})
	rules := routeRules(t, cfg)
	at := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("blockprof", "block"))
	if at < 0 {
		t.Fatal("правило block не найдено")
	}
	if rules[at]["action"] != "reject" {
		t.Errorf("action = %v, ждали reject", rules[at]["action"])
	}
	if _, has := rules[at]["outbound"]; has {
		t.Error("у reject-правила остался outbound")
	}
}

func TestProfileRulesRejectedInKillSwitchPanic(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "panicprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "panicprof",
		KillSwitchArmed:  true,
		KillSwitchPanic:  true,
	})
	rules := routeRules(t, cfg)
	for _, action := range proxy.RoutingActions {
		at := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("panicprof", action))
		if at < 0 {
			t.Fatalf("правило %s исчезло в panic", action)
		}
		if rules[at]["action"] != "reject" {
			t.Errorf("в panic правило %s = %v, ждали reject", action, rules[at]["action"])
		}
	}
}

func TestProfileConfigAcceptedByPinnedCore(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "coreprof")
	for _, tc := range []struct {
		name string
		opts BuildOptions
	}{
		{"обычный", BuildOptions{RoutingProfileID: "coreprof"}},
		{"порядок", BuildOptions{RoutingProfileID: "coreprof", RoutingOrder: "direct-proxy-block"}},
		{"с исключениями", BuildOptions{RoutingProfileID: "coreprof", ExcludedDomains: "bank.example"}},
		{"panic", BuildOptions{RoutingProfileID: "coreprof", KillSwitchArmed: true, KillSwitchPanic: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := BuildSingBoxConfigV2(routingTestURI, dir, encodeOptions(tc.opts))
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			// Ядро декодирует с DisallowUnknownFields и проверяет опции
			// действия по действию: выдуманный ключ здесь — мёртвый движок,
			// а не проигнорированная ручка.
			var parsed option.Options
			ctx := include.Context(context.Background())
			if err := singjson.UnmarshalContext(ctx, []byte(out), &parsed); err != nil {
				t.Fatalf("закреплённое ядро отвергло конфиг: %v\nconfig: %s", err, out)
			}
		})
	}
}
