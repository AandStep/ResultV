//go:build smartfakeip

// Answers one question: what does a third-party app's name lookup get back
// while the adaptive engine is on?
//
// The user's report (2026-09-22) is that with the toggle on, Claude Code dies
// with ENOTFOUND on api.anthropic.com — and on Windows a DNS timeout reaches an
// application as exactly that, so a silent rule is indistinguishable from a
// missing host. This stands the real DNS block up — the same servers and the
// same rule order buildDNS emits with AdaptiveSmart on, against the user's own
// node and block-list — and asks it as an inbound would (Exchange, where FakeIP
// is allowed), so the answer is the one the application would have received.
//
// Deliberately PROXY mode: no TUN is created, no route, DNS or firewall state
// on the machine is touched, so this is safe to run while a session is live.
//
// ONE CAVEAT, and it will waste an hour if it is not read. While the app holds
// a live tunnel with strict_route on, its WFP filters drop this process's own
// UDP/53 to the physical adapters, so the "local" server inside the stand
// answers nothing and every dial-time lookup takes the full deadline. That is
// the stand's environment, not the shape under test — the app's own engine is
// allowed through its own filters. Either disconnect first, or run with
// RESULTV_TEST_DIRECT_RESOLVER=google to point direct at the tunnel resolver;
// with that, a fakeip destination answered in ~330ms (2026-09-22), against ten
// seconds and a 502 before the outbound was given a resolver at all.
//
//	go test -tags "smartfakeip,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_naive_outbound,with_purego,with_grpc" \
//	  -run TestAdaptiveDNSAnswersApplications ./internal/proxy/ -v -timeout 5m

package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	mDNS "github.com/miekg/dns"
)

const smartFakeIPLocalPort = 24095

