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
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const fileVersion = 1

type cacheFile struct {
	Version int                          `json:"version"`
	Salt    []byte                       `json:"salt"`
	Spaces  map[string]map[string]Record `json:"spaces"`
}

// NewSalt draws the per-install salt. It keys the hash of every stored name,
// so that two installs never produce the same digest for the same site.
func NewSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// Load reads the cache. A missing, unreadable or corrupt file is not an error:
// the verdict store is an optimisation, and starting empty costs a few extra
// probes, while refusing to start costs the whole feature.
func Load(path string, now func() time.Time) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		salt, saltErr := NewSalt()
		if saltErr != nil {
			return nil, saltErr
		}
		return New(salt, now), nil
	}
	var file cacheFile
	if err := json.Unmarshal(raw, &file); err != nil || file.Version != fileVersion || len(file.Salt) == 0 {
		salt, saltErr := NewSalt()
		if saltErr != nil {
			return nil, saltErr
		}
		return New(salt, now), nil
	}
	s := New(file.Salt, now)
	for ns, space := range file.Spaces {
		copied := make(map[string]Record, len(space))
		for k, rec := range space {
			copied[k] = rec
		}
		s.spaces[ns] = copied
	}
	s.Prune()
	return s, nil
}

// Prune drops expired records from every namespace.
func (s *Store) Prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for ns, space := range s.spaces {
		for key, rec := range space {
			if !rec.ExpiresAt.IsZero() && !rec.ExpiresAt.After(now) {
				delete(space, key)
				delete(s.plain, key)
			}
		}
		if len(space) == 0 {
			delete(s.spaces, ns)
		}
	}
}

// Save prunes, then writes the cache through a temporary file so a crash
// mid-write cannot leave a half-parsed cache behind.
func (s *Store) Save(path string) error {
	s.Prune()

	s.mu.RLock()
	file := cacheFile{Version: fileVersion, Salt: s.salt, Spaces: make(map[string]map[string]Record, len(s.spaces))}
	for ns, space := range s.spaces {
		copied := make(map[string]Record, len(space))
		for k, rec := range space {
			copied[k] = rec
		}
		file.Spaces[ns] = copied
	}
	s.mu.RUnlock()

	blob, err := json.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
