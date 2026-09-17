// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"encoding/json"
	"os"
	"testing"

	"resultproxy-wails/internal/proxy"
)

type adaptiveSmartView struct {
	Inbounds []struct {
		Type       string `json:"type"`
		Tag        string `json:"tag"`
		Listen     string `json:"listen"`
		ListenPort int    `json:"listen_port"`
	} `json:"inbounds"`
	Outbounds []struct {
		Type       string `json:"type"`
		Tag        string `json:"tag"`
		Server     string `json:"server"`
		ServerPort int    `json:"server_port"`
	} `json:"outbounds"`
	Route struct {
		Rules []struct {
			Inbound  []string `json:"inbound"`
			Network  []string `json:"network"`
			RuleSet  []string `json:"rule_set"`
			Action   string   `json:"action"`
			Outbound string   `json:"outbound"`
		} `json:"rules"`
		RuleSet []struct {
			Type   string `json:"type"`
			Tag    string `json:"tag"`
			Format string `json:"format"`
			Path   string `json:"path"`
		} `json:"rule_set"`
		Final string `json:"final"`
	} `json:"route"`
}

func buildAdaptive(t *testing.T, opts BuildOptions) adaptiveSmartView {
	t.Helper()
	b, _ := json.Marshal(opts)
	cfg, err := BuildSingBoxConfigFromEntryV2(entryFixture, t.TempDir(), string(b))
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	var parsed adaptiveSmartView
	if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return parsed
}

func smartOpts() BuildOptions {
	return BuildOptions{SmartMode: true, AdaptiveSmart: true}
}

func TestAdaptiveSmart_AddsRaceInboundAndRelayOutbound(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())

	var foundIn bool
	for _, in := range cfg.Inbounds {
		if in.Tag == "smart-race-in" {
			foundIn = true
			if in.Type != "mixed" {
				t.Errorf("инбаунд гонки type = %q, ожидался mixed (он же отвечает проберу по CONNECT)", in.Type)
			}
			if in.Listen != "127.0.0.1" || in.ListenPort != SmartRaceInboundPort {
				t.Errorf("инбаунд гонки на %s:%d, ожидался 127.0.0.1:%d", in.Listen, in.ListenPort, SmartRaceInboundPort)
			}
		}
	}
	if !foundIn {
		t.Fatalf("инбаунда smart-race-in нет: %+v", cfg.Inbounds)
	}

	var foundOut bool
	for _, out := range cfg.Outbounds {
		if out.Tag == "smart-relay" {
			foundOut = true
			if out.Type != "http" {
				t.Errorf("аутбаунд реле type = %q, ожидался http", out.Type)
			}
			if out.Server != "127.0.0.1" || out.ServerPort != SmartRelayPort {
				t.Errorf("аутбаунд реле на %s:%d, ожидался 127.0.0.1:%d", out.Server, out.ServerPort, SmartRelayPort)
			}
		}
	}
	if !foundOut {
		t.Fatalf("аутбаунда smart-relay нет: %+v", cfg.Outbounds)
	}
}

// Правило туннельной ноги обязано стоять ДО правила, отправляющего остаток в
// реле. Иначе нога вернётся в реле и соединение закольцуется.
func TestAdaptiveSmart_TunnelLegRuleComesBeforeRelayRule(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())

	legIdx, relayIdx := -1, -1
	for i, r := range cfg.Route.Rules {
		if len(r.Inbound) == 1 && r.Inbound[0] == "smart-race-in" {
			legIdx = i
			if r.Outbound != "proxy" {
				t.Errorf("туннельная нога уходит в %q, ожидался proxy", r.Outbound)
			}
		}
		if r.Outbound == "smart-relay" {
			relayIdx = i
		}
	}
	if legIdx < 0 {
		t.Fatalf("правила туннельной ноги нет: %+v", cfg.Route.Rules)
	}
	if relayIdx < 0 {
		t.Fatalf("правила «остальное в реле» нет: %+v", cfg.Route.Rules)
	}
	if legIdx >= relayIdx {
		t.Fatalf("нога на позиции %d, реле на %d — нога обязана быть раньше, иначе петля", legIdx, relayIdx)
	}
}

// Выученное «ходит напрямую» решается правилом, а не реле: правило rule_set
// стоит перед правилом реле.
func TestAdaptiveSmart_LearnedDirectRuleComesBeforeRelayRule(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())

	directIdx, relayIdx := -1, -1
	for i, r := range cfg.Route.Rules {
		if len(r.RuleSet) == 1 && r.RuleSet[0] == "verdict-direct" {
			directIdx = i
			if r.Outbound != "direct" {
				t.Errorf("выученное прямое уходит в %q, ожидался direct", r.Outbound)
			}
		}
		if r.Outbound == "smart-relay" {
			relayIdx = i
		}
	}
	if directIdx < 0 || relayIdx < 0 || directIdx >= relayIdx {
		t.Fatalf("порядок неверен: verdict-direct=%d, smart-relay=%d", directIdx, relayIdx)
	}
}

