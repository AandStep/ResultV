package proxy

import (
	"testing"
	"resultproxy-wails/internal/config"
)

func TestExtractAutoGroupName(t *testing.T) {
	cases := []struct{
		desc    string
		entries []config.ProxyEntry
		wantOK  bool
		wantName string
	}{
		{
			desc: "space-appended protocol",
			entries: []config.ProxyEntry{
				{Name: "🇨🇦 impVPN Auto VLESS + Reality + gRPC"},
				{Name: "🇨🇦 impVPN Auto HYSTERIA2"},
				{Name: "🇩🇪 impVPN Auto TROJAN + Reality"},
				{Name: "🇨🇦 impVPN Auto VLESS + Reality + XHTTP"},
			},
			wantOK: true,
			wantName: "impVPN Auto",
		},
		{
			desc: "pipe-separated suffix",
			entries: []config.ProxyEntry{
				{Name: "🇨🇦 impVPN Auto | VLESS + Reality"},
				{Name: "🇨🇦 impVPN Auto | HYSTERIA2"},
				{Name: "🇩🇪 impVPN Auto | TROJAN + Reality"},
			},
			wantOK: true,
			wantName: "impVPN Auto",
		},
		{
			desc: "all identical",
			entries: []config.ProxyEntry{
				{Name: "🇨🇦 impVPN Auto"},
				{Name: "🇩🇪 impVPN Auto"},
			},
			wantOK: true,
			wantName: "impVPN Auto",
		},
		{
			desc: "completely different",
			entries: []config.ProxyEntry{
				{Name: "US Fast Server"},
				{Name: "EU Slow Server"},
				{Name: "Asia Medium"},
			},
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			name, ok := ExtractAutoGroupName(tc.entries)
			if ok != tc.wantOK {
				t.Errorf("ok=%v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && name != tc.wantName {
				t.Errorf("name=%q, want %q", name, tc.wantName)
			}
		})
	}
}

func TestSplitAutoEntriesMulti(t *testing.T) {
	cases := []struct {
		desc            string
		entries         []config.ProxyEntry
		wantGroupNames  []string
		wantGroupSizes  []int
		wantIndvCount   int
	}{
		{
			desc: "single flag auto-group",
			entries: []config.ProxyEntry{
				{Name: "🇨🇦 impVPN Auto VLESS"},
				{Name: "🇨🇦 impVPN Auto HYSTERIA2"},
				{Name: "🇨🇦 impVPN Auto TROJAN"},
			},
			wantGroupNames: []string{"impVPN Auto"},
			wantGroupSizes: []int{3},
			wantIndvCount:  0,
		},
		{
			desc: "two flags two auto-groups",
			entries: []config.ProxyEntry{
				{Name: "🇷🇺 RU Auto VLESS"},
				{Name: "🇷🇺 RU Auto HYSTERIA2"},
				{Name: "🇺🇸 US Auto VLESS"},
				{Name: "🇺🇸 US Auto HYSTERIA2"},
			},
			wantGroupNames: []string{"RU Auto", "US Auto"},
			wantGroupSizes: []int{2, 2},
			wantIndvCount:  0,
		},
		{
			desc: "auto groups plus individuals",
			entries: []config.ProxyEntry{
				{Name: "🇷🇺 RU Auto VLESS"},
				{Name: "🇷🇺 RU Auto TROJAN"},
				{Name: "🇩🇪 DE Single Server"},
				{Name: "🇺🇸 US Auto VLESS"},
				{Name: "🇺🇸 US Auto HYSTERIA2"},
			},
			wantGroupNames: []string{"RU Auto", "US Auto"},
			wantGroupSizes: []int{2, 2},
			wantIndvCount:  1,
		},
		{
			desc: "single auto entry per flag falls through to individuals",
			entries: []config.ProxyEntry{
				{Name: "🇷🇺 RU Auto VLESS"},
				{Name: "🇺🇸 US Auto VLESS"},
			},
			wantGroupNames: nil,
			wantGroupSizes: nil,
			wantIndvCount:  2,
		},
		{
			desc: "no auto entries",
			entries: []config.ProxyEntry{
				{Name: "🇷🇺 RU Server #1"},
				{Name: "🇺🇸 US Server #1"},
			},
			wantGroupNames: nil,
			wantGroupSizes: nil,
			wantIndvCount:  2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			groups, individuals := SplitAutoEntriesMulti(tc.entries)
			if len(groups) != len(tc.wantGroupNames) {
				t.Fatalf("groups=%d, want %d", len(groups), len(tc.wantGroupNames))
			}
			for i, g := range groups {
				if g.Name != tc.wantGroupNames[i] {
					t.Errorf("group[%d].Name=%q, want %q", i, g.Name, tc.wantGroupNames[i])
				}
				if len(g.Members) != tc.wantGroupSizes[i] {
					t.Errorf("group[%d].Members size=%d, want %d", i, len(g.Members), tc.wantGroupSizes[i])
				}
			}
			if len(individuals) != tc.wantIndvCount {
				t.Errorf("individuals=%d, want %d", len(individuals), tc.wantIndvCount)
			}
		})
	}
}
