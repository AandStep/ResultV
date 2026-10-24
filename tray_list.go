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

package main

import (
	"slices"
	"strings"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/system"
)

const trayManualGroupKey = "my"

type trayGroup struct {
	key     string
	label   string
	proxies []config.ProxyEntry
}

// buildTrayServerList lays the tray Servers submenu out the way the home
// screen lists servers (serverGroups in HomeScreen.jsx): AUTO heads stay,
// their members and SECTION rows go, a subscription with "show on home" off
// is left out; one group per subscription, then own servers, in the user's
// group order, labelled only when there is more than one; favourites first
// inside a group.
func buildTrayServerList(cfg config.AppConfig) system.TrayServerList {
	members := make(map[string]bool)
	for _, p := range cfg.Proxies {
		if isAutoEntry(p) {
			for _, id := range extractAutoMembers(p.Extra) {
				if id != "" {
					members[id] = true
				}
			}
		}
	}
	hidden := make(map[string]bool)
	known := make(map[string]bool)
	for _, s := range cfg.Subscriptions {
		known[s.URL] = true
		if s.ShowOnHome != nil && !*s.ShowOnHome {
			hidden[s.URL] = true
		}
	}

	var listed []config.ProxyEntry
	for _, p := range cfg.Proxies {
		if strings.EqualFold(strings.TrimSpace(p.Type), "SECTION") || members[p.ID] || hidden[p.SubscriptionURL] {
			continue
		}
		listed = append(listed, p)
	}

	var groups []trayGroup
	for _, s := range cfg.Subscriptions {
		var own []config.ProxyEntry
		for _, p := range listed {
			if p.SubscriptionURL == s.URL {
				own = append(own, p)
			}
		}
		if len(own) > 0 {
			groups = append(groups, trayGroup{key: s.ID, label: s.Name, proxies: own})
		}
	}
	var manual []config.ProxyEntry
	for _, p := range listed {
		if p.SubscriptionURL == "" || !known[p.SubscriptionURL] {
			manual = append(manual, p)
		}
	}
	if len(manual) > 0 {
		groups = append(groups, trayGroup{key: trayManualGroupKey, label: "Мои сервера", proxies: manual})
	}

	favorites := make(map[string]bool, len(cfg.Settings.Favorites))
	for _, id := range cfg.Settings.Favorites {
		favorites[id] = true
	}
	groups = orderTrayGroups(groups, cfg.Settings.GroupOrder)

	list := system.TrayServerList{AutoHeadOf: make(map[string]string)}
	for _, g := range groups {
		label := g.label
		if len(groups) < 2 {
			label = ""
		}
		list.Groups = append(list.Groups, system.TrayServerGroup{Label: label, Proxies: favoritesFirst(g.proxies, favorites)})
	}

	// A member shared by several listed heads stands for the one the user
	// picked last.
	for _, g := range list.Groups {
		for _, p := range g.Proxies {
			if !isAutoEntry(p) {
				continue
			}
			for _, id := range extractAutoMembers(p.Extra) {
				if _, taken := list.AutoHeadOf[id]; !taken || p.ID == cfg.Settings.LastSelectedProxyID {
					list.AutoHeadOf[id] = p.ID
				}
			}
		}
	}
	return list
}

func isAutoEntry(p config.ProxyEntry) bool {
	return strings.EqualFold(strings.TrimSpace(p.Type), "AUTO")
}

// orderTrayGroups is orderGroups from frontend/src/utils/groupOrder.js: a
// group missing from the saved order goes right after the group it followed
// without one.
func orderTrayGroups(groups []trayGroup, order []string) []trayGroup {
	if len(order) == 0 {
		return groups
	}
	index := func(list []trayGroup, key string) int {
		return slices.IndexFunc(list, func(g trayGroup) bool { return g.key == key })
	}
	var out []trayGroup
	for _, key := range order {
		if i := index(groups, key); i != -1 && index(out, key) == -1 {
			out = append(out, groups[i])
		}
	}
	for i, g := range groups {
		if index(out, g.key) != -1 {
			continue
		}
		at := 0
		for j := i - 1; j >= 0; j-- {
			if pos := index(out, groups[j].key); pos != -1 {
				at = pos + 1
				break
			}
		}
		out = slices.Insert(out, at, g)
	}
	return out
}

func favoritesFirst(proxies []config.ProxyEntry, favorites map[string]bool) []config.ProxyEntry {
	if len(favorites) == 0 {
		return proxies
	}
	out := make([]config.ProxyEntry, 0, len(proxies))
	for _, p := range proxies {
		if favorites[p.ID] {
			out = append(out, p)
		}
	}
	for _, p := range proxies {
		if !favorites[p.ID] {
			out = append(out, p)
		}
	}
	return out
}
