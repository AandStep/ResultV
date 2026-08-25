package proxy

import (
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
)

// vlessOutbound renders one xray vless outbound with the given tag/host/port.
func vlessOutbound(tag, host string, port int) string {
	return `{"tag":"` + tag + `","protocol":"vless","settings":{"vnext":[{"address":"` +
		host + `","port":` + itoaTest(port) +
		`,"users":[{"id":"af815621-b245-4149-89da-dd184cfc4b3d","encryption":"none"}]}]},` +
		`"streamSettings":{"network":"tcp","security":"none"}}`
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// balancerConfig renders one xray config that declares a balancer.
func balancerConfig(remarks string, selector []string, fallbackTag string, outbounds ...string) string {
	sel := `"` + strings.Join(selector, `","`) + `"`
	return `{"remarks":"` + remarks + `","routing":{"balancers":[{"tag":"bal-auto","selector":[` + sel +
		`],"fallbackTag":"` + fallbackTag + `"}]},"outbounds":[` + strings.Join(outbounds, ",") + `]}`
}

// plainConfig renders one xray config with a single proxy outbound (no balancer).
func plainConfig(remarks string, outbounds ...string) string {
	return `{"remarks":"` + remarks + `","routing":{"rules":[]},"outbounds":[` +
		strings.Join(outbounds, ",") + `]}`
}

const svcOutbounds = `{"tag":"direct","protocol":"freedom"},` +
	`{"tag":"block","protocol":"blackhole"},` +
	`{"tag":"dns-out","protocol":"dns"}`

// A balancer config must stamp every pooled outbound with AutoGroup = remarks,
// and must not leak the nodes it did not select.
func TestParseSubscriptionBodyStampsAutoGroupFromBalancer(t *testing.T) {
	body := "[" + strings.Join([]string{
		balancerConfig("🚀 impVPN Auto", []string{"basic-proxy", "premium-proxy"}, "basic-proxy",
			vlessOutbound("basic-proxy", "n1.example", 443),
			vlessOutbound("basic-proxy-2", "n2.example", 443),
			vlessOutbound("premium-proxy", "p1.example", 443),
			vlessOutbound("premium-limit-proxy", "pl1.example", 443),
			svcOutbounds),
		balancerConfig("⚡ Авто | ✅ Когда не глушат интернет", []string{"basic-proxy"}, "basic-proxy",
			vlessOutbound("basic-proxy", "n1.example", 443),
			vlessOutbound("basic-proxy-2", "n2.example", 443),
			vlessOutbound("premium-proxy", "p1.example", 443),
			svcOutbounds),
		plainConfig("🇺🇸 США | VLESS TCP | №1", vlessOutbound("proxy", "n1.example", 443), svcOutbounds),
	}, ",") + "]"

	entries, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}

	var groupA, groupB, plain []string
	for _, e := range entries {
		switch e.AutoGroup {
		case "🚀 impVPN Auto":
			groupA = append(groupA, e.IP)
		case "⚡ Авто | ✅ Когда не глушат интернет":
			groupB = append(groupB, e.IP)
		case "":
			plain = append(plain, e.IP)
		default:
			t.Fatalf("unexpected AutoGroup %q", e.AutoGroup)
		}
	}

	// "premium-proxy" must NOT swallow "premium-limit-proxy" — xray matches
	// selectors by prefix, and those are two different tiers.
	wantA := []string{"n1.example", "n2.example", "p1.example"}
	if strings.Join(groupA, ",") != strings.Join(wantA, ",") {
		t.Errorf("group A = %v, want %v", groupA, wantA)
	}
	wantB := []string{"n1.example", "n2.example"}
	if strings.Join(groupB, ",") != strings.Join(wantB, ",") {
		t.Errorf("group B = %v, want %v", groupB, wantB)
	}
	if strings.Join(plain, ",") != "n1.example" {
		t.Errorf("plain = %v, want [n1.example]", plain)
	}
	// direct/block/dns-out must never become members.
	for _, e := range entries {
		if e.IP == "" {
			t.Errorf("service outbound leaked as entry: %+v", e)
		}
	}
}

// fallbackTag joins the pool even when it is absent from selector — the
// balancer routes to it when every probe fails.
func TestParseSubscriptionBodyBalancerFallbackTagJoinsPool(t *testing.T) {
	body := "[" + balancerConfig("Auto", []string{"basic-proxy"}, "rescue-proxy",
		vlessOutbound("basic-proxy", "n1.example", 443),
		vlessOutbound("rescue-proxy", "r1.example", 443),
		svcOutbounds) + "]"

	entries, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if e.AutoGroup != "Auto" {
			t.Errorf("entry %s: AutoGroup=%q, want %q", e.IP, e.AutoGroup, "Auto")
		}
	}
}