// Реле забирает только TCP: гонки по UDP нет, и QUIC продолжает ходить как
// ходил.
func TestAdaptiveSmart_RelayRuleTakesTCPOnly(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())
	for _, r := range cfg.Route.Rules {
		if r.Outbound != "smart-relay" {
			continue
		}
		if len(r.Network) != 1 || r.Network[0] != "tcp" {
			t.Fatalf("правило реле ловит network=%v, ожидался только tcp", r.Network)
		}
		return
	}
	t.Fatal("правила реле нет")
}

// Файл rule-set обязан существовать к моменту старта ядра, иначе
// NewLocalRuleSet роняет запуск. Сборка конфига его и создаёт.
func TestAdaptiveSmart_RuleSetFileIsCreatedByConfigBuild(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(smartOpts())
	if _, err := BuildSingBoxConfigFromEntryV2(entryFixture, dir, string(b)); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(proxy.SmartDirectSetPath(dir)); err != nil {
		t.Fatalf("после сборки конфига файла rule-set нет — ядро не стартует: %v", err)
	}
}

// Выключенный тумблер не оставляет в конфиге ни инбаунда, ни аутбаунда, ни
// правил: выключено значит выключено.
func TestAdaptiveSmart_Off_LeavesConfigUntouched(t *testing.T) {
	cfg := buildAdaptive(t, BuildOptions{SmartMode: true})
	for _, in := range cfg.Inbounds {
		if in.Tag == "smart-race-in" {
			t.Fatal("инбаунд гонки при выключенном тумблере")
		}
	}
	for _, out := range cfg.Outbounds {
		if out.Tag == "smart-relay" {
			t.Fatal("аутбаунд реле при выключенном тумблере")
		}
	}
	for _, r := range cfg.Route.Rules {
		if r.Outbound == "smart-relay" {
			t.Fatal("правило реле при выключенном тумблере")
		}
	}
}

// Вне Smart-режима фича не работает: в Global всё и так идёт через туннель,
// а гонять гонку было бы чистой тратой.
func TestAdaptiveSmart_OutsideSmartMode_IsInert(t *testing.T) {
	cfg := buildAdaptive(t, BuildOptions{AdaptiveSmart: true})
	for _, r := range cfg.Route.Rules {
		if r.Outbound == "smart-relay" {
			t.Fatal("правило реле вне Smart-режима")
		}
	}
}

// Правило реле обязано быть ПОСЛЕДНИМ в списке. Встань оно раньше Smart-списка
// или правил ad-block — проглотило бы и список, и режущие правила, то есть
// отменило бы обе работающие фичи разом.
//
// Проверяется именно «последнее», а не «позже вон того»: сравнение с
// конкретным правилом проходит вхолостую, когда этого правила в конфиге нет
// (скомпилированного Smart-списка в t.TempDir() заведомо нет), и тест зеленеет,
// ничего не доказав.
func TestAdaptiveSmart_RelayRuleIsLast(t *testing.T) {
	opts := smartOpts()
	opts.BlockedDomains = "ads.example"
	cfg := buildAdaptive(t, opts)

	if len(cfg.Route.Rules) == 0 {
		t.Fatal("правил нет вовсе")
	}
	last := cfg.Route.Rules[len(cfg.Route.Rules)-1]
	if last.Outbound != "smart-relay" {
		t.Fatalf("последнее правило уходит в %q, ожидался smart-relay; всё: %+v", last.Outbound, cfg.Route.Rules)
	}
	// И сразу перед ним — выученные «прямые»: их решает правило, а не реле.
	prev := cfg.Route.Rules[len(cfg.Route.Rules)-2]
	if len(prev.RuleSet) != 1 || prev.RuleSet[0] != "verdict-direct" {
		t.Fatalf("перед реле стоит %+v, ожидалось правило verdict-direct", prev)
	}
}

// Кил-свитч в состоянии «сработал» обязан перекрыть и путь через реле.
// Прямая нога реле идёт по настоящей сети пользователя, мимо туннеля: пережив
// панику, правило реле пустило бы трафик именно тогда, когда его обязано не
// быть.
func TestAdaptiveSmart_KillSwitchPanic_RejectsRelayRule(t *testing.T) {
	opts := smartOpts()
	opts.KillSwitchArmed = true
	opts.KillSwitchPanic = true
	cfg := buildAdaptive(t, opts)

	for i, r := range cfg.Route.Rules {
		if r.Outbound == "smart-relay" {
			t.Fatalf("правило %d ведёт в smart-relay при сработавшем кил-свитче: %+v", i, r)
		}
	}
	if cfg.Route.Final != "block" {
		t.Fatalf("route.final = %q, ожидался block", cfg.Route.Final)
	}
}

// А во взведённом, но не сработавшем состоянии реле работает как обычно:
// взведённый кил-свитч — это только наблюдение, маршрутизацию он не трогает.
func TestAdaptiveSmart_KillSwitchArmedOnly_KeepsRelayRule(t *testing.T) {
	opts := smartOpts()
	opts.KillSwitchArmed = true
	cfg := buildAdaptive(t, opts)

	for _, r := range cfg.Route.Rules {
		if r.Outbound == "smart-relay" {
			return
		}
	}
	t.Fatalf("правила реле нет при взведённом (но не сработавшем) кил-свитче: %+v", cfg.Route.Rules)
}