func TestAdaptiveDNSAnswersApplications(t *testing.T) {
	node := dnsTrFindNode(t)
	say("узел: %s %s:%d", node.Type, node.IP, node.Port)

	dataDir := t.TempDir()
	blocked, cidrs := smartFakeIPLists(t)
	srsPath, err := CompileSmartRuleSet(dataDir, blocked)
	if err != nil {
		t.Fatalf("rule-set: %v", err)
	}
	say("блок-список: %d доменов, %d сетей", len(blocked), len(cidrs))

	engineCfg := EngineConfig{
		Proxy:              node,
		Mode:               ProxyModeProxy,
		LocalPort:          smartFakeIPLocalPort,
		RoutingMode:        ModeSmart,
		BlockedDomains:     blocked,
		BlockedCIDRs:       cidrs,
		SmartRuleSetPath:   srsPath,
		DataDir:            dataDir,
		AdaptiveSmart:      true,
		SelfExecutablePath: `C:\Program Files\ResultV\ResultV.exe`,
	}
	cfg, err := BuildProxyModeConfig(engineCfg)
	if err != nil {
		t.Fatalf("конфиг: %v", err)
	}
	// The DNS block is the thing under test, and only the tunnel branch of
	// buildDNS emits the adaptive shape. Everything else stays proxy mode.
	tunnelCfg := engineCfg
	tunnelCfg.Mode = ProxyModeTunnel
	cfg.DNS = buildDNS(tunnelCfg)
	// The outbound list comes from the proxy-mode build, where FakeIP is off;
	// give direct the same resolver tunnel mode gives it, or the stand tests a
	// shape the app never ships.
	for i := range cfg.Outbounds {
		if cfg.Outbounds[i].Tag == "direct" {
			resolver := directDomainResolverTag(tunnelCfg)
			if env := os.Getenv("RESULTV_TEST_DIRECT_RESOLVER"); env != "" {
				resolver = env
			}
			cfg.Outbounds[i].DomainResolver = resolver
			say("  domain_resolver у direct: %q", resolver)
		}
	}
	if cfg.Experimental == nil {
		cfg.Experimental = &SBExperimental{}
	}
	if cfg.Experimental.CacheFile == nil {
		cfg.Experimental.CacheFile = &SBCacheFile{Enabled: true, Path: dataDir + `\cache.db`}
	}
	cfg.Experimental.CacheFile.StoreFakeIP = true
	cfg.Log = &SBLog{Level: "debug"}

	for i, rule := range cfg.DNS.Rules {
		raw, _ := json.Marshal(rule)
		say("  правило DNS %d: %s", i+1, string(raw))
	}
	say("  final=%s", cfg.DNS.Final)

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boxCtx := extendedBoxContext(ctx)
	var options option.Options
	if err := singjson.UnmarshalContext(boxCtx, raw, &options); err != nil {
		t.Fatalf("ядро не приняло конфиг: %v", err)
	}
	instance, err := box.New(box.Options{Context: boxCtx, Options: options})
	if err != nil {
		t.Fatalf("box.New: %v", err)
	}
	if err := instance.Start(); err != nil {
		t.Fatalf("старт: %v", err)
	}
	defer func() {
		done := make(chan struct{})
		go func() { _ = instance.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	}()
	time.Sleep(2 * time.Second)

	dnsRouter := service.FromContext[adapter.DNSRouter](boxCtx)
	if dnsRouter == nil {
		t.Fatal("DNSRouter не найден в контексте")
	}

	names := []string{
		"api.anthropic.com",
		"claude.ai",
		"www.anthropic.com",
		"sentry.io",
		"www.youtube.com",
		"mail.ru",
		"example.com",
		"registry.npmjs.org",
	}
	var silent int
	for _, name := range names {
		msg := new(mDNS.Msg)
		msg.SetQuestion(mDNS.Fqdn(name), mDNS.TypeA)
		queryCtx, queryCancel := context.WithTimeout(context.Background(), 12*time.Second)
		started := time.Now()
		// Exchange, not Lookup: an application's query arrives through the
		// inbound, which is the only path where a fakeip transport may answer.
		resp, err := dnsRouter.Exchange(queryCtx, msg, adapter.DNSQueryOptions{})
		elapsed := time.Since(started).Round(time.Millisecond)
		queryCancel()
		if err != nil {
			silent++
			say("  %-24s %8s  ОШИБКА: %v   <- приложение увидит ENOTFOUND", name, elapsed, err)
			continue
		}
		var addrs []string
		for _, answer := range resp.Answer {
			if a, isA := answer.(*mDNS.A); isA {
				addrs = append(addrs, a.A.String())
			}
		}
		if len(addrs) == 0 {
			silent++
			say("  %-24s %8s  rcode=%s, ни одной A-записи   <- приложение увидит ENOTFOUND",
				name, elapsed, mDNS.RcodeToString[resp.Rcode])
			continue
		}
		kind := "настоящий"
		if addr, parseErr := netip.ParseAddr(addrs[0]); parseErr == nil && isFakeIPAddr(addr.AsSlice()) {
			kind = "FakeIP"
		}
		say("  %-24s %8s  %-9s %v", name, elapsed, kind, addrs)
	}
	if silent > 0 {
		t.Errorf("без адреса осталось имён: %d из %d", silent, len(names))
	}

	// A TXT query matches no rule (the fakeip catch-all is scoped to A/AAAA),
	// so it lands on dns.final — the very server the direct outbound now names.
	{
		msg := new(mDNS.Msg)
		msg.SetQuestion(mDNS.Fqdn("example.com"), mDNS.TypeTXT)
		queryCtx, queryCancel := context.WithTimeout(context.Background(), 12*time.Second)
		started := time.Now()
		resp, err := dnsRouter.Exchange(queryCtx, msg, adapter.DNSQueryOptions{})
		queryCancel()
		say("  сам резолвер local (TXT example.com) %8s  err=%v answers=%d",
			time.Since(started).Round(time.Millisecond), err, len(answersOf(resp)))
	}

	// A name is only half the story: the client then dials the fake address,
	// and the router has to turn it back into a destination. This is the half
	// an application reports as "cannot reach", so it is measured here too.
	for _, probe := range []struct{ name, addr string }{
		{"example.com", "198.18.0.3"},
		{"registry.npmjs.org", "198.18.0.4"},
	} {
		started := time.Now()
		status, err := smartFakeIPFetch(probe.addr, probe.name)
		elapsed := time.Since(started).Round(time.Millisecond)
		if err != nil {
			say("  соединение на %s (%s) %8s  ОШИБКА: %v", probe.addr, probe.name, elapsed, err)
			t.Errorf("соединение на фейковый адрес %s не дошло: %v", probe.addr, err)
			continue
		}
		say("  соединение на %s (%s) %8s  %s", probe.addr, probe.name, elapsed, status)
	}
}

// smartFakeIPFetch asks the engine's own inbound for a fake address, the way a
// client that just received one would.
func smartFakeIPFetch(addr, host string) (string, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", smartFakeIPLocalPort), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	request := fmt.Sprintf("GET http://%s/ HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", addr, host)
	if _, err = conn.Write([]byte(request)); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// smartFakeIPLists reads the same cached block-lists the running app uses, so
// the rule order under test is the user's, not a fixture's.
func smartFakeIPLists(t *testing.T) ([]string, []string) {
	t.Helper()
	router := NewRouter()
	if raw, err := os.ReadFile(dnsTrUserData + `\blocked_cache.json`); err == nil {
		var cache struct {
			Domains []string `json:"domains"`
		}
		if json.Unmarshal(raw, &cache) == nil && len(cache.Domains) > 0 {
			router.SetBlockedDomains(cache.Domains)
		}
	}
	if raw, err := os.ReadFile(dnsTrUserData + `\blocked_cidr_cache.json`); err == nil {
		var cache struct {
			CIDRs []string `json:"cidrs"`
		}
		if json.Unmarshal(raw, &cache) == nil && len(cache.CIDRs) > 0 {
			router.SetBlockedCIDRs(cache.CIDRs)
		}
	}
	return router.GetBlockedDomains(), router.GetBlockedCIDRs()
}

func answersOf(resp *mDNS.Msg) []mDNS.RR {
	if resp == nil {
		return nil
	}
	return resp.Answer
}
