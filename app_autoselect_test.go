// Copyright (C) 2026 ResultV
//
// Licensed under the terms of the GNU General Public License v3 or later.

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatAutoMemberTable_ListsEveryMemberWithRTTAndReason(t *testing.T) {
	rows := []autoMemberProbe{
		{Name: "DE #1", Addr: "1.2.3.4:443", Type: "VLESS", RTTms: 42, OK: true},
		{Name: "RU #2", Addr: "5.6.7.8:443", Type: "TROJAN", RTTms: 1, OK: true},
		{Name: "NL #3", Addr: "9.9.9.9:443", Type: "VLESS", OK: false, Reason: "timeout"},
	}

	got := formatAutoMemberTable("impVPN Auto", rows)

	if len(got) != 4 {
		t.Fatalf("ожидали заголовок + 3 строки, получили %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "impVPN Auto") || !strings.Contains(got[0], "3") {
		t.Errorf("заголовок должен называть группу и число членов, получили %q", got[0])
	}
	if !strings.Contains(got[2], "RU #2") || !strings.Contains(got[2], "1ms") {
		t.Errorf("строка члена должна содержать имя и RTT, получили %q", got[2])
	}
	if !strings.Contains(got[3], "timeout") {
		t.Errorf("недоступный член должен показывать reason, получили %q", got[3])
	}
}

func TestFormatAutoMemberTable_EmptyMembersStillReportsGroup(t *testing.T) {
	got := formatAutoMemberTable("Auto", nil)
	if len(got) != 1 {
		t.Fatalf("ожидали только заголовок, получили %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "0") {
		t.Errorf("заголовок должен сообщать 0 членов, получили %q", got[0])
	}
}

func TestExtractAutoMembers_ParsesBothExtraEncodings(t *testing.T) {
	plain := json.RawMessage(`{"members":["m1","m2"]}`)
	if got := extractAutoMembers(plain); len(got) != 2 || got[0] != "m1" {
		t.Errorf("прямой JSON: ожидали [m1 m2], получили %v", got)
	}

	quoted := json.RawMessage(`"{\"members\":[\"m3\"]}"`)
	if got := extractAutoMembers(quoted); len(got) != 1 || got[0] != "m3" {
		t.Errorf("JSON-в-строке: ожидали [m3], получили %v", got)
	}

	if got := extractAutoMembers(nil); len(got) != 0 {
		t.Errorf("пустой extra: ожидали пусто, получили %v", got)
	}
}
