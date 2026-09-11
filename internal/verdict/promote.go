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

package verdict

import (
	"net/netip"
	"strings"
)

// PromoteThreshold is how many distinct children must give the same answer
// before it is lifted onto their parent.
//
// Three, not two: two is one unlucky pair of timeouts on a flaky Wi-Fi, and
// the cost of a wrong promotion is a whole domain routed the wrong way.
const PromoteThreshold = 3

// notePromotionLocked records one learned child and lifts the answer onto the
// parent once enough distinct children agree. Caller must hold s.mu.
func (s *Store) notePromotionLocked(key string, d Decision) {
	parent := promotionParent(key)
	if parent == "" {
		return
	}
	byNS := s.children[s.ns]
	if byNS == nil {
		byNS = make(map[string]map[string]Decision)
		s.children[s.ns] = byNS
	}
	parentHash := s.hash(parent)
	kids := byNS[parentHash]
	if kids == nil {
		kids = make(map[string]Decision)
		byNS[parentHash] = kids
	}
	kids[s.hash(key)] = d

	agree := 0
	for _, kd := range kids {
		if kd == d {
			agree++
		}
	}
	if agree < PromoteThreshold {
		return
	}
	// Disagreement inside the group is a reason to stay silent, not to pick a
	// winner: a domain where some hosts work and some do not is exactly the
	// case a blanket suffix rule gets wrong.
	if len(kids) != agree {
		return
	}
	now := s.now()
	s.putLocked(parent, Record{
		Decision:  d,
		Source:    SourceLearned,
		Observed:  now,
		ExpiresAt: now.Add(ttlFor(d)),
	})
}

// promotionParent is the key a learned entry votes for: the parent domain for
// a name, the covering prefix for a bare address.
//
// A key that is already an aggregate votes for nothing. Without this guard
// ParentDomain would happily chew "149.154.167.0/24" down to "154.167.0/24" —
// a key nothing ever looks up, quietly accumulating for the life of the store.
func promotionParent(key string) string {
	if strings.Contains(key, "/") {
		return ""
	}
	if addr, err := netip.ParseAddr(key); err == nil {
		return IPParent(addr)
	}
	return ParentDomain(key)
}

// LearnIP records what the engine measured for a destination that never
// presented a name — Telegram's MTProto, Discord's voice media.
func (s *Store) LearnIP(addr netip.Addr, d Decision) {
	key := IPKey(addr)
	if key == "" {
		return
	}
	now := s.now()
	s.put(key, Record{Decision: d, Source: SourceLearned, Observed: now, ExpiresAt: now.Add(ttlFor(d))})
}

// LookupIP answers for a bare address: the address itself first, then the
// prefix its neighbours may have promoted.
func (s *Store) LookupIP(addr netip.Addr) (Record, bool) {
	key := IPKey(addr)
	if key == "" {
		return Record{}, false
	}
	keys := []string{key}
	if parent := IPParent(addr); parent != "" && parent != key {
		keys = append(keys, parent)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lookupKeysLocked(keys)
}
