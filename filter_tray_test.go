// Copyright (C) 2026 ResultV
//
// Licensed under the terms of the GNU General Public License v3 or later.

package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/system"
)

func mustExtra(t *testing.T, members []string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"members": members})
	if err != nil {
		t.Fatalf("marshal members: %v", err)
	}
	return raw
}

type trayGroupIDs struct {
	label string
	ids   []string
}

func trayLayout(list system.TrayServerList) []trayGroupIDs {
	var out []trayGroupIDs
	for _, g := range list.Groups {
		ids := []string{}
		for _, p := range g.Proxies {
			ids = append(ids, p.ID)
		}
		out = append(out, trayGroupIDs{g.Label, ids})
	}
	return out
}

// The tray lists servers the way the home screen does.
func TestBuildTrayServerList(t *testing.T) {
	hide := false
	subA := config.Subscription{ID: "sa", Name: "impVPN", URL: "https://a"}
	subB := config.Subscription{ID: "sb", Name: "Second", URL: "https://b"}
	subHidden := config.Subscription{ID: "sh", Name: "Hidden", URL: "https://h", ShowOnHome: &hide}

	head := config.ProxyEntry{ID: "auto", Name: "Авто", Type: "AUTO", SubscriptionURL: subA.URL, Extra: mustExtra(t, []string{"m1", "m2"})}
	m1 := config.ProxyEntry{ID: "m1", Name: "🇦🇹 VLESS #1", Type: "VLESS", SubscriptionURL: subA.URL}
	m2 := config.ProxyEntry{ID: "m2", Name: "🇩🇪 VLESS #2", Type: "VLESS", SubscriptionURL: subA.URL}
	a1 := config.ProxyEntry{ID: "a1", Name: "Австрия", Type: "VLESS", SubscriptionURL: subA.URL}
	a2 := config.ProxyEntry{ID: "a2", Name: "Германия", Type: "VLESS", SubscriptionURL: subA.URL}
	section := config.ProxyEntry{ID: "sec", Name: "── Premium ──", Type: "SECTION", SubscriptionURL: subA.URL}
	b1 := config.ProxyEntry{ID: "b1", Name: "Швеция", Type: "TROJAN", SubscriptionURL: subB.URL}
	h1 := config.ProxyEntry{ID: "h1", Name: "Скрытый", Type: "VLESS", SubscriptionURL: subHidden.URL}
	own := config.ProxyEntry{ID: "own", Name: "Свой", Type: "VLESS"}
	orphan := config.ProxyEntry{ID: "orphan", Name: "Сирота", Type: "VLESS", SubscriptionURL: "https://gone"}

	all := []config.ProxyEntry{head, m1, m2, a1, section, a2, b1, h1, own, orphan}
	subs := []config.Subscription{subA, subB, subHidden}

	cases := []struct {
		name     string
		proxies  []config.ProxyEntry
		settings config.AppSettings
		want     []trayGroupIDs
	}{
		{
			name:    "groups by subscription, own servers last",
			proxies: all,
			want: []trayGroupIDs{
				{"impVPN", []string{"auto", "a1", "a2"}},
				{"Second", []string{"b1"}},
				{"Мои сервера", []string{"own", "orphan"}},
			},
		},
		{
			name:    "a single group has no header",
			proxies: []config.ProxyEntry{head, m1, m2, a1, h1},
			want:    []trayGroupIDs{{"", []string{"auto", "a1"}}},
		},
		{
			name:     "saved group order, unknown group after its predecessor",
			proxies:  all,
			settings: config.AppSettings{GroupOrder: []string{"my", "sa"}},
			want: []trayGroupIDs{
				{"Мои сервера", []string{"own", "orphan"}},
				{"impVPN", []string{"auto", "a1", "a2"}},
				{"Second", []string{"b1"}},
			},
		},
		{
			name:     "favourites rise inside their own group",
			proxies:  all,
			settings: config.AppSettings{Favorites: []string{"a2", "orphan"}},
			want: []trayGroupIDs{
				{"impVPN", []string{"a2", "auto", "a1"}},
				{"Second", []string{"b1"}},
				{"Мои сервера", []string{"orphan", "own"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trayLayout(buildTrayServerList(config.AppConfig{Proxies: tc.proxies, Subscriptions: subs, Settings: tc.settings}))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestBuildTrayServerListAutoHeadOf(t *testing.T) {
	first := config.ProxyEntry{ID: "h1", Name: "Авто 1", Type: "AUTO", Extra: mustExtra(t, []string{"m1", "m2"})}
	second := config.ProxyEntry{ID: "h2", Name: "Авто 2", Type: "auto", Extra: mustExtra(t, []string{"m2"})}
	m1 := config.ProxyEntry{ID: "m1", Type: "VLESS"}
	m2 := config.ProxyEntry{ID: "m2", Type: "VLESS"}
	proxies := []config.ProxyEntry{first, second, m1, m2}

	list := buildTrayServerList(config.AppConfig{Proxies: proxies})
	if want := map[string]string{"m1": "h1", "m2": "h1"}; !reflect.DeepEqual(list.AutoHeadOf, want) {
		t.Fatalf("AutoHeadOf = %v, want %v", list.AutoHeadOf, want)
	}

	picked := buildTrayServerList(config.AppConfig{Proxies: proxies, Settings: config.AppSettings{LastSelectedProxyID: "h2"}})
	if picked.AutoHeadOf["m2"] != "h2" || picked.AutoHeadOf["m1"] != "h1" {
		t.Fatalf("shared member should follow the picked head: %v", picked.AutoHeadOf)
	}
}
