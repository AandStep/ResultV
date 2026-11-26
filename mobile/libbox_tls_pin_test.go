package mobile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestMobileConfigEnforcesImportedCertificatePin(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	peer := server.Listener.Addr().(*net.TCPAddr)
	leafPin := sha256.Sum256(server.Certificate().Raw)
	keyPin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	dataDir := t.TempDir()
	entryForPin := func(pin string) config.ProxyEntry {
		extra, err := json.Marshal(map[string]any{
			"uuid":    "af815621-b245-4149-89da-dd184cfc4b3d",
			"network": "grpc", "security": "tls", "insecure": true,
			"pinnedPeerCertSha256": pin,
		})
		if err != nil {
			t.Fatal(err)
		}
		return config.ProxyEntry{Type: "VLESS", IP: peer.IP.String(), Port: peer.Port, Extra: extra}
	}
	assertPinned := func(entry config.ProxyEntry, opts BuildOptions) {
		t.Helper()
		entryJSON, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := BuildSingBoxConfigFromEntryV2(string(entryJSON), dataDir, encodeOptions(opts))
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Outbounds []struct {
				Tag string
				TLS struct {
					Insecure bool
					Keys     [][]byte `json:"certificate_public_key_sha256"`
				}
			}
		}
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatal(err)
		}
		for _, out := range cfg.Outbounds {
			if out.Tag == "proxy" {
				if out.TLS.Insecure || len(out.TLS.Keys) != 1 || !bytes.Equal(out.TLS.Keys[0], keyPin[:]) {
					t.Fatal("proxy TLS must enforce the verified public key")
				}
				return
			}
		}
		t.Fatal("missing proxy outbound")
	}

	SetTunnelActive(true)
	t.Cleanup(func() { SetTunnelActive(false) })
	entry := entryForPin(hex.EncodeToString(leafPin[:]))
	assertPinned(entry, BuildOptions{})
	if _, err := buildSingBoxConfigFromEntry(entryForPin(strings.Repeat("ab", sha256.Size)), dataDir, BuildOptions{}); err == nil {
		t.Fatal("a different leaf fingerprint must be rejected")
	}
	server.Close()
	assertPinned(entry, BuildOptions{KillSwitchArmed: true, KillSwitchPanic: true})
	SetTunnelActive(false)
	SetTunnelActive(true)
	if _, err := buildSingBoxConfigFromEntry(entry, dataDir, BuildOptions{}); err == nil {
		t.Fatal("a new session must verify the server again")
	}
}
