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
