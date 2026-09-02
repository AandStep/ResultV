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

// Routing profiles: storing them, choosing the active one, and importing one
// from a deep link.
//
// A profile only takes effect in Global mode. In Smart the client works out
// routing itself, and a profile there would be fighting the very thing Smart
// exists to do — so the UI hides profiles in Smart and this layer never applies
// one either way.
//
// ALIASING RULE, the one that has bitten this file's neighbours twice on
// review: a.config.GetConfig() hands back a value whose slices share the live
// cache's backing array. Every mutation here therefore builds a fresh slice
// rather than editing in place.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// MaxRoutingProfiles caps how many profiles are kept. Deep links are
// attacker-reachable: without a cap, repeatedly opening one would grow the
// config file without end.
const MaxRoutingProfiles = 50

func newRoutingProfileID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// GetRoutingProfiles returns the stored profiles and which one is active.
func (a *App) GetRoutingProfiles() map[string]any {
	if a == nil || a.config == nil {
		return map[string]any{"profiles": []config.RoutingProfile{}, "activeId": ""}
	}
	rr := a.config.GetConfig().RoutingRules
	profiles := make([]config.RoutingProfile, len(rr.Profiles))
	copy(profiles, rr.Profiles)
	return map[string]any{"profiles": profiles, "activeId": rr.ActiveProfileID}
}

// PreviewRoutingDeepLink decodes a routing link without storing anything, so
// the UI can show what it is about to add and let the user refuse.
func (a *App) PreviewRoutingDeepLink(url string) (config.RoutingProfile, error) {
	p, err := proxy.DecodeRoutingDeepLink(url)
	if err != nil {
		return config.RoutingProfile{}, err
	}
	return p, nil
}

// ImportRoutingDeepLink decodes a routing link and stores the profile.
//
// A re-opened link updates the profile it already created instead of adding a
// twin: panels publish one stable link per profile and republish it whenever
// the rules change. What makes it "the same profile" is sameRoutingProfile.
func (a *App) ImportRoutingDeepLink(url string, makeActive bool) (config.RoutingProfile, error) {
	if a == nil || a.config == nil {
		return config.RoutingProfile{}, fmt.Errorf("config manager not initialized")
	}
	incoming, err := proxy.DecodeRoutingDeepLink(url)
	if err != nil {
		a.log.Error(fmt.Sprintf("Не удалось разобрать ссылку маршрутизации: %v", err))
		return config.RoutingProfile{}, err
	}
	saved, err := a.upsertRoutingProfile(incoming, makeActive)
	if err != nil {
		return config.RoutingProfile{}, err
	}
	a.log.Info(fmt.Sprintf("[МАРШРУТИЗАЦИЯ] профиль %q импортирован: %d direct, %d proxy, %d block",
		saved.Name, saved.RuleCount("direct"), saved.RuleCount("proxy"), saved.RuleCount("block")))
	// Rules that were never compiled route nothing. Compilation reaches the
	// network, so a failure here is reported but does not undo the import: the
	// profile is stored and can be retried without re-opening the link.
	if _, cerr := a.compileRoutingProfile(saved, false); cerr != nil {
		a.log.Error(fmt.Sprintf("Профиль %q сохранён, но правила не собраны: %v", saved.Name, cerr))
		return saved, nil
	}
	a.syncRoutingListSpecs()
	return saved, nil
}

// upsertRoutingProfile stores a decoded profile, replacing the one it matches.
func (a *App) upsertRoutingProfile(incoming config.RoutingProfile, makeActive bool) (config.RoutingProfile, error) {
	rr := a.config.GetConfig().RoutingRules

	out := make([]config.RoutingProfile, len(rr.Profiles))
	copy(out, rr.Profiles)

	replaced := false
	for i, existing := range out {
		if !sameRoutingProfile(existing, incoming) {
			continue
		}
		// The user may have renamed it; a republish of the same profile must
		// not undo that.
		incoming.ID = existing.ID
		if existing.Name != "" {
			incoming.Name = existing.Name
		}
		out[i] = incoming
		replaced = true
		break
	}
	if !replaced {
		if len(out) >= MaxRoutingProfiles {
			return config.RoutingProfile{}, fmt.Errorf(
				"хранится уже %d профилей маршрутизации — удалите лишние", len(out))
		}
		incoming.ID = newRoutingProfileID()
		out = append(out, incoming)
	}

	rr.Profiles = out
	if makeActive || rr.ActiveProfileID == "" {
		rr.ActiveProfileID = incoming.ID
	}
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		return config.RoutingProfile{}, err
	}
	return incoming, nil
}

