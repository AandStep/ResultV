package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompileSmartSRS_RoundTripMatches(t *testing.T) {
	dir := t.TempDir()
	path := SmartSRSPath(dir)
	if err := CompileSmartSRS([]string{"x.com", "instagram.com", "youtube.com"}, path); err != nil {
		t.Fatalf("CompileSmartSRS: %v", err)
	}
	if !localSmartSRSUsable(path) {
		t.Fatal("compiled SRS should be usable")
	}
	m, err := LoadSmartDomainMatcher(path)
	if err != nil {
		t.Fatalf("LoadSmartDomainMatcher: %v", err)
	}
	// Pins the same semantics as engine_smart_test.go: bare AND subdomain.
	for _, host := range []string{"x.com", "instagram.com", "www.instagram.com", "i.instagram.com"} {
		if !m.Match(host) {
			t.Errorf("matcher must match %q", host)
		}
	}
	for _, host := range []string{"fakeinstagram.com", "example.org"} {
		if m.Match(host) {
			t.Errorf("matcher must NOT match %q", host)
		}
	}
}

func TestCompileSmartSRS_AtomicNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := SmartSRSPath(dir)
	if err := CompileSmartSRS([]string{"x.com"}, path); err != nil {
		t.Fatal(err)
	}
	// A failed compile must not clobber a good existing file.
	if err := CompileSmartSRS(nil, path); err == nil {
		t.Fatal("empty domain list should error, not write")
	}
	if !localSmartSRSUsable(path) {
		t.Fatal("previous good SRS must survive a failed compile")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file must not be left behind")
	}
}

func TestLocalSmartSRSUsable_RejectsAndRemovesCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := SmartSRSPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not an srs file at all, but long enough to pass a size check"), 0o600); err != nil {
		t.Fatal(err)
	}
	if localSmartSRSUsable(path) {
		t.Fatal("corrupt SRS must not be reported usable")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("corrupt SRS must be deleted so a refresh can replace it")
	}
}

func TestBuildSmartRuleSet_OnlyWhenUsable(t *testing.T) {
	dir := t.TempDir()
	if got := buildSmartRuleSet(dir); len(got) != 0 {
		t.Fatalf("no SRS on disk should yield no rule_set, got %+v", got)
	}
	if err := CompileSmartSRS([]string{"x.com"}, SmartSRSPath(dir)); err != nil {
		t.Fatal(err)
	}
	got := buildSmartRuleSet(dir)
	if len(got) != 1 {
		t.Fatalf("expected 1 rule_set, got %d", len(got))
	}
	if got[0].Type != "local" || got[0].Format != "binary" || got[0].Tag != smartRuleSetTag {
		t.Fatalf("unexpected rule_set: %+v", got[0])
	}
}

func TestBuildRoute_SmartSRS_PreferredOverInline(t *testing.T) {
	dir := t.TempDir()
	if err := CompileSmartSRS([]string{"x.com", "instagram.com"}, SmartSRSPath(dir)); err != nil {
		t.Fatal(err)
	}
	route := buildRoute(EngineConfig{
		Mode:      ProxyModeTunnel,
		IsAndroid: true,
		DataDir:   dir,
		SmartMode: true,
		// Deliberately ALSO passed inline — SRS must win, and the config must
		// not carry 150k domains twice.
		SmartBlockedDomains: []string{"x.com", "instagram.com"},
	})
	if route.Final != "direct" {
		t.Fatalf("smart with SRS should use final=direct, got %q", route.Final)
	}
	var haveSet bool
	for _, rs := range route.RuleSet {
		if rs.Tag == smartRuleSetTag && rs.Type == "local" && rs.Format == "binary" {
			haveSet = true
		}
	}
	if !haveSet {
		t.Fatalf("expected a local smart rule_set, got %+v", route.RuleSet)
	}
	var smartRule *SBRouteRule
	for i := range route.Rules {
		for _, tag := range route.Rules[i].RuleSet {
			if tag == smartRuleSetTag {
				smartRule = &route.Rules[i]
			}
		}
	}
	if smartRule == nil {
		t.Fatal("expected a route rule referencing the smart rule_set")
	}
	if smartRule.Outbound != "proxy" {
		t.Fatalf("smart rule_set must route to proxy, got %q", smartRule.Outbound)
	}
	// Tighten check to specific test domains, not just any domain_suffix rule
	// with outbound="proxy". This keeps the test immune to other rules like
	// YouTube core domains → proxy.
	for _, r := range route.Rules {
		if r.Outbound == "proxy" && len(r.RuleSet) == 0 {
			for _, suffix := range r.DomainSuffix {
				if suffix == "x.com" || suffix == "instagram.com" {
					t.Fatalf("inline smart domains must NOT be emitted when SRS is present: %+v", r)
				}
			}
		}
	}
}

