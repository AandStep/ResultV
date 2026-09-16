// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

// gomobile bindings for routing profiles.
//
// Everything crosses as JSON strings: gomobile binds only basic types, so a
// []string or a struct cannot make the trip. The shape on the wire is exactly
// what config.RoutingProfile marshals to — renaming a field here would lose it
// silently on the round trip through the Kotlin store.
//
// This layer holds no state. The profile list lives in Kotlin
// (routing_profiles.json), matching every other repository on this platform;
// what stays here is the part that must not exist twice — the rule that decides
// whether an arriving profile replaces a stored one.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// IsRoutingDeepLink reports whether a resultv:// link carries a routing profile
// rather than a subscription.
//
// The split is decided here rather than by a prefix check in Kotlin: there are
// three accepted prefixes, and a Kotlin copy of that list would drift from the
// parser. Getting it wrong in the permissive direction kills subscription
// imports silently.
func IsRoutingDeepLink(rawURL string) bool {
	return proxy.IsRoutingDeepLink(rawURL)
}

// PreviewRoutingDeepLink decodes a routing link WITHOUT storing anything and
// without reaching the network, so the UI can show what it is about to add and
// let the user refuse.
//
// The returned profile has no id — that is assigned by MergeRoutingProfile,
// which is the only place that knows what is already stored.
func PreviewRoutingDeepLink(rawURL string) (string, error) {
	p, err := proxy.DecodeRoutingDeepLink(rawURL)
	if err != nil {
		return "", err
	}
	blob, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshaling routing profile: %w", err)
	}
	return string(blob), nil
}

// routingStore is the JSON shape of the Kotlin-side profile store.
type routingStore struct {
	Profiles []config.RoutingProfile `json:"profiles"`
	ActiveID string                  `json:"activeId"`
}

// routingMergeResult is what MergeRoutingProfile hands back: the whole new
// store plus the profile as stored, so Kotlin does not have to find it again.
type routingMergeResult struct {
	Profiles []config.RoutingProfile `json:"profiles"`
	ActiveID string                  `json:"activeId"`
	Saved    config.RoutingProfile   `json:"saved"`
}

// MergeRoutingProfile folds an incoming profile into the stored list.
//
// storedJSON may be empty or "{}" — a first import has no store yet. It may NOT
// be malformed: that would mean the store on disk is damaged, and quietly
// starting from an empty list there would throw away every profile the user had.
func MergeRoutingProfile(storedJSON, incomingJSON string, makeActive bool) (string, error) {
	var store routingStore
	if s := strings.TrimSpace(storedJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &store); err != nil {
			return "", fmt.Errorf("parsing routing store: %w", err)
		}
	}
	var incoming config.RoutingProfile
	if err := json.Unmarshal([]byte(incomingJSON), &incoming); err != nil {
		return "", fmt.Errorf("parsing routing profile: %w", err)
	}
	profiles, activeID, saved, err := proxy.UpsertRoutingProfile(
		store.Profiles, incoming, store.ActiveID, makeActive)
	if err != nil {
		return "", err
	}
	blob, err := json.Marshal(routingMergeResult{
		Profiles: profiles, ActiveID: activeID, Saved: saved,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling routing store: %w", err)
	}
	return string(blob), nil
}

// CompileRoutingProfile expands a profile's rules into cached rule-sets.
//
// Reaches the network when the profile references geo databases or linked
// lists, so callers must keep it off the main thread.
func CompileRoutingProfile(profileJSON, dataDir string, refreshGeo bool) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required on mobile (pass context.filesDir)")
	}
	var p config.RoutingProfile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return "", fmt.Errorf("parsing routing profile: %w", err)
	}
	rep, err := proxy.CompileRoutingProfile(context.Background(), p, dataDir, refreshGeo)
	if err != nil {
		return "", err
	}
	blob, merr := json.Marshal(rep)
	if merr != nil {
		return "", fmt.Errorf("marshaling compile report: %w", merr)
	}
	return string(blob), nil
}

// RoutingProfileStatus reports which of the three actions have a usable
// compiled rule-set on disk.
//
// This is how the UI answers "собран / не собран" without storing a flag that
// could drift from the files — and the files are the only thing the engine
// actually looks at.
func RoutingProfileStatus(dataDir, profileID string) (string, error) {
	out := map[string]bool{}
	for _, action := range proxy.RoutingActions {
		out[action] = proxy.RoutingProfileSRSReady(dataDir, profileID, action)
	}
	blob, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshaling routing status: %w", err)
	}
	return string(blob), nil
}

// RemoveRoutingProfile deletes a profile's cached rule-sets. The config entry is
// Kotlin's to remove; a cache left here would go on routing traffic by a profile
// that is gone.
func RemoveRoutingProfile(dataDir, profileID string) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("dataDir is required")
	}
	proxy.RemoveRoutingProfileSRS(dataDir, profileID)
	return nil
}

// ExtractSubscriptionRouting pulls provider-declared routing out of a
// subscription response: the Routing-Lists header (base64 JSON) or a
// routingLists key in a JSON body.
//
// Always a JSON array, "[]" when there is none — Kotlin reads this with
// JSONArray, and a null there would be a crash rather than an empty list.
func ExtractSubscriptionRouting(headerVal, body string) (string, error) {
	lists := proxy.ExtractSubscriptionRoutingLists(headerVal, body)
	if lists == nil {
		lists = []config.RoutingList{}
	}
	blob, err := json.Marshal(lists)
	if err != nil {
		return "", fmt.Errorf("marshaling subscription routing: %w", err)
	}
	return string(blob), nil
}
