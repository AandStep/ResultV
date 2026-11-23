package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestXrayJSONUsesExplicitXHTTPUplinkMethod(t *testing.T) {
	const template = `[{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"example.test","port":443,"users":[{"id":"af815621-b245-4149-89da-dd184cfc4b3d"}]}]},"streamSettings":{"network":"xhttp","security":"tls","xhttpSettings":XHTTP_SETTINGS}}]}]`
	for _, tc := range []struct{ name, settings, want string }{
		{"extra", `{"mode":"packet-up","method":"POST","extra":{"uplinkHTTPMethod":"GET","uplinkDataPlacement":"header"}}`, "GET"},
		{"top-level", `{"mode":"packet-up","uplinkHTTPMethod":"GET"}`, "GET"},
		{"extra replaces top-level", `{"mode":"packet-up","uplinkHTTPMethod":"GET","extra":{"uplinkDataPlacement":"header"}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(template, "XHTTP_SETTINGS", tc.settings, 1)
			entries, err := ParseSubscriptionBody(body)
			if err != nil || len(entries) != 1 {
				t.Fatalf("expected one imported profile, got %d: %v", len(entries), err)
			}
			entry := entries[0]
			outbound := buildProxyOutbound(ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra})
			if outbound.Transport == nil || outbound.Transport.UplinkHTTPMethod != tc.want {
				t.Fatalf("expected uplink method %q, got %+v", tc.want, outbound.Transport)
			}
		})
	}
}

func TestXrayJSONKeepsTLSCertificatePin(t *testing.T) {
	const body = `[{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"example.test","port":443,"users":[{"id":"af815621-b245-4149-89da-dd184cfc4b3d"}]}]},"streamSettings":{"network":"grpc","security":"tls","tlsSettings":{"serverName":"example.test","pinnedPeerCertSha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}]}]`
	entries, err := ParseSubscriptionBody(body)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one imported profile, got %d: %v", len(entries), err)
	}
	var extra map[string]any
	if err := json.Unmarshal(entries[0].Extra, &extra); err != nil {
		t.Fatal(err)
	}
	if extra["pinnedPeerCertSha256"] != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatal("import lost the TLS certificate pin")
	}
}

func TestXrayCertificatePinVerifiesLeafBeforeSettingCorePin(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	leaf := server.Certificate()
	certPin := sha256.Sum256(leaf.Raw)
	keyPin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	wrongPin := strings.Repeat("ab", sha256.Size)

	for _, tc := range []struct {
		name   string
		pin    string
		reject string
	}{
		{"matching leaf", hex.EncodeToString(certPin[:]), ""},
		{"matching leaf in a list", wrongPin + "," + hex.EncodeToString(certPin[:]), ""},
		{"wrong certificate", wrongPin, "does not match"},
		{"public key passed as certificate pin", hex.EncodeToString(keyPin[:]), "does not match"},
		{"malformed fingerprint", "1234", "invalid TLS certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extra, err := json.Marshal(map[string]string{
				"uuid":                 "af815621-b245-4149-89da-dd184cfc4b3d",
				"network":              "grpc",
				"security":             "tls",
				"pinnedPeerCertSha256": tc.pin,
			})
			if err != nil {
				t.Fatal(err)
			}
			proxy := ProxyConfig{IP: host, Port: port, Type: "VLESS", Extra: extra}
			cfg, err := BuildPingProbeConfig(proxy, 1080, "")
			if err != nil {
				t.Fatal(err)
			}
			_, err = prepareXrayCertificatePin(t.Context(), proxy, &cfg, nil)
			for _, out := range cfg.Outbounds {
				if out.Tag != "proxy" {
					continue
				}
				keys := out.TLS.CertificatePublicKeySHA256
				if tc.reject != "" {
					if err == nil || !strings.Contains(err.Error(), tc.reject) || len(keys) != 0 {
						t.Fatalf("expected rejection without setting a key, got %v (%d keys)", err, len(keys))
					}
					return
				}
				if err != nil || len(keys) != 1 || !bytes.Equal(keys[0], keyPin[:]) || out.TLS.Insecure {
					t.Fatalf("expected a verified key pin, got %v (%d keys, insecure=%t)", err, len(keys), out.TLS.Insecure)
				}
				assertCoreAcceptsConfig(t, cfg)
				return
			}
			t.Fatal("missing proxy outbound")
		})
	}
}