func TestBuildRoute_SmartInlineStillWorksWithoutSRS(t *testing.T) {
	// Desktop path: no SRS on disk, domains passed inline.
	route := buildRoute(EngineConfig{
		Mode:                ProxyModeTunnel,
		DataDir:             t.TempDir(),
		SmartMode:           true,
		SmartBlockedDomains: []string{"instagram.com"},
	})
	if route.Final != "direct" {
		t.Fatalf("inline smart should still use final=direct, got %q", route.Final)
	}
	var found bool
	for _, r := range route.Rules {
		for _, s := range r.DomainSuffix {
			if s == "instagram.com" && r.Outbound == "proxy" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("inline smart domains must still be emitted when no SRS exists")
	}
}

func TestBuildRoute_SmartNoListAtAll_StaysGlobal(t *testing.T) {
	route := buildRoute(EngineConfig{
		Mode:      ProxyModeTunnel,
		DataDir:   t.TempDir(),
		SmartMode: true,
	})
	if route.Final != "proxy" {
		t.Fatalf("smart with no list must keep Global final=proxy, got %q", route.Final)
	}
}

func TestBuildRoute_SmartSRS_WithAdBlock_RuleOrder(t *testing.T) {
	// Coverage for SmartMode:true + AdBlock:true interaction with SRS.
	// Rule order is load-bearing: ad-block rules must precede the smart rule.
	dir := t.TempDir()
	if err := CompileSmartSRS([]string{"blocked.example.com"}, SmartSRSPath(dir)); err != nil {
		t.Fatal(err)
	}
	route := buildRoute(EngineConfig{
		Mode:                ProxyModeTunnel,
		IsAndroid:           true,
		DataDir:             dir,
		SmartMode:           true,
		AdBlock:             true,
		SmartBlockedDomains: []string{"blocked.example.com"},
	})

	// Verify the smart rule_set rule is emitted.
	var smartRuleIdx int = -1
	for i, r := range route.Rules {
		for _, tag := range r.RuleSet {
			if tag == smartRuleSetTag {
				smartRuleIdx = i
			}
		}
	}
	if smartRuleIdx == -1 {
		t.Fatal("expected a route rule referencing the smart rule_set")
	}

	// Verify ad-block reject rules precede the smart rule.
	// Ad-block emits reject rules for its tag-based matches; find one.
	var adBlockRejectIdx int = -1
	for i, r := range route.Rules {
		if r.Action == "reject" && len(r.RuleSet) > 0 {
			adBlockRejectIdx = i
			break
		}
	}
	if adBlockRejectIdx != -1 && adBlockRejectIdx >= smartRuleIdx {
		t.Fatalf("ad-block reject rule (index %d) must precede smart rule (index %d)", adBlockRejectIdx, smartRuleIdx)
	}

	// Verify smart rule uses the SRS tag.
	smartRule := &route.Rules[smartRuleIdx]
	if smartRule.Outbound != "proxy" {
		t.Fatalf("smart rule must route to proxy, got %q", smartRule.Outbound)
	}
	var hasSRSTag bool
	for _, tag := range smartRule.RuleSet {
		if tag == smartRuleSetTag {
			hasSRSTag = true
		}
	}
	if !hasSRSTag {
		t.Fatalf("smart rule must reference %q tag, got %v", smartRuleSetTag, smartRule.RuleSet)
	}
}
