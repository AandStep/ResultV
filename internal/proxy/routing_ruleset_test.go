// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileRoutingSRSRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "abc123", "proxy")
	p := ParsedRoutingList{
		Domains:      []string{"example.com", "example.org"},
		ExactDomains: []string{"only.example.net"},
		CIDRs:        []string{"10.0.0.0/8"},
	}
	if err := CompileRoutingSRS(p, path); err != nil {
		t.Fatalf("CompileRoutingSRS: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("читаем результат: %v", err)
	}
	if err := validateSRS(data); err != nil {
		t.Fatalf("ядро не принимает свой же SRS: %v", err)
	}
	if !RoutingProfileSRSReady(dir, "abc123", "proxy") {
		t.Error("RoutingProfileSRSReady = false сразу после записи")
	}
}

func TestCompileRoutingSRSRejectsEmptyAndKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "abc123", "direct")
	good := ParsedRoutingList{Domains: []string{"example.com"}}
	if err := CompileRoutingSRS(good, path); err != nil {
		t.Fatalf("первая запись: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("читаем первую запись: %v", err)
	}
	if err := CompileRoutingSRS(ParsedRoutingList{}, path); err == nil {
		t.Fatal("пустой список записался, ждали ошибку")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("прежний файл исчез после неудачной записи: %v", err)
	}
	if string(before) != string(after) {
		t.Error("неудачная запись затёрла прежний SRS")
	}
}

func TestCompileRoutingSRSAcceptsExactOnlyList(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "exactonly", "block")
	p := ParsedRoutingList{ExactDomains: []string{"only.example.net"}}
	if err := CompileRoutingSRS(p, path); err != nil {
		t.Fatalf("список из одних точных имён отвергнут: %v", err)
	}
	if !RoutingProfileSRSReady(dir, "exactonly", "block") {
		t.Error("RoutingProfileSRSReady = false для точного списка")
	}
}

func TestCompileRoutingSRSAcceptsCIDROnlyList(t *testing.T) {
	// Действие profile.BlockIp может нести одни подсети и ни одного домена.
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "cidronly", "block")
	p := ParsedRoutingList{CIDRs: []string{"10.0.0.0/8", "2001:db8::/32"}}
	if err := CompileRoutingSRS(p, path); err != nil {
		t.Fatalf("список из одних подсетей отвергнут: %v", err)
	}
	if !RoutingProfileSRSReady(dir, "cidronly", "block") {
		t.Error("RoutingProfileSRSReady = false для списка подсетей")
	}
}

func TestRoutingProfileRuleSetTagStable(t *testing.T) {
	if got := RoutingProfileRuleSetTag("abc123", "proxy"); got != "prof-abc123-proxy" {
		t.Errorf("тег = %q, ждали prof-abc123-proxy", got)
	}
}

func TestRoutingProfileSRSPathUnderRoutingDir(t *testing.T) {
	got := RoutingProfileSRSPath("/data", "abc123", "block")
	want := filepath.Join("/data", "routing", "prof-abc123-block.srs")
	if got != want {
		t.Errorf("путь = %q, ждали %q", got, want)
	}
}

func TestValidRoutingProfileIDRejectsSeparators(t *testing.T) {
	for _, bad := range []string{"", "..", ".", "a/b", `a\b`, "a b", "../../etc/passwd", strings.Repeat("a", 65)} {
		if ValidRoutingProfileID(bad) {
			t.Errorf("ValidRoutingProfileID(%q) = true, ждали false", bad)
		}
	}
	for _, ok := range []string{"abc123", "0123456789abcdef", "a-b_c"} {
		if !ValidRoutingProfileID(ok) {
			t.Errorf("ValidRoutingProfileID(%q) = false, ждали true", ok)
		}
	}
}

func TestCompileRoutingSRSRefusesBadID(t *testing.T) {
	dir := t.TempDir()
	if err := CompileRoutingSRS(
		ParsedRoutingList{Domains: []string{"example.com"}},
		RoutingProfileSRSPath(dir, "../escape", "proxy"),
	); err == nil {
		t.Fatal("id с обходом каталога принят")
	}
}

func TestRemoveRoutingProfileSRSDeletesAllThree(t *testing.T) {
	dir := t.TempDir()
	for _, action := range RoutingActions {
		if err := CompileRoutingSRS(
			ParsedRoutingList{Domains: []string{"example.com"}},
			RoutingProfileSRSPath(dir, "gone", action),
		); err != nil {
			t.Fatalf("подготовка %s: %v", action, err)
		}
	}
	RemoveRoutingProfileSRS(dir, "gone")
	for _, action := range RoutingActions {
		if RoutingProfileSRSReady(dir, "gone", action) {
			t.Errorf("SRS для %s пережил удаление", action)
		}
	}
}

func TestRoutingProfileSRSReadyRejectsTruncatedFile(t *testing.T) {
	// Обрезанный SRS, на который ссылается local rule_set, валит старт ядра.
	// Значит "готов" обязан означать "ядро это прочитает", а не "файл есть".
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "broken", "proxy")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("подготовка каталога: %v", err)
	}
	if err := os.WriteFile(path, []byte("не SRS, но длиннее минимума в 32 байта — точно"), 0o600); err != nil {
		t.Fatalf("подготовка файла: %v", err)
	}
	if RoutingProfileSRSReady(dir, "broken", "proxy") {
		t.Fatal("мусор признан годным rule_set")
	}
	// И самолечение: негодный файл убран, чтобы следующая сборка писала чистым.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("негодный файл остался на диске")
	}
}

// Точное имя не должно ловить поддомены. Слей ExactDomains с Domains — и
// правило молча расширится на всё поддерево, а автор списка писал ровно
// обратное: `full:` для того и существует.
func TestCompileRoutingSRSKeepsExactDomainsApart(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "apart", "proxy")
	p := ParsedRoutingList{
		Domains:      []string{"suffix.example"},
		ExactDomains: []string{"exact.example"},
	}
	if err := CompileRoutingSRS(p, path); err != nil {
		t.Fatalf("CompileRoutingSRS: %v", err)
	}
	m, err := LoadRoutingDomainMatcher(path)
	if err != nil {
		t.Fatalf("LoadRoutingDomainMatcher: %v", err)
	}
	if !m.Match("exact.example") {
		t.Error("точное имя не совпало само с собой")
	}
	if m.Match("sub.exact.example") {
		t.Error("точное имя поймало поддомен — правило расширилось молча")
	}
	if !m.Match("suffix.example") {
		t.Error("суффикс не поймал сам хост")
	}
	if !m.Match("sub.suffix.example") {
		t.Error("суффикс не поймал поддомен")
	}
}
