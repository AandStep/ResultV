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

package proxy

import (
	"os"
	"testing"
)

// TestMain installs a throwaway, disk-backed node-stat store for the whole
// package before any test runs.
//
// nodeStats() lazily creates a process-wide store on first use, so without this
// the store a test writes to is whatever the previous test left behind — and
// with dataDir == "" nothing would be written to disk, but every RecordProbe in
// the package would still accumulate in one shared map. A temp dir here gives
// the package a defined starting point and keeps Flush() away from the user's
// real data directory.
//
// This is the floor, not the isolation: tests whose assertions depend on a
// node's stats must call isolateNodeStats so a neighbouring test's probes
// cannot reach them. See its comment.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "resultvpc-proxy-tests-")
	if err != nil {
		panic(err)
	}
	SetNodeStatStore(NewNodeStatStore(dir))

	code := m.Run()

	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// isolateNodeStats gives one test its own empty node-stat store and restores
// the previous one afterwards.
//
// Ranking reads ConsecFails/LastFailAt/EWMA out of that store (see scoreNode),
// so a test that leaves failures in the shared store silently changes the order
// a later test measures. That is not hypothetical: with every test dumping into
// one store, TestRankAutoCandidates_OrdersByRTTAndCapsAtFive only passed
// because the tests that record failures happen to be declared after it in the
// file — and it duly failed under `-count=2`, where the second run starts with
// the first run's failures already recorded.
// Returns the fresh store so a test can seed history into it; callers that
// only need the isolation can ignore the result.
func isolateNodeStats(t *testing.T) *NodeStatStore {
	t.Helper()
	old := nodeStats()
	t.Cleanup(func() { SetNodeStatStore(old) })
	store := NewNodeStatStore(t.TempDir())
	SetNodeStatStore(store)
	return store
}
