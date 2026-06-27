package mobile

import (
	"encoding/json"
	"testing"
)

const ksTestURI = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&type=tcp#ks"

func buildKS(t *testing.T, armed, panic bool) map[string]any {
	t.Helper()
	out, err := BuildSingBoxConfigV2(ksTestURI, t.TempDir(), encodeOptions(BuildOptions{
		KillSwitchArmed: armed,
		KillSwitchPanic: panic,
	}))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func routeFinal(m map[string]any) string {
	r, _ := m["route"].(map[string]any)
	f, _ := r["final"].(string)
	return f
}

func hasUrltestGroup(m map[string]any) bool {
	obs, _ := m["outbounds"].([]any)
	for _, o := range obs {
		ob, _ := o.(map[string]any)
		if ob["type"] == "urltest" && ob["tag"] == "ks-test" {
			return true
		}
	}
	return false
}

func TestKillSwitchArmed_AddsGroupAndFinal(t *testing.T) {
	m := buildKS(t, true, false)
	if !hasUrltestGroup(m) {
		t.Fatal("armed: expected ks-test urltest group")
	}
	if got := routeFinal(m); got != "ks-test" {
		t.Fatalf("armed: route.final = %q, want ks-test", got)
	}
}

func TestKillSwitchPanic_FinalBlock(t *testing.T) {
	m := buildKS(t, true, true)
	if !hasUrltestGroup(m) {
		t.Fatal("panic: expected ks-test urltest group to remain for probing")
	}
	if got := routeFinal(m); got != "block" {
		t.Fatalf("panic: route.final = %q, want block", got)
	}
}

func TestKillSwitchOff_NoGroup(t *testing.T) {
	m := buildKS(t, false, false)
	if hasUrltestGroup(m) {
		t.Fatal("off: ks-test group must be absent")
	}
	if got := routeFinal(m); got != "proxy" {
		t.Fatalf("off: route.final = %q, want proxy", got)
	}
}
