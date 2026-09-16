// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Deciding whether an arriving profile is a new version of a stored one.
//
// Ported from the desktop's app_routingprofile.go, minus its *App receiver: the
// store lives on the Kotlin side here, so this layer is handed the slice and
// hands back a new one. It never touches disk and never reaches the network.
//
// Why this stayed in Go at all, when the store did not: the matching rule below
// is subtle, and getting it wrong is invisible until it bites. One copy, with
// the tests that came with it.
//
// Everything here builds fresh slices rather than editing in place. The caller
// marshals its own live state into these, and an in-place edit would mutate
// what it still holds.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
)

// MaxRoutingProfiles caps how many profiles are kept. Deep links are
// attacker-reachable — anyone can send one — and without a cap, repeatedly
// opening one would grow the stored file without end.
const MaxRoutingProfiles = 50

// NewRoutingProfileID mints an id for a profile. Hex only, so it is always a
// safe file-name component (see ValidRoutingProfileID).
func NewRoutingProfileID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read kills the process rather than return an error on
		// every platform we ship to, so this branch is unreachable in practice.
		// It exists so a future platform where it isn't cannot mint an empty id
		// that would then fail ValidRoutingProfileID and lose the profile.
		return fmt.Sprintf("t%015x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// SameRoutingProfile decides whether an arriving profile is a new version of a
// stored one.
//
// The handle is the PUBLISHED name plus the origin, never the displayed one: a
// panel keeps its name stable across republishes, and it is the only identity
// the payload offers — the JSON carries no id. Comparing displayed names would
// fork a profile in two the first time the user renamed it and reopened the
// link, which is exactly what a re-import is supposed to avoid.
//
// A SUBSCRIPTION profile skips the handle entirely: its subscription already is
// an id, and it is a better one. The desktop compares handles here too, and can
// afford to — it names the profile after the subscription in its own store. On
// mobile the name comes off the wire: a panel sends Profile-Title as
// `base64:<UTF-8>` so an emoji survives the header, this layer stores it as it
// arrived, and Kotlin decodes it for display. Stored `base64:…` then stops
// matching the decoded title that arrives next, and the sync would fork the
// profile precisely when the panel renamed itself.
func SameRoutingProfile(stored, incoming config.RoutingProfile) bool {
	if stored.Source != incoming.Source {
		return false
	}
	if stored.SubscriptionID != incoming.SubscriptionID {
		return false
	}
	if stored.Source == "subscription" && stored.SubscriptionID != "" {
		return true
	}
	return strings.EqualFold(
		strings.TrimSpace(routingProfileHandle(stored)),
		strings.TrimSpace(routingProfileHandle(incoming)))
}

// routingProfileRenamed reports whether the DISPLAYED name is the user's doing.
//
// A stored name still equal to the publisher's handle is the publisher's own
// name, not a choice anyone made — and the publisher is free to change it. Only
// a name that has drifted from the handle is worth protecting from the next
// republish.
func routingProfileRenamed(p config.RoutingProfile) bool {
	if strings.TrimSpace(p.Name) == "" {
		return false
	}
	return !strings.EqualFold(
		strings.TrimSpace(p.Name),
		strings.TrimSpace(routingProfileHandle(p)))
}

// routingProfileHandle falls back to the displayed name for profiles stored
// before OriginName existed, and for hand-made ones that never had a publisher.
func routingProfileHandle(p config.RoutingProfile) string {
	if p.OriginName != "" {
		return p.OriginName
	}
	return p.Name
}

// UpsertRoutingProfile stores a profile, replacing the one it matches.
//
// TWO OPERATIONS, and confusing them is what this function got wrong once.
//
//   - An EDIT arrives carrying the id of a profile already stored. It is the
//     user speaking, so everything they typed wins — the name included.
//   - A REPUBLISH arrives with no id, and is matched by the publisher's handle
//     (SameRoutingProfile). There the stored name is kept: the user may have
//     renamed the profile, and re-opening the panel's link must not undo that.
//
// The desktop keeps these apart as two functions, SaveRoutingProfile and
// upsertRoutingProfile. Folding them into one and applying the republish rule
// to both left the editor unable to rename anything: it typed a new name, the
// merge put the old one back, and nothing appeared to happen.
//
// Returns the new slice, the new active id, and the profile as stored. activeID
// is the caller's current choice; makeActive asks for the incoming profile to
// take over. A first profile always becomes active — importing one and having
// nothing happen reads as a failure.
func UpsertRoutingProfile(
	stored []config.RoutingProfile,
	incoming config.RoutingProfile,
	activeID string,
	makeActive bool,
) ([]config.RoutingProfile, string, config.RoutingProfile, error) {
	out := make([]config.RoutingProfile, len(stored))
	copy(out, stored)

	// An edit: matched by id, the user's text wins.
	if incoming.ID != "" {
		for i, existing := range out {
			if existing.ID != incoming.ID {
				continue
			}
			// Provenance is not the editor's to change: a profile that came
			// from a subscription stays that profile's, so a later sync still
			// recognises it. OriginName goes with it — it is the handle a
			// re-import matches on, and letting a rename move it would break
			// exactly the case it exists for. Linked lists and the plaintext
			// consent travel with it too: the editor has no field for them, so
			// it must not be able to lose them.
			incoming.Source = existing.Source
			incoming.SubscriptionID = existing.SubscriptionID
			incoming.OriginName = existing.OriginName
			incoming.ListURLs = existing.ListURLs
			incoming.AllowInsecure = existing.AllowInsecure
			out[i] = incoming
			if makeActive {
				activeID = incoming.ID
			}
			return out, activeID, incoming, nil
		}
	}

	// A republish: matched by the publisher's handle.
	replaced := false
	for i, existing := range out {
		if !SameRoutingProfile(existing, incoming) {
			continue
		}
		// The user may have renamed it; a republish of the same profile must
		// not undo that. A stored name that never drifted from the handle is
		// the publisher's own, so the publisher's new one wins — otherwise a
		// panel could never rename a profile it had already delivered. An
		// arriving profile with no name never blanks the visible one.
		incoming.ID = existing.ID
		if existing.Name != "" &&
			(routingProfileRenamed(existing) || strings.TrimSpace(incoming.Name) == "") {
			incoming.Name = existing.Name
		}
		out[i] = incoming
		replaced = true
		break
	}
	if !replaced {
		// The cap guards growth, so it only applies to a profile that adds a
		// row. A republish of one already stored replaces in place and is let
		// through even at the limit.
		if len(out) >= MaxRoutingProfiles {
			return nil, activeID, config.RoutingProfile{}, fmt.Errorf(
				"хранится уже %d профилей маршрутизации — удалите лишние", len(out))
		}
		if !ValidRoutingProfileID(incoming.ID) {
			incoming.ID = NewRoutingProfileID()
		}
		out = append(out, incoming)
	}
	if makeActive || activeID == "" {
		activeID = incoming.ID
	}
	return out, activeID, incoming, nil
}
