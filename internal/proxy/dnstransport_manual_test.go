//go:build dnstransport

// Drives the core's OWN DNS transport (the "custom-N" tcp server with
// detour=proxy that tunnel mode builds) through the node, one query a second,
// and prints where it stops answering.
//
// sing-box 1.14 put a queryMultiplexer in front of the TCP DNS transport. Until
// a background probe proves the resolver supports reuse it dials a fresh
// connection per query (1.13 behaviour); once the probe succeeds every query
// moves onto ONE shared connection whose liveness check is `conn != nil`. This
// runs long enough to cross that switch.
//
// Output goes to stderr, not t.Logf: if a lookup wedges past the test timeout,
// buffered test logs are never printed and the run tells us nothing.
//
//	go test -tags "dnstransport,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_naive_outbound,with_purego,with_grpc" \
//	  -run TestDNSTransportThroughNode ./internal/proxy/ -v -timeout 5m

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/config"
)

const (
	dnsTrUserData  = `C:\Users\andbe\AppData\Roaming\ResultV`
	dnsTrLocalPort = 24094
)

func say(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func TestDNSTransportThroughNode(t *testing.T) {
	node := dnsTrFindNode(t)
	say("узел: %s %s:%d", node.Type, node.IP, node.Port)

	cfg, err := BuildProxyModeConfig(EngineConfig{
		Proxy: node, Mode: ProxyModeProxy, LocalPort: dnsTrLocalPort, DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("конфиг: %v", err)
	}
	// Exactly what tunnel mode emits for a user with a custom resolver set:
	// DNS-over-TCP to 8.8.8.8, routed through the node.
	// RESULTV_TEST_DNS=tcp reproduces the pre-fix shape (a bare multiplexed
	// tcp server); anything else builds what the app ships now.
	servers := tunnelDNSResolver("custom-1", "8.8.8.8", 0, "proxy")
	if os.Getenv("RESULTV_TEST_DNS") == "tcp" {
		servers = []SBDNSServer{{Type: "tcp", Tag: "custom-1", Server: "8.8.8.8", Detour: "proxy"}}
	}
	servers = append(servers, SBDNSServer{Type: "local", Tag: "local"})
	cfg.DNS = &SBDNS{Servers: servers, Final: "custom-1"}
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

	domains := []string{"i.ytimg.com", "www.youtube.com", "googlevideo.com", "ggpht.com", "yt3.ggpht.com", "youtube.com"}
	// Bursts, not a metronome. A browser opening YouTube asks for dozens of
	// names at once, so the multiplexer carries many in-flight queries on one
	// connection -- the state a one-per-second probe never reaches.
	burst := 24
	if env := os.Getenv("RESULTV_TEST_BURST"); env != "" {
		fmt.Sscanf(env, "%d", &burst)
	}
	var okCount, failCount, firstFail int
	var countAccess sync.Mutex
	for round := 1; round <= 8; round++ {
		var wg sync.WaitGroup
		for i := 0; i < burst; i++ {
			wg.Add(1)
			go func(round, i int) {
				defer wg.Done()
				// A unique label defeats the core's own DNS cache, so every query
				// is a real trip to the resolver rather than a cache read.
				query := fmt.Sprintf("r%dn%d.%s", round, i, domains[i%len(domains)])
				lookupCtx, lookupCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer lookupCancel()
				started := time.Now()
				_, lookupErr := dnsRouter.Lookup(lookupCtx, query, adapter.DNSQueryOptions{})
				elapsed := time.Since(started).Round(time.Millisecond)
				// NXDOMAIN is a fine answer here: it proves the round trip completed.
				answered := lookupErr == nil ||
					strings.Contains(lookupErr.Error(), "empty result") ||
					strings.Contains(lookupErr.Error(), "NXDOMAIN") ||
					strings.Contains(lookupErr.Error(), "name error")
				countAccess.Lock()
				defer countAccess.Unlock()
				if answered {
					okCount++
					return
				}
				failCount++
				if firstFail == 0 {
					firstFail = round
					say("  первый отказ: круг %d, %s за %s: %v", round, query, elapsed, lookupErr)
				}
			}(round, i)
		}
		wg.Wait()
		countAccess.Lock()
		say("  круг %-2d (%d параллельно): ответов %d, тишины %d", round, burst, okCount, failCount)
		countAccess.Unlock()
		time.Sleep(2 * time.Second)
	}
	say("ИТОГ: ответов %d, тишины %d, первая тишина на круге %d", okCount, failCount, firstFail)
	if failCount > 0 {
		t.Errorf("DNS через узел перестал отвечать: %d из %d без ответа", failCount, okCount+failCount)
	}
}

func dnsTrFindNode(t *testing.T) ProxyConfig {
	t.Helper()
	crypto, err := config.NewCryptoService(dnsTrUserData)
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	manager := config.NewManager(crypto)
	if err := manager.Init(dnsTrUserData); err != nil {
		t.Fatalf("конфиг: %v", err)
	}
	for _, entry := range manager.GetConfig().Proxies {
		var extra map[string]interface{}
		if len(entry.Extra) > 0 {
			_ = json.Unmarshal(entry.Extra, &extra)
		}
		// RESULTV_TEST_NODE_HOST picks the node; default is the gRPC one under
		// investigation. Pointing it at a non-gRPC node answers whether the fault
		// is the transport or the DNS layer above it.
		want := os.Getenv("RESULTV_TEST_NODE_HOST")
		if want == "" {
			want = "dev3.failusha.digital"
		}
		if !strings.EqualFold(entry.IP, want) {
			continue
		}
		_ = extra
		node := ProxyConfig{
			ID: entry.ID, IP: entry.IP, Port: entry.Port, Type: entry.Type,
			Username: entry.Username, Password: entry.Password,
			URI: entry.URI, Extra: entry.Extra, SubscriptionURL: entry.SubscriptionURL,
		}
		node.ResolvedIP = resolvePinnedServerIP(node.IP)
		// Address the node by its literal IP. The resolver under test is the
		// node's own DNS path, so leaving the server as a domain would make
		// resolving it depend on itself -- a deadlock the real config avoids with
		// a predefined hosts entry (server-pin).
		if node.ResolvedIP != "" {
			node.IP = node.ResolvedIP
		}
		return node
	}
	t.Skip("gRPC-узел dev3 не найден")
	return ProxyConfig{}
}
