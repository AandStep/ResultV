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
	"path/filepath"
	"slices"
	"testing"
	"time"

	"resultproxy-wails/internal/system/processtree"
)

func familyRuleIndex(rules []SBRouteRule, tag, outbound string) int {
	for i, r := range rules {
		if r.Action == "route" && r.Outbound == outbound && slices.Contains(r.PackageName, tag) {
			return i
		}
	}
	return -1
}

func proc(pid, ppid uint32, image string) processtree.Process {
	return processtree.Process{PID: pid, ParentPID: ppid, Created: int64(pid), Image: image}
}

var (
	pExplorer = proc(100, 50, `C:\Windows\explorer.exe`)
	pSteam    = proc(200, 100, `C:\Program Files (x86)\Steam\steam.exe`)
	pHelper   = proc(210, 200, `C:\Program Files (x86)\Steam\bin\cef\steamwebhelper.exe`)
	pGame     = proc(300, 200, `D:\SteamLibrary\steamapps\common\dota 2 beta\game\bin\win64\dota2.exe`)
	pChrome   = proc(310, 200, `C:\Program Files\Google\Chrome\Application\chrome.exe`)
	pRenderer = proc(320, 310, `C:\Program Files\Google\Chrome\Application\chrome.exe`)
	pServices = proc(600, 500, `C:\Windows\System32\services.exe`)
	pEAC      = proc(610, 600, `C:\Program Files (x86)\EasyAntiCheat_EOS\EasyAntiCheat_EOS.exe`)
)

func TestFamilyOfChain_NearestListedDecides(t *testing.T) {
	steamOnly := newAppLists([]string{"steam.exe"}, nil)
	cases := []struct {
		name  string
		chain []processtree.Process
		lists appLists
		want  appFamily
	}{
		{"the game inherits the launcher", []processtree.Process{pGame, pSteam, pExplorer}, steamOnly, familyDirect},
		{"the launcher itself", []processtree.Process{pSteam, pExplorer}, steamOnly, familyDirect},
		{"an unrelated process", []processtree.Process{pExplorer}, steamOnly, familyNone},
		{"a listed child beats its listed parent",
			[]processtree.Process{pHelper, pSteam, pExplorer},
			newAppLists([]string{"steam.exe"}, []string{"steamwebhelper.exe"}), familyVPN},
		{"a browser opened by the launcher stays in the tunnel",
			[]processtree.Process{pRenderer, pChrome, pSteam, pExplorer}, steamOnly, familyNone},
		{"a browser the user listed is excluded",
			[]processtree.Process{pRenderer, pChrome, pSteam},
			newAppLists([]string{"chrome.exe"}, nil), familyDirect},
	}
	for _, c := range cases {
		if got := familyOfChain(c.chain, c.lists); got != c.want {
			t.Errorf("%s: family %d, want %d", c.name, got, c.want)
		}
	}
}

// A path-qualified entry must not widen into its bare name, under a DOS path
// or the device path an ETW event reports.
func TestAppLists_PathQualifiedEntryStaysNarrow(t *testing.T) {
	lists := newAppLists([]string{`Battle.net\Agent\Agent.exe`}, nil)
	for image, want := range map[string]appFamily{
		`C:\ProgramData\Battle.net\Agent\Agent.exe`:                      familyDirect,
		`\Device\HarddiskVolume3\ProgramData\Battle.net\Agent\Agent.exe`: familyDirect,
		`C:\Program Files\Docker\Agent.exe`:                              familyNone,
		`C:\ProgramData\Battle.net\Agent\Agent.8519\Agent.exe`:           familyNone,
	} {
		if got := lists.own(image); got != want {
			t.Errorf("%s: family %d, want %d", image, got, want)
		}
	}
}

type fakeLineage struct {
	procs map[uint32]processtree.Process
	young bool
	walks int
}

func newFakeLineage(procs ...processtree.Process) *fakeLineage {
	l := &fakeLineage{procs: map[uint32]processtree.Process{}}
	for _, p := range procs {
		l.procs[p.PID] = p
	}
	return l
}

func (l *fakeLineage) Identify(pid uint32) (processtree.Process, bool) {
	p, ok := l.procs[pid]
	return p, ok
}