// A config without routing.balancers keeps today's behaviour: no AutoGroup
// stamp, every proxy outbound becomes a plain entry named after remarks.
func TestParseSubscriptionBodyPlainConfigHasNoAutoGroup(t *testing.T) {
	body := "[" + plainConfig("🇬🇪 Грузия | VLESS TCP | №4",
		vlessOutbound("proxy", "g1.example", 1443), svcOutbounds) + "]"

	entries, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].AutoGroup != "" {
		t.Errorf("AutoGroup=%q, want empty", entries[0].AutoGroup)
	}
	if entries[0].Name != "🇬🇪 Грузия | VLESS TCP | №4" {
		t.Errorf("Name=%q", entries[0].Name)
	}
}

// Two provider-declared sections must stay two groups, in first-appearance
// order, with the section names the provider gave them.
func TestSplitAutoEntriesStructuralTwoGroups(t *testing.T) {
	entries := []config.ProxyEntry{
		{ID: "1", Name: "🚀 impVPN Auto", Type: "VLESS", IP: "n1", Port: 443, AutoGroup: "🚀 impVPN Auto"},
		{ID: "2", Name: "🚀 impVPN Auto", Type: "VLESS", IP: "n2", Port: 443, AutoGroup: "🚀 impVPN Auto"},
		{ID: "3", Name: "⚡ Авто | ✅ Когда не глушат", Type: "VLESS", IP: "n1", Port: 443, AutoGroup: "⚡ Авто | ✅ Когда не глушат"},
		{ID: "4", Name: "⚡ Авто | ✅ Когда не глушат", Type: "VLESS", IP: "n2", Port: 443, AutoGroup: "⚡ Авто | ✅ Когда не глушат"},
		{ID: "5", Name: "🇺🇸 США | VLESS TCP | №1", Type: "VLESS", IP: "n1", Port: 443},
	}

	groups, individual, ok := SplitAutoEntries(entries)
	if !ok {
		t.Fatal("ok=false, want true")
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2: %+v", len(groups), groups)
	}
	if groups[0].Name != "🚀 impVPN Auto" || len(groups[0].Members) != 2 {
		t.Errorf("group 0 = %q/%d", groups[0].Name, len(groups[0].Members))
	}
	if groups[1].Name != "⚡ Авто | ✅ Когда не глушат" || len(groups[1].Members) != 2 {
		t.Errorf("group 1 = %q/%d", groups[1].Name, len(groups[1].Members))
	}
	if len(individual) != 1 || individual[0].ID != "5" {
		t.Errorf("individual = %+v, want [id 5]", individual)
	}
}

// A structural group of one is still a group: the provider declared a
// balancer, so the node must not masquerade as an ordinary server card.
func TestSplitAutoEntriesStructuralSingleMemberGroup(t *testing.T) {
	entries := []config.ProxyEntry{
		{ID: "1", Name: "Авто", Type: "VLESS", IP: "n1", Port: 443, AutoGroup: "Авто"},
		{ID: "2", Name: "🇺🇸 США | VLESS TCP | №1", Type: "VLESS", IP: "n2", Port: 443},
	}
	groups, individual, ok := SplitAutoEntries(entries)
	if !ok || len(groups) != 1 || len(groups[0].Members) != 1 {
		t.Fatalf("groups=%+v ok=%v", groups, ok)
	}
	if len(individual) != 1 || individual[0].ID != "2" {
		t.Errorf("individual = %+v", individual)
	}
}

// No AutoGroup anywhere → the legacy name heuristic, still capped at one
// group. This is the shape a line-based vless:// subscription arrives in.
func TestSplitAutoEntriesFallbackByName(t *testing.T) {
	entries := []config.ProxyEntry{
		{ID: "1", Name: "🇨🇦 impVPN Auto | VLESS + Reality", Type: "VLESS", IP: "n1", Port: 443},
		{ID: "2", Name: "🇩🇪 impVPN Auto | HYSTERIA2", Type: "HYSTERIA2", IP: "n2", Port: 443},
		{ID: "3", Name: "🇺🇸 США | VLESS TCP | №1", Type: "VLESS", IP: "n3", Port: 443},
	}
	groups, individual, ok := SplitAutoEntries(entries)
	if !ok {
		t.Fatal("ok=false, want true")
	}
	if len(groups) != 1 || groups[0].Name != "impVPN Auto" || len(groups[0].Members) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	if len(individual) != 1 || individual[0].ID != "3" {
		t.Errorf("individual = %+v", individual)
	}
}

// Nothing auto-ish at all → no groups, everything individual (unchanged).
func TestSplitAutoEntriesNoGroups(t *testing.T) {
	entries := []config.ProxyEntry{
		{ID: "1", Name: "US Fast Server", Type: "VLESS", IP: "n1", Port: 443},
		{ID: "2", Name: "EU Slow Server", Type: "VLESS", IP: "n2", Port: 443},
	}
	groups, individual, ok := SplitAutoEntries(entries)
	if ok || len(groups) != 0 {
		t.Fatalf("ok=%v groups=%+v, want false/none", ok, groups)
	}
	if len(individual) != 2 {
		t.Errorf("individual = %d, want 2", len(individual))
	}
}
