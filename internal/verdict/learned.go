// SPDX-License-Identifier: GPL-3.0-or-later
// Proprietary license and copyright notice at https://vpncore.org/legal

package verdict

// Learned returns named decisions and a count of the remaining live decisions
// in the active network. The latter only exist as hashes after a restart.
func (s *Store) Learned(decision Decision) (map[string]Record, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make(map[string]Record)
	hidden := 0
	now := s.now()
	for hash, rec := range s.spaces[s.ns] {
		if rec.Source != SourceLearned || rec.Decision != decision ||
			(!rec.ExpiresAt.IsZero() && !rec.ExpiresAt.After(now)) {
			continue
		}
		if name, ok := s.plain[hash]; ok {
			items[name] = rec
		} else {
			hidden++
		}
	}
	return items, hidden
}

// ForgetLearned removes a learned destination and its promotion vote in the
// active network. Explicit and built-in routes remain in effect.
func (s *Store) ForgetLearned(host string) {
	key := NormalizeHost(host)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := s.hash(key)
	if rec, ok := s.spaces[s.ns][hash]; ok && rec.Source == SourceLearned {
		delete(s.spaces[s.ns], hash)
	}
	if parent := promotionParent(key); parent != "" {
		delete(s.children[s.ns][s.hash(parent)], hash)
	}
}

// ClearLearned removes learned decisions across networks, including hashed
// destinations from previous sessions.
func (s *Store) ClearLearned() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, space := range s.spaces {
		for hash, rec := range space {
			if rec.Source == SourceLearned {
				delete(space, hash)
			}
		}
	}
	clear(s.children)
}