// sameRoutingProfile decides whether an arriving profile is a new version of a
// stored one.
//
// The handle is the PUBLISHED name plus the origin, never the displayed one: a
// panel keeps its name stable across republishes, and it is the only identity
// the payload offers — the JSON carries no id. Comparing displayed names would
// fork a profile in two the first time the user renamed it and reopened the
// link, which is exactly what a re-import is supposed to avoid.
func sameRoutingProfile(stored, incoming config.RoutingProfile) bool {
	if stored.Source != incoming.Source {
		return false
	}
	if stored.SubscriptionID != incoming.SubscriptionID {
		return false
	}
	return strings.EqualFold(
		strings.TrimSpace(routingProfileHandle(stored)),
		strings.TrimSpace(routingProfileHandle(incoming)))
}

// routingProfileHandle falls back to the displayed name for profiles stored
// before OriginName existed, and for hand-made ones that never had a publisher.
func routingProfileHandle(p config.RoutingProfile) string {
	if p.OriginName != "" {
		return p.OriginName
	}
	return p.Name
}

// SaveRoutingProfile stores a profile edited by hand. An empty ID creates one.
func (a *App) SaveRoutingProfile(p config.RoutingProfile) (config.RoutingProfile, error) {
	if a == nil || a.config == nil {
		return config.RoutingProfile{}, fmt.Errorf("config manager not initialized")
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return config.RoutingProfile{}, fmt.Errorf("у профиля должно быть название")
	}
	if p.RuleCount("direct")+p.RuleCount("proxy")+p.RuleCount("block") == 0 {
		return config.RoutingProfile{}, fmt.Errorf("профиль без правил ничего не делает")
	}
	p.UpdatedAt = time.Now().Unix()

	rr := a.config.GetConfig().RoutingRules
	out := make([]config.RoutingProfile, len(rr.Profiles))
	copy(out, rr.Profiles)

	if p.ID == "" {
		if len(out) >= MaxRoutingProfiles {
			return config.RoutingProfile{}, fmt.Errorf(
				"хранится уже %d профилей маршрутизации — удалите лишние", len(out))
		}
		if p.Source == "" {
			p.Source = "manual"
		}
		p.ID = newRoutingProfileID()
		out = append(out, p)
	} else {
		found := false
		for i, existing := range out {
			if existing.ID != p.ID {
				continue
			}
			// Provenance is not the editor's to change: a profile that came
			// from a subscription stays that profile's, so a later sync still
			// recognises it. OriginName goes with it — it is the handle a
			// re-import matches on, and letting a rename move it would break
			// exactly the case it exists for.
			p.Source = existing.Source
			p.SubscriptionID = existing.SubscriptionID
			p.OriginName = existing.OriginName
			out[i] = p
			found = true
			break
		}
		if !found {
			return config.RoutingProfile{}, fmt.Errorf("профиль не найден")
		}
	}

	rr.Profiles = out
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		return config.RoutingProfile{}, err
	}
	if _, cerr := a.compileRoutingProfile(p, false); cerr != nil {
		a.log.Error(fmt.Sprintf("Профиль %q сохранён, но правила не собраны: %v", p.Name, cerr))
		return p, nil
	}
	a.syncRoutingListSpecs()
	return p, nil
}

// DeleteRoutingProfile removes a profile. Deleting the active one leaves none
// active rather than silently promoting a neighbour: which rules are in force
// is not something to decide on the user's behalf.
func (a *App) DeleteRoutingProfile(id string) error {
	if a == nil || a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("invalid routing profile id")
	}
	rr := a.config.GetConfig().RoutingRules
	out := make([]config.RoutingProfile, 0, len(rr.Profiles))
	found := false
	for _, p := range rr.Profiles {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return fmt.Errorf("профиль не найден")
	}
	rr.Profiles = out
	if rr.ActiveProfileID == id {
		rr.ActiveProfileID = ""
	}
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		return err
	}
	// Cached rule-sets outlive the config entry unless they are removed here,
	// and a stale file would keep routing traffic by a profile that is gone.
	a.removeRoutingProfileRuleSets(id)
	a.syncRoutingListSpecs()
	return nil
}

// SetActiveRoutingProfile chooses which profile is in force. An empty id turns
// profiles off without deleting them.
func (a *App) SetActiveRoutingProfile(id string) error {
	if a == nil || a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	rr := a.config.GetConfig().RoutingRules
	if id != "" {
		known := false
		for _, p := range rr.Profiles {
			if p.ID == id {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("профиль не найден")
		}
	}
	rr.ActiveProfileID = id
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		return err
	}
	a.syncRoutingListSpecs()
	return nil
}

// ActiveRoutingProfile returns the profile in force, and whether there is one.
func (a *App) ActiveRoutingProfile() (config.RoutingProfile, bool) {
	if a == nil || a.config == nil {
		return config.RoutingProfile{}, false
	}
	rr := a.config.GetConfig().RoutingRules
	if rr.ActiveProfileID == "" {
		return config.RoutingProfile{}, false
	}
	for _, p := range rr.Profiles {
		if p.ID == rr.ActiveProfileID {
			return p, true
		}
	}
	return config.RoutingProfile{}, false
}
