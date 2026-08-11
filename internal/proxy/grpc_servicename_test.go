package proxy

import (
	"net/url"
	"strings"
	"testing"
)

// xrayWirePath reimplements Xray's own path construction
// (transport/internet/grpc/config.go getServiceName/getTunStreamName) so the
// expectations below come from the protocol definition rather than from our
// implementation. The server the client talks to is Xray, so this is the
// authority on what must appear in the HTTP/2 :path header.
func xrayWirePath(serviceName string) string {
	if !strings.HasPrefix(serviceName, "/") {
		return "/" + url.PathEscape(serviceName) + "/Tun"
	}
	lastIndex := strings.LastIndex(serviceName, "/")
	if lastIndex < 1 {
		lastIndex = 1
	}
	parts := strings.Split(serviceName[1:lastIndex], "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	ending := serviceName[strings.LastIndex(serviceName, "/")+1:]
	stream := url.PathEscape(strings.Split(ending, "|")[0])
	return "/" + strings.Join(parts, "/") + "/" + stream
}

func TestXrayGRPCServiceName_MatchesXrayWirePath(t *testing.T) {
	cases := []string{
		"plainname",                  // old-school, no inner slash
		"apple-com/2265cbcc",         // old-school with inner slash (this provider)
		"www-debian-org/4c4394da",    // old-school with inner slash
		"dl-google-com/b7414bee",     // old-school with inner slash
		"/47559/I53GwKHO",            // already a custom path — must pass through
		"/a/b/c",                     // custom path, multiple segments
		"name with space",            // needs escaping
	}
	for _, sn := range cases {
		got := xrayGRPCServiceName(sn)
		// sing-box passes a leading-slash service_name to gRPC verbatim as the
		// method, so the emitted value IS the wire path.
		if want := xrayWirePath(sn); got != want {
			t.Errorf("xrayGRPCServiceName(%q) = %q, want %q (Xray wire path)", sn, got, want)
		}
	}
}

func TestXrayGRPCServiceName_LeavesCustomPathsAlone(t *testing.T) {
	for _, sn := range []string{"/47559/I53GwKHO", "/a/b/c"} {
		if got := xrayGRPCServiceName(sn); got != sn {
			t.Errorf("a serviceName that already is a custom path must pass through: %q -> %q", sn, got)
		}
	}
}

func TestXrayGRPCServiceName_EmptyStaysEmpty(t *testing.T) {
	if got := xrayGRPCServiceName(""); got != "" {
		t.Errorf("empty serviceName must stay empty so the core keeps its own default, got %q", got)
	}
}

func TestXrayGRPCServiceName_AppendsTunOnlyWhereXrayDoes(t *testing.T) {
	// The nodes that must NOT get "/Tun" are exactly the ones carrying a leading
	// slash: there the last segment already IS the stream name.
	if got := xrayGRPCServiceName("/47559/I53GwKHO"); strings.HasSuffix(got, "/Tun") {
		t.Errorf("must not append /Tun to a custom path, got %q", got)
	}
	if got := xrayGRPCServiceName("plainname"); got != "/plainname/Tun" {
		t.Errorf("old-school name must gain Xray's default stream name, got %q", got)
	}
}

// TestGRPCOutbound_ServiceNameMatchesBuild pins the coupling between the build
// tag and what we emit: with the full gRPC transport we hand sing-box the
// explicit path, with the lite one we must leave service_name alone.
func TestGRPCOutbound_ServiceNameMatchesBuild(t *testing.T) {
	extra := `{"network":"grpc","serviceName":"apple-com/2265cbcc","security":"reality",` +
		`"sni":"apple.com","pbk":"key","uuid":"f57e06ec-6be1-435d-b4f1-cbbac94dd027"}`
	out := buildProxyOutboundRaw(ProxyConfig{
		IP: "1.2.3.4", Port: 443, Type: "VLESS", Extra: []byte(extra),
	})
	if out.Transport == nil || out.Transport.Type != "grpc" {
		t.Fatalf("expected a grpc transport, got %+v", out.Transport)
	}
	want := "apple-com/2265cbcc"
	if grpcFullTransport {
		want = "/apple-com%2F2265cbcc/Tun"
	}
	if out.Transport.ServiceName != want {
		t.Fatalf("service_name = %q, want %q (grpcFullTransport=%v)",
			out.Transport.ServiceName, want, grpcFullTransport)
	}
}

// wantGRPCServiceName is the service_name the config must carry for a plain
// Xray serviceName in the current build. Anchored to xrayWirePath (Xray's own
// algorithm) rather than to our implementation, so the assertion stays honest.
func wantGRPCServiceName(serviceName string) string {
	if grpcFullTransport {
		return xrayWirePath(serviceName)
	}
	return serviceName
}
