// SPDX-License-Identifier: GPL-3.0-or-later
// Proprietary license and copyright notice at https://vpncore.org/legal

package proxy

import (
	"os"
	"sort"

	"resultproxy-wails/internal/verdict"
)

// SmartLearnedRoutes lists the destinations Adaptive Smart learned to send
// through the VPN. Hidden counts destinations saved only as hashes.
type SmartLearnedRoutes struct {
	Items  []string `json:"items"`
	Hidden int      `json:"hidden"`
}

// ManageSmartLearnedRoutes reads the learned routes, optionally removing one
// named route or clearing all learned decisions. A nil relay reads the cache
// directly while the VPN is stopped.
func ManageSmartLearnedRoutes(dataDir string, relay *SmartRelay, forgetHost string, reset bool) (SmartLearnedRoutes, error) {
	forgetHost = verdict.NormalizeHost(forgetHost)
	var store *verdict.Store
	if relay != nil {
		store = relay.store
	} else {
		cache, err := loadSavedSmartLearnedRoutes(dataDir)
		if err != nil {
			return SmartLearnedRoutes{}, err
		}
		store = cache
	}
	if reset || forgetHost != "" {
		if relay == nil {
			editSmartLearnedRoutes(store, forgetHost, reset)
			if err := saveSmartLearnedRoutes(dataDir, store); err != nil {
				return SmartLearnedRoutes{}, err
			}
		} else if err := relay.editLearnedRoutes(forgetHost, reset); err != nil {
			return SmartLearnedRoutes{}, err
		}
	}
	names, hidden := store.Learned(verdict.Proxy)
	items := make([]string, 0, len(names))
	for name := range names {
		items = append(items, name)
	}
	sort.Strings(items)
	return SmartLearnedRoutes{Items: items, Hidden: hidden}, nil
}

func editSmartLearnedRoutes(store *verdict.Store, host string, reset bool) {
	if reset {
		store.ClearLearned()
	} else {
		store.ForgetLearned(host)
	}
}

func (r *SmartRelay) editLearnedRoutes(host string, reset bool) error {
	r.learnMu.Lock()
	defer r.learnMu.Unlock()
	r.learnGeneration++
	if reset {
		r.losses.clear()
	} else {
		r.losses.reset(host)
	}
	editSmartLearnedRoutes(r.store, host, reset)
	if r.opts.MemoryOnly {
		// Apply the explicit removal to any disk cache left from a previous
		// session without writing any decisions learned in the current one.
		if _, err := os.Stat(SmartVerdictStorePath(r.opts.DataDir)); os.IsNotExist(err) {
			if reset {
				return RenderSmartDirectSet(SmartDirectSetPath(r.opts.DataDir), nil)
			}
			return nil
		}
		cache, err := loadSavedSmartLearnedRoutes(r.opts.DataDir)
		if err != nil {
			return err
		}
		editSmartLearnedRoutes(cache, host, reset)
		return saveSmartLearnedRoutes(r.opts.DataDir, cache)
	}
	r.persistMu.Lock()
	defer r.persistMu.Unlock()
	return saveSmartLearnedRoutes(r.opts.DataDir, r.store)
}

func loadSavedSmartLearnedRoutes(dataDir string) (*verdict.Store, error) {
	store, err := verdict.Load(SmartVerdictStorePath(dataDir), nil)
	if err != nil {
		return nil, err
	}
	store.Rehydrate(ReadSmartDirectSet(SmartDirectSetPath(dataDir)))
	return store, nil
}

func saveSmartLearnedRoutes(dataDir string, store *verdict.Store) error {
	if err := store.Save(SmartVerdictStorePath(dataDir)); err != nil {
		return err
	}
	return RenderSmartDirectSet(SmartDirectSetPath(dataDir), directNamesOf(store))
}
