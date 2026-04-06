// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestVLESSURIEmbeddedExtraAndOutboundXHTTP(t *testing.T) {
	q := url.Values{}
	q.Set("type", "xhttp")
	q.Set("path", "/xhttp/h3")
	q.Set("host", "cdn.example")
	q.Set("security", "tls")
	q.Set("sni", "example.com")
	q.Set("mode", "stream-one")
	q.Set("method", "POST")
	q.Set("extra", `{"xPaddingBytes":"200-800","headers":{"Referer":"https://ref.example/"}}`)

	line := "vless://af815621-b245-4149-89da-dd184cfc4b3d@example.com:443?" + q.Encode()

	entry, err := ParseProxyURI(line)
	if err != nil {
		t.Fatal(err)
	}

	var ex map[string]interface{}
	if err := json.Unmarshal(entry.Extra, &ex); err != nil {
		t.Fatal(err)
	}
	if got := stringFromExtraValue(ex["x_padding_bytes"]); got != "200-800" {
		t.Fatalf("x_padding_bytes: got %q", got)
	}
	h, ok := ex["headers"].(map[string]interface{})
	if !ok || stringFromExtraValue(h["Referer"]) != "https://ref.example/" {
		t.Fatalf("headers: %+v", ex["headers"])
	}
	if stringFromExtraValue(ex["method"]) != "POST" {
		t.Fatalf("method: %v", ex["method"])
	}

	out := buildProxyOutbound(ProxyConfig{
		IP:    entry.IP,
		Port:  entry.Port,
		Type:  entry.Type,
		Extra: entry.Extra,
	})
	tr := out.Transport
	if tr == nil || tr.Type != "xhttp" {
		t.Fatalf("transport: %+v", tr)
	}
	if tr.Path != "/xhttp/h3" || tr.Host != "cdn.example" || tr.Mode != "stream-one" {
		t.Fatalf("transport fields: %+v", tr)
	}
	if tr.UplinkHTTPMethod != "POST" {
		t.Fatalf("UplinkHTTPMethod: %q", tr.UplinkHTTPMethod)
	}
	if tr.XPaddingBytes != "200-800" {
		t.Fatalf("XPaddingBytes: %q", tr.XPaddingBytes)
	}
	if tr.Headers == nil || tr.Headers["Referer"] != "https://ref.example/" {
		t.Fatalf("Headers: %+v", tr.Headers)
	}
}

func TestXHTTPOmitPaddingWhenAbsent(t *testing.T) {
	q := url.Values{}
	q.Set("type", "xhttp")
	q.Set("path", "/p")
	q.Set("security", "none")
	line := "vless://af815621-b245-4149-89da-dd184cfc4b3d@example.com:80?" + q.Encode()
	entry, err := ParseProxyURI(line)
	if err != nil {
		t.Fatal(err)
	}
	out := buildProxyOutbound(ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra})
	// sing-box-extended requires positive x_padding_bytes; we default when link omits it.
	if out.Transport == nil || out.Transport.XPaddingBytes != "100-1000" {
		t.Fatalf("expected default XPaddingBytes, got %q", out.Transport.XPaddingBytes)
	}
}

func TestXHTTPPassthroughXmuxScNoGRPC(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "xhttp",
		"security": "tls",
		"path":     "/x",
		"host":     "cdn",
		"sni":      "cdn",
		"xmux": map[string]interface{}{
			"maxConcurrency":   "10-20",
			"hKeepAlivePeriod": float64(30),
		},
		"scMaxEachPostBytes": "100-200",
		"noGRPCHeader":      true,
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	out := buildProxyOutbound(ProxyConfig{IP: "example.com", Port: 443, Type: "VLESS", Extra: raw})
	tr := out.Transport
	if tr == nil || tr.Type != "xhttp" {
		t.Fatalf("transport: %+v", tr)
	}
	if tr.Xmux == nil {
		t.Fatal("expected xmux JSON")
	}
	var xmux map[string]interface{}
	if err := json.Unmarshal(tr.Xmux, &xmux); err != nil {
		t.Fatal(err)
	}
	if xmux["max_concurrency"] != "10-20" || xmux["h_keep_alive_period"] != float64(30) {
		t.Fatalf("xmux: %+v", xmux)
	}
	if tr.NoGRPCHeader == nil || !*tr.NoGRPCHeader {
		t.Fatal("no_grpc_header")
	}
	if tr.ScMaxEachPostBytes == nil {
		t.Fatal("sc_max_each_post_bytes")
	}
}