func (l *fakeLineage) Ancestry(pid uint32) []processtree.Process {
	l.walks++
	var chain []processtree.Process
	for p, ok := l.procs[pid]; ok; p, ok = l.procs[p.ParentPID] {
		chain = append(chain, p)
	}
	return chain
}

func (l *fakeLineage) Live() []processtree.Process {
	var out []processtree.Process
	for _, p := range l.procs {
		out = append(out, p)
	}
	return out
}

func (l *fakeLineage) Young(processtree.Process, time.Duration) bool { return l.young }

func TestFamilyResolver_MemoizesPerProcess(t *testing.T) {
	tree := newFakeLineage(pExplorer, pSteam, pGame)
	r := newFamilyResolver(newAppLists([]string{"steam.exe"}, nil), tree)
	for range 3 {
		if got := r.family(pGame.PID, pGame.Image); got != familyDirect {
			t.Fatalf("family %d, want direct", got)
		}
	}
	if tree.walks != 1 {
		t.Fatalf("walked the tree %d times for one process", tree.walks)
	}
}

// A process seconds old may still be missing its parent: its answer must not
// outlive the moment the parent's start event arrives.
func TestFamilyResolver_YoungAnswerExpiresQuickly(t *testing.T) {
	tree := newFakeLineage(pExplorer, pGame)
	tree.young = true
	now := time.Unix(1000, 0)
	r := newFamilyResolver(newAppLists([]string{"steam.exe"}, nil), tree)
	r.now = func() time.Time { return now }
	if got := r.family(pGame.PID, pGame.Image); got != familyNone {
		t.Fatalf("family %d before the parent is known, want none", got)
	}
	tree.procs[pSteam.PID] = pSteam
	now = now.Add(familyMemoYoungTTL + time.Millisecond)
	if got := r.family(pGame.PID, pGame.Image); got != familyDirect {
		t.Fatalf("family %d once the parent is known, want direct", got)
	}
}

func TestFamilyResolver_FallsBackToTheCorePath(t *testing.T) {
	r := newFamilyResolver(newAppLists([]string{"steam.exe"}, nil), newFakeLineage())
	if got := r.family(999, `C:\Steam\steam.exe`); got != familyDirect {
		t.Fatalf("family %d for a process the tree cannot see, want direct", got)
	}
}

func TestFamilyResolver_SelfIsNeverInAFamily(t *testing.T) {
	self := proc(selfPID, pExplorer.PID, `C:\ResultV\ResultV.exe`)
	r := newFamilyResolver(newAppLists([]string{"explorer.exe"}, nil), newFakeLineage(pExplorer, self))
	if got := r.family(selfPID, self.Image); got != familyNone {
		t.Fatalf("the app's own process got family %d", got)
	}
}

func TestFamilyResolver_CompanionFollowsTheRunningFamily(t *testing.T) {
	lists := newAppLists([]string{"steam.exe"}, []string{"Battle.net.exe"})

	r := newFamilyResolver(lists, newFakeLineage(pServices, pEAC, pExplorer, pSteam))
	if got := r.family(pEAC.PID, pEAC.Image); got != familyDirect {
		t.Fatalf("anti-cheat beside a running excluded launcher: family %d, want direct", got)
	}

	battle := proc(700, 100, `C:\Program Files (x86)\Battle.net\Battle.net.exe`)
	r = newFamilyResolver(lists, newFakeLineage(pServices, pEAC, pExplorer, battle))
	if got := r.family(pEAC.PID, pEAC.Image); got != familyVPN {
		t.Fatalf("anti-cheat beside a running VPN launcher: family %d, want vpn", got)
	}

	r = newFamilyResolver(lists, newFakeLineage(pServices, pEAC, pExplorer))
	if got := r.family(pEAC.PID, pEAC.Image); got != familyNone {
		t.Fatalf("anti-cheat with no family running: family %d, want none", got)
	}
}

