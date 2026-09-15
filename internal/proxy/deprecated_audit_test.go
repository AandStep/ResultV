//go:build depaudit

// Audit: build the configs this client actually emits and let the core name
// every deprecated feature in them. Behind a build tag; run manually.
//
//	go test -tags "depaudit,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_naive_outbound,with_purego,with_grpc" \
//	  -run TestDeprecatedAudit ./internal/proxy/ -v

package proxy

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/experimental/deprecated"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/config"
)

const auditUserData = `C:\Users\andbe\AppData\Roaming\ResultV`

type collectingManager struct {
	access sync.Mutex
	notes  []deprecated.Note
}

func (c *collectingManager) ReportDeprecated(feature deprecated.Note) {
	c.access.Lock()
	defer c.access.Unlock()
	for _, seen := range c.notes {
		if seen.Name == feature.Name {
			return
		}
	}
	c.notes = append(c.notes, feature)
}

func TestDeprecatedAudit(t *testing.T) {
	node := auditFindNode(t, "grpc")
	t.Logf("узел для аудита: %s %s:%d", node.Type, node.IP, node.Port)

	cases := []struct {
		name  string
		build func() (SingBoxConfig, error)
	}{
		{"tunnel+smart", func() (SingBoxConfig, error) {
			return BuildTunnelModeConfig(EngineConfig{
				Proxy: node, Mode: ProxyModeTunnel, RoutingMode: ModeSmart,
				DataDir: t.TempDir(), DNSLeakProtection: true,
			})
		}},
		{"tunnel+global", func() (SingBoxConfig, error) {
			return BuildTunnelModeConfig(EngineConfig{
				Proxy: node, Mode: ProxyModeTunnel, RoutingMode: ModeGlobal,
				DataDir: t.TempDir(), DNSLeakProtection: true,
			})
		}},
		{"proxy", func() (SingBoxConfig, error) {
			return BuildProxyModeConfig(EngineConfig{
				Proxy: node, Mode: ProxyModeProxy, LocalPort: 24098, DataDir: t.TempDir(),
			})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := tc.build()
			if err != nil {
				t.Fatalf("конфиг не собрался: %v", err)
			}
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			manager := &collectingManager{}
			ctx := service.ContextWith[deprecated.Manager](context.Background(), deprecated.Manager(manager))
			boxCtx := extendedBoxContext(ctx)

			var options option.Options
			if err := singjson.UnmarshalContext(boxCtx, raw, &options); err != nil {
				t.Fatalf("ЯДРО НЕ ПРИНЯЛО КОНФИГ: %v", err)
			}
			instance, err := box.New(box.Options{Context: boxCtx, Options: options})
			if err != nil {
				t.Errorf("box.New: %v", err)
			} else {
				_ = instance.Close()
			}
			if len(manager.notes) == 0 {
				t.Logf("  устаревшего не найдено")
				return
			}
			for _, note := range manager.notes {
				impending := ""
				if note.Impending() {
					impending = "  [БЛИЗКО К УДАЛЕНИЮ]"
				}
				t.Errorf("  УСТАРЕЛО: %s — %s (удаляется в %s)%s",
					note.Name, strings.TrimSpace(note.Message()), note.DeprecatedVersion, impending)
			}
		})
	}
}

func auditFindNode(t *testing.T, network string) ProxyConfig {
	t.Helper()
	crypto, err := config.NewCryptoService(auditUserData)
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	manager := config.NewManager(crypto)
	if err := manager.Init(auditUserData); err != nil {
		t.Fatalf("конфиг не прочитан: %v", err)
	}
	for _, entry := range manager.GetConfig().Proxies {
		var extra map[string]interface{}
		if len(entry.Extra) > 0 {
			_ = json.Unmarshal(entry.Extra, &extra)
		}
		if strings.ToLower(getStringField(extra, "network", "")) != network {
			continue
		}
		node := ProxyConfig{
			ID: entry.ID, IP: entry.IP, Port: entry.Port, Type: entry.Type,
			Username: entry.Username, Password: entry.Password,
			URI: entry.URI, Extra: entry.Extra, SubscriptionURL: entry.SubscriptionURL,
		}
		node.ResolvedIP = resolvePinnedServerIP(node.IP)
		return node
	}
	t.Skipf("в конфиге нет узла с транспортом %s", network)
	return ProxyConfig{}
}
