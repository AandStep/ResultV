// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package proxy

import (
	"encoding/json"
	"testing"
)

// TestXHTTPObfs_FullProfileReachesTheCore: a provider's obfs profile sets session
// and seq placement too — xPaddingObfs alone does not make the client match the
// server.
func TestXHTTPObfs_FullProfileReachesTheCore(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"mode":                 "packet-up",
		"sessionPlacement":     "cookie",
		"sessionKey":           "sk",
		"seqPlacement":         "header",
		"seqKey":               "X-Q",
		"uplinkDataPlacement":  "cookie",
		"uplinkDataKey":        "uk",
		"sessionIdTable":       "Base62",
		"sessionIdLength":      "8-16",
		"uplinkChunkSize":      "1000-2000",
		"congestionController": "bbr",
		"cwnd":                 64,
		"scMaxBufferedPosts":   30,
	})
	tr := outboundFromExtra(t, "VLESS", extra).Transport
	if tr == nil {
		t.Fatal("no transport")
	}
	if tr.SessionPlacement != "cookie" || tr.SessionKey != "sk" {
		t.Fatalf("session = %q/%q", tr.SessionPlacement, tr.SessionKey)
	}
	if tr.SeqPlacement != "header" || tr.SeqKey != "X-Q" {
		t.Fatalf("seq = %q/%q", tr.SeqPlacement, tr.SeqKey)
	}
	if tr.UplinkDataPlacement != "cookie" || tr.UplinkDataKey != "uk" {
		t.Fatalf("uplink data = %q/%q", tr.UplinkDataPlacement, tr.UplinkDataKey)
	}
	if tr.SessionIDTable != "Base62" || string(tr.SessionIDLength) != `"8-16"` {
		t.Fatalf("session id = %q/%s", tr.SessionIDTable, tr.SessionIDLength)
	}
	if string(tr.UplinkChunkSize) != `"1000-2000"` {
		t.Fatalf("uplink_chunk_size = %s", tr.UplinkChunkSize)
	}
	if tr.CongestionController != "bbr" || tr.CWND != 64 || tr.ScMaxBufferedPosts != 30 {
		t.Fatalf("congestion = %q/%d, buffered = %d", tr.CongestionController, tr.CWND, tr.ScMaxBufferedPosts)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXHTTPObfs_UplinkDataPlacementIsModeGated: the core allows cookie/header
// only in packet-up mode and errors out otherwise — which kills the engine start.
func TestXHTTPObfs_UplinkDataPlacementIsModeGated(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"mode":                "auto",
		"uplinkDataPlacement": "cookie",
	})
	tr := outboundFromExtra(t, "VLESS", extra).Transport
	if tr.UplinkDataPlacement != "" {
		t.Fatalf("mode=auto kept uplink_data_placement=%q", tr.UplinkDataPlacement)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXHTTPObfs_JunkDropped: unsupported enum values abort the whole instance.
func TestXHTTPObfs_JunkDropped(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"sessionPlacement":     "body",
		"seqPlacement":         "nowhere",
		"congestionController": "hyperspeed",
		"cwnd":                 -5,
		"scMaxBufferedPosts":   -1,
	})
	tr := outboundFromExtra(t, "VLESS", extra).Transport
	if tr.SessionPlacement != "" || tr.SeqPlacement != "" || tr.CongestionController != "" {
		t.Fatalf("junk enums forwarded: %+v", tr)
	}
	if tr.CWND != 0 || tr.ScMaxBufferedPosts != 0 {
		t.Fatalf("negative numbers forwarded: %d / %d", tr.CWND, tr.ScMaxBufferedPosts)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXHTTPObfs_RangeFieldsRejectGarbage: badoption.Range[int] fails to decode
// on a non-numeric string ("strconv.ParseInt: parsing \"abc\"") or on an
// inverted range ("invalid range"), and either error aborts the whole engine
// start — Task 2 only kept the xmux object's keys clean, these session/uplink
// fields still forwarded whatever the node sent.
func TestXHTTPObfs_RangeFieldsRejectGarbage(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"sessionIdLength": "abc",
		"uplinkChunkSize": "2000-1000",
	})
	tr := outboundFromExtra(t, "VLESS", extra).Transport
	if tr.SessionIDLength != nil {
		t.Fatalf("session_id_length = %s, want dropped", tr.SessionIDLength)
	}
	if tr.UplinkChunkSize != nil {
		t.Fatalf("uplink_chunk_size = %s, want dropped", tr.UplinkChunkSize)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXHTTPObfs_RangeFieldsAcceptValidForms: the fix must not break the two
// shapes the core actually parses — a bare positive number and a positive
// "a-b" range.
func TestXHTTPObfs_RangeFieldsAcceptValidForms(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"sessionIdLength": "8-16",
		"uplinkChunkSize": 30,
	})
	tr := outboundFromExtra(t, "VLESS", extra).Transport
	if string(tr.SessionIDLength) != `"8-16"` {
		t.Fatalf("session_id_length = %s, want \"8-16\"", tr.SessionIDLength)
	}
	if string(tr.UplinkChunkSize) != `30` {
		t.Fatalf("uplink_chunk_size = %s, want 30", tr.UplinkChunkSize)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXHTTPObfs_AbsentStaysAbsent keeps the old wire shape for plain nodes.
func TestXHTTPObfs_AbsentStaysAbsent(t *testing.T) {
	tr := outboundFromExtra(t, "VLESS", xhttpNodeExtra(nil)).Transport
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{
		"session_placement", "session_key", "seq_placement", "seq_key",
		"uplink_data_placement", "uplink_data_key", "session_id_table",
		"session_id_length", "uplink_chunk_size", "congestion_controller",
		"cwnd", "sc_max_buffered_posts",
	} {
		if _, ok := m[k]; ok {
			t.Fatalf("%s emitted for a node that never asked: %s", k, raw)
		}
	}
}
