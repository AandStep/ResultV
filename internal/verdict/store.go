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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Store holds verdicts for one machine, partitioned by network.
//
// Keys are hashed everywhere, not only on disk: keeping one representation
// means the in-memory map and the file are the same shape and no conversion
// step can drift. The plaintext lives in a second map that is never persisted
// and never outlives the session — it exists so the UI can answer "why is this
// site in the tunnel" (see spec §8).
type Store struct {
	mu   sync.RWMutex
	now  func() time.Time
	salt []byte

	ns     string
	spaces map[string]map[string]Record

	plain map[string]string

	// children counts, per namespace, which hashed child keys were learned
	// under which hashed parent key and with what answer. Promotion reads it.
	children map[string]map[string]map[string]vote
}

func New(salt []byte, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		now:      now,
		salt:     append([]byte(nil), salt...),
		ns:       "default",
		spaces:   make(map[string]map[string]Record),
		plain:    make(map[string]string),
		children: make(map[string]map[string]map[string]vote),
	}
}

// SetNamespace switches the active network. Nothing is dropped: the previous
// network's knowledge is still there when the user comes home.
func (s *Store) SetNamespace(ns string) {
	if ns == "" {
		ns = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ns = ns
}

func (s *Store) Namespace() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ns
}

func (s *Store) hash(key string) string {
	mac := hmac.New(sha256.New, s.salt)
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// Seed records a verdict that came from outside the engine: a user rule, the
// hand-proven floor, or a fetched list. No TTL — these are replaced, not aged.
func (s *Store) Seed(host string, d Decision, src Source) {
	s.put(NormalizeHost(host), Record{Decision: d, Source: src, Observed: s.now()})
}

// Learn records what the engine measured itself.
func (s *Store) Learn(host string, d Decision) {
	now := s.now()
	rec := Record{Decision: d, Source: SourceLearned, Observed: now, ExpiresAt: now.Add(ttlFor(d))}
	s.put(NormalizeHost(host), rec)
}

// LearnUnverified records a proxy verdict that rests on lost races alone.
func (s *Store) LearnUnverified(host string) {
	s.put(NormalizeHost(host), s.unverifiedRecord())
}

func (s *Store) unverifiedRecord() Record {
	now := s.now()
	return Record{Decision: Proxy, Source: SourceLearned, Observed: now, ExpiresAt: now.Add(TTLUnverified), Unverified: true}
}

// Forget drops what was learned about one name on the active network.
func (s *Store) Forget(host string) {
	key := NormalizeHost(host)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hash(key)
	if rec, ok := s.spaces[s.ns][h]; ok && rec.Source == SourceLearned {
		delete(s.spaces[s.ns], h)
	}
	if parent := promotionParent(key); parent != "" {
		delete(s.children[s.ns][s.hash(parent)], h)
	}
}

// ClearLearned drops every learned verdict on every network and reports how
// many there were.
func (s *Store) ClearLearned() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for _, space := range s.spaces {
		for h, rec := range space {
			if rec.Source == SourceLearned {
				delete(space, h)
				dropped++
			}
		}
	}
	s.children = make(map[string]map[string]map[string]vote)
	return dropped
}

// LearnedCount is how many live learned verdicts with this answer the active
// network holds, including names this session never saw in plaintext.
func (s *Store) LearnedCount(d Decision) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	n := 0
	for _, rec := range s.spaces[s.ns] {
		if rec.Source != SourceLearned || rec.Decision != d {
			continue
		}
		if !rec.ExpiresAt.IsZero() && !rec.ExpiresAt.After(now) {
			continue
		}
		n++
	}
	return n
}

func ttlFor(d Decision) time.Duration {
	if d == Proxy {
		return TTLProxy
	}
	return TTLDirect
}

func (s *Store) put(key string, rec Record) {
	if key == "" || rec.Decision == Unknown {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putLocked(key, rec)
}

func (s *Store) putLocked(key string, rec Record) {
	space := s.spaces[s.ns]
	if space == nil {
		space = make(map[string]Record)
		s.spaces[s.ns] = space
	}
	h := s.hash(key)
	// A weaker source never overwrites a stronger one; the same source always
	// does, so a fresh measurement replaces a stale one.
	if old, ok := space[h]; ok && old.Source > rec.Source {
		return
	}
	space[h] = rec
	s.plain[h] = key
	if rec.Source == SourceLearned && !rec.Unverified {
		s.notePromotionLocked(key, rec.Decision)
	}
}

// Lookup walks the name upwards and answers with the strongest source that
// matched, breaking ties towards the more specific name.
func (s *Store) Lookup(host string) (Record, bool) {
	key := NormalizeHost(host)
	if key == "" {
		return Record{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lookupKeysLocked(Suffixes(key))
}

func (s *Store) lookupKeysLocked(keys []string) (Record, bool) {
	space := s.spaces[s.ns]
	if space == nil {
		return Record{}, false
	}
	now := s.now()
	var best Record
	var found bool
	// keys arrives most-specific-first, so a strict > keeps the first (most
	// specific) candidate when two sources tie.
	for i, key := range keys {
		rec, ok := space[s.hash(key)]
		if !ok {
			continue
		}
		if !rec.ExpiresAt.IsZero() && !rec.ExpiresAt.After(now) {
			continue
		}
		// Censors block single hosts by SNI, so a direct measurement says
		// nothing about a sibling.
		if i > 0 && rec.Source == SourceLearned && rec.Decision == Direct {
			continue
		}
		if !found || rec.Source > best.Source {
			best, found = rec, true
		}
	}
	return best, found
}

// Names returns the live verdicts whose plaintext this session happens to
// know. Anything loaded from disk and not seen since is absent by design.
func (s *Store) Names() map[string]Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	space := s.spaces[s.ns]
	out := make(map[string]Record, len(space))
	now := s.now()
	for h, rec := range space {
		name, ok := s.plain[h]
		if !ok {
			continue
		}
		if !rec.ExpiresAt.IsZero() && !rec.ExpiresAt.After(now) {
			continue
		}
		out[name] = rec
	}
	return out
}