func TestBuildRoute_AppFamiliesPrecedeRoutingLists(t *testing.T) {
	list := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(list, []byte(`{"version":3,"rules":[{"domain_suffix":["example.com"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := EngineConfig{
		Mode:         ProxyModeTunnel,
		RoutingMode:  ModeGlobal,
		Proxy:        ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		AppWhitelist: []string{"steam.exe"},
		AppForceVPN:  []string{"Telegram.exe"},
		Whitelist:    []string{"example.org"},
		RoutingLists: []RoutingListSpec{{Tag: "rl-test", Path: list, Action: "proxy"}},
	}
	rules := buildRoute(cfg).Rules
	vpn := familyRuleIndex(rules, familyTagVPN, "proxy")
	direct := familyRuleIndex(rules, familyTagDirect, appDirectTag)
	listIdx := indexOfRule(rules, func(r SBRouteRule) bool { return slices.Contains(r.RuleSet, "rl-test") })
	sites := indexOfRule(rules, func(r SBRouteRule) bool { return slices.Contains(r.DomainSuffix, "example.org") })
	if vpn < 0 || direct < 0 || listIdx < 0 || sites < 0 {
		t.Fatalf("missing rule: vpn=%d direct=%d list=%d sites=%d, rules=%+v", vpn, direct, listIdx, sites, rules)
	}
	if !(vpn < direct && direct < listIdx && direct < sites) {
		t.Fatalf("order vpn=%d direct=%d list=%d sites=%d: the app families must come first", vpn, direct, listIdx, sites)
	}
	for _, r := range rules {
		if len(r.ProcessPathRegex) > 0 && r.Outbound != "direct" {
			continue
		}
		for _, rx := range r.ProcessPathRegex {
			if rx == processPathRegex("steam.exe") || rx == processPathRegex("Telegram.exe") {
				t.Fatalf("app lists must route by family only, found %+v", r)
			}
		}
	}
}

func TestBuildTunnelModeConfig_AppDirectBuildsInCore(t *testing.T) {
	cfg := EngineConfig{
		Mode:         ProxyModeTunnel,
		RoutingMode:  ModeGlobal,
		Proxy:        ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		AppWhitelist: []string{"steam.exe"},
		AppForceVPN:  []string{"Telegram.exe"},
		DataDir:      t.TempDir(),
	}
	out := mustBuildTunnelModeConfig(t, cfg)
	if !slices.ContainsFunc(out.Outbounds, func(o SBOutbound) bool { return o.Type == appDirectTag && o.Tag == appDirectTag }) {
		t.Fatalf("app-direct outbound missing: %+v", out.Outbounds)
	}
	assertCoreBuildsConfig(t, out)

	if !out.DNS.ReverseMapping {
		t.Fatal("app-direct needs the reverse mapping to know which addresses came from a lookup")
	}

	for name, c := range map[string]EngineConfig{
		"no excluded apps": func() EngineConfig { c := cfg; c.AppWhitelist = nil; return c }(),
		"smart mode":       func() EngineConfig { c := cfg; c.RoutingMode = ModeSmart; return c }(),
	} {
		out := mustBuildTunnelModeConfig(t, c)
		if slices.ContainsFunc(out.Outbounds, func(o SBOutbound) bool { return o.Tag == appDirectTag }) {
			t.Errorf("%s: app-direct must not be built", name)
		}
		if out.DNS.ReverseMapping {
			t.Errorf("%s: reverse mapping must stay off", name)
		}
	}
}

// Listing an app next to its launcher, or twice, never cancels anything out:
// unlike the old site list, a match is a match however many entries hit it.
func TestFamilyResolver_OverlappingEntriesStayDirect(t *testing.T) {
	lists := newAppLists([]string{"steam.exe", "EasyAntiCheat_EOS.exe", "steam.exe", "dota2.exe"}, nil)
	tree := newFakeLineage(pServices, pEAC, pExplorer, pSteam, pGame)
	r := newFamilyResolver(lists, tree)
	for _, p := range []processtree.Process{pSteam, pGame, pEAC} {
		if got := r.family(p.PID, p.Image); got != familyDirect {
			t.Errorf("%s: family %d, want direct", p.Image, got)
		}
	}
	delete(tree.procs, pSteam.PID)
	r = newFamilyResolver(lists, tree)
	if got := r.family(pEAC.PID, pEAC.Image); got != familyDirect {
		t.Errorf("a listed anti-cheat must stay direct with no launcher running, got %d", got)
	}
}
