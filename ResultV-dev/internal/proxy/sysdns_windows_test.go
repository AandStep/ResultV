// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

//go:build windows

package proxy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseAdapterDNS_SingleObject(t *testing.T) {
	raw := []byte(`{"InterfaceIndex":12,"InterfaceAlias":"Ethernet","ServerAddresses":["192.168.1.1","8.8.8.8"]}`)
	got, err := parseAdapterDNS(raw)
	if err != nil {
		t.Fatalf("parseAdapterDNS: %v", err)
	}
	want := []adapterDNS{{InterfaceIndex: 12, InterfaceAlias: "Ethernet", ServerAddresses: []string{"192.168.1.1", "8.8.8.8"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseAdapterDNS_Array(t *testing.T) {
	raw := []byte(`[{"InterfaceIndex":12,"InterfaceAlias":"Ethernet","ServerAddresses":["1.1.1.1"]},` +
		`{"InterfaceIndex":15,"InterfaceAlias":"Wi-Fi","ServerAddresses":["8.8.8.8"]}]`)
	got, err := parseAdapterDNS(raw)
	if err != nil {
		t.Fatalf("parseAdapterDNS: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 adapters, got %d", len(got))
	}
}

func TestParseAdapterDNS_EmptyOutput(t *testing.T) {
	got, err := parseAdapterDNS(nil)
	if err != nil {
		t.Fatalf("parseAdapterDNS: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestParseAdapterDNS_HandlesUTF16BOM(t *testing.T) {
	raw := append([]byte{0xFF, 0xFE}, []byte(`{"InterfaceIndex":1,"InterfaceAlias":"x","ServerAddresses":["1.1.1.1"]}`)...)
	got, err := parseAdapterDNS(raw)
	if err != nil {
		t.Fatalf("parseAdapterDNS: %v", err)
	}
	if len(got) != 1 || got[0].InterfaceAlias != "x" {
		t.Fatalf("BOM stripping failed: %+v", got)
	}
}

func TestParseAdapterDNS_HandlesUTF8BOM(t *testing.T) {
	raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"InterfaceIndex":1,"InterfaceAlias":"x","ServerAddresses":["1.1.1.1"]}`)...)
	got, err := parseAdapterDNS(raw)
	if err != nil {
		t.Fatalf("parseAdapterDNS: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("BOM stripping failed: %+v", got)
	}
}

func TestIsSafeDNSToken(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1.1.1.1", true},
		{"8.8.8.8", true},
		{"2606:4700:4700::1111", true},
		{"", false},
		{"1.1.1.1; rm -rf", false},
		{"`whoami`", false},
		{"$(echo)", false},
		{"localhost", false},
	}
	for _, c := range cases {
		if got := isSafeDNSToken(c.in); got != c.want {
			t.Errorf("isSafeDNSToken(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestWriteSnapshot_AtomicRename(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "snap.json")

	in := []adapterDNS{
		{InterfaceIndex: 5, InterfaceAlias: "Ethernet", ServerAddresses: []string{"192.168.1.1"}},
		{InterfaceIndex: 9, InterfaceAlias: "Wi-Fi", ServerAddresses: nil},
	}
	if err := writeSnapshot(path, in); err != nil {
		t.Fatalf("writeSnapshot: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var got dnsSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if !reflect.DeepEqual(got.Adapters, in) {
		t.Fatalf("roundtrip mismatch:\n got=%+v\nwant=%+v", got.Adapters, in)
	}

	if entries, _ := os.ReadDir(tmp); len(entries) != 1 {
		t.Fatalf("expected only the final snapshot in tmp dir, got entries=%v", entries)
	}
}

func TestRestore_NoSnapshot_IsNoop(t *testing.T) {
	w := &windowsSystemDNS{snapshotPath: filepath.Join(t.TempDir(), "missing.json")}
	if err := w.Restore(); err != nil {
		t.Fatalf("expected no error when snapshot is absent, got: %v", err)
	}
}

func TestRestore_CorruptSnapshotDropsFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "snap.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := &windowsSystemDNS{snapshotPath: path}
	if err := w.Restore(); err == nil {
		t.Fatal("expected parse error for corrupt snapshot")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected corrupt snapshot to be deleted, stat err=%v", err)
	}
}

func TestFilterPhysicalAdapterDNS_DropsTun0(t *testing.T) {
	in := []adapterDNS{
		{InterfaceIndex: 3, InterfaceAlias: "tun0", ServerAddresses: []string{"1.1.1.1"}},
		{InterfaceIndex: 12, InterfaceAlias: "Ethernet", ServerAddresses: []string{"192.168.1.1"}},
	}
	got := filterPhysicalAdapterDNS(in)
	if len(got) != 1 || got[0].InterfaceAlias != "Ethernet" {
		t.Fatalf("got %+v, want only Ethernet", got)
	}
}

func TestIsPermissionDeniedError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"powershell: exit status 1 (out=PermissionDenied: ... Set-DnsClientServerAddress)", true},
		{"elevated powershell: access is denied", true},
		{"отказано в доступе", true},
		{"restore dns on \"Ethernet\": timeout", false},
	}
	for _, c := range cases {
		if got := isPermissionDeniedError(errors.New(c.msg)); got != c.want {
			t.Errorf("isPermissionDeniedError(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
	if !isPermissionDeniedError(ErrDNSRequiresAdmin) {
		t.Fatal("expected ErrDNSRequiresAdmin to be detected")
	}
}

func TestOverride_RequiresAdmin(t *testing.T) {
	prev := dnsAdminCheck
	dnsAdminCheck = func() bool { return false }
	defer func() { dnsAdminCheck = prev }()

	w := &windowsSystemDNS{snapshotPath: filepath.Join(t.TempDir(), "snap.json")}
	err := w.Override([]string{"1.1.1.1"})
	if !errors.Is(err, ErrDNSRequiresAdmin) {
		t.Fatalf("expected ErrDNSRequiresAdmin, got %v", err)
	}
	if _, err := os.Stat(w.snapshotPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot must not be written without admin; stat err=%v", err)
	}
}

func TestBuildRestorePSScript_SkipsTunnelAdapters(t *testing.T) {
	script := buildRestorePSScript([]adapterDNS{
		{InterfaceIndex: 3, InterfaceAlias: "tun0", ServerAddresses: []string{"1.1.1.1"}},
		{InterfaceIndex: 14, InterfaceAlias: "Ethernet", ServerAddresses: []string{"192.168.1.1"}},
	})
	if strings.Contains(script, "InterfaceIndex 3") {
		t.Fatalf("tun0 must be skipped, script=%q", script)
	}
	if !strings.Contains(script, "InterfaceIndex 14") {
		t.Fatalf("Ethernet must be present, script=%q", script)
	}
}

func TestIsAdapterGoneError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"powershell: exit status 1 (out=CmdletizationQuery_NotFound_InterfaceIndex)", true},
		{"powershell: exit status 1 (out=ObjectNotFound: (3:UInt32))", true},
		{"restore dns on \"Ethernet\": timeout", false},
	}
	for _, c := range cases {
		if got := isAdapterGoneError(errors.New(c.msg)); got != c.want {
			t.Errorf("isAdapterGoneError(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestOverride_EmptyServerListErrors(t *testing.T) {
	w := &windowsSystemDNS{snapshotPath: filepath.Join(t.TempDir(), "snap.json")}
	if err := w.Override(nil); err == nil {
		t.Fatal("expected error for empty server list")
	}
	if _, err := os.Stat(w.snapshotPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot must not be written when args are invalid; stat err=%v", err)
	}
}
