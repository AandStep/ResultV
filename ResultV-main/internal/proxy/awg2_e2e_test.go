// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

// TestAWG2HeaderRangeEndToEnd verifies that a real-world AWG 2.0 config
// with H-range syntax (like the user's 123.conf) survives the full pipeline:
// parser → SBEndpoint → JSON marshal → upstream sing-box-extended unmarshal
// into option.Endpoint with *Xbadoption.Range.
func TestAWG2HeaderRangeEndToEnd(t *testing.T) {
	extra := `{
		"private_key": "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
		"public_key": "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
		"pre_shared_key": "k9QzZj39E5bFTJ9m7zlK4BWKTSPjYHX8OVOgpFp6Zuk=",
		"address": ["10.8.1.2/24"],
		"allowed_ips": ["0.0.0.0/0"],
		"persistent_keepalive_interval": 25,
		"amnezia": {
			"jc": 8,
			"jmin": 64,
			"jmax": 900,
			"s1": 88,
			"s2": 155,
			"s3": 32,
			"s4": 16,
			"h1": "122163117-125750861",
			"h2": "708517833-711678982",
			"h3": "1169962535-1174185828",
			"h4": "2133918740-2137138052"
		}
	}`

	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{
			IP:    "brr.meowmeowcat.top",
			Port:  51820,
			Type:  "AMNEZIAWG",
			Extra: json.RawMessage(extra),
		},
		Mode: ProxyModeTunnel,
	})

	if len(cfg.Endpoints) != 1 {
		t.Fatalf("endpoints: %d", len(cfg.Endpoints))
	}
	ep := cfg.Endpoints[0]
	if ep.Amnezia == nil {
		t.Fatal("amnezia missing")
	}
	if ep.Amnezia.H1 != "122163117-125750861" {
		t.Fatalf("H1 = %q, want range string", ep.Amnezia.H1)
	}

	full, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if !strings.Contains(string(full), `"h1":"122163117-125750861"`) {
		t.Fatalf("config JSON does not contain H1 range string: %s", string(full))
	}

	// Round-trip through the upstream sing-box option parser — this is the
	// exact path SingBoxEngine.Start uses (singbox.go:268). If H1's range
	// string fails to deserialize into *Xbadoption.Range, the engine would
	// fail to start with the user's config.
	ctx := include.Context(context.Background())
	var opts option.Options
	if err := singjson.UnmarshalContext(ctx, full, &opts); err != nil {
		t.Fatalf("upstream sing-box-extended rejected H-range config: %v\nJSON: %s", err, string(full))
	}
	if len(opts.Endpoints) != 1 {
		t.Fatalf("upstream endpoints: %d", len(opts.Endpoints))
	}

	// Verify the parsed *Xbadoption.Range carries the actual values, not nil.
	// If H1-H4 stay nil after the upstream parse, the wireguard transport will
	// silently skip writing "h1=..." to IPC and the device will fall back to
	// the WireGuard default headers (1-4) — packets get dropped by the AWG
	// 2.0 server and the handshake hangs forever.
	wgEp, ok := opts.Endpoints[0].Options.(*option.WireGuardEndpointOptions)
	if !ok {
		t.Fatalf("endpoint options type: %T", opts.Endpoints[0].Options)
	}
	if wgEp.Amnezia == nil {
		t.Fatal("upstream Amnezia is nil after parse")
	}
	if wgEp.Amnezia.H1 == nil || wgEp.Amnezia.H1.From != 122163117 || wgEp.Amnezia.H1.To != 125750861 {
		t.Fatalf("H1 parsed wrong: %+v", wgEp.Amnezia.H1)
	}
	if wgEp.Amnezia.H4 == nil || wgEp.Amnezia.H4.From != 2133918740 || wgEp.Amnezia.H4.To != 2137138052 {
		t.Fatalf("H4 parsed wrong: %+v", wgEp.Amnezia.H4)
	}
	if wgEp.Amnezia.S3 != 32 || wgEp.Amnezia.S4 != 16 {
		t.Fatalf("S3/S4 not preserved: s3=%d s4=%d", wgEp.Amnezia.S3, wgEp.Amnezia.S4)
	}
	t.Logf("upstream parse OK: H1=%s S3=%d S4=%d JC=%d",
		wgEp.Amnezia.H1.String(), wgEp.Amnezia.S3, wgEp.Amnezia.S4, wgEp.Amnezia.JC)
}
