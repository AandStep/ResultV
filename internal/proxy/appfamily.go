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
	"regexp"
	"strings"
	"sync"
	"time"

	"resultproxy-wails/internal/system/processtree"
)

// appFamily is what the app lists say about a process and everything it
// started: excluding steam.exe excludes every game Steam launches.
type appFamily uint8

const (
	familyNone appFamily = iota
	familyDirect
	familyVPN
)

// The family reaches the rules as a package name, matched by package_name.
// Only Android fills that field itself, so the tags cannot collide with a
// real package on the platforms this client runs on.
const (
	familyTagDirect = "resultv.direct"
	familyTagVPN    = "resultv.vpn"
)

func (f appFamily) tag() string {
	switch f {
	case familyDirect:
		return familyTagDirect
	case familyVPN:
		return familyTagVPN
	}
	return ""
}

// familyBarriers never inherit a family. A browser opened from a game
// launcher becomes the browser for everything the user does next, and
// inheriting would quietly take all of it out of the tunnel. Listing a
// browser explicitly still works.
var familyBarriers = map[string]struct{}{
	"chrome.exe":        {},
	"chromium.exe":      {},
	"msedge.exe":        {},
	"brave.exe":         {},
	"firefox.exe":       {},
	"opera.exe":         {},
	"vivaldi.exe":       {},
	"browser.exe":       {},
	"waterfox.exe":      {},
	"librewolf.exe":     {},
	"yandex.exe":        {},
	"yandexbrowser.exe": {},
}

// familyCompanions are services Windows starts for a game through the service
// manager, so they have no game among their ancestors. They follow whichever
// family is running.
var familyCompanions = map[string]struct{}{
	"easyanticheat.exe":     {},
	"easyanticheat_eos.exe": {},
	"beservice.exe":         {},
	"beservice_x64.exe":     {},
	"pnkbstra.exe":          {},
	"pnkbstrb.exe":          {},
	"steamservice.exe":      {},
}

type appLists struct {
	direct []*regexp.Regexp
	vpn    []*regexp.Regexp
}

func newAppLists(direct, vpn []string) appLists {
	return appLists{direct: compileAppList(direct), vpn: compileAppList(vpn)}
}

func compileAppList(entries []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, rx := range appWhitelistPathRegexes(entries) {
		if re, err := regexp.Compile(rx); err == nil {
			out = append(out, re)
		}
	}
	return out
}

func (l appLists) empty() bool { return len(l.direct) == 0 && len(l.vpn) == 0 }

// own is the family a process has by its own name. An explicit "via VPN"
// wins over an accidental overlap with the exclusions.
func (l appLists) own(image string) appFamily {
	if image == "" {
		return familyNone
	}
	for _, re := range l.vpn {
		if re.MatchString(image) {
			return familyVPN
		}
	}
	for _, re := range l.direct {
		if re.MatchString(image) {
			return familyDirect
		}
	}
	return familyNone
}

// familyOfChain lets the nearest listed process decide: the process itself
// when it is listed, otherwise its closest listed ancestor.
func familyOfChain(chain []processtree.Process, lists appLists) appFamily {
	for _, p := range chain {
		if f := lists.own(p.Image); f != familyNone {
			return f
		}
		if _, barrier := familyBarriers[imageBase(p.Image)]; barrier {
			return familyNone
		}
	}
	return familyNone
}

func imageBase(image string) string {
	if i := strings.LastIndexAny(image, `\/`); i >= 0 {
		image = image[i+1:]
	}
	return strings.ToLower(image)
}

var selfPID = uint32(os.Getpid())

type lineage interface {
	Identify(pid uint32) (processtree.Process, bool)
	Ancestry(pid uint32) []processtree.Process
	Live() []processtree.Process
	Young(p processtree.Process, d time.Duration) bool
}

type familyKey struct {
	pid     uint32
	created int64
}

type familyMemo struct {
	family  appFamily
	expires time.Time
}

const (
	familyMemoTTL      = 30 * time.Minute
	familyMemoYoungTTL = 2 * time.Second
	familyMemoMax      = 4096
	companionTTL       = 5 * time.Second
)

// familyResolver answers per connection. One per engine boot, so a change of
// the lists starts from a clean memo.
type familyResolver struct {
	lists appLists
	tree  lineage
	now   func() time.Time

	mu        sync.Mutex
	memo      map[familyKey]familyMemo
	companion familyMemo
}

func newFamilyResolver(lists appLists, tree lineage) *familyResolver {
	return &familyResolver{lists: lists, tree: tree, now: time.Now, memo: make(map[familyKey]familyMemo)}
}

// family resolves pid. path is what the core found for the connection, used
// when the tree cannot see the process at all.
func (r *familyResolver) family(pid uint32, path string) appFamily {
	// The app's own probes must keep their path whatever started the app.
	if pid == selfPID {
		return familyNone
	}
	now := r.now()
	self, known := r.tree.Identify(pid)
	key := familyKey{pid: pid, created: self.Created}
	if known {
		r.mu.Lock()
		m, ok := r.memo[key]
		r.mu.Unlock()
		if ok && now.Before(m.expires) {
			return m.family
		}
	}

	chain := r.tree.Ancestry(pid)
	if len(chain) == 0 {
		return r.lists.own(path)
	}
	self = chain[0]
	key = familyKey{pid: pid, created: self.Created}

	f := familyOfChain(chain, r.lists)
	if f == familyNone {
		if _, ok := familyCompanions[imageBase(self.Image)]; ok {
			f = r.companionFamily(now)
		}
	}

	// A process seconds old may still be waiting on its parent's start event,
	// so its answer is only kept briefly.
	ttl := familyMemoTTL
	if r.tree.Young(self, 10*time.Second) {
		ttl = familyMemoYoungTTL
	}
	r.mu.Lock()
	if len(r.memo) >= familyMemoMax {
		r.memo = make(map[familyKey]familyMemo)
	}
	r.memo[key] = familyMemo{family: f, expires: now.Add(ttl)}
	r.mu.Unlock()
	return f
}

// companionFamily is direct while any excluded family is running, otherwise
// VPN while a VPN family is.
func (r *familyResolver) companionFamily(now time.Time) appFamily {
	r.mu.Lock()
	if now.Before(r.companion.expires) {
		f := r.companion.family
		r.mu.Unlock()
		return f
	}
	r.mu.Unlock()

	live := r.tree.Live()
	alive := make(map[familyKey]struct{}, len(live))
	found := familyNone
	for _, p := range live {
		alive[familyKey{pid: p.PID, created: p.Created}] = struct{}{}
		found = betterCompanion(found, r.lists.own(p.Image))
	}
	r.mu.Lock()
	for k, m := range r.memo {
		if _, ok := alive[k]; ok && now.Before(m.expires) {
			found = betterCompanion(found, m.family)
		}
	}
	r.companion = familyMemo{family: found, expires: now.Add(companionTTL)}
	r.mu.Unlock()
	return found
}

func betterCompanion(cur, next appFamily) appFamily {
	if cur == familyDirect || next == familyNone {
		return cur
	}
	return next
}
