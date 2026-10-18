package mobile

import (
	"context"
	"strings"
	"testing"

	"resultproxy-wails/internal/proxy"
)

const realityTestEntry = `{"name":"r","type":"VLESS","ip":"203.0.113.7","port":443,"extra":{"uuid":"af815621-b245-4149-89da-dd184cfc4b3d","security":"reality","pbk":"-lCQjfUSO29QFSrFrbIyHPfaSQzjIWRg7lMhaPStUzA","sid":"f2b30c6723854005","sni":"www.apple.com","network":"tcp"}}`

func stubLearnRealityMLKEM(t *testing.T, need bool) *int {
	t.Helper()
	old := learnRealityMLKEM
	t.Cleanup(func() { learnRealityMLKEM = old })
	calls := 0
	learnRealityMLKEM = func(_ context.Context, p proxy.ProxyConfig) proxy.ProxyConfig {
		calls++
		p.RealityMLKEM = need
		return p
	}
	return &calls
}

// The config build is where every Android connect passes, so the learned
// choice has to reach the outbound from there.
func TestBuildConfigCarriesLearnedRealityMLKEM(t *testing.T) {
	for _, need := range []bool{true, false} {
		calls := stubLearnRealityMLKEM(t, need)
		cfg, err := BuildSingBoxConfigFromEntryV2(realityTestEntry, t.TempDir(), encodeOptions(BuildOptions{}))
		if err != nil {
			t.Fatal(err)
		}
		if *calls != 1 {
			t.Fatalf("learn calls = %d, want 1", *calls)
		}
		if got := strings.Contains(cfg, `"support_x25519mlkem768": true`); got != need {
			t.Fatalf("need=%v: support_x25519mlkem768 in config = %v", need, got)
		}
	}
}

func TestBuildConfigSkipsRealityProbeInKillSwitchPanic(t *testing.T) {
	calls := stubLearnRealityMLKEM(t, true)
	if _, err := BuildSingBoxConfigFromEntryV2(realityTestEntry, t.TempDir(), encodeOptions(BuildOptions{
		KillSwitchArmed: true,
		KillSwitchPanic: true,
	})); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 {
		t.Fatalf("panic config must not wait on a probe: learn calls = %d", *calls)
	}
}
